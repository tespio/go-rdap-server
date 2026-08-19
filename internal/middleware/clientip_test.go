package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func testHandler(t *testing.T) http.Handler {
	t.Helper()
	mw := TrustedProxyClientIP([]string{"127.0.0.1", "10.0.0.0/8"})
	return mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(GetClientIP(r)))
	}))
}

func doReq(h http.Handler, remoteAddr string, headers map[string]string) string {
	req := httptest.NewRequest("GET", "http://example.com/", nil)
	req.RemoteAddr = remoteAddr
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec.Body.String()
}

func TestUntrustedClientCannotSpoofXFF(t *testing.T) {
	h := testHandler(t)

	// An arbitrary Internet client (not a trusted proxy) sends X-Forwarded-For.
	// The header MUST be ignored; the rate limiter must see the real socket peer.
	got := doReq(h, "203.0.113.5:12345", map[string]string{
		"X-Forwarded-For": "1.2.3.4",
		"X-Real-IP":       "5.6.7.8",
	})
	if got != "203.0.113.5" {
		t.Fatalf("untrusted XFF spoof leaked through: got %q, want %q", got, "203.0.113.5")
	}
}

func TestTrustedProxyXFFIsHonored(t *testing.T) {
	h := testHandler(t)

	// A trusted proxy (10.x in the allowlist) forwarding the real client IP.
	got := doReq(h, "10.1.2.3:443", map[string]string{
		"X-Forwarded-For": "198.51.100.42",
	})
	if got != "198.51.100.42" {
		t.Fatalf("trusted proxy XFF not honored: got %q, want %q", got, "198.51.100.42")
	}
}

func TestTrustedProxyXRealIPIsHonored(t *testing.T) {
	h := testHandler(t)
	got := doReq(h, "10.1.2.3:443", map[string]string{
		"X-Real-IP": "198.51.100.99",
	})
	if got != "198.51.100.99" {
		t.Fatalf("trusted proxy X-Real-IP not honored: got %q, want %q", got, "198.51.100.99")
	}
}

func TestTrustedProxyPrefersXFFOverXRealIP(t *testing.T) {
	h := testHandler(t)
	got := doReq(h, "127.0.0.1:8443", map[string]string{
		"X-Forwarded-For": "198.51.100.10",
		"X-Real-IP":       "198.51.100.11",
	})
	if got != "198.51.100.10" {
		t.Fatalf("expected XFF to take precedence, got %q", got)
	}
}

func TestTrustedProxyNoHeaderUsesPeer(t *testing.T) {
	h := testHandler(t)
	got := doReq(h, "127.0.0.1:8443", nil)
	if got != "127.0.0.1" {
		t.Fatalf("expected socket peer when no headers, got %q", got)
	}
}

func TestXFFFirstElementUsed(t *testing.T) {
	h := testHandler(t)
	// X-Forwarded-For is a comma list; the leftmost is the original client.
	got := doReq(h, "10.0.0.1:443", map[string]string{
		"X-Forwarded-For": "198.51.100.1, 10.0.0.1, 10.0.0.2",
	})
	if got != "198.51.100.1" {
		t.Fatalf("expected leftmost XFF element, got %q", got)
	}
}

func TestTrustedListEmptyIgnoresAllHeaders(t *testing.T) {
	mw := TrustedProxyClientIP(nil)
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(GetClientIP(r)))
	}))
	got := doReq(h, "203.0.113.9:5555", map[string]string{"X-Forwarded-For": "8.8.8.8"})
	if got != "203.0.113.9" {
		t.Fatalf("empty trusted list must ignore headers, got %q", got)
	}
}
