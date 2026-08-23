package handlers

import (
	"crypto/tls"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/tespio/go-rdap-server/internal/config"
	"github.com/tespio/go-rdap-server/internal/rdap"
	"github.com/tespio/go-rdap-server/internal/service"
	"github.com/tespio/go-rdap-server/internal/store"
)

func newTestHandler(t *testing.T, searchEnabled bool) http.Handler {
	t.Helper()
	cfg := config.RDAPConfig{
		BaseURL:          "https://rdap.example.com",
		RegistrarBaseURL: "https://rdap.example.org/rdap/",
		Mode:             "registrar",
		MaxSearchLimit:   100,
		SearchEnabled:    searchEnabled,
	}
	return newTestHandlerCfg(t, cfg)
}

func newTestHandlerCfg(t *testing.T, cfg config.RDAPConfig) http.Handler {
	t.Helper()
	st, err := store.NewMemoryStore(config.StorageConfig{})
	if err != nil {
		t.Fatalf("memory store: %v", err)
	}
	svc := service.New(st, cfg)
	h := New(svc, cfg, config.RateConfig{}, 8443)
	r := chi.NewRouter()
	h.RegisterRoutes(r)
	return r
}

func TestSearchEnabledReturnsResults(t *testing.T) {
	router := newTestHandler(t, true)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/domains?name=example*", nil)
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp rdap.DomainSearchResult
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.DomainSearchResults) == 0 {
		t.Fatal("expected at least one domain result")
	}
}

func TestSearchDisabledReturnsNotImplemented(t *testing.T) {
	router := newTestHandler(t, false)

	for _, path := range []string{
		"/domains?name=example*",
		"/entities?fn=Example*",
		"/nameservers?name=ns1*",
	} {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, path, nil)
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusNotImplemented {
			t.Errorf("%s: expected 501, got %d: %s", path, rec.Code, rec.Body.String())
		}
	}
}

func TestSearchDisabledHEADMatchesGET(t *testing.T) {
	router := newTestHandler(t, false)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodHead, "/domains?name=example*", nil)
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotImplemented {
		t.Errorf("HEAD expected 501, got %d", rec.Code)
	}
}

func doRequest(t *testing.T, h http.Handler, method, target string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(method, target, nil)
	h.ServeHTTP(rec, req)
	return rec
}

func TestLookupDomainHandler(t *testing.T) {
	router := newTestHandler(t, false)

	rec := doRequest(t, router, http.MethodGet, "/domain/example.com")
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var d rdap.Domain
	if err := json.Unmarshal(rec.Body.Bytes(), &d); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if d.Handle != "EX1-NAME" || d.LDHName != "example.com" {
		t.Errorf("domain = %+v", d)
	}
	if len(d.Status) != 1 || d.Status[0] != "active" {
		t.Errorf("status = %v", d.Status)
	}

	// Case + trailing dot normalization.
	rec = doRequest(t, router, http.MethodGet, "/domain/EXAMPLE.COM.")
	if rec.Code != http.StatusOK {
		t.Errorf("normalized lookup expected 200, got %d", rec.Code)
	}

	// HEAD matches GET status.
	rec = doRequest(t, router, http.MethodHead, "/domain/example.com")
	if rec.Code != http.StatusOK {
		t.Errorf("HEAD expected 200, got %d", rec.Code)
	}

	// Invalid / missing domain.
	for _, target := range []string{
		"/domain/not-a-domain.invalid",
		"/domain/!!bad!!",
	} {
		rec = doRequest(t, router, http.MethodGet, target)
		if rec.Code != http.StatusNotFound && rec.Code != http.StatusBadRequest {
			t.Errorf("%s: expected 4xx, got %d", target, rec.Code)
		}
	}
}

func TestLookupEntityHandler(t *testing.T) {
	router := newTestHandler(t, false)

	rec := doRequest(t, router, http.MethodGet, "/entity/2")
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var e rdap.Entity
	if err := json.Unmarshal(rec.Body.Bytes(), &e); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if e.Handle != "2" {
		t.Errorf("handle = %q", e.Handle)
	}

	rec = doRequest(t, router, http.MethodGet, "/entity/nonexistent")
	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rec.Code)
	}
}

func TestLookupNameserverHandler(t *testing.T) {
	router := newTestHandler(t, false)

	rec := doRequest(t, router, http.MethodGet, "/nameserver/ns1.example.com")
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var ns rdap.Nameserver
	if err := json.Unmarshal(rec.Body.Bytes(), &ns); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if ns.Handle != "NS1-NAME" {
		t.Errorf("handle = %q", ns.Handle)
	}

	rec = doRequest(t, router, http.MethodGet, "/nameserver/nonexistent.example")
	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rec.Code)
	}
}

func TestLookupIPNetworkHandler(t *testing.T) {
	router := newTestHandler(t, false)

	rec := doRequest(t, router, http.MethodGet, "/ip/8.8.8.0/24")
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var n rdap.IPNetwork
	if err := json.Unmarshal(rec.Body.Bytes(), &n); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if n.Handle != "NET-8-8-8-0-24" {
		t.Errorf("handle = %q", n.Handle)
	}

	// Bare IP is expanded to a /32 or /128.
	rec = doRequest(t, router, http.MethodGet, "/ip/8.8.8.8")
	if rec.Code != http.StatusNotFound {
		t.Errorf("bare IP (not seeded as /32): expected 404, got %d", rec.Code)
	}

	rec = doRequest(t, router, http.MethodGet, "/ip/not-an-ip")
	if rec.Code != http.StatusBadRequest {
		t.Errorf("invalid IP expected 400, got %d", rec.Code)
	}
}

func TestLookupAutnumHandler(t *testing.T) {
	router := newTestHandler(t, false)

	rec := doRequest(t, router, http.MethodGet, "/autnum/15169")
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var a rdap.Autnum
	if err := json.Unmarshal(rec.Body.Bytes(), &a); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if a.Handle != "AS15169" || a.StartAutnum != 15169 {
		t.Errorf("autnum = %+v", a)
	}

	// Invalid ASN.
	rec = doRequest(t, router, http.MethodGet, "/autnum/not-a-number")
	if rec.Code != http.StatusBadRequest {
		t.Errorf("invalid ASN expected 400, got %d", rec.Code)
	}
}

func TestHelpHandler(t *testing.T) {
	router := newTestHandler(t, false)

	rec := doRequest(t, router, http.MethodGet, "/help")
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var h rdap.Help
	if err := json.Unmarshal(rec.Body.Bytes(), &h); err != nil {
		t.Fatalf("decode: %v", err)
	}
	// Rate limiting disabled in the test handler config, so no rate-limit details;
	// search disabled -> "Search Disabled" notice.
	var hasSearchDisabled bool
	for _, n := range h.Notices {
		if n.Title == "Search Disabled" {
			hasSearchDisabled = true
		}
	}
	if !hasSearchDisabled {
		t.Error("expected 'Search Disabled' notice in /help")
	}

	// "/" also serves help.
	rec = doRequest(t, router, http.MethodGet, "/")
	if rec.Code != http.StatusOK {
		t.Errorf("root expected 200, got %d", rec.Code)
	}
}

func TestSearchEntitiesHandler(t *testing.T) {
	router := newTestHandler(t, true)

	// fn search (matched against handles in the memory store).
	rec := doRequest(t, router, http.MethodGet, "/entities?fn=REG1*")
	if rec.Code != http.StatusOK {
		t.Fatalf("fn search expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var er rdap.EntitySearchResult
	if err := json.Unmarshal(rec.Body.Bytes(), &er); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(er.EntitySearchResults) == 0 {
		t.Error("expected entity results")
	}

	// handle search.
	rec = doRequest(t, router, http.MethodGet, "/entities?handle=REG1*")
	if rec.Code != http.StatusOK {
		t.Fatalf("handle search expected 200, got %d", rec.Code)
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &er); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(er.EntitySearchResults) == 0 {
		t.Error("expected entity results for handle search")
	}

	// Missing parameter -> 400.
	rec = doRequest(t, router, http.MethodGet, "/entities")
	if rec.Code != http.StatusBadRequest {
		t.Errorf("missing param expected 400, got %d", rec.Code)
	}

	// Both fn and handle -> ambiguous -> 400.
	rec = doRequest(t, router, http.MethodGet, "/entities?fn=REG1*&handle=REG1*")
	if rec.Code != http.StatusBadRequest {
		t.Errorf("ambiguous expected 400, got %d", rec.Code)
	}
}

func TestSearchNameserversHandler(t *testing.T) {
	router := newTestHandler(t, true)

	// name search.
	rec := doRequest(t, router, http.MethodGet, "/nameservers?name=ns1*")
	if rec.Code != http.StatusOK {
		t.Fatalf("name search expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var nr rdap.NameserverSearchResult
	if err := json.Unmarshal(rec.Body.Bytes(), &nr); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(nr.NameserverSearchResults) == 0 {
		t.Error("expected nameserver results")
	}

	// ip search.
	rec = doRequest(t, router, http.MethodGet, "/nameservers?ip=8.8.8.8")
	if rec.Code != http.StatusOK {
		t.Fatalf("ip search expected 200, got %d", rec.Code)
	}

	// Missing parameter -> 400.
	rec = doRequest(t, router, http.MethodGet, "/nameservers")
	if rec.Code != http.StatusBadRequest {
		t.Errorf("missing param expected 400, got %d", rec.Code)
	}

	// Both name and ip -> ambiguous -> 400.
	rec = doRequest(t, router, http.MethodGet, "/nameservers?name=ns1*&ip=8.8.8.8")
	if rec.Code != http.StatusBadRequest {
		t.Errorf("ambiguous expected 400, got %d", rec.Code)
	}
}

func TestSearchDomainsByNSAndLimit(t *testing.T) {
	router := newTestHandler(t, true)

	// nsLdhName search.
	rec := doRequest(t, router, http.MethodGet, "/domains?nsLdhName=ns1.example.com")
	if rec.Code != http.StatusOK {
		t.Fatalf("nsLdhName search expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var dr rdap.DomainSearchResult
	if err := json.Unmarshal(rec.Body.Bytes(), &dr); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(dr.DomainSearchResults) == 0 {
		t.Error("expected domain results")
	}

	// Both name and nsLdhName -> ambiguous -> 400.
	rec = doRequest(t, router, http.MethodGet, "/domains?name=example*&nsLdhName=ns1.example.com")
	if rec.Code != http.StatusBadRequest {
		t.Errorf("ambiguous expected 400, got %d", rec.Code)
	}

	// Missing parameter -> 400.
	rec = doRequest(t, router, http.MethodGet, "/domains")
	if rec.Code != http.StatusBadRequest {
		t.Errorf("missing param expected 400, got %d", rec.Code)
	}
}

func TestRequestURLSchemes(t *testing.T) {
	cfg := config.RDAPConfig{
		BaseURL:        "https://rdap.example.com",
		Mode:           "registrar",
		MaxSearchLimit: 100,
		SearchEnabled:  false,
	}
	st, err := store.NewMemoryStore(config.StorageConfig{})
	if err != nil {
		t.Fatalf("memory store: %v", err)
	}
	svc := service.New(st, cfg)

	cases := []struct {
		name       string
		host       string
		tls        bool
		xfp        string
		port       int
		wantPrefix string
	}{
		{"plain http adds port", "rdap.example.com", false, "", 8443, "http://rdap.example.com:8443/domain/example.com"},
		{"default http port omitted", "rdap.example.com", false, "", 80, "http://rdap.example.com/domain/example.com"},
		{"https via TLS", "rdap.example.com", true, "", 443, "https://rdap.example.com/domain/example.com"},
		{"https via forwarded proto", "rdap.example.com", false, "https", 8443, "https://rdap.example.com:8443/domain/example.com"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			hh := New(svc, cfg, config.RateConfig{}, tc.port)
			req := httptest.NewRequest(http.MethodGet, "/domain/example.com", nil)
			req.Host = tc.host
			if tc.tls {
				req.TLS = &tls.ConnectionState{}
			}
			if tc.xfp != "" {
				req.Header.Set("X-Forwarded-Proto", tc.xfp)
			}
			got := hh.requestURL(req)
			if got != tc.wantPrefix {
				t.Errorf("requestURL = %q, want %q", got, tc.wantPrefix)
			}
		})
	}
}

func TestHelpHandlerRateLimited(t *testing.T) {
	st, err := store.NewMemoryStore(config.StorageConfig{})
	if err != nil {
		t.Fatalf("memory store: %v", err)
	}
	cfg := config.RDAPConfig{
		BaseURL:        "https://rdap.example.com",
		Mode:           "registrar",
		MaxSearchLimit: 100,
		SearchEnabled:  false,
	}
	rate := config.RateConfig{Enabled: true, Requests: 100, Window: 60 * 1e9, Burst: 50}
	svc := service.New(st, cfg)
	h := New(svc, cfg, rate, 8443)
	r := chi.NewRouter()
	h.RegisterRoutes(r)

	rec := doRequest(t, r, http.MethodGet, "/help")
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var help rdap.Help
	if err := json.Unmarshal(rec.Body.Bytes(), &help); err != nil {
		t.Fatalf("decode: %v", err)
	}
	var hasRate bool
	for _, n := range help.Notices {
		if n.Title == "Rate Limiting" && len(n.Description) > 0 &&
			n.Description[0] != "Access to this RDAP server is rate-limited. Excessive queries may be throttled." {
			hasRate = true
		}
	}
	if !hasRate {
		t.Error("expected rate-limit notice documenting the limit in /help")
	}
}

func TestReverseSearchHandler(t *testing.T) {
	cfg := config.RDAPConfig{
		BaseURL:          "https://rdap.example.com",
		RegistrarBaseURL: "https://rdap.example.org/rdap/",
		Mode:             "registrar",
		MaxSearchLimit:   100,
		SearchEnabled:    false,
		Extensions:       []string{"reverse_search"},
	}
	router := newTestHandlerCfg(t, cfg)

	// Reverse search by handle.
	rec := doRequest(t, router, http.MethodGet, "/domains/reverse_search/entity?handle=REG1*")
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp rdap.DomainSearchResult
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.DomainSearchResults) != 1 || resp.DomainSearchResults[0].LDHName != "example.com" {
		t.Errorf("results = %+v", resp.DomainSearchResults)
	}
	if len(resp.ReverseSearchPropertiesMapping) != 1 ||
		resp.ReverseSearchPropertiesMapping[0].Property != "handle" ||
		resp.ReverseSearchPropertiesMapping[0].PropertyPath != "$.entities[*].handle" {
		t.Errorf("mapping = %+v", resp.ReverseSearchPropertiesMapping)
	}
	hasRS := false
	for _, c := range resp.Conformance.Conformance {
		if c == "reverse_search" {
			hasRS = true
		}
	}
	if !hasRS {
		t.Errorf("conformance missing reverse_search: %v", resp.Conformance.Conformance)
	}

	// Missing property -> 400.
	rec = doRequest(t, router, http.MethodGet, "/domains/reverse_search/entity")
	if rec.Code != http.StatusBadRequest {
		t.Errorf("missing param expected 400, got %d", rec.Code)
	}

	// Multiple properties -> 400.
	rec = doRequest(t, router, http.MethodGet, "/domains/reverse_search/entity?handle=x&role=y")
	if rec.Code != http.StatusBadRequest {
		t.Errorf("ambiguous expected 400, got %d", rec.Code)
	}
}

func TestReverseSearchDisabledHandler(t *testing.T) {
	router := newTestHandler(t, false)
	rec := doRequest(t, router, http.MethodGet, "/domains/reverse_search/entity?handle=REG1*")
	if rec.Code != http.StatusNotImplemented {
		t.Errorf("expected 501 when extension disabled, got %d", rec.Code)
	}
}

func TestExtensionConformanceInLookups(t *testing.T) {
	cfg := config.RDAPConfig{
		BaseURL:          "https://rdap.example.com",
		RegistrarBaseURL: "https://rdap.example.org/rdap/",
		Mode:             "registrar",
		MaxSearchLimit:   100,
		Extensions:       []string{"ttl0", "geofeed1", "cidr0"},
		TTL0:             &config.TTL0Config{Domain: map[string]int{"NS": 3600}, Nameserver: map[string]int{"A": 60}},
		Geofeed:          &config.GeofeedConfig{URL: "https://geofeed.example.com/feed.csv"},
	}
	router := newTestHandlerCfg(t, cfg)

	// IP network: geofeed1 + cidr0 conformance.
	rec := doRequest(t, router, http.MethodGet, "/ip/8.8.8.0/24")
	if rec.Code != http.StatusOK {
		t.Fatalf("ip expected 200, got %d", rec.Code)
	}
	var n rdap.IPNetwork
	if err := json.Unmarshal(rec.Body.Bytes(), &n); err != nil {
		t.Fatalf("decode: %v", err)
	}
	hasConf := func(conf []string, want string) bool {
		for _, c := range conf {
			if c == want {
				return true
			}
		}
		return false
	}
	// Response wrapper embeds conformance in the IPNetworkResponse; unmarshal
	// into a map to inspect rdapConformance at the top level.
	var m map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &m); err != nil {
		t.Fatalf("decode map: %v", err)
	}
	conf, _ := m["rdapConformance"].([]interface{})
	var confStrs []string
	for _, c := range conf {
		confStrs = append(confStrs, c.(string))
	}
	if !hasConf(confStrs, "geofeed1") || !hasConf(confStrs, "cidr0") {
		t.Errorf("ip conformance = %v", confStrs)
	}
	_ = n
}
