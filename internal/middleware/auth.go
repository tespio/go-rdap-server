package middleware

import (
	"context"
	"net/http"
	"strings"

	"github.com/tespio/go-rdap-server/internal/auth"
	"github.com/tespio/go-rdap-server/internal/config"
)

type AuthMiddleware struct {
	auth auth.Authenticator
}

func NewAuthMiddleware(cfg config.AuthConfig) *AuthMiddleware {
	return &AuthMiddleware{
		auth: auth.NewJWTAuthenticator(cfg),
	}
}

// Authenticate requires a valid OAuth 2.0 / JWT bearer token on every request.
// On failure it returns 401 with the RDAP error object and a WWW-Authenticate
// header per RFC 6750 §3 (Bearer challenge), so clients can react properly.
func (am *AuthMiddleware) Authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			w.Header().Set("WWW-Authenticate", `Bearer realm="rdap", error="invalid_request", error_description="Authorization header required"`)
			http.Error(w, `{"errorCode":401,"title":"Unauthorized","description":["Authorization header required"]}`, http.StatusUnauthorized)
			return
		}

		token := strings.TrimPrefix(authHeader, "Bearer ")
		if token == authHeader || token == "" {
			w.Header().Set("WWW-Authenticate", `Bearer realm="rdap", error="invalid_token", error_description="Bearer token required"`)
			http.Error(w, `{"errorCode":401,"title":"Unauthorized","description":["Bearer token required"]}`, http.StatusUnauthorized)
			return
		}

		claims, err := am.auth.ValidateToken(token)
		if err != nil {
			w.Header().Set("WWW-Authenticate", `Bearer realm="rdap", error="invalid_token", error_description="Invalid or expired token"`)
			http.Error(w, `{"errorCode":401,"title":"Unauthorized","description":["Invalid or expired token"]}`, http.StatusUnauthorized)
			return
		}

		ctx := context.WithValue(r.Context(), auth.ClaimsKey, claims)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
