package auth

import (
	"context"
	"crypto/rsa"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/rdap-server/rdap/internal/config"
)

type Claims struct {
	Subject   string   `json:"sub"`
	Issuer    string   `json:"iss"`
	Audience  []string `json:"aud"`
	Expiry    int64    `json:"exp"`
	IssuedAt  int64    `json:"iat"`
	Scope     string   `json:"scope,omitempty"`
}

type Authenticator interface {
	ValidateToken(token string) (*Claims, error)
}

type JWTAuthenticator struct {
	cfg        config.AuthConfig
	jwksClient *http.Client
	keys       map[string]*rsa.PublicKey
}

func NewJWTAuthenticator(cfg config.AuthConfig) *JWTAuthenticator {
	return &JWTAuthenticator{
		cfg: cfg,
		jwksClient: &http.Client{
			Timeout: 10 * time.Second,
		},
		keys: make(map[string]*rsa.PublicKey),
	}
}

func (j *JWTAuthenticator) ValidateToken(token string) (*Claims, error) {
	// Simplified JWT validation - in production, use a proper JWT library
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return nil, fmt.Errorf("invalid token format")
	}

	// Decode claims (middle part)
	claimsBytes, err := decodeBase64URL(parts[1])
	if err != nil {
		return nil, fmt.Errorf("decode claims: %w", err)
	}

	var claims Claims
	if err := json.Unmarshal(claimsBytes, &claims); err != nil {
		return nil, fmt.Errorf("parse claims: %w", err)
	}

	// Validate expiry
	if claims.Expiry > 0 && time.Now().Unix() > claims.Expiry {
		return nil, fmt.Errorf("token expired")
	}

	// Validate issuer
	if j.cfg.Issuer != "" && claims.Issuer != j.cfg.Issuer {
		return nil, fmt.Errorf("invalid issuer")
	}

	// Validate audience
	if j.cfg.Audience != "" {
		validAud := false
		for _, aud := range claims.Audience {
			if aud == j.cfg.Audience {
				validAud = true
				break
			}
		}
		if !validAud {
			return nil, fmt.Errorf("invalid audience")
		}
	}

	return &claims, nil
}

func decodeBase64URL(s string) ([]byte, error) {
	// Add padding
	switch len(s) % 4 {
	case 2:
		s += "=="
	case 3:
		s += "="
	}

	// Replace URL-safe characters
	s = strings.ReplaceAll(s, "-", "+")
	s = strings.ReplaceAll(s, "_", "/")

	// Standard base64 decode
	decoded := make([]byte, len(s)*3/4)
	n := 0
	for i := 0; i < len(s); i++ {
		c := s[i]
		var val byte
		switch {
		case c >= 'A' && c <= 'Z':
			val = c - 'A'
		case c >= 'a' && c <= 'z':
			val = c - 'a' + 26
		case c >= '0' && c <= '9':
			val = c - '0' + 52
		case c == '+':
			val = 62
		case c == '/':
			val = 63
		case c == '=':
			break
		default:
			return nil, fmt.Errorf("invalid character: %c", c)
		}

		switch i % 4 {
		case 0:
			decoded[n] = val << 2
		case 1:
			decoded[n] |= val >> 4
			n++
			decoded[n] = val << 4
		case 2:
			decoded[n] |= val >> 2
			n++
			decoded[n] = val << 6
		case 3:
			decoded[n] |= val
			n++
		}
	}

	return decoded[:n], nil
}

// NoopAuthenticator is used when auth is disabled
type NoopAuthenticator struct{}

func NewNoopAuthenticator() *NoopAuthenticator {
	return &NoopAuthenticator{}
}

func (n *NoopAuthenticator) ValidateToken(token string) (*Claims, error) {
	return &Claims{
		Subject: "anonymous",
		Issuer:  "rdap-server",
		Scope:   "read",
	}, nil
}

// context key
type contextKey string

func (c contextKey) String() string {
	return string(c)
}

var ClaimsKey = contextKey("rdap-claims")

func GetClaims(ctx context.Context) *Claims {
	if claims, ok := ctx.Value(ClaimsKey).(*Claims); ok {
		return claims
	}
	return nil
}
