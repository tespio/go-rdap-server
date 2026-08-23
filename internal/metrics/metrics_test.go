package metrics

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/tespio/go-rdap-server/internal/config"
)

func TestNewServerDisabledReturnsNil(t *testing.T) {
	if s := NewServer(config.MetricsConfig{Enabled: false}); s != nil {
		t.Errorf("expected nil server when disabled, got %+v", s)
	}
}

func TestNewServerEnabled(t *testing.T) {
	s := NewServer(config.MetricsConfig{Enabled: true, Host: "127.0.0.1", Port: 9091})
	if s == nil {
		t.Fatal("expected non-nil server")
	}
	if s.Addr != "127.0.0.1:9091" {
		t.Errorf("Addr = %q", s.Addr)
	}
	if s.Handler == nil {
		t.Error("handler is nil")
	}

	// healthz responds.
	rec := httptest.NewRecorder()
	s.Handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if rec.Code != http.StatusOK {
		t.Errorf("healthz status = %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/health+json" {
		t.Errorf("healthz content-type = %q", ct)
	}

	// metrics endpoint responds with Prometheus text.
	rec = httptest.NewRecorder()
	s.Handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	if rec.Code != http.StatusOK {
		t.Errorf("metrics status = %d", rec.Code)
	}
	if len(rec.Body.String()) == 0 {
		t.Error("metrics body empty")
	}
}
