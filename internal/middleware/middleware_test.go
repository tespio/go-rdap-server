package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

func TestLogger(t *testing.T) {
	logs, obs := observer.New(zap.InfoLevel)
	logger := zap.New(logs)

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTeapot)
		w.Write([]byte("hi"))
	})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/domain/example.com", nil)
	req.RemoteAddr = "1.2.3.4:5678"
	req.Header.Set("User-Agent", "test-agent")

	Logger(logger)(next).ServeHTTP(rec, req)

	if rec.Code != http.StatusTeapot {
		t.Fatalf("status = %d", rec.Code)
	}
	if obs.Len() != 1 {
		t.Fatalf("expected 1 log, got %d", obs.Len())
	}
	entry := obs.All()[0]
	if entry.Message != "request" {
		t.Errorf("message = %q", entry.Message)
	}
	fields := map[string]interface{}{}
	for _, f := range entry.Context {
		switch {
		case f.String != "":
			fields[f.Key] = f.String
		case f.Integer != 0:
			fields[f.Key] = f.Integer
		case f.Interface != nil:
			fields[f.Key] = f.Interface
		}
	}
	if fields["method"] != "GET" || fields["path"] != "/domain/example.com" {
		t.Errorf("fields = %v", fields)
	}
	if fields["status"] != int64(http.StatusTeapot) {
		t.Errorf("status field = %v", fields["status"])
	}
	if fields["remote"] != "1.2.3.4:5678" || fields["user_agent"] != "test-agent" {
		t.Errorf("remote/user_agent = %v/%v", fields["remote"], fields["user_agent"])
	}
}

func TestResponseWriterWriteHeader(t *testing.T) {
	rw := &responseWriter{ResponseWriter: httptest.NewRecorder(), statusCode: http.StatusOK}
	rw.WriteHeader(http.StatusCreated)
	if rw.statusCode != http.StatusCreated {
		t.Errorf("statusCode = %d", rw.statusCode)
	}
}

func TestRDAPContentType(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := w.Header().Get("Content-Type"); got != "application/rdap+json" {
			t.Errorf("content-type = %q", got)
		}
		w.WriteHeader(http.StatusOK)
	})
	rec := httptest.NewRecorder()
	RDAPContentType(next).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestSecurityHeaders(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	rec := httptest.NewRecorder()
	SecurityHeaders(next).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	for _, header := range []string{
		"Access-Control-Allow-Origin",
		"Access-Control-Allow-Methods",
		"Access-Control-Allow-Headers",
		"Access-Control-Max-Age",
		"X-Content-Type-Options",
		"X-Frame-Options",
		"X-XSS-Protection",
		"Referrer-Policy",
		"Strict-Transport-Security",
	} {
		if rec.Header().Get(header) == "" {
			t.Errorf("missing header %s", header)
		}
	}
	if rec.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Errorf("X-Content-Type-Options = %q", rec.Header().Get("X-Content-Type-Options"))
	}
}

func TestMetricsMiddleware(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	rec := httptest.NewRecorder()
	Metrics(next).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestContextKeyString(t *testing.T) {
	if got := contextKey("foo").String(); got != "foo" {
		t.Errorf("String = %q", got)
	}
}

func TestGetRequestID(t *testing.T) {
	ctx := context.WithValue(context.Background(), RequestIDKey, "req-123")
	if got := GetRequestID(ctx); got != "req-123" {
		t.Errorf("GetRequestID = %q", got)
	}
	if got := GetRequestID(context.Background()); got != "" {
		t.Errorf("GetRequestID(empty) = %q", got)
	}
	// Non-string value.
	ctx = context.WithValue(context.Background(), RequestIDKey, 42)
	if got := GetRequestID(ctx); got != "" {
		t.Errorf("GetRequestID(non-string) = %q", got)
	}
}

func TestGetClientIPFallback(t *testing.T) {
	// No clientIPKey in context -> falls back to peer IP from RemoteAddr.
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "203.0.113.9:9999"
	if got := GetClientIP(req); got != "203.0.113.9" {
		t.Errorf("GetClientIP = %q", got)
	}
}

func TestPeerIPMalformed(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "no-port-here"
	if got := peerIP(req); got != "no-port-here" {
		t.Errorf("peerIP(malformed) = %q", got)
	}
	if got := remotePort(req); got != "" {
		t.Errorf("remotePort(malformed) = %q", got)
	}
}

func TestForwardedFor(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Forwarded-For", "203.0.113.5, 10.0.0.1")
	if got := forwardedFor(req); got != "203.0.113.5" {
		t.Errorf("forwardedFor = %q", got)
	}

	req = httptest.NewRequest(http.MethodGet, "/", nil)
	if got := forwardedFor(req); got != "" {
		t.Errorf("forwardedFor(empty) = %q", got)
	}

	req = httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Forwarded-For", "  ,  ")
	if got := forwardedFor(req); got != "" {
		t.Errorf("forwardedFor(blank) = %q", got)
	}
}

func TestParseCIDRsAndAllowed(t *testing.T) {
	trusted := parseCIDRs([]string{"127.0.0.1", "10.0.0.0/8", "  ", "not-a-cidr", "2001:db8::/32"})
	// "  " is trimmed to empty and skipped; "not-a-cidr" fails to parse.
	if len(trusted) != 3 {
		t.Fatalf("expected 3 parsed CIDRs, got %d", len(trusted))
	}
	if !ipAllowed("127.0.0.1", trusted) {
		t.Error("127.0.0.1 should be allowed")
	}
	if !ipAllowed("10.20.30.40", trusted) {
		t.Error("10.20.30.40 should be allowed")
	}
	if ipAllowed("8.8.8.8", trusted) {
		t.Error("8.8.8.8 should not be allowed")
	}
	if !ipAllowed("2001:db8::1", trusted) {
		t.Error("2001:db8::1 should be allowed (within 2001:db8::/32)")
	}
	if ipAllowed("2001:4860:4860::8888", trusted) {
		t.Error("2001:4860:: should not be allowed")
	}
	if ipAllowed("garbage", trusted) {
		t.Error("garbage should not be allowed")
	}
	// Empty trusted list -> nothing allowed.
	if ipAllowed("127.0.0.1", nil) {
		t.Error("empty trusted list should allow nothing")
	}
}

func TestTrustedProxyClientIP(t *testing.T) {
	run := func(t *testing.T, trusted []string, remoteAddr string, xff, xrip string) (gotRemote string, gotCtx string) {
		t.Helper()
		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			gotRemote = r.RemoteAddr
			gotCtx = GetClientIP(r)
		})
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.RemoteAddr = remoteAddr
		if xff != "" {
			req.Header.Set("X-Forwarded-For", xff)
		}
		if xrip != "" {
			req.Header.Set("X-Real-IP", xrip)
		}
		TrustedProxyClientIP(trusted)(next).ServeHTTP(rec, req)
		return
	}

	t.Run("trusted peer with XFF", func(t *testing.T) {
		remote, ctx := run(t, []string{"10.0.0.1"}, "10.0.0.1:1234", "203.0.113.7, 10.0.0.1", "")
		if !strings.HasPrefix(remote, "203.0.113.7:") {
			t.Errorf("remote = %q", remote)
		}
		if ctx != "203.0.113.7" {
			t.Errorf("ctx ip = %q", ctx)
		}
	})

	t.Run("trusted peer with X-Real-IP", func(t *testing.T) {
		remote, ctx := run(t, []string{"10.0.0.1"}, "10.0.0.1:1234", "", "203.0.113.9")
		if !strings.HasPrefix(remote, "203.0.113.9:") {
			t.Errorf("remote = %q", remote)
		}
		if ctx != "203.0.113.9" {
			t.Errorf("ctx ip = %q", ctx)
		}
	})

	t.Run("untrusted peer ignores XFF", func(t *testing.T) {
		remote, ctx := run(t, []string{"10.0.0.1"}, "198.51.100.2:1234", "203.0.113.7", "")
		if !strings.HasPrefix(remote, "198.51.100.2:") {
			t.Errorf("remote = %q", remote)
		}
		if ctx != "198.51.100.2" {
			t.Errorf("ctx ip = %q", ctx)
		}
	})

	t.Run("no trusted list ignores XFF", func(t *testing.T) {
		remote, ctx := run(t, nil, "198.51.100.2:1234", "203.0.113.7", "")
		if !strings.HasPrefix(remote, "198.51.100.2:") {
			t.Errorf("remote = %q", remote)
		}
		if ctx != "198.51.100.2" {
			t.Errorf("ctx ip = %q", ctx)
		}
	})

	t.Run("trusted peer no forwarded headers", func(t *testing.T) {
		remote, ctx := run(t, []string{"10.0.0.1"}, "10.0.0.1:1234", "", "")
		if !strings.HasPrefix(remote, "10.0.0.1:") {
			t.Errorf("remote = %q", remote)
		}
		if ctx != "10.0.0.1" {
			t.Errorf("ctx ip = %q", ctx)
		}
	})
}
