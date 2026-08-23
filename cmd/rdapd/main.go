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

	"github.com/tespio/go-rdap-server/internal/config"
	"github.com/tespio/go-rdap-server/internal/metrics"
	"github.com/tespio/go-rdap-server/internal/server"
	"github.com/tespio/go-rdap-server/internal/store"
	"go.uber.org/zap"
)

func main() {
	configPath := flag.String("config", "config.yaml", "path to config file")
	flag.Parse()

	logger, _ := zap.NewProduction()
	defer logger.Sync()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	if err := run(*configPath, logger, quit); err != nil {
		logger.Fatal("server failed", zap.Error(err))
	}
}

// run loads the config, starts the servers, and blocks until a signal is
// received, then shuts everything down gracefully. It is separated from main so
// the startup/shutdown logic is unit-testable.
func run(configPath string, logger *zap.Logger, quit <-chan os.Signal) error {
	cfg, err := config.Load(configPath)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	st, err := store.New(cfg.Storage)
	if err != nil {
		return fmt.Errorf("failed to init store: %w", err)
	}
	defer st.Close()

	metricsSrv := metrics.NewServer(cfg.Metrics)
	if metricsSrv != nil {
		go func() {
			logger.Info("metrics server starting", zap.String("addr", metricsSrv.Addr))
			if err := metricsSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				logger.Error("metrics server error", zap.Error(err))
			}
		}()
	}

	srv := server.New(cfg, st, logger)

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
			logger.Error("server error", zap.Error(err))
			return
		}
	}()

	<-quit
	logger.Info("shutting down servers...")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		return fmt.Errorf("server forced to shutdown: %w", err)
	}
	if metricsSrv != nil {
		if err := metricsSrv.Shutdown(ctx); err != nil {
			logger.Error("metrics server forced to shutdown", zap.Error(err))
		}
	}

	logger.Info("server exited")
	return nil
}
