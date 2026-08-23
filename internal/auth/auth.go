package auth

import (
	"context"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/tespio/go-rdap-server/internal/config"
)

type Claims struct {
	Subject  string   `json:"sub"`
	Issuer   string   `json:"iss"`
	Audience []string `json:"aud"`
	Expiry   int64    `json:"exp"`
	IssuedAt int64    `json:"iat"`
	Scope    string   `json:"scope,omitempty"`
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
	return base64.RawURLEncoding.DecodeString(s)
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
