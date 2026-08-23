package service

import (
	"testing"

	"github.com/tespio/go-rdap-server/internal/config"
	"github.com/tespio/go-rdap-server/internal/domain"
	"github.com/tespio/go-rdap-server/internal/rdap"
	"github.com/tespio/go-rdap-server/internal/store"
)

func extService(t *testing.T, mutate func(*config.RDAPConfig)) *Service {
	t.Helper()
	st, err := store.NewMemoryStore(config.StorageConfig{})
	if err != nil {
		t.Fatalf("memory store: %v", err)
	}
	cfg := config.RDAPConfig{
		BaseURL:          "https://rdap.example.com",
		RegistrarBaseURL: "https://rdap.example.org/rdap/",
		Mode:             "registrar",
		MaxSearchLimit:   100,
	}
	if mutate != nil {
		mutate(&cfg)
	}
	return New(st, cfg)
}

func hasConformance(t *testing.T, conf []string, want string) {
	t.Helper()
	for _, c := range conf {
		if c == want {
			return
		}
	}
	t.Fatalf("conformance %v missing %q", conf, want)
}

func TestTTL0EnabledOnDomain(t *testing.T) {
	svc := extService(t, func(c *config.RDAPConfig) {
		c.Extensions = []string{"ttl0"}
		c.TTL0 = &config.TTL0Config{
			Domain:     map[string]int{"NS": 3600, "DS": 300},
			Nameserver: map[string]int{"A": 86400},
			Remarks:    []config.ExtensionRemark{{Title: "TTL policy", Description: []string{"Provisional."}}},
		}
	})
	d, err := svc.LookupDomain("example.com", testReqURL)
	if err != nil {
		t.Fatalf("LookupDomain: %v", err)
	}
	if d.TTL0Data == nil {
		t.Fatal("expected ttl0_data on domain")
	}
	if d.TTL0Data.Values["NS"] != 3600 || d.TTL0Data.Values["DS"] != 300 {
		t.Errorf("ttl0_data.values = %v", d.TTL0Data.Values)
	}
	if len(d.TTL0Data.Remarks) != 1 || d.TTL0Data.Remarks[0].Title != "TTL policy" {
		t.Errorf("ttl0_data.remarks = %+v", d.TTL0Data.Remarks)
	}
	hasConformance(t, d.Conformance.Conformance, "ttl0")
	// Embedded nameservers carry nameserver TTLs.
	if len(d.Nameservers) == 0 || d.Nameservers[0].TTL0Data == nil {
		t.Fatal("expected ttl0_data on embedded nameservers")
	}
	if d.Nameservers[0].TTL0Data.Values["A"] != 86400 {
		t.Errorf("nameserver ttl0_data.values = %v", d.Nameservers[0].TTL0Data.Values)
	}
}

func TestTTL0DisabledNoData(t *testing.T) {
	svc := extService(t, nil)
	d, err := svc.LookupDomain("example.com", testReqURL)
	if err != nil {
		t.Fatalf("LookupDomain: %v", err)
	}
	if d.TTL0Data != nil {
		t.Error("expected no ttl0_data when extension disabled")
	}
	for _, c := range d.Conformance.Conformance {
		if c == "ttl0" {
			t.Error("unexpected ttl0 conformance identifier")
		}
	}
	// Empty TTL map yields nil.
	if data := ttl0Data(nil, nil); data != nil {
		t.Error("ttl0Data(nil) should be nil")
	}
}

func TestGeofeedAndCIDR0EnabledOnIPNetwork(t *testing.T) {
	svc := extService(t, func(c *config.RDAPConfig) {
		c.Extensions = []string{"geofeed1", "cidr0"}
		c.Geofeed = &config.GeofeedConfig{URL: "https://geofeed.example.com/feed.csv"}
	})
	n, err := svc.LookupIPNetwork("8.8.8.0/24", testReqURL)
	if err != nil {
		t.Fatalf("LookupIPNetwork: %v", err)
	}
	var hasGeofeed, hasSelf bool
	for _, l := range n.Links {
		switch l.Rel {
		case "geofeed":
			hasGeofeed = true
			if l.Href != "https://geofeed.example.com/feed.csv" || l.Type != "application/geofeed+csv" {
				t.Errorf("geofeed link = %+v", l)
			}
		case "self":
			hasSelf = true
		}
	}
	if !hasGeofeed {
		t.Error("expected rel=geofeed link")
	}
	if !hasSelf {
		t.Error("expected self link preserved")
	}
	if len(n.CIDR0CIDRs) != 1 {
		t.Fatalf("cidr0_cidrs = %+v", n.CIDR0CIDRs)
	}
	if n.CIDR0CIDRs[0].V4Prefix != "8.8.8.0" || n.CIDR0CIDRs[0].Length != 24 {
		t.Errorf("cidr0 entry = %+v", n.CIDR0CIDRs[0])
	}
}

func TestGeofeedDisabled(t *testing.T) {
	svc := extService(t, nil)
	n, err := svc.LookupIPNetwork("8.8.8.0/24", testReqURL)
	if err != nil {
		t.Fatalf("LookupIPNetwork: %v", err)
	}
	for _, l := range n.Links {
		if l.Rel == "geofeed" {
			t.Error("unexpected geofeed link when extension disabled")
		}
	}
	if len(n.CIDR0CIDRs) != 0 {
		t.Error("unexpected cidr0_cidrs when extension disabled")
	}
}

func TestCIDR0FromCIDR(t *testing.T) {
	v4, ok := cidr0FromCIDR("8.8.8.0/24")
	if !ok || v4.V4Prefix != "8.8.8.0" || v4.Length != 24 {
		t.Errorf("v4 cidr0 = %+v ok=%v", v4, ok)
	}
	v6, ok := cidr0FromCIDR("2001:4860::/32")
	if !ok || v6.V6Prefix != "2001:4860::" || v6.Length != 32 {
		t.Errorf("v6 cidr0 = %+v ok=%v", v6, ok)
	}
	if _, ok := cidr0FromCIDR("not-a-cidr"); ok {
		t.Error("expected failure for invalid cidr")
	}
}

func TestReverseSearchDomainsByEntity(t *testing.T) {
	svc := extService(t, func(c *config.RDAPConfig) {
		c.Extensions = []string{"reverse_search"}
	})

	// By handle.
	domains, err := svc.ReverseSearchDomainsByEntity("handle", "REG1*", 10, testReqURL)
	if err != nil {
		t.Fatalf("reverse handle: %v", err)
	}
	if len(domains) != 1 || domains[0].LDHName != "example.com" {
		t.Errorf("handle reverse results = %+v", domains)
	}

	// By role.
	domains, err = svc.ReverseSearchDomainsByEntity("role", "registrant", 10, testReqURL)
	if err != nil {
		t.Fatalf("reverse role: %v", err)
	}
	if len(domains) != 1 {
		t.Errorf("role reverse results = %d", len(domains))
	}

	// By email.
	domains, err = svc.ReverseSearchDomainsByEntity("email", "registrant@*", 10, testReqURL)
	if err != nil {
		t.Fatalf("reverse email: %v", err)
	}
	if len(domains) != 1 {
		t.Errorf("email reverse results = %d", len(domains))
	}

	// No match.
	domains, err = svc.ReverseSearchDomainsByEntity("handle", "nobody*", 10, testReqURL)
	if err != nil {
		t.Fatalf("reverse no-match: %v", err)
	}
	if len(domains) != 0 {
		t.Errorf("expected no results, got %d", len(domains))
	}
}

func TestReverseSearchDisabledReturnsUnsupported(t *testing.T) {
	svc := extService(t, nil)
	if _, err := svc.ReverseSearchDomainsByEntity("handle", "REG1*", 10, testReqURL); err != store.ErrReverseSearchUnsupported {
		t.Errorf("expected ErrReverseSearchUnsupported, got %v", err)
	}
	if props := svc.ReverseSearchProperties(); props != nil {
		t.Errorf("expected no reverse search properties when disabled, got %+v", props)
	}
}

func TestReverseSearchPropertiesAndMapping(t *testing.T) {
	svc := extService(t, func(c *config.RDAPConfig) {
		c.Extensions = []string{"reverse_search"}
	})
	props := svc.ReverseSearchProperties()
	if len(props) != 4 {
		t.Fatalf("expected 4 reverse search properties, got %d", len(props))
	}
	if props[0].SearchableResourceType != "domains" || props[0].RelatedResourceType != "entity" {
		t.Errorf("props[0] = %+v", props[0])
	}

	mapping := ReverseSearchMapping([]string{"handle", "role", "fn", "email"})
	if len(mapping) != 4 {
		t.Fatalf("expected 4 mappings, got %d", len(mapping))
	}
	if mapping[0].PropertyPath != "$.entities[*].handle" {
		t.Errorf("mapping[0] = %+v", mapping[0])
	}
	if got := ReverseSearchMapping([]string{"unknown"}); len(got) != 0 {
		t.Errorf("unknown property should be skipped, got %+v", got)
	}
}

// Ensure a bare domain lookup with all extensions enabled still produces a
// valid rdap.Domain (no panics, expected fields intact).
func TestExtensionsCombinedNoRegression(t *testing.T) {
	svc := extService(t, func(c *config.RDAPConfig) {
		c.Extensions = []string{"ttl0", "geofeed1", "cidr0", "reverse_search"}
		c.TTL0 = &config.TTL0Config{Domain: map[string]int{"NS": 3600}, Nameserver: map[string]int{"A": 60}}
		c.Geofeed = &config.GeofeedConfig{URL: "https://geofeed.example.com/feed.csv"}
	})
	d, err := svc.LookupDomain("example.com", testReqURL)
	if err != nil {
		t.Fatalf("LookupDomain: %v", err)
	}
	if d.LDHName != "example.com" || len(d.Entities) != 2 {
		t.Errorf("domain = %+v", d)
	}
	if len(d.Entities[0].VCardArray) == 0 {
		t.Error("vcard missing")
	}
}

var _ = rdap.Domain{}
var _ = domain.Domain{}
