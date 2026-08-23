package auth

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"testing"
	"time"

	"github.com/tespio/go-rdap-server/internal/config"
)

func encodePart(v interface{}) string {
	b, _ := json.Marshal(v)
	return base64.RawURLEncoding.EncodeToString(b)
}

func makeToken(t *testing.T, claims Claims) string {
	t.Helper()
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"RS256","typ":"JWT"}`))
	return header + "." + encodePart(claims) + ".fakesignature"
}

func TestValidateTokenValid(t *testing.T) {
	now := time.Now().Unix()
	token := makeToken(t, Claims{
		Subject:  "user1",
		Issuer:   "https://auth.example.com",
		Audience: []string{"rdap.example.com"},
		Expiry:   now + 3600,
		IssuedAt: now,
		Scope:    "read",
	})
	j := NewJWTAuthenticator(config.AuthConfig{
		Issuer:   "https://auth.example.com",
		Audience: "rdap.example.com",
	})
	claims, err := j.ValidateToken(token)
	if err != nil {
		t.Fatalf("ValidateToken: %v", err)
	}
	if claims.Subject != "user1" || claims.Scope != "read" {
		t.Errorf("claims = %+v", claims)
	}
}

func TestValidateTokenErrors(t *testing.T) {
	now := time.Now().Unix()

	cases := []struct {
		name  string
		token string
	}{
		{"not enough parts", "only.two"},
		{"bad base64 claims", "abc.notbase64!!.sig"},
		{"expired", makeToken(t, Claims{Subject: "u", Issuer: "https://auth.example.com", Audience: []string{"rdap.example.com"}, Expiry: now - 10})},
		{"wrong issuer", makeToken(t, Claims{Subject: "u", Issuer: "https://evil.example.com", Audience: []string{"rdap.example.com"}, Expiry: now + 3600})},
		{"wrong audience", makeToken(t, Claims{Subject: "u", Issuer: "https://auth.example.com", Audience: []string{"other.example.com"}, Expiry: now + 3600})},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			j := NewJWTAuthenticator(config.AuthConfig{
				Issuer:   "https://auth.example.com",
				Audience: "rdap.example.com",
			})
			if _, err := j.ValidateToken(tc.token); err == nil {
				t.Errorf("expected error for %s", tc.name)
			}
		})
	}
}

func TestValidateTokenNoExpiry(t *testing.T) {
	// Expiry == 0 -> not enforced.
	token := makeToken(t, Claims{Subject: "u", Issuer: "https://auth.example.com", Audience: []string{"rdap.example.com"}})
	j := NewJWTAuthenticator(config.AuthConfig{
		Issuer:   "https://auth.example.com",
		Audience: "rdap.example.com",
	})
	if _, err := j.ValidateToken(token); err != nil {
		t.Fatalf("ValidateToken(no exp): %v", err)
	}
}

func TestValidateTokenNoIssuerOrAudienceConfig(t *testing.T) {
	// No issuer/audience configured -> those checks are skipped.
	token := makeToken(t, Claims{Subject: "u", Expiry: time.Now().Unix() + 3600})
	j := NewJWTAuthenticator(config.AuthConfig{})
	if _, err := j.ValidateToken(token); err != nil {
		t.Fatalf("ValidateToken(no config): %v", err)
	}
}

func TestDecodeBase64URL(t *testing.T) {
	// "hello" in base64url.
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
