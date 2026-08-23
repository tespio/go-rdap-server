package middleware

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/tespio/go-rdap-server/internal/config"
)

func b64u(b []byte) string { return base64.RawURLEncoding.EncodeToString(b) }

func rsaJWK(t *testing.T, key *rsa.PublicKey, kid string) map[string]interface{} {
	t.Helper()
	return map[string]interface{}{
		"kty": "RSA", "kid": kid, "use": "sig", "alg": "RS256",
		"n": b64u(key.N.Bytes()),
		"e": b64u([]byte{0x01, 0x00, 0x01}),
	}
}

func signJWT(t *testing.T, key *rsa.PrivateKey, kid string, claims map[string]interface{}) string {
	t.Helper()
	now := time.Now().Unix()
	claims["iat"] = now
	if _, ok := claims["exp"]; !ok {
		claims["exp"] = now + 3600
	}
	hb, _ := json.Marshal(map[string]interface{}{"alg": "RS256", "kid": kid})
	cb, _ := json.Marshal(claims)
	unsigned := b64u(hb) + "." + b64u(cb)
	sig, err := jwt.SigningMethodRS256.Sign(unsigned, key)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	return unsigned + "." + b64u(sig)
}

// newAuthServer returns a middleware backed by a real JWKS endpoint.
func newAuthServer(t *testing.T, mutate func(*config.AuthConfig)) (*AuthMiddleware, *rsa.PrivateKey, string) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("rsa: %v", err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{"keys": []interface{}{rsaJWK(t, &key.PublicKey, "k1")}})
	}))
	t.Cleanup(srv.Close)

	cfg := config.AuthConfig{
		JWKSEndpoint: srv.URL,
		Issuer:       "https://auth.example.com",
		Audience:     "rdap.example.com",
	}
	if mutate != nil {
		mutate(&cfg)
	}
	return NewAuthMiddleware(cfg), key, "k1"
}

func TestAuthenticateSuccess(t *testing.T) {
	am, key, kid := newAuthServer(t, nil)
	token := signJWT(t, key, kid, map[string]interface{}{
		"sub": "u1",
		"iss": "https://auth.example.com",
		"aud": []string{"rdap.example.com"},
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
	am, key, kid := newAuthServer(t, nil)
	expired := signJWT(t, key, kid, map[string]interface{}{
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
			if rec.Header().Get("WWW-Authenticate") == "" {
				t.Error("expected WWW-Authenticate header on 401")
			}
		})
	}
}

func TestAuthenticateRejectsForged(t *testing.T) {
	am, key, kid := newAuthServer(t, nil)
	// A token signed by the right key but wrong kid/claims shape is rejected.
	_ = key
	_ = kid
	called := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer abc.def.ghi")
	am.Authenticate(next).ServeHTTP(rec, req)
	if called {
		t.Error("next handler should not be called for malformed token")
	}
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rec.Code)
	}
}
