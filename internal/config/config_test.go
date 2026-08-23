package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func writeConfig(t *testing.T, content string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return p
}

func TestLoadDefaults(t *testing.T) {
	p := writeConfig(t, `
rdap:
  base_url: "https://rdap.example.com"
`)
	cfg, err := Load(p)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if cfg.Server.Port != 8443 {
		t.Errorf("server.port default = %d", cfg.Server.Port)
	}
	if cfg.Server.ReadTimeout != 10*time.Second || cfg.Server.WriteTimeout != 10*time.Second {
		t.Errorf("timeout defaults = %v/%v", cfg.Server.ReadTimeout, cfg.Server.WriteTimeout)
	}
	if cfg.Server.MaxHeaderBytes != 1<<20 {
		t.Errorf("max_header_bytes default = %d", cfg.Server.MaxHeaderBytes)
	}
	if cfg.RDAP.Mode != "registrar" {
		t.Errorf("rdap.mode default = %q", cfg.RDAP.Mode)
	}
	if cfg.RDAP.MaxDomainLen != 253 {
		t.Errorf("rdap.max_domain_length default = %d", cfg.RDAP.MaxDomainLen)
	}
	if cfg.RDAP.MaxSearchLimit != 100 {
		t.Errorf("rdap.max_search_limit default = %d", cfg.RDAP.MaxSearchLimit)
	}
	if cfg.RDAP.Version != "1.2" {
		t.Errorf("rdap.version default = %q", cfg.RDAP.Version)
	}
	if cfg.RDAP.SearchEnabled {
		t.Error("rdap.search_enabled default should be false")
	}
	if cfg.RDAP.RegistrarBaseURL != "https://rdap.example.com" {
		t.Errorf("rdap.registrar_base_url should default to base_url, got %q", cfg.RDAP.RegistrarBaseURL)
	}
	if cfg.Metrics.Port != 9090 {
		t.Errorf("metrics.port default = %d", cfg.Metrics.Port)
	}
	if cfg.Rate.Requests != 100 || cfg.Rate.Window != time.Minute {
		t.Errorf("rate defaults = %d/%v", cfg.Rate.Requests, cfg.Rate.Window)
	}
}

func TestLoadFullConfig(t *testing.T) {
	p := writeConfig(t, `
server:
  host: "127.0.0.1"
  port: 9443
storage:
  driver: "postgres"
  dsn: "postgres://u:p@localhost/db"
rdap:
  mode: "registry"
  tlds: ["com", "net"]
  base_url: "https://rdap.example.com"
  registrar_base_url: "https://rdap.reg.example/rdap/"
  max_search_limit: 50
  search_enabled: true
  server_name: "My RDAP"
  version: "9.9"
auth:
  enabled: true
  jwks_endpoint: "https://auth.example.com/jwks"
  issuer: "https://auth.example.com"
  audience: "rdap.example.com"
rate_limiting:
  enabled: true
  requests: 500
  window: 30s
  burst: 100
  trusted_ips: ["10.0.0.1"]
`)
	cfg, err := Load(p)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if cfg.Server.Host != "127.0.0.1" || cfg.Server.Port != 9443 {
		t.Errorf("server = %+v", cfg.Server)
	}
	if cfg.Storage.Driver != "postgres" || cfg.Storage.DSN != "postgres://u:p@localhost/db" {
		t.Errorf("storage = %+v", cfg.Storage)
	}
	if cfg.RDAP.Mode != "registry" || !cfg.RDAP.SearchEnabled || cfg.RDAP.MaxSearchLimit != 50 {
		t.Errorf("rdap = %+v", cfg.RDAP)
	}
	if len(cfg.RDAP.TLDs) != 2 {
		t.Errorf("tlds = %v", cfg.RDAP.TLDs)
	}
	if cfg.Auth.Enabled != true || cfg.Auth.Issuer != "https://auth.example.com" {
		t.Errorf("auth = %+v", cfg.Auth)
	}
	if !cfg.Rate.Enabled || cfg.Rate.Requests != 500 || cfg.Rate.Window != 30*time.Second || cfg.Rate.Burst != 100 {
		t.Errorf("rate = %+v", cfg.Rate)
	}
	if len(cfg.Rate.TrustedIPs) != 1 || cfg.Rate.TrustedIPs[0] != "10.0.0.1" {
		t.Errorf("trusted_ips = %v", cfg.Rate.TrustedIPs)
	}
}

func TestLoadErrors(t *testing.T) {
	// Missing file.
	if _, err := Load(filepath.Join(t.TempDir(), "missing.yaml")); err == nil {
		t.Error("expected error for missing file")
	}

	// Missing required base_url.
	p := writeConfig(t, "rdap:\n  mode: registrar\n")
	if _, err := Load(p); err == nil {
		t.Error("expected error when base_url missing")
	}

	// Invalid mode.
	p = writeConfig(t, "rdap:\n  base_url: \"https://x.com\"\n  mode: \"bogus\"\n")
	if _, err := Load(p); err == nil {
		t.Error("expected error for invalid mode")
	}

	// Malformed YAML.
	p = writeConfig(t, "rdap: [unclosed")
	if _, err := Load(p); err == nil {
		t.Error("expected error for malformed YAML")
	}
}

func TestAddr(t *testing.T) {
	// Empty host binds all interfaces.
	cfg := &Config{Server: ServerConfig{Host: "", Port: 8443}}
	if got := cfg.Addr(); got != ":8443" {
		t.Errorf("Addr(empty host) = %q", got)
	}
	cfg = &Config{Server: ServerConfig{Host: "0.0.0.0", Port: 8443}}
	if got := cfg.Addr(); got != ":8443" {
		t.Errorf("Addr(0.0.0.0) = %q", got)
	}
	cfg = &Config{Server: ServerConfig{Host: "127.0.0.1", Port: 8443}}
	if got := cfg.Addr(); got != "127.0.0.1:8443" {
		t.Errorf("Addr(127.0.0.1) = %q", got)
	}
}

func TestMetricsAddr(t *testing.T) {
	m := MetricsConfig{Host: "0.0.0.0", Port: 9090}
	if got := m.Addr(); got != "0.0.0.0:9090" {
		t.Errorf("MetricsAddr = %q", got)
	}
}
