package config

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Server  ServerConfig  `yaml:"server"`
	Storage StorageConfig `yaml:"storage"`
	RDAP    RDAPConfig    `yaml:"rdap"`
	Auth    AuthConfig    `yaml:"auth"`
	Metrics MetricsConfig `yaml:"metrics"`
	Rate    RateConfig    `yaml:"rate_limiting"`
}

type ServerConfig struct {
	Host           string        `yaml:"host"`
	Port           int           `yaml:"port"`
	ReadTimeout    time.Duration `yaml:"read_timeout"`
	WriteTimeout   time.Duration `yaml:"write_timeout"`
	IdleTimeout    time.Duration `yaml:"idle_timeout"`
	MaxHeaderBytes int           `yaml:"max_header_bytes"`
	TLSCertFile    string        `yaml:"tls_cert_file"`
	TLSKeyFile     string        `yaml:"tls_key_file"`
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
	// SearchEnabled controls the RFC 7482 search endpoints
	// (/domains?name=..., /entities?fn=..., /nameservers?name=...).
	// Searches are an optional RDAP capability and most registrars/registries
	// disable them in production due to abuse/DoS risk (a wildcard search can
	// walk large portions of the database). When disabled (default), search
	// routes return HTTP 501 Not Implemented per RFC 9082 §5.1.
	SearchEnabled bool `yaml:"search_enabled"`
	// Extensions enables optional IANA-registered RDAP extensions. Each entry
	// must be a registered extension identifier (see
	// https://www.iana.org/assignments/rdap-extensions). Supported values:
	//   "ttl0"            — DNS TTL values on domain/nameserver objects (draft-ietf-regext-rdap-ttl-extension)
	//   "geofeed1"        — geofeed link on IP network objects (RFC 9877)
	//   "cidr0"           — cidr0_cidrs array on IP network objects (NRO)
	//   "reverse_search"  — reverse search endpoints (RFC 9536)
	// When enabled, the server appends the extension's identifier to the
	// rdapConformance array and (where applicable) emits the extension's JSON
	// members. All extensions default to OFF so the ICANN-mandated conformance
	// array is unchanged unless explicitly enabled.
	Extensions []string `yaml:"extensions"`
	// TTL0 supplies the TTL values emitted for the ttl0 extension. Only used
	// when "ttl0" is in Extensions.
	TTL0 *TTL0Config `yaml:"ttl0"`
	// Geofeed configures the geofeed1 extension. Only used when "geofeed1" is in
	// Extensions.
	Geofeed *GeofeedConfig `yaml:"geofeed"`
	// ToS customizes the Terms of Service notice text and link. Optional; when
	// unset a generic notice is used.
	ToS *ToSConfig `yaml:"tos"`
	// CustomNotices appends registrar/registry-specific notices to responses.
	// Optional. The ICANN-mandated notices (Status Codes, RDDS Inaccuracy) are
	// always included regardless of this setting.
	CustomNotices []CustomNoticeConfig `yaml:"custom_notices"`
}

// TTL0Config holds the record-type → TTL values emitted under the ttl0
// extension. Only used when rdap.extensions includes "ttl0".
type TTL0Config struct {
	// Domain maps DNS record type mnemonics (e.g. "NS", "DS") to TTL seconds
	// for domain objects.
	Domain map[string]int `yaml:"domain"`
	// Nameserver maps DNS record type mnemonics (e.g. "A", "AAAA") to TTL
	// seconds for nameserver objects.
	Nameserver map[string]int `yaml:"nameserver"`
	// Remarks is an optional array of human-readable remarks about the TTL
	// values (per RFC 9083 §4.3).
	Remarks []ExtensionRemark `yaml:"remarks"`
}

// ExtensionRemark is a remark object appended under an extension.
type ExtensionRemark struct {
	Title       string   `yaml:"title"`
	Description []string `yaml:"description"`
}

// GeofeedConfig configures the geofeed1 extension (RFC 9877). Only used when
// rdap.extensions includes "geofeed1".
type GeofeedConfig struct {
	// URL is the HTTPS geofeed file URL (RFC 9632) attached to IP network
	// objects via a rel="geofeed" link with type application/geofeed+csv.
	URL string `yaml:"url"`
}

// ToSConfig customizes the Terms of Service notice.
type ToSConfig struct {
	// Title overrides the notice title (default "Terms of Service").
	Title string `yaml:"title"`
	// Description is the notice body text (e.g. company name and terms).
	Description []string `yaml:"description"`
	// URL is the terms-of-service link target. If empty, links to /help.
	URL string `yaml:"url"`
}

// CustomNoticeConfig is a registrar/registry-specific RDAP notice.
type CustomNoticeConfig struct {
	Title       string   `yaml:"title"`
	Description []string `yaml:"description"`
	URL         string   `yaml:"url"`
	Rel         string   `yaml:"rel"`
}

type AuthConfig struct {
	Enabled      bool   `yaml:"enabled"`
	JWKSEndpoint string `yaml:"jwks_endpoint"`
	Issuer       string `yaml:"issuer"`
	Audience     string `yaml:"audience"`
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
		c.RDAP.Version = "1.2"
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

// supportedExtensions is the set of IANA-registered RDAP extensions this server
// can enable. Unknown identifiers are rejected at load time so typos fail fast.
var supportedExtensions = map[string]bool{
	"ttl0":           true, // DNS TTL values (draft-ietf-regext-rdap-ttl-extension)
	"geofeed1":       true, // RFC 9877 geofeed link
	"cidr0":          true, // NRO CIDR expressions
	"reverse_search": true, // RFC 9536 reverse search
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
	for _, ext := range c.RDAP.Extensions {
		if !supportedExtensions[ext] {
			return fmt.Errorf("invalid rdap.extensions entry %q: must be one of %v", ext, extensionNames())
		}
	}
	if c.RDAP.TTL0 != nil && !c.RDAP.ExtensionsEnabled("ttl0") {
		// Allowed: config present but extension off. Keep it simple, no error.
	}
	return nil
}

func extensionNames() []string {
	names := make([]string, 0, len(supportedExtensions))
	for name := range supportedExtensions {
		names = append(names, name)
	}
	return names
}

// ExtensionsEnabled reports whether the given extension identifier is enabled.
func (c *RDAPConfig) ExtensionsEnabled(ext string) bool {
	for _, e := range c.Extensions {
		if e == ext {
			return true
		}
	}
	return false
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
