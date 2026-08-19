package server

import (
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/go-chi/httprate"
	"github.com/rdap-server/rdap/internal/config"
	"github.com/rdap-server/rdap/internal/handlers"
	"github.com/rdap-server/rdap/internal/middleware"
	"github.com/rdap-server/rdap/internal/store"
	"go.uber.org/zap"
)

type Server struct {
	*http.Server
	handler http.Handler
	logger  *zap.Logger
}

func New(cfg *config.Config, store store.Interface, logger *zap.Logger) *Server {
	r := chi.NewRouter()

	// Global middleware
	r.Use(chimw.RequestID)
	r.Use(chimw.Compress(5))
	// Client IP resolution must run BEFORE rate limiting. It only trusts
	// X-Forwarded-For / X-Real-IP when the direct peer is a trusted proxy,
	// preventing clients from spoofing their IP to bypass rate limits.
	r.Use(middleware.TrustedProxyClientIP(cfg.Rate.TrustedIPs))
	r.Use(middleware.Logger(logger))
	r.Use(chimw.Recoverer)
	r.Use(chimw.Timeout(30 * time.Second))

	// CORS
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   []string{"*"},
		AllowedMethods:   []string{"GET", "HEAD", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Content-Type", "Authorization"},
		ExposedHeaders:   []string{"Link", "X-RateLimit-Limit", "X-RateLimit-Remaining", "X-RateLimit-Reset"},
		MaxAge:           86400,
	}))

	// Rate limiting
	if cfg.Rate.Enabled {
		rateLimiter := httprate.LimitByIP(cfg.Rate.Requests, time.Duration(cfg.Rate.Window))
		r.Use(rateLimiter)
	}

	// RDAP content type
	r.Use(middleware.RDAPContentType)

	// Security headers
	r.Use(middleware.SecurityHeaders)

	// Authentication (optional)
	if cfg.Auth.Enabled {
		authMiddleware := middleware.NewAuthMiddleware(cfg.Auth)
		r.Use(authMiddleware.Authenticate)
	}

	// Metrics
	r.Use(middleware.Metrics)

	// Register handlers
	handler := handlers.New(store, cfg.RDAP, cfg.Server.Port)
	handler.RegisterRoutes(r)

	// Health check
	r.Get("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/health+json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, `{"status":"ok","version":"%s"}`, cfg.RDAP.Version)
	})

	srv := &http.Server{
		Addr:           cfg.Addr(),
		Handler:        r,
		ReadTimeout:    cfg.Server.ReadTimeout,
		WriteTimeout:   cfg.Server.WriteTimeout,
		IdleTimeout:    cfg.Server.IdleTimeout,
		MaxHeaderBytes: cfg.Server.MaxHeaderBytes,
	}

	return &Server{
		Server:  srv,
		handler: r,
		logger:  logger,
	}
}
