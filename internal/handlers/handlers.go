package handlers

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/tespio/go-rdap-server/internal/config"
	"github.com/tespio/go-rdap-server/internal/rdap"
	"github.com/tespio/go-rdap-server/internal/service"
	"github.com/tespio/go-rdap-server/internal/store"
)

type Handler struct {
	svc  *service.Service
	cfg  config.RDAPConfig
	port int
	rate config.RateConfig
}

func New(svc *service.Service, cfg config.RDAPConfig, rate config.RateConfig, serverPort int) *Handler {
	return &Handler{svc: svc, cfg: cfg, port: serverPort, rate: rate}
}

func (h *Handler) RegisterRoutes(r chi.Router) {
	r.Get("/help", h.Help)
	r.Head("/help", h.Help)
	r.Get("/", h.Help)

	r.Get("/domain/{domainName}", h.LookupDomain)
	r.Head("/domain/{domainName}", h.LookupDomain)
	r.Get("/entity/{handle}", h.LookupEntity)
	r.Head("/entity/{handle}", h.LookupEntity)
	r.Get("/nameserver/{name}", h.LookupNameserver)
	r.Head("/nameserver/{name}", h.LookupNameserver)
	r.Get("/ip/*", h.LookupIPNetwork)
	r.Head("/ip/*", h.LookupIPNetwork)
	r.Get("/autnum/{asn}", h.LookupAutnum)
	r.Head("/autnum/{asn}", h.LookupAutnum)

	// Search endpoints (RFC 7482). Optional capability — most registrars/
	// registries disable these in production (abuse/DoS risk). When disabled
	// the routes still exist but return 501 Not Implemented (RFC 9082 §5.1),
	// so clients get an explicit answer instead of a bare 404.
	if h.cfg.SearchEnabled {
		r.Get("/domains", h.SearchDomains)
		r.Head("/domains", h.SearchDomains)
		r.Get("/entities", h.SearchEntities)
		r.Head("/entities", h.SearchEntities)
		r.Get("/nameservers", h.SearchNameservers)
		r.Head("/nameservers", h.SearchNameservers)
	} else {
		r.Get("/domains", h.SearchNotImplemented)
		r.Head("/domains", h.SearchNotImplemented)
		r.Get("/entities", h.SearchNotImplemented)
		r.Head("/entities", h.SearchNotImplemented)
		r.Get("/nameservers", h.SearchNotImplemented)
		r.Head("/nameservers", h.SearchNotImplemented)
	}

	// Reverse search (RFC 9536). Only the domains→entity reverse search is
	// implemented; other combinations are not registered (404). When the
	// storage backend cannot serve reverse searches the handler returns 501.
	r.Get("/domains/reverse_search/entity", h.ReverseSearchDomains)
	r.Head("/domains/reverse_search/entity", h.ReverseSearchDomains)
}

func (h *Handler) requestURL(r *http.Request) string {
	scheme := "http"
	if r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https") {
		scheme = "https"
	}
	host := r.Host
	if _, _, err := net.SplitHostPort(host); err != nil {
		// Host header has no explicit port; add the configured port unless it is
		// the default for the scheme (80/443).
		port := h.port
		if !(scheme == "http" && port == 80) && !(scheme == "https" && port == 443) {
			host = net.JoinHostPort(host, strconv.Itoa(port))
		}
	}
	u := url.URL{Scheme: scheme, Host: host, Path: r.URL.Path}
	if r.URL.RawQuery != "" {
		u.RawQuery = r.URL.RawQuery
	}
	return u.String()
}

func (h *Handler) noticeOpts() *rdap.NoticeOptions {
	return service.NoticeOptionsFromConfig(h.cfg)
}

func (h *Handler) Help(w http.ResponseWriter, r *http.Request) {
	rateInfo := rdap.RateLimitInfo{}
	if h.rate.Enabled {
		rateInfo = rdap.RateLimitInfo{
			Enabled:  true,
			Requests: h.rate.Requests,
			Window:   h.rate.Window,
			Burst:    h.rate.Burst,
		}
	}
	help := rdap.NewHelp(h.cfg.BaseURL, h.noticeOpts(), rateInfo, rdap.SearchInfo{Enabled: h.cfg.SearchEnabled})
	// reverse_search (RFC 9536): advertise supported properties + conformance.
	if h.cfg.ExtensionsEnabled("reverse_search") {
		help.ReverseSearchProperties = h.svc.ReverseSearchProperties()
		help.Conformance = rdap.WithExtensions(help.Conformance, "reverse_search")
	}
	writeJSON(w, http.StatusOK, help)
}

func (h *Handler) LookupDomain(w http.ResponseWriter, r *http.Request) {
	domainName := chi.URLParam(r, "domainName")
	if domainName == "" {
		writeError(w, http.StatusBadRequest, 400, "Invalid domain name", "Domain name parameter is required")
		return
	}

	ldhName, _, err := rdap.NormalizeDomainName(domainName)
	if err != nil {
		writeError(w, http.StatusBadRequest, 400, "Invalid domain name", err.Error())
		return
	}

	resp, err := h.svc.LookupDomain(ldhName, h.requestURL(r))
	if err != nil {
		writeError(w, http.StatusNotFound, 404, "Domain not found", fmt.Sprintf("No domain found for: %s", domainName))
		return
	}

	writeJSON(w, http.StatusOK, resp)
}

func (h *Handler) LookupEntity(w http.ResponseWriter, r *http.Request) {
	handle := chi.URLParam(r, "handle")
	if handle == "" {
		writeError(w, http.StatusBadRequest, 400, "Invalid entity handle", "Entity handle parameter is required")
		return
	}

	entity, err := h.svc.LookupEntity(handle, h.requestURL(r))
	if err != nil {
		writeError(w, http.StatusNotFound, 404, "Entity not found", fmt.Sprintf("No entity found for handle: %s", handle))
		return
	}

	reqURL := h.requestURL(r)
	resp := rdap.EntityResponse{
		Entity:      entity,
		Conformance: rdap.NewConformance(),
		Notices:     rdap.NewNoticesWithICANN(reqURL, h.cfg.BaseURL, h.noticeOpts()),
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *Handler) LookupNameserver(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	if name == "" {
		writeError(w, http.StatusBadRequest, 400, "Invalid nameserver name", "Nameserver name parameter is required")
		return
	}

	ns, err := h.svc.LookupNameserver(name, h.requestURL(r))
	if err != nil {
		writeError(w, http.StatusNotFound, 404, "Nameserver not found", fmt.Sprintf("No nameserver found for: %s", name))
		return
	}

	reqURL := h.requestURL(r)
	conf := rdap.NewConformance()
	if h.cfg.ExtensionsEnabled("ttl0") {
		conf = rdap.WithExtensions(conf, "ttl0")
	}
	resp := rdap.NameserverResponse{
		Nameserver:  ns,
		Conformance: conf,
		Notices:     rdap.NewNoticesWithICANN(reqURL, h.cfg.BaseURL, h.noticeOpts()),
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *Handler) LookupIPNetwork(w http.ResponseWriter, r *http.Request) {
	// /ip/* uses chi's wildcard so a CIDR ("8.8.8.0/24", contains a slash)
	// is captured in full rather than being split across path segments.
	network := chi.URLParam(r, "*")
	if network == "" {
		writeError(w, http.StatusBadRequest, 400, "Invalid network", "Network parameter is required")
		return
	}

	_, err := netip.ParsePrefix(network)
	if err != nil {
		if _, err := netip.ParseAddr(network); err != nil {
			writeError(w, http.StatusBadRequest, 400, "Invalid network", "Must be a valid IP address or CIDR notation")
			return
		}
		if strings.Contains(network, ":") {
			network += "/128"
		} else {
			network += "/32"
		}
	}

	ipnet, err := h.svc.LookupIPNetwork(network, h.requestURL(r))
	if err != nil {
		writeError(w, http.StatusNotFound, 404, "IP network not found", fmt.Sprintf("No IP network found for: %s", network))
		return
	}

	reqURL := h.requestURL(r)
	resp := rdap.IPNetworkResponse{
		IPNetwork:   ipnet,
		Conformance: h.ipNetworkConformance(),
		Notices:     rdap.NewNoticesWithICANN(reqURL, h.cfg.BaseURL, h.noticeOpts()),
	}
	writeJSON(w, http.StatusOK, resp)
}

// ipNetworkConformance returns the base conformance plus any enabled extension
// identifiers that apply to IP network responses (geofeed1, cidr0).
func (h *Handler) ipNetworkConformance() rdap.Conformance {
	conf := rdap.NewConformance()
	if h.cfg.ExtensionsEnabled("geofeed1") {
		conf = rdap.WithExtensions(conf, "geofeed1")
	}
	if h.cfg.ExtensionsEnabled("cidr0") {
		conf = rdap.WithExtensions(conf, "cidr0")
	}
	return conf
}

func (h *Handler) LookupAutnum(w http.ResponseWriter, r *http.Request) {
	asnStr := chi.URLParam(r, "asn")
	if asnStr == "" {
		writeError(w, http.StatusBadRequest, 400, "Invalid ASN", "ASN parameter is required")
		return
	}

	asn, err := strconv.ParseUint(asnStr, 10, 32)
	if err != nil {
		writeError(w, http.StatusBadRequest, 400, "Invalid ASN", "ASN must be a valid 32-bit unsigned integer")
		return
	}
	// asn is guaranteed to fit in uint32 by the bitSize=32 parse above.
	asn32 := uint32(asn)

	resp := rdap.AutnumResponse{
		Autnum: rdap.Autnum{
			Common: rdap.Common{
				ObjectClassName: "autnum",
				Handle:          fmt.Sprintf("AS%d", asn32),
				Status:          []string{"active"},
			},
			StartAutnum: asn32,
			EndAutnum:   asn32,
			Name:        fmt.Sprintf("AS%d", asn32),
		},
		Conformance: rdap.NewConformance(),
		Notices:     rdap.NewNoticesWithICANN(h.requestURL(r), h.cfg.BaseURL, h.noticeOpts()),
	}

	writeJSON(w, http.StatusOK, resp)
}

// SearchNotImplemented responds 501 Not Implemented when search endpoints are
// disabled via rdap.search_enabled: false (the default). RFC 9082 §5.1: a
// server that does not implement searches must respond 501.
func (h *Handler) SearchNotImplemented(w http.ResponseWriter, r *http.Request) {
	writeError(w, http.StatusNotImplemented, 501, "Search not implemented", "Search queries are disabled on this server")
}

// ReverseSearchDomains implements RFC 9536 reverse search for
// /domains/reverse_search/entity. Supported query properties: handle, role,
// fn, email. At least one property predicate is required. When the storage
// backend cannot serve reverse searches, responds 501 per RFC 9536 §7.
func (h *Handler) ReverseSearchDomains(w http.ResponseWriter, r *http.Request) {
	if !h.cfg.ExtensionsEnabled("reverse_search") {
		writeError(w, http.StatusNotImplemented, 501, "Reverse search not implemented", "The reverse_search extension is not enabled on this server")
		return
	}

	q := r.URL.Query()
	limit := h.cfg.MaxSearchLimit
	if l := q.Get("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 && parsed <= limit {
			limit = parsed
		}
	}

	// Collect the property predicates present in the query.
	type predicate struct{ property, pattern string }
	var preds []predicate
	for _, p := range []string{"handle", "role", "fn", "email"} {
		if v := q.Get(p); v != "" {
			preds = append(preds, predicate{property: p, pattern: v})
		}
	}
	if len(preds) == 0 {
		writeError(w, http.StatusBadRequest, 400, "Missing search parameter", "One of 'handle', 'role', 'fn', or 'email' is required")
		return
	}
	// Only a single property predicate is supported for the reverse search.
	if len(preds) > 1 {
		writeError(w, http.StatusBadRequest, 400, "Ambiguous search", "Only one reverse search property may be specified")
		return
	}

	results, err := h.svc.ReverseSearchDomainsByEntity(preds[0].property, preds[0].pattern, limit, h.requestURL(r))
	if err != nil {
		if errors.Is(err, store.ErrReverseSearchUnsupported) {
			writeError(w, http.StatusNotImplemented, 501, "Reverse search not implemented", "The storage backend does not support reverse search")
			return
		}
		writeError(w, http.StatusInternalServerError, 500, "Reverse search failed", err.Error())
		return
	}

	reqURL := h.requestURL(r)
	resp := rdap.DomainSearchResult{
		Conformance:                    rdap.WithExtensions(rdap.NewConformance(), "reverse_search"),
		DomainSearchResults:            results,
		Notices:                        rdap.NewNoticesWithICANN(reqURL, h.cfg.BaseURL, h.noticeOpts()),
		ReverseSearchPropertiesMapping: service.ReverseSearchMapping([]string{preds[0].property}),
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *Handler) SearchDomains(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	namePattern := q.Get("name")
	nsPattern := q.Get("nsLdhName")

	limit := h.cfg.MaxSearchLimit
	if l := q.Get("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 && parsed <= limit {
			limit = parsed
		}
	}

	if namePattern != "" && nsPattern != "" {
		writeError(w, http.StatusBadRequest, 400, "Ambiguous search", "Cannot specify both 'name' and 'nsLdhName' parameters")
		return
	}

	var results []rdap.Domain
	var err error
	reqURL := h.requestURL(r)

	if namePattern != "" {
		results, err = h.svc.SearchDomainsByName(namePattern, limit, reqURL)
	} else if nsPattern != "" {
		results, err = h.svc.SearchDomainsByNS(nsPattern, limit, reqURL)
	} else {
		writeError(w, http.StatusBadRequest, 400, "Missing search parameter", "Either 'name' or 'nsLdhName' parameter is required")
		return
	}

	if err != nil {
		writeError(w, http.StatusInternalServerError, 500, "Search failed", err.Error())
		return
	}

	resp := rdap.DomainSearchResult{
		Conformance:         rdap.NewConformance(),
		DomainSearchResults: results,
		Notices:             rdap.NewNoticesWithICANN(reqURL, h.cfg.BaseURL, h.noticeOpts()),
	}

	writeJSON(w, http.StatusOK, resp)
}

func (h *Handler) SearchEntities(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	fnPattern := q.Get("fn")
	handlePattern := q.Get("handle")

	limit := h.cfg.MaxSearchLimit
	if l := q.Get("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 && parsed <= limit {
			limit = parsed
		}
	}

	if fnPattern != "" && handlePattern != "" {
		writeError(w, http.StatusBadRequest, 400, "Ambiguous search", "Cannot specify both 'fn' and 'handle' parameters")
		return
	}

	var results []rdap.Entity
	var err error
	reqURL := h.requestURL(r)

	if fnPattern != "" {
		results, err = h.svc.SearchEntitiesByName(fnPattern, limit, reqURL)
	} else if handlePattern != "" {
		results, err = h.svc.SearchEntitiesByHandle(handlePattern, limit, reqURL)
	} else {
		writeError(w, http.StatusBadRequest, 400, "Missing search parameter", "Either 'fn' or 'handle' parameter is required")
		return
	}

	if err != nil {
		writeError(w, http.StatusInternalServerError, 500, "Search failed", err.Error())
		return
	}

	resp := rdap.EntitySearchResult{
		Conformance:         rdap.NewConformance(),
		EntitySearchResults: results,
		Notices:             rdap.NewNoticesWithICANN(reqURL, h.cfg.BaseURL, h.noticeOpts()),
	}

	writeJSON(w, http.StatusOK, resp)
}

func (h *Handler) SearchNameservers(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	namePattern := q.Get("name")
	ip := q.Get("ip")

	limit := h.cfg.MaxSearchLimit
	if l := q.Get("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 && parsed <= limit {
			limit = parsed
		}
	}

	if namePattern != "" && ip != "" {
		writeError(w, http.StatusBadRequest, 400, "Ambiguous search", "Cannot specify both 'name' and 'ip' parameters")
		return
	}

	var results []rdap.Nameserver
	var err error
	reqURL := h.requestURL(r)

	if namePattern != "" {
		results, err = h.svc.SearchNameserversByName(namePattern, limit, reqURL)
	} else if ip != "" {
		results, err = h.svc.SearchNameserversByIP(ip, limit, reqURL)
	} else {
		writeError(w, http.StatusBadRequest, 400, "Missing search parameter", "Either 'name' or 'ip' parameter is required")
		return
	}

	if err != nil {
		writeError(w, http.StatusInternalServerError, 500, "Search failed", err.Error())
		return
	}

	resp := rdap.NameserverSearchResult{
		Conformance:             rdap.NewConformance(),
		NameserverSearchResults: results,
		Notices:                 rdap.NewNoticesWithICANN(reqURL, h.cfg.BaseURL, h.noticeOpts()),
	}

	writeJSON(w, http.StatusOK, resp)
}
