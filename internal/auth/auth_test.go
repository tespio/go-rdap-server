package auth

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/tespio/go-rdap-server/internal/config"
)

const testIssuer = "https://auth.example.com"
const testAudience = "rdap.example.com"

// ---------------------------------------------------------------------------
// Test helpers
// ---------------------------------------------------------------------------

func b64u(b []byte) string { return base64.RawURLEncoding.EncodeToString(b) }

func rsaJWK(t *testing.T, key *rsa.PublicKey, kid string) map[string]interface{} {
	t.Helper()
	return map[string]interface{}{
		"kty": "RSA",
		"kid": kid,
		"use": "sig",
		"alg": "RS256",
		"n":   b64u(key.N.Bytes()),
		"e":   b64u(big.NewInt(int64(key.E)).Bytes()),
	}
}

func ecJWK(t *testing.T, key *ecdsa.PublicKey, kid string) map[string]interface{} {
	t.Helper()
	return map[string]interface{}{
		"kty": "EC",
		"kid": kid,
		"use": "sig",
		"alg": "ES256",
		"crv": "P-256",
		"x":   b64u(key.X.Bytes()),
		"y":   b64u(key.Y.Bytes()),
	}
}

// jwksServer serves a JWKS document backed by the given keys.
func jwksServer(t *testing.T, keys ...map[string]interface{}) (*httptest.Server, string) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{"keys": keys})
	}))
	t.Cleanup(srv.Close)
	return srv, srv.URL
}

// signToken creates a signed JWT with the given claims using the provided key.
func signToken(t *testing.T, key interface{}, method jwt.SigningMethod, kid string, claims map[string]interface{}) string {
	t.Helper()
	header := map[string]interface{}{
		"alg": method.Alg(),
		"typ": "JWT",
		"kid": kid,
	}
	now := time.Now().Unix()
	claims["iss"] = testIssuer
	claims["aud"] = []string{testAudience}
	claims["iat"] = now
	claims["exp"] = now + 3600

	hb, _ := json.Marshal(header)
	cb, _ := json.Marshal(claims)
	unsigned := b64u(hb) + "." + b64u(cb)
	sig, err := method.Sign(unsigned, key)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	return unsigned + "." + b64u(sig)
}

func testAuthenticator(t *testing.T, mutate func(*config.AuthConfig)) *JWTAuthenticator {
	t.Helper()
	cfg := config.AuthConfig{
		Issuer:   testIssuer,
		Audience: testAudience,
	}
	if mutate != nil {
		mutate(&cfg)
	}
	return NewJWTAuthenticator(cfg)
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

func TestValidateRSTokenWithJWKS(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("rsa: %v", err)
	}
	_, jwksURL := jwksServer(t, rsaJWK(t, &key.PublicKey, "rsa-1"))
	j := testAuthenticator(t, func(c *config.AuthConfig) { c.JWKSEndpoint = jwksURL })

	token := signToken(t, key, jwt.SigningMethodRS256, "rsa-1", map[string]interface{}{
		"sub": "user1", "scope": "read",
	})
	claims, err := j.ValidateToken(token)
	if err != nil {
		t.Fatalf("ValidateToken: %v", err)
	}
	if claims.Subject != "user1" || claims.Scope != "read" {
		t.Errorf("claims = %+v", claims)
	}
	if claims.Issuer != testIssuer || claims.Audience[0] != testAudience {
		t.Errorf("issuer/audience = %q / %v", claims.Issuer, claims.Audience)
	}
}

func TestValidateESTokenWithJWKS(t *testing.T) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("ecdsa: %v", err)
	}
	_, jwksURL := jwksServer(t, ecJWK(t, &key.PublicKey, "ec-1"))
	j := testAuthenticator(t, func(c *config.AuthConfig) { c.JWKSEndpoint = jwksURL })

	token := signToken(t, key, jwt.SigningMethodES256, "ec-1", map[string]interface{}{
		"sub": "ec-user",
	})
	if _, err := j.ValidateToken(token); err != nil {
		t.Fatalf("ValidateToken ES256: %v", err)
	}
}

func TestValidateTokenIssuerFallbackJWKS(t *testing.T) {
	// When only issuer is configured, the JWKS URL defaults to
	// issuer/.well-known/jwks.json.
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("rsa: %v", err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/.well-known/jwks.json" {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{"keys": []interface{}{rsaJWK(t, &key.PublicKey, "k1")}})
	}))
	t.Cleanup(srv.Close)

	j := testAuthenticator(t, func(c *config.AuthConfig) { c.Issuer = srv.URL })
	// Sign with the test server URL as the issuer (signToken hardcodes
	// testIssuer; rebuild manually here).
	now := time.Now().Unix()
	hb, _ := json.Marshal(map[string]interface{}{"alg": "RS256", "kid": "k1"})
	cb, _ := json.Marshal(map[string]interface{}{
		"sub": "u", "iss": srv.URL, "aud": []string{testAudience}, "iat": now, "exp": now + 3600,
	})
	unsigned := b64u(hb) + "." + b64u(cb)
	sig, _ := jwt.SigningMethodRS256.Sign(unsigned, key)
	token := unsigned + "." + b64u(sig)
	if _, err := j.ValidateToken(token); err != nil {
		t.Fatalf("ValidateToken: %v", err)
	}
}

func TestValidateTokenRejectsForgedSignature(t *testing.T) {
	// A token signed by an unknown key must be rejected â€” this is the core
	// security property OAuth2/JWKS adds over claim-only validation.
	goodKey, _ := rsa.GenerateKey(rand.Reader, 2048)
	evilKey, _ := rsa.GenerateKey(rand.Reader, 2048)
	_, jwksURL := jwksServer(t, rsaJWK(t, &goodKey.PublicKey, "good"))
	j := testAuthenticator(t, func(c *config.AuthConfig) { c.JWKSEndpoint = jwksURL })

	// Same kid, but signed with the wrong private key.
	token := signToken(t, evilKey, jwt.SigningMethodRS256, "good", map[string]interface{}{"sub": "u"})
	if _, err := j.ValidateToken(token); err == nil {
		t.Fatal("expected forged token to be rejected")
	}
}

func TestValidateTokenExpired(t *testing.T) {
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	_, jwksURL := jwksServer(t, rsaJWK(t, &key.PublicKey, "k"))
	j := testAuthenticator(t, func(c *config.AuthConfig) { c.JWKSEndpoint = jwksURL })

	// Build an expired token manually (signToken always sets exp=now+3600).
	now := time.Now().Unix()
	hb, _ := json.Marshal(map[string]interface{}{"alg": "RS256", "kid": "k"})
	cb, _ := json.Marshal(map[string]interface{}{
		"sub": "u", "iss": testIssuer, "aud": []string{testAudience},
		"iat": now - 7200, "exp": now - 3600,
	})
	unsigned := b64u(hb) + "." + b64u(cb)
	sig, _ := jwt.SigningMethodRS256.Sign(unsigned, key)
	token := unsigned + "." + b64u(sig)
	if _, err := j.ValidateToken(token); err == nil {
		t.Fatal("expected expired token to be rejected")
	}
}

func TestValidateTokenWrongIssuer(t *testing.T) {
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	_, jwksURL := jwksServer(t, rsaJWK(t, &key.PublicKey, "k"))
	j := testAuthenticator(t, func(c *config.AuthConfig) { c.JWKSEndpoint = jwksURL })

	hb, _ := json.Marshal(map[string]interface{}{"alg": "RS256", "kid": "k"})
	cb, _ := json.Marshal(map[string]interface{}{
		"sub": "u", "iss": "https://evil.example.com", "aud": []string{testAudience},
		"iat": time.Now().Unix(), "exp": time.Now().Add(time.Hour).Unix(),
	})
	unsigned := b64u(hb) + "." + b64u(cb)
	sig, _ := jwt.SigningMethodRS256.Sign(unsigned, key)
	token := unsigned + "." + b64u(sig)
	if _, err := j.ValidateToken(token); err == nil {
		t.Fatal("expected wrong-issuer token to be rejected")
	}
}

func TestValidateTokenMalformed(t *testing.T) {
	j := testAuthenticator(t, func(c *config.AuthConfig) { c.JWKSEndpoint = "http://127.0.0.1:1/jwks" })
	for _, tok := range []string{"", "only.two", "a.b.c.d"} {
		if _, err := j.ValidateToken(tok); err == nil {
			t.Errorf("expected error for malformed token %q", tok)
		}
	}
}

func TestValidateTokenRFC9560Claims(t *testing.T) {
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	_, jwksURL := jwksServer(t, rsaJWK(t, &key.PublicKey, "k"))
	j := testAuthenticator(t, func(c *config.AuthConfig) { c.JWKSEndpoint = jwksURL })

	token := signToken(t, key, jwt.SigningMethodRS256, "k", map[string]interface{}{
		"sub":                   "user1",
		"rdap_allowed_purposes": []string{"domainNameControl", "dnsTransparency"},
		"rdap_dnt_allowed":      true,
	})
	claims, err := j.ValidateToken(token)
	if err != nil {
		t.Fatalf("ValidateToken: %v", err)
	}
	if len(claims.AllowedPurposes) != 2 || claims.AllowedPurposes[0] != "domainNameControl" {
		t.Errorf("allowed purposes = %v", claims.AllowedPurposes)
	}
	if !claims.DoNotTrackAllowed {
		t.Error("dnt_allowed should be true")
	}
}

func TestParseJWK(t *testing.T) {
	if _, err := parseJWK(map[string]interface{}{"kty": "foo"}); err == nil {
		t.Error("expected error for unknown kty")
	}
	if _, err := parseJWK(map[string]interface{}{"kty": "RSA"}); err == nil {
		t.Error("expected error for missing RSA components")
	}
	if _, err := parseJWK(map[string]interface{}{"kty": "EC", "crv": "P-256"}); err == nil {
		t.Error("expected error for missing EC components")
	}
	if _, err := parseJWK(map[string]interface{}{"kty": "EC", "crv": "weird", "x": "AAAA", "y": "AAAA"}); err == nil {
		t.Error("expected error for unsupported EC curve")
	}
}

func TestDecodeBase64URL(t *testing.T) {
	b, err := decodeBase64URL("aGVsbG8")
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if string(b) != "hello" {
		t.Errorf("decoded = %q", string(b))
	}
	if _, err := decodeBase64URL("a!b"); err == nil {
		t.Error("expected error for invalid char")
	}
}

func TestNoopAuthenticator(t *testing.T) {
	n := NewNoopAuthenticator()
	claims, err := n.ValidateToken("whatever")
	if err != nil {
		t.Fatalf("ValidateToken: %v", err)
	}
	if claims.Subject != "anonymous" || claims.Issuer != "rdap-server" {
		t.Errorf("claims = %+v", claims)
	}
}

func TestContextKeyString(t *testing.T) {
	if got := contextKey("k").String(); got != "k" {
		t.Errorf("String = %q", got)
	}
}

func TestGetClaims(t *testing.T) {
	if got := GetClaims(context.Background()); got != nil {
		t.Errorf("GetClaims(empty) = %+v", got)
	}
	claims := &Claims{Subject: "s"}
	ctx := context.WithValue(context.Background(), ClaimsKey, claims)
	if got := GetClaims(ctx); got != claims {
		t.Errorf("GetClaims = %+v", got)
	}
}

func TestSupportedAlgorithms(t *testing.T) {
	// Default: all algorithms accepted.
	j := testAuthenticator(t, nil)
	all := j.supportedAlgorithms()
	if len(all) != 9 {
		t.Errorf("expected 9 default algorithms, got %d: %v", len(all), all)
	}

	// Restriction filters.
	j = testAuthenticator(t, func(c *config.AuthConfig) { c.Algorithms = []string{"RS256"} })
	if got := j.supportedAlgorithms(); len(got) != 1 || got[0] != "RS256" {
		t.Errorf("restricted = %v", got)
	}

	// Unknown algorithms are dropped; empty result falls back to all.
	j = testAuthenticator(t, func(c *config.AuthConfig) { c.Algorithms = []string{"HS256", "bogus"} })
	if got := j.supportedAlgorithms(); len(got) != 9 {
		t.Errorf("unknown alg fallback = %v", got)
	}
}

func TestJWKSRefreshErrors(t *testing.T) {
	// Unreachable JWKS -> token validation fails.
	j := testAuthenticator(t, func(c *config.AuthConfig) {
		c.JWKSEndpoint = "http://127.0.0.1:1/nope"
	})
	// Need a well-formed token to reach the keyfunc path.
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	token := signToken(t, key, jwt.SigningMethodRS256, "any", map[string]interface{}{"sub": "u"})
	if _, err := j.ValidateToken(token); err == nil {
		t.Error("expected error when JWKS unreachable")
	}
}

func TestJWKSNoSupportedKeys(t *testing.T) {
	// JWKS server returns only an unsupported kty.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"keys": []interface{}{map[string]interface{}{"kty": "oct", "kid": "x"}},
		})
	}))
	t.Cleanup(srv.Close)
	j := testAuthenticator(t, func(c *config.AuthConfig) { c.JWKSEndpoint = srv.URL })
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	token := signToken(t, key, jwt.SigningMethodRS256, "x", map[string]interface{}{"sub": "u"})
	if _, err := j.ValidateToken(token); err == nil {
		t.Error("expected error when JWKS has no supported keys")
	}
}

func TestJWKSNon200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)
	j := testAuthenticator(t, func(c *config.AuthConfig) { c.JWKSEndpoint = srv.URL })
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	token := signToken(t, key, jwt.SigningMethodRS256, "x", map[string]interface{}{"sub": "u"})
	if _, err := j.ValidateToken(token); err == nil {
		t.Error("expected error on JWKS HTTP error")
	}
}
