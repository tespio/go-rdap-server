package middleware

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/tespio/go-rdap-server/internal/config"
)

func enc(v interface{}) string {
	b, _ := json.Marshal(v)
	return base64.RawURLEncoding.EncodeToString(b)
}

func testToken(claims map[string]interface{}) string {
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"RS256"}`))
	return header + "." + enc(claims) + ".sig"
}

func TestAuthenticateSuccess(t *testing.T) {
	cfg := config.AuthConfig{
		Issuer:   "https://auth.example.com",
		Audience: "rdap.example.com",
	}
	am := NewAuthMiddleware(cfg)
	token := testToken(map[string]interface{}{
		"sub": "u1",
		"iss": "https://auth.example.com",
		"aud": []string{"rdap.example.com"},
		"exp": time.Now().Add(time.Hour).Unix(),
	})

	var called bool
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/domain/example.com", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	am.Authenticate(next).ServeHTTP(rec, req)

	if !called {
		t.Error("next handler was not called")
	}
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d", rec.Code)
	}
}

func TestAuthenticateFailures(t *testing.T) {
	cfg := config.AuthConfig{Issuer: "https://auth.example.com", Audience: "rdap.example.com"}
	am := NewAuthMiddleware(cfg)
	expired := testToken(map[string]interface{}{
		"iss": "https://auth.example.com",
		"aud": []string{"rdap.example.com"},
		"exp": time.Now().Add(-time.Hour).Unix(),
	})

	cases := []struct {
		name  string
		token string
	}{
		{"missing header", ""},
		{"non-bearer", "Basic abc123"},
		{"invalid token", "Bearer not.a.token"},
		{"expired", "Bearer " + expired},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			called := false
			next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				called = true
			})
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			if tc.token != "" {
				req.Header.Set("Authorization", tc.token)
			}
			am.Authenticate(next).ServeHTTP(rec, req)
			if called {
				t.Error("next handler should not be called")
			}
			if rec.Code != http.StatusUnauthorized {
				t.Errorf("status = %d, want 401", rec.Code)
			}
		})
	}
}
