package config

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Server   ServerConfig   `yaml:"server"`
	Storage  StorageConfig  `yaml:"storage"`
	RDAP     RDAPConfig     `yaml:"rdap"`
	Auth     AuthConfig     `yaml:"auth"`
	Metrics  MetricsConfig  `yaml:"metrics"`
	Rate     RateConfig     `yaml:"rate_limiting"`
}

type ServerConfig struct {
	Host            string        `yaml:"host"`
	Port            int           `yaml:"port"`
	ReadTimeout     time.Duration `yaml:"read_timeout"`
	WriteTimeout    time.Duration `yaml:"write_timeout"`
	IdleTimeout     time.Duration `yaml:"idle_timeout"`
	MaxHeaderBytes  int           `yaml:"max_header_bytes"`
	TLSCertFile     string        `yaml:"tls_cert_file"`
	TLSKeyFile      string        `yaml:"tls_key_file"`
}

type StorageConfig struct {
	Driver   string `yaml:"driver"`
	DSN      string `yaml:"dsn"`
	MaxOpen  int    `yaml:"max_open_conns"`
	MaxIdle  int    `yaml:"max_idle_conns"`
	CacheTTL string `yaml:"cache_ttl"`
}

type RDAPConfig struct {
	TLDs             []string `yaml:"tlds"`
	Mode             string   `yaml:"mode"`
	BaseURL          string   `yaml:"base_url"`
	RegistrarBaseURL string   `yaml:"registrar_base_url"`
	MaxDomainLen     int      `yaml:"max_domain_length"`
	MaxSearchLimit   int      `yaml:"max_search_limit"`
	Port43Whois      string   `yaml:"port43_whois"`
	ServerName       string   `yaml:"server_name"`
	Version          string   `yaml:"version"`
}

type AuthConfig struct {
	Enabled   bool   `yaml:"enabled"`
	JWKSEndpoint string `yaml:"jwks_endpoint"`
	Issuer    string `yaml:"issuer"`
	Audience  string `yaml:"audience"`
}

type MetricsConfig struct {
	Enabled bool   `yaml:"enabled"`
	Host    string `yaml:"host"`
	Port    int    `yaml:"port"`
}

type RateConfig struct {
	Enabled    bool          `yaml:"enabled"`
	Requests   int           `yaml:"requests"`
	Window     time.Duration `yaml:"window"`
	Burst      int           `yaml:"burst"`
	TrustedIPs []string      `yaml:"trusted_ips"`
}

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}

	cfg := &Config{}
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	cfg.setDefaults()
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	return cfg, nil
}

func (c *Config) setDefaults() {
	if c.Server.Port == 0 {
		c.Server.Port = 8443
	}
	if c.Server.ReadTimeout == 0 {
		c.Server.ReadTimeout = 10 * time.Second
	}
	if c.Server.WriteTimeout == 0 {
		c.Server.WriteTimeout = 10 * time.Second
	}
	if c.Server.IdleTimeout == 0 {
		c.Server.IdleTimeout = 60 * time.Second
	}
	if c.Server.MaxHeaderBytes == 0 {
		c.Server.MaxHeaderBytes = 1 << 20
	}
	if c.RDAP.MaxDomainLen == 0 {
		c.RDAP.MaxDomainLen = 253
	}
	if c.RDAP.MaxSearchLimit == 0 {
		c.RDAP.MaxSearchLimit = 100
	}
	if c.RDAP.RegistrarBaseURL == "" {
		c.RDAP.RegistrarBaseURL = c.RDAP.BaseURL
	}
	if c.RDAP.Version == "" {
		c.RDAP.Version = "1.0"
	}
	if c.Metrics.Port == 0 {
		c.Metrics.Port = 9090
	}
	if c.Rate.Requests == 0 {
		c.Rate.Requests = 100
	}
	if c.Rate.Window == 0 {
		c.Rate.Window = time.Minute
	}
}

func (c *Config) validate() error {
	if c.RDAP.Mode == "" {
		c.RDAP.Mode = "registrar"
	}
	if c.RDAP.Mode != "registrar" && c.RDAP.Mode != "registry" {
		return fmt.Errorf("invalid rdap.mode %q: must be \"registrar\" or \"registry\"", c.RDAP.Mode)
	}
	if c.RDAP.BaseURL == "" {
		return fmt.Errorf("rdap.base_url is required")
	}
	return nil
}

func (c *Config) Addr() string {
	if c.Server.Host == "" || c.Server.Host == "0.0.0.0" {
		// Bind all interfaces (IPv4 + IPv6 dual-stack).
		return fmt.Sprintf(":%d", c.Server.Port)
	}
	return fmt.Sprintf("%s:%d", c.Server.Host, c.Server.Port)
}

func (c *MetricsConfig) Addr() string {
	return fmt.Sprintf("%s:%d", c.Host, c.Port)
}
