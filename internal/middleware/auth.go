package middleware

import (
	"context"
	"net/http"
	"strings"

	"github.com/rdap-server/rdap/internal/auth"
	"github.com/rdap-server/rdap/internal/config"
)

type AuthMiddleware struct {
	auth auth.Authenticator
}

func NewAuthMiddleware(cfg config.AuthConfig) *AuthMiddleware {
	return &AuthMiddleware{
		auth: auth.NewJWTAuthenticator(cfg),
	}
}

func (am *AuthMiddleware) Authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			http.Error(w, `{"errorCode":401,"title":"Unauthorized","description":["Authorization header required"]}`, http.StatusUnauthorized)
			return
		}

		token := strings.TrimPrefix(authHeader, "Bearer ")
		if token == authHeader {
			http.Error(w, `{"errorCode":401,"title":"Unauthorized","description":["Bearer token required"]}`, http.StatusUnauthorized)
			return
		}

		claims, err := am.auth.ValidateToken(token)
		if err != nil {
			http.Error(w, `{"errorCode":401,"title":"Unauthorized","description":["Invalid or expired token"]}`, http.StatusUnauthorized)
			return
		}

		ctx := context.WithValue(r.Context(), "claims", claims)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
