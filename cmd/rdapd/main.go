package main

import (
	"context"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/tespio/go-rdap-server/internal/config"
	"github.com/tespio/go-rdap-server/internal/metrics"
	"github.com/tespio/go-rdap-server/internal/server"
	"github.com/tespio/go-rdap-server/internal/store"
	"github.com/tespio/go-rdap-server/internal/whois"
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
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

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

	// Legacy WHOIS gateway (RFC 3912). When enabled, serves plain-text WHOIS
	// responses rendered from the same registry data, so one binary can replace
	// both the RDAP and WHOIS services during the migration.
	var whoisSrv *whois.Server
	if cfg.Whois.Enabled {
		whoisSrv = whois.New(cfg.WhoisAddr(), whois.StoreLookup(st), logger)
		if err := whoisSrv.Listen(); err != nil {
			return err
		}
		go func() {
			if err := whoisSrv.Serve(ctx); err != nil && err != net.ErrClosed {
				logger.Error("whois server error", zap.Error(err))
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
	cancel() // stop the WHOIS gateway listener
	logger.Info("shutting down servers...")

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("server forced to shutdown: %w", err)
	}
	if metricsSrv != nil {
		if err := metricsSrv.Shutdown(shutdownCtx); err != nil {
			logger.Error("metrics server forced to shutdown", zap.Error(err))
		}
	}
	if whoisSrv != nil {
		if err := whoisSrv.Shutdown(shutdownCtx); err != nil {
			logger.Error("whois server forced to shutdown", zap.Error(err))
		}
	}

	logger.Info("server exited")
	return nil
}
