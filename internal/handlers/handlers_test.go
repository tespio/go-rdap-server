package handlers

import (
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
