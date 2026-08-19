package metrics

import (
	"fmt"
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/tespio/go-rdap-server/internal/config"
)

var (
	HTTPRequestsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "rdap_http_requests_total",
			Help: "Total number of HTTP requests",
		},
		[]string{"method", "path", "status"},
	)

	HTTPRequestDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "rdap_http_request_duration_seconds",
			Help:    "HTTP request duration in seconds",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"method", "path"},
	)

	ActiveConnections = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "rdap_active_connections",
			Help: "Current number of active connections",
		},
	)

	RateLimitHits = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "rdap_rate_limit_hits_total",
			Help: "Total number of rate limit hits",
		},
	)

	DomainLookups = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "rdap_domain_lookups_total",
			Help: "Total number of domain lookups",
		},
		[]string{"tld", "found"},
	)

	EntityLookups = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "rdap_entity_lookups_total",
			Help: "Total number of entity lookups",
		},
	)

	SearchQueries = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "rdap_search_queries_total",
			Help: "Total number of search queries",
		},
		[]string{"type"},
	)

	StorageErrors = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "rdap_storage_errors_total",
			Help: "Total number of storage errors",
		},
		[]string{"operation"},
	)

	Up = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "rdap_up",
			Help: "1 if the RDAP server is up, 0 otherwise",
		},
	)
)

func init() {
	prometheus.MustRegister(HTTPRequestsTotal)
	prometheus.MustRegister(HTTPRequestDuration)
	prometheus.MustRegister(ActiveConnections)
	prometheus.MustRegister(RateLimitHits)
	prometheus.MustRegister(DomainLookups)
	prometheus.MustRegister(EntityLookups)
	prometheus.MustRegister(SearchQueries)
	prometheus.MustRegister(StorageErrors)
	prometheus.MustRegister(Up)

	Up.Set(1)
}

func NewServer(cfg config.MetricsConfig) *http.Server {
	if !cfg.Enabled {
		return nil
	}

	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/health+json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, `{"status":"ok"}`)
	})

	return &http.Server{
		Addr:    cfg.Addr(),
		Handler: mux,
	}
}
