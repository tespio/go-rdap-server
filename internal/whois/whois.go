// Package whois implements a legacy port 43 WHOIS gateway (RFC 3912) that
// serves plain-text WHOIS responses rendered from the same registry data the
// RDAP server serves. This lets a single binary replace both the RDAP and
// WHOIS services during the RDAP migration.
package whois

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"time"

	"go.uber.org/zap"
)

// LookupFunc resolves a domain name to a domain aggregate for rendering.
type LookupFunc func(ctx context.Context, name string) (WhoisDomain, error)

// WhoisDomain is the minimal data the renderer needs from a domain lookup.
// It decouples the WHOIS gateway from the full RDAP/domain model so tests and
// alternate backends can supply data easily.
type WhoisDomain struct {
	LDHName     string
	UnicodeName string
	Status      []string
	// Registrar is the sponsoring registrar display name.
	Registrar string
	// RegistrarID is the IANA Registrar ID when known.
	RegistrarID string
	// Registrant is the registrant display name.
	Registrant string
	// Contacts lists contact records (registrant, admin, tech, billing, abuse).
	Contacts []WhoisContact
	// Nameservers lists the domain's nameservers.
	Nameservers []WhoisNameserver
	// CreatedAt, UpdatedAt, ExpiresAt are the ISO-8601 event dates.
	CreatedAt time.Time
	UpdatedAt time.Time
	ExpiresAt time.Time
	// DNSSEC indicates whether the zone is DNSSEC-signed.
	DNSSEC bool
}

// WhoisContact is one contact rendered in a WHOIS response.
type WhoisContact struct {
	Role     string
	Name     string
	Org      string
	Email    string
	Phone    string
	Address  string
	Redacted bool
}

// WhoisNameserver is a nameserver rendered in a WHOIS response.
type WhoisNameserver struct {
	Name string
	IPV4 []string
	IPV6 []string
}

// ErrNotFound is returned when the queried object does not exist.
var ErrNotFound = errors.New("object not found")

// ErrNotImplemented is returned for query types the gateway does not support.
var ErrNotImplemented = errors.New("query type not supported")

// Server is the WHOIS gateway. It listens on the configured port and answers
// RFC 3912-style queries.
type Server struct {
	addr   string
	lookup LookupFunc
	logger *zap.Logger
	ln     net.Listener
}

// New builds a WHOIS gateway. lookup resolves domain queries; pass
// RendererLookup to use the RDAP store-backed renderer.
func New(addr string, lookup LookupFunc, logger *zap.Logger) *Server {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &Server{addr: addr, lookup: lookup, logger: logger}
}

// Serve starts the WHOIS listener and blocks until the context is cancelled.
func (s *Server) Serve(ctx context.Context) error {
	ln, err := net.Listen("tcp", s.addr)
	if err != nil {
		return fmt.Errorf("whois listen %s: %w", s.addr, err)
	}
	s.ln = ln
	defer ln.Close()
	s.logger.Info("whois server listening", zap.String("addr", s.addr))

	go func() {
		<-ctx.Done()
		ln.Close()
	}()

	for {
		conn, err := ln.Accept()
		if err != nil {
			select {
			case <-ctx.Done():
				return nil
			default:
				s.logger.Warn("whois accept error", zap.Error(err))
				continue
			}
		}
		go s.handle(conn)
	}
}

// Shutdown closes the listener.
func (s *Server) Shutdown(ctx context.Context) error {
	if s.ln != nil {
		return s.ln.Close()
	}
	return nil
}

// handle serves a single WHOIS query connection (RFC 3912: read one
// CR/LF-terminated query, respond, close).
func (s *Server) handle(conn net.Conn) {
	defer conn.Close()
	conn.SetReadDeadline(time.Now().Add(10 * time.Second))

	reader := bufio.NewReader(conn)
	line, err := reader.ReadString('\n')
	if err != nil {
		return
	}
	line = strings.TrimSpace(line)
	if line == "" {
		fmt.Fprint(conn, "No query received.\r\n")
		return
	}

	s.logger.Info("whois query", zap.String("query", line), zap.String("remote", conn.RemoteAddr().String()))
	output := s.answer(line)
	conn.Write([]byte(output))
}

// answer resolves a WHOIS query line and returns the response text.
func (s *Server) answer(line string) string {
	query, kind := parseQuery(line)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	switch kind {
	case "domain":
		d, err := s.lookup(ctx, query)
		if err != nil {
			if errors.Is(err, ErrNotFound) {
				return "NOT FOUND\r\n\r\n% No entries found for the selected source(s).\r\n"
			}
			return fmt.Sprintf("%% ERROR: %s\r\n", err)
		}
		return RenderDomain(d)
	case "nameserver", "entity", "ip", "autnum":
		// The gateway currently resolves domains (the most common WHOIS query).
		// Other object types are answered with a clear explanation rather than a
		// bare error, so legacy clients get a useful response.
		return fmt.Sprintf(
			"%% This WHOIS gateway serves domain name lookups.\r\n"+
				"%% Query type %q is not supported here; use the RDAP service at /rdap instead.\r\n"+
				"%% e.g. GET /rdap/domain/%s\r\n",
			kind, query)
	default:
		return "INVALID QUERY\r\n\r\n% Unrecognized query.\r\n"
	}
}

// parseQuery splits a WHOIS query line into (target, kind). It supports the
// common forms:
//
//	example.com            -> domain
//	domain example.com     -> domain
//	ns ns1.example.com     -> nameserver
//	entity REG1-NAME       -> entity
//	8.8.8.0/24             -> ip
//	AS15169                -> autnum
func parseQuery(line string) (string, string) {
	line = strings.TrimSpace(line)
	if line == "" {
		return "", ""
	}
	fields := strings.Fields(line)
	if len(fields) >= 2 {
		// First token may be a keyword; second is the target.
		switch strings.ToLower(fields[0]) {
		case "domain", "dom":
			return fields[1], "domain"
		case "nameserver", "ns":
			return fields[1], "nameserver"
		case "entity", "contact":
			return fields[1], "entity"
		case "ip", "ipnetwork", "network":
			return fields[1], "ip"
		case "asn", "autnum":
			return fields[1], "autnum"
		}
	}

	// Auto-detect single-token queries.
	t := fields[0]
	lower := strings.ToLower(t)
	switch {
	case strings.HasPrefix(lower, "as") && isNumeric(lower[2:]):
		return t, "autnum"
	case strings.Contains(t, "/"):
		return t, "ip"
	case strings.HasPrefix(lower, "ns") && strings.Contains(t, "."):
		return t, "nameserver"
	case strings.Contains(t, "."):
		return t, "domain"
	case isNumeric(t):
		return t, "entity"
	default:
		// A bare token with no dot (e.g. a contact handle like REG1-NAME) is
		// treated as an entity; bare hostnames without dots are invalid domains
		// anyway.
		return t, "entity"
	}
}

func isNumeric(s string) bool {
	if s == "" {
		return false
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}
