package whois

import (
	"context"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/tespio/go-rdap-server/internal/config"
	"github.com/tespio/go-rdap-server/internal/store"
	"go.uber.org/zap"
)

func TestParseQuery(t *testing.T) {
	cases := []struct {
		in        string
		wantQuery string
		wantKind  string
	}{
		{"example.com", "example.com", "domain"},
		{"domain example.com", "example.com", "domain"},
		{"dom example.com", "example.com", "domain"},
		{"ns ns1.example.com", "ns1.example.com", "nameserver"},
		{"nameserver ns1.example.com", "ns1.example.com", "nameserver"},
		{"entity REG1-NAME", "REG1-NAME", "entity"},
		{"contact REG1-NAME", "REG1-NAME", "entity"},
		{"ip 8.8.8.0/24", "8.8.8.0/24", "ip"},
		{"asn 15169", "15169", "autnum"},
		{"AS15169", "AS15169", "autnum"},
		{"8.8.8.0/24", "8.8.8.0/24", "ip"},
		{"ns1.example.com", "ns1.example.com", "nameserver"},
		{"REG1-NAME", "REG1-NAME", "entity"},
		{"", "", ""},
	}
	for _, tc := range cases {
		q, k := parseQuery(tc.in)
		if q != tc.wantQuery || k != tc.wantKind {
			t.Errorf("parseQuery(%q) = (%q,%q), want (%q,%q)", tc.in, q, k, tc.wantQuery, tc.wantKind)
		}
	}
}

func TestQueryWithDomainAggregateStore(t *testing.T) {
	st, err := store.NewMemoryStore(config.StorageConfig{})
	if err != nil {
		t.Fatalf("memory store: %v", err)
	}
	s := New("127.0.0.1:0", StoreLookup(st), zap.NewNop())
	if err := s.Listen(); err != nil {
		t.Fatalf("listen: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go s.Serve(ctx)
	if s.Addr() == nil {
		t.Fatal("whois server has no bound address")
	}

	conn, err := net.Dial("tcp", s.Addr().String())
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(5 * time.Second))

	if _, err := conn.Write([]byte("example.com\r\n")); err != nil {
		t.Fatalf("write: %v", err)
	}

	var out strings.Builder
	buf := make([]byte, 4096)
	for {
		n, err := conn.Read(buf)
		out.Write(buf[:n])
		if err != nil {
			break
		}
	}
	text := out.String()
	for _, want := range []string{
		"Domain Name: example.com",
		"Example Registrar Inc.",
		"ns1.example.com",
		"Registrant:",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("whois response missing %q\n---\n%s", want, text)
		}
	}
}

func TestAnswerNotFound(t *testing.T) {
	s := New("127.0.0.1:0", func(ctx context.Context, name string) (WhoisDomain, error) {
		return WhoisDomain{}, ErrNotFound
	}, zap.NewNop())
	out := s.answer("nope.invalid")
	if !strings.Contains(out, "NOT FOUND") {
		t.Errorf("expected NOT FOUND, got %q", out)
	}
}

func TestAnswerUnsupportedType(t *testing.T) {
	s := New("127.0.0.1:0", func(ctx context.Context, name string) (WhoisDomain, error) {
		return WhoisDomain{}, ErrNotFound
	}, zap.NewNop())
	out := s.answer("ns ns1.example.com")
	if !strings.Contains(out, "nameserver") || !strings.Contains(out, "not supported") {
		t.Errorf("expected nameserver explanation, got %q", out)
	}
}

func TestAnswerDomainLookup(t *testing.T) {
	s := New("127.0.0.1:0", func(ctx context.Context, name string) (WhoisDomain, error) {
		if name != "example.com" {
			return WhoisDomain{}, ErrNotFound
		}
		return WhoisDomain{LDHName: "example.com", Registrar: "Example Registrar Inc."}, nil
	}, zap.NewNop())
	out := s.answer("example.com")
	if !strings.Contains(out, "Domain Name: example.com") {
		t.Errorf("expected domain rendering, got %q", out)
	}
}

func TestAnswerEmpty(t *testing.T) {
	s := New("127.0.0.1:0", func(ctx context.Context, name string) (WhoisDomain, error) {
		return WhoisDomain{}, ErrNotFound
	}, zap.NewNop())
	out := s.answer("")
	if !strings.Contains(out, "INVALID QUERY") {
		t.Errorf("expected INVALID QUERY for empty line, got %q", out)
	}
	// A blank line (only whitespace) also yields invalid query.
	if out := s.answer("   "); !strings.Contains(out, "INVALID QUERY") {
		t.Errorf("expected INVALID QUERY for whitespace, got %q", out)
	}
}
