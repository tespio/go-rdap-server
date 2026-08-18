package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/rdap-server/rdap/internal/config"
	"github.com/rdap-server/rdap/internal/metrics"
	"github.com/rdap-server/rdap/internal/server"
	"github.com/rdap-server/rdap/internal/store"
	"go.uber.org/zap"
)

func main() {
	configPath := flag.String("config", "config.yaml", "path to config file")
	flag.Parse()

	logger, _ := zap.NewProduction()
	defer logger.Sync()

	cfg, err := config.Load(*configPath)
	if err != nil {
		logger.Fatal("failed to load config", zap.Error(err))
	}

	store, err := store.New(cfg.Storage)
	if err != nil {
		logger.Fatal("failed to init store", zap.Error(err))
	}
	defer store.Close()

	metricsSrv := metrics.NewServer(cfg.Metrics)
	go func() {
		logger.Info("metrics server starting", zap.String("addr", metricsSrv.Addr))
		if err := metricsSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("metrics server error", zap.Error(err))
		}
	}()

	srv := server.New(cfg, store, logger)

	go func() {
		logger.Info("RDAP server starting",
			zap.String("addr", srv.Addr),
			zap.String("tlds", fmt.Sprintf("%v", cfg.RDAP.TLDs)),
		)
		var err error
		if cfg.Server.TLSCertFile != "" && cfg.Server.TLSKeyFile != "" {
			logger.Info("RDAP server using TLS",
				zap.String("cert", cfg.Server.TLSCertFile))
			err = srv.ListenAndServeTLS(cfg.Server.TLSCertFile, cfg.Server.TLSKeyFile)
		} else {
			err = srv.ListenAndServe()
		}
		if err != nil && err != http.ErrServerClosed {
			logger.Fatal("server error", zap.Error(err))
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	logger.Info("shutting down servers...")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		logger.Fatal("server forced to shutdown", zap.Error(err))
	}
	if err := metricsSrv.Shutdown(ctx); err != nil {
		logger.Error("metrics server forced to shutdown", zap.Error(err))
	}

	logger.Info("server exited")
}
