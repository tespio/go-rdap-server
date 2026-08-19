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
	st, err := store.NewMemoryStore(config.StorageConfig{})
	if err != nil {
		t.Fatalf("memory store: %v", err)
	}
	cfg := config.RDAPConfig{
		BaseURL:        "https://rdap.example.com",
		Mode:           "registrar",
		MaxSearchLimit: 100,
		SearchEnabled:  searchEnabled,
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
