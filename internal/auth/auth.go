// Package auth implements OAuth 2.0 / OpenID Connect bearer-token validation
// for the RDAP server.
//
// Tokens are validated as JSON Web Tokens (RFC 7519): the signature is
// verified against the authorization server's JSON Web Key Set (JWKS, RFC
// 7517), and the standard claims (issuer, audience, expiry, not-before) are
// checked. This complements the previously claim-only JWT handling with real
// cryptographic verification, so a token cannot be forged by an attacker who
// merely knows the expected issuer/audience.
package auth

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/tespio/go-rdap-server/internal/config"
)

// Claims carries the identity and authorization information associated with a
// validated token. It is the canonical form exposed to the request handler via
// the request context.
type Claims struct {
	Subject  string   `json:"sub"`
	Issuer   string   `json:"iss"`
	Audience []string `json:"aud"`
	Expiry   int64    `json:"exp"`
	IssuedAt int64    `json:"iat"`
	Scope    string   `json:"scope,omitempty"`

	// AllowedPurposes is the RFC 9560 "rdap_allowed_purposes" claim, if
	// present. It lists the purposes (e.g. "domainNameControl") for which the
	// end user may request access to protected resources.
	AllowedPurposes []string `json:"rdap_allowed_purposes,omitempty"`
	// DoNotTrackAllowed is the RFC 9560 "rdap_dnt_allowed" claim, if present.
	DoNotTrackAllowed bool `json:"rdap_dnt_allowed,omitempty"`
}

// Authenticator validates bearer tokens and returns the associated claims.
type Authenticator interface {
	ValidateToken(token string) (*Claims, error)
}

// jwtClaims is the on-wire claim shape used during parsing. Standard claims
// plus the RDAP-specific claims defined in RFC 9560.
type jwtClaims struct {
	Subject           string   `json:"sub"`
	Issuer            string   `json:"iss"`
	Audience          []string `json:"aud"`
	Scope             string   `json:"scope,omitempty"`
	AllowedPurposes   []string `json:"rdap_allowed_purposes,omitempty"`
	DoNotTrackAllowed bool     `json:"rdap_dnt_allowed,omitempty"`
	jwt.RegisteredClaims
}

// JWTAuthenticator validates JWT access tokens against an authorization
// server's JWKS endpoint. It fetches and caches the key set, and verifies the
// token signature plus the issuer/audience/expiry/not-before claims.
type JWTAuthenticator struct {
	cfg        config.AuthConfig
	jwksClient *http.Client
	jwksURL    string

	mu     sync.RWMutex
	keySet map[string]interface{} // kid -> parsed public key (rsa/ecdsa)
}

// NewJWTAuthenticator builds a token validator. The JWKS endpoint may be a full
// URL to the key set, or a base issuer URL (OIDC discovery is not performed;
// operators must point directly at the JWKS document, or at an issuer whose
// /.well-known/jwks.json resolves the standard location).
func NewJWTAuthenticator(cfg config.AuthConfig) *JWTAuthenticator {
	jwksURL := strings.TrimSuffix(cfg.JWKSEndpoint, "/")
	if jwksURL == "" {
		jwksURL = strings.TrimSuffix(cfg.Issuer, "/") + "/.well-known/jwks.json"
	}
	return &JWTAuthenticator{
		cfg:        cfg,
		jwksClient: &http.Client{Timeout: 10 * time.Second},
		jwksURL:    jwksURL,
		keySet:     map[string]interface{}{},
	}
}

// jwksDocument is the JWKS response body (RFC 7517 §5).
type jwksDocument struct {
	Keys []map[string]interface{} `json:"keys"`
}

// refreshKeys fetches the JWKS and caches the parsed public keys.
func (j *JWTAuthenticator) refreshKeys() error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, j.jwksURL, nil)
	if err != nil {
		return fmt.Errorf("build jwks request: %w", err)
	}
	resp, err := j.jwksClient.Do(req)
	if err != nil {
		return fmt.Errorf("fetch jwks %s: %w", j.jwksURL, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("fetch jwks %s: HTTP %d", j.jwksURL, resp.StatusCode)
	}
	var doc jwksDocument
	if err := json.NewDecoder(resp.Body).Decode(&doc); err != nil {
		return fmt.Errorf("decode jwks: %w", err)
	}
	if len(doc.Keys) == 0 {
		return fmt.Errorf("jwks contains no keys")
	}

	keySet := map[string]interface{}{}
	for _, k := range doc.Keys {
		kid, _ := k["kid"].(string)
		key, err := parseJWK(k)
		if err != nil {
			continue // skip unsupported key types; at least one must parse
		}
		if kid == "" {
			// Key without a kid: use it as the only key (kid-agnostic tokens).
			keySet[""] = key
		} else {
			keySet[kid] = key
		}
	}
	if len(keySet) == 0 {
		return fmt.Errorf("jwks contained no supported public keys (rsa/ecdsa)")
	}

	j.mu.Lock()
	defer j.mu.Unlock()
	j.keySet = keySet
	return nil
}

// parseJWK converts a single JWK object into a Go public key.
func parseJWK(k map[string]interface{}) (interface{}, error) {
	kty, _ := k["kty"].(string)
	switch kty {
	case "RSA":
		n, okN := base64URLBigInt(k, "n")
		e, okE := base64URLBigInt(k, "e")
		if !okN || !okE || n.Sign() <= 0 || e.Sign() <= 0 || !e.IsInt64() {
			return nil, fmt.Errorf("invalid RSA JWK")
		}
		return &rsa.PublicKey{N: n, E: int(e.Int64())}, nil
	case "EC":
		crv, _ := k["crv"].(string)
		var curve elliptic.Curve
		switch crv {
		case "P-256":
			curve = elliptic.P256()
		case "P-384":
			curve = elliptic.P384()
		case "P-521":
			curve = elliptic.P521()
		default:
			return nil, fmt.Errorf("unsupported EC curve %q", crv)
		}
		x, okX := base64URLBigInt(k, "x")
		y, okY := base64URLBigInt(k, "y")
		if !okX || !okY || x.Sign() <= 0 || y.Sign() <= 0 {
			return nil, fmt.Errorf("invalid EC JWK")
		}
		return &ecdsa.PublicKey{Curve: curve, X: x, Y: y}, nil
	default:
		return nil, fmt.Errorf("unsupported JWK kty %q", kty)
	}
}

func base64URLBigInt(k map[string]interface{}, name string) (*big.Int, bool) {
	s, _ := k[name].(string)
	if s == "" {
		return nil, false
	}
	b, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return nil, false
	}
	return new(big.Int).SetBytes(b), true
}

// keyfunc returns the public key for a token's "kid", refreshing the key set
// from the JWKS endpoint on demand (and on a cache miss).
func (j *JWTAuthenticator) keyfunc(token *jwt.Token) (interface{}, error) {
	kid, _ := token.Header["kid"].(string)

	j.mu.RLock()
	key, ok := j.keySet[kid]
	j.mu.RUnlock()
	if ok {
		return key, nil
	}

	// Cache miss: refresh once and retry. A stale/rotated key is a common
	// cause, so a single refresh is the standard mitigation.
	if err := j.refreshKeys(); err != nil {
		return nil, fmt.Errorf("refresh jwks: %w", err)
	}
	j.mu.RLock()
	key, ok = j.keySet[kid]
	j.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("no public key for kid %q", kid)
	}
	return key, nil
}

// ValidateToken parses and validates a JWT access token, returning its claims.
func (j *JWTAuthenticator) ValidateToken(token string) (*Claims, error) {
	claims := &jwtClaims{}
	parser := jwt.NewParser(
		jwt.WithValidMethods(j.supportedAlgorithms()),
		jwt.WithExpirationRequired(),
		jwt.WithIssuedAt(),
	)
	parsed, err := parser.ParseWithClaims(token, claims, j.keyfunc)
	if err != nil {
		return nil, fmt.Errorf("invalid token: %w", err)
	}
	if !parsed.Valid {
		return nil, fmt.Errorf("invalid token")
	}

	// Issuer check.
	if j.cfg.Issuer != "" && claims.Issuer != j.cfg.Issuer {
		return nil, fmt.Errorf("invalid issuer %q (want %q)", claims.Issuer, j.cfg.Issuer)
	}
	// Audience check.
	if j.cfg.Audience != "" && !audienceContains(claims.Audience, j.cfg.Audience) {
		return nil, fmt.Errorf("invalid audience %v (want %q)", claims.Audience, j.cfg.Audience)
	}

	return &Claims{
		Subject:           claims.Subject,
		Issuer:            claims.Issuer,
		Audience:          claims.Audience,
		Expiry:            claims.ExpiresAt.Unix(),
		IssuedAt:          claims.IssuedAt.Unix(),
		Scope:             claims.Scope,
		AllowedPurposes:   claims.AllowedPurposes,
		DoNotTrackAllowed: claims.DoNotTrackAllowed,
	}, nil
}

// supportedAlgorithms restricts signature algorithms to the modern asymmetric
// family, and to any the operator explicitly allowed.
func (j *JWTAuthenticator) supportedAlgorithms() []string {
	// Default: the algorithms this server verifies.
	all := []string{"RS256", "RS384", "RS512", "ES256", "ES384", "ES512", "PS256", "PS384", "PS512"}
	if len(j.cfg.Algorithms) == 0 {
		return all
	}
	var out []string
	for _, a := range j.cfg.Algorithms {
		for _, allowed := range all {
			if a == allowed {
				out = append(out, a)
			}
		}
	}
	if len(out) == 0 {
		return all
	}
	return out
}

func audienceContains(aud []string, want string) bool {
	for _, a := range aud {
		if a == want {
			return true
		}
	}
	return false
}

func decodeBase64URL(s string) ([]byte, error) {
	return base64.RawURLEncoding.DecodeString(s)
}

// NoopAuthenticator is used when auth is disabled.
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
