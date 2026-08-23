package server

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/tespio/go-rdap-server/internal/config"
	"github.com/tespio/go-rdap-server/internal/store"
	"go.uber.org/zap"
)

func testServer(t *testing.T, mutate func(*config.Config)) http.Handler {
	t.Helper()
	cfg := &config.Config{}
	cfg.Server = config.ServerConfig{Port: 8443}
	cfg.RDAP = config.RDAPConfig{
		BaseURL: "https://rdap.example.com",
		Mode:    "registrar",
	}
	cfg.Metrics = config.MetricsConfig{Enabled: false}
	cfg.Rate = config.RateConfig{Enabled: false}
	if mutate != nil {
		mutate(cfg)
	}
	st, err := store.NewMemoryStore(cfg.Storage)
	if err != nil {
		t.Fatalf("memory store: %v", err)
	}
	logger := zap.NewNop()
	s := New(cfg, st, logger)
	return s.handler
}

func TestServerRoutes(t *testing.T) {
	h := testServer(t, nil)

	cases := []struct {
		method string
		target string
		want   int
	}{
		{http.MethodGet, "/help", http.StatusOK},
		{http.MethodGet, "/", http.StatusOK},
		{http.MethodGet, "/healthz", http.StatusOK},
		{http.MethodGet, "/domain/example.com", http.StatusOK},
		{http.MethodGet, "/entity/2", http.StatusOK},
		{http.MethodGet, "/nameserver/ns1.example.com", http.StatusOK},
		{http.MethodGet, "/ip/8.8.8.0/24", http.StatusOK},
		{http.MethodGet, "/autnum/15169", http.StatusOK},
		{http.MethodGet, "/domains?name=example*", http.StatusNotImplemented}, // search disabled by default
		{http.MethodGet, "/domain/not-a-domain.invalid", http.StatusNotFound},
	}
	for _, tc := range cases {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(tc.method, tc.target, nil)
		h.ServeHTTP(rec, req)
		if rec.Code != tc.want {
			t.Errorf("%s %s: got %d, want %d", tc.method, tc.target, rec.Code, tc.want)
		}
	}
}

func TestServerHealthz(t *testing.T) {
	h := testServer(t, nil)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("healthz status = %d", rec.Code)
	}
	if got := rec.Header().Get("Content-Type"); got != "application/health+json" {
		t.Errorf("healthz content-type = %q", got)
	}
}

func TestServerAddr(t *testing.T) {
	cfg := &config.Config{}
	cfg.Server = config.ServerConfig{Host: "0.0.0.0", Port: 8443}
	cfg.RDAP = config.RDAPConfig{BaseURL: "https://rdap.example.com", Mode: "registrar"}
	st, err := store.NewMemoryStore(cfg.Storage)
	if err != nil {
		t.Fatalf("store: %v", err)
	}
	s := New(cfg, st, zap.NewNop())
	if s.Addr != ":8443" {
		t.Errorf("Addr = %q", s.Addr)
	}
	if s.handler == nil {
		t.Error("handler is nil")
	}
}
