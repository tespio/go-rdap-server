package store

import (
	"strings"
	"testing"
	"time"

	"github.com/tespio/go-rdap-server/internal/config"
	"github.com/tespio/go-rdap-server/internal/domain"
)

func newMemoryStore(t *testing.T) *MemoryStore {
	t.Helper()
	s, err := NewMemoryStore(config.StorageConfig{})
	if err != nil {
		t.Fatalf("NewMemoryStore: %v", err)
	}
	return s
}

func TestMemoryStoreLookupDomain(t *testing.T) {
	s := newMemoryStore(t)

	d, err := s.LookupDomain("EXAMPLE.COM")
	if err != nil {
		t.Fatalf("LookupDomain: %v", err)
	}
	if d.LDHName != "example.com" {
		t.Errorf("LDHName = %q", d.LDHName)
	}
	if d.Handle != "EX1-NAME" {
		t.Errorf("handle = %q", d.Handle)
	}

	// Trailing dot is trimmed.
	if _, err := s.LookupDomain("example.com."); err != nil {
		t.Errorf("trailing dot should be accepted: %v", err)
	}

	// Unknown domain errors.
	if _, err := s.LookupDomain("nope.invalid"); err == nil {
		t.Error("expected error for unknown domain")
	}
}

func TestMemoryStoreGetDomainAggregate(t *testing.T) {
	s := newMemoryStore(t)

	agg, err := s.GetDomainAggregate("example.com")
	if err != nil {
		t.Fatalf("GetDomainAggregate: %v", err)
	}
	if agg.Domain == nil || agg.Domain.Handle != "EX1-NAME" {
		t.Fatalf("aggregate domain = %+v", agg.Domain)
	}
	// Registrar (handle 2) resolved.
	if agg.Registrar == nil || agg.Registrar.Handle != "2" {
		t.Fatalf("aggregate registrar = %+v", agg.Registrar)
	}
	// Registrant + technical contacts resolved.
	if _, ok := agg.Contacts["REG1-NAME"]; !ok {
		t.Errorf("missing registrant contact in aggregate: %v", agg.Contacts)
	}
	if _, ok := agg.Contacts["888"]; !ok {
		t.Errorf("missing technical contact in aggregate: %v", agg.Contacts)
	}
	// Nameservers resolved by handle.
	if _, ok := agg.Nameservers["NS1-NAME"]; !ok {
		t.Errorf("missing nameserver NS1-NAME: %v", agg.Nameservers)
	}
	if _, ok := agg.Nameservers["NS2-NAME"]; !ok {
		t.Errorf("missing nameserver NS2-NAME: %v", agg.Nameservers)
	}

	if _, err := s.GetDomainAggregate("nope.invalid"); err == nil {
		t.Error("expected error for unknown domain")
	}
}

func TestMemoryStoreLookupContact(t *testing.T) {
	s := newMemoryStore(t)

	c, err := s.LookupContact("2")
	if err != nil {
		t.Fatalf("LookupContact: %v", err)
	}
	if c.Handle != "2" || len(c.Roles) == 0 || c.Roles[0] != domain.RoleRegistrar {
		t.Errorf("contact = %+v", c)
	}
	if c.VCard == nil || c.VCard.FullName != "Example Registrar Inc." {
		t.Errorf("vcard = %+v", c.VCard)
	}

	if _, err := s.LookupContact("nonexistent"); err == nil {
		t.Error("expected error for unknown contact")
	}
}

func TestMemoryStoreLookupNameserver(t *testing.T) {
	s := newMemoryStore(t)

	ns, err := s.LookupNameserver("NS1.EXAMPLE.COM.")
	if err != nil {
		t.Fatalf("LookupNameserver: %v", err)
	}
	if ns.Handle != "NS1-NAME" {
		t.Errorf("handle = %q", ns.Handle)
	}
	if len(ns.IPV4) != 1 || ns.IPV4[0] != "8.8.8.8" {
		t.Errorf("ipv4 = %v", ns.IPV4)
	}

	if _, err := s.LookupNameserver("nonexistent.example"); err == nil {
		t.Error("expected error for unknown nameserver")
	}
}

func TestMemoryStoreLookupIPNetwork(t *testing.T) {
	s := newMemoryStore(t)

	n, err := s.LookupIPNetwork("8.8.8.0/24")
	if err != nil {
		t.Fatalf("LookupIPNetwork: %v", err)
	}
	if n.Name != "GOOGLE" || n.Handle != "NET-8-8-8-0-24" {
		t.Errorf("network = %+v", n)
	}

	if _, err := s.LookupIPNetwork("1.2.3.0/24"); err == nil {
		t.Error("expected error for unknown network")
	}
}

func TestMemoryStoreLookupAutnum(t *testing.T) {
	s := newMemoryStore(t)

	a, err := s.LookupAutnum(15169)
	if err != nil {
		t.Fatalf("LookupAutnum: %v", err)
	}
	if a.Name != "GOOGLE" || a.Handle != "AS15169" {
		t.Errorf("autnum = %+v", a)
	}

	if _, err := s.LookupAutnum(1); err == nil {
		t.Error("expected error for unknown autnum")
	}
}

func TestMemoryStoreSearchDomainsByName(t *testing.T) {
	s := newMemoryStore(t)

	results, err := s.SearchDomainsByName("example*", 0)
	if err != nil {
		t.Fatalf("SearchDomainsByName: %v", err)
	}
	if len(results) != 1 || results[0].LDHName != "example.com" {
		t.Errorf("results = %+v", results)
	}

	// No match.
	empty, err := s.SearchDomainsByName("nomatch*", 0)
	if err != nil {
		t.Fatalf("SearchDomainsByName no-match: %v", err)
	}
	if len(empty) != 0 {
		t.Errorf("expected no results, got %d", len(empty))
	}

	// Limit is honored.
	limited, err := s.SearchDomainsByName("*", 1)
	if err != nil {
		t.Fatalf("SearchDomainsByName limited: %v", err)
	}
	if len(limited) > 1 {
		t.Errorf("limit not honored: %d results", len(limited))
	}
}

func TestMemoryStoreSearchDomainsByNS(t *testing.T) {
	s := newMemoryStore(t)

	results, err := s.SearchDomainsByNS("ns1.example.com", 0)
	if err != nil {
		t.Fatalf("SearchDomainsByNS: %v", err)
	}
	if len(results) != 1 || results[0].LDHName != "example.com" {
		t.Errorf("results = %+v", results)
	}

	// Unknown nameserver -> empty, no error.
	empty, err := s.SearchDomainsByNS("nonexistent.example", 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(empty) != 0 {
		t.Errorf("expected no results, got %d", len(empty))
	}
}

func TestMemoryStoreSearchContacts(t *testing.T) {
	s := newMemoryStore(t)

	// SearchContactsByName matches against contact handles (e.g. "REG1-NAME").
	byName, err := s.SearchContactsByName("REG*", 0)
	if err != nil {
		t.Fatalf("SearchContactsByName: %v", err)
	}
	if len(byName) != 1 || byName[0].Handle != "REG1-NAME" {
		t.Errorf("byName = %+v", byName)
	}

	byHandle, err := s.SearchContactsByHandle("REG1*", 0)
	if err != nil {
		t.Fatalf("SearchContactsByHandle: %v", err)
	}
	if len(byHandle) != 1 || byHandle[0].Handle != "REG1-NAME" {
		t.Errorf("byHandle = %+v", byHandle)
	}
}

func TestMemoryStoreSearchNameservers(t *testing.T) {
	s := newMemoryStore(t)

	byName, err := s.SearchNameserversByName("ns1*", 0)
	if err != nil {
		t.Fatalf("SearchNameserversByName: %v", err)
	}
	if len(byName) != 1 || byName[0].Handle != "NS1-NAME" {
		t.Errorf("byName = %+v", byName)
	}

	byIP, err := s.SearchNameserversByIP("8.8.8.8", 0)
	if err != nil {
		t.Fatalf("SearchNameserversByIP: %v", err)
	}
	if len(byIP) != 1 || byIP[0].Handle != "NS1-NAME" {
		t.Errorf("byIP = %+v", byIP)
	}

	byIP6, err := s.SearchNameserversByIP("2606:4700:4700::1111", 0)
	if err != nil {
		t.Fatalf("SearchNameserversByIP v6: %v", err)
	}
	if len(byIP6) != 1 || byIP6[0].Handle != "NS2-NAME" {
		t.Errorf("byIP6 = %+v", byIP6)
	}
}

func TestMemoryStorePingClose(t *testing.T) {
	s := newMemoryStore(t)
	if err := s.Ping(); err != nil {
		t.Errorf("Ping: %v", err)
	}
	if err := s.Close(); err != nil {
		t.Errorf("Close: %v", err)
	}
}

func TestMemoryStoreGlobInvalid(t *testing.T) {
	s := newMemoryStore(t)
	if _, err := s.SearchDomainsByName("[", 0); err == nil {
		t.Error("expected error for invalid glob pattern")
	}
	if _, err := s.SearchContactsByName("[", 0); err == nil {
		t.Error("expected error for invalid glob pattern (contacts)")
	}
	if _, err := s.SearchNameserversByName("[", 0); err == nil {
		t.Error("expected error for invalid glob pattern (nameservers)")
	}
}

func TestMemoryStoreQuestionWildcard(t *testing.T) {
	s := newMemoryStore(t)

	// "example?com" -> "example.com" matches via single-char wildcard.
	results, err := s.SearchDomainsByName("example?com", 0)
	if err != nil {
		t.Fatalf("SearchDomainsByName(?): %v", err)
	}
	if len(results) != 1 || results[0].LDHName != "example.com" {
		t.Errorf("results = %+v", results)
	}
}

func TestStoreNewDriver(t *testing.T) {
	if _, err := New(config.StorageConfig{Driver: "memory"}); err != nil {
		t.Errorf("memory driver: %v", err)
	}
	if _, err := New(config.StorageConfig{Driver: "bogus"}); err == nil {
		t.Error("expected error for unsupported driver")
	}
	// Lowercased seeds as a bonus check (domain map is case-insensitive lookup).
	s, err := New(config.StorageConfig{Driver: "memory"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, err := s.LookupDomain(strings.ToUpper("example.com")); err != nil {
		t.Errorf("case-insensitive lookup failed: %v", err)
	}
}

func TestNewMemoryStoreCacheTTL(t *testing.T) {
	// Custom valid TTL.
	s, err := NewMemoryStore(config.StorageConfig{CacheTTL: "30s"})
	if err != nil {
		t.Fatalf("NewMemoryStore: %v", err)
	}
	if s.cacheTTL != 30*time.Second {
		t.Errorf("cacheTTL = %v, want 30s", s.cacheTTL)
	}

	// Invalid TTL -> default 5m.
	s, err = NewMemoryStore(config.StorageConfig{CacheTTL: "not-a-duration"})
	if err != nil {
		t.Fatalf("NewMemoryStore: %v", err)
	}
	if s.cacheTTL != 5*time.Minute {
		t.Errorf("cacheTTL = %v, want default 5m", s.cacheTTL)
	}

	// Empty TTL -> default 5m.
	s, err = NewMemoryStore(config.StorageConfig{})
	if err != nil {
		t.Fatalf("NewMemoryStore: %v", err)
	}
	if s.cacheTTL != 5*time.Minute {
		t.Errorf("cacheTTL = %v, want default 5m", s.cacheTTL)
	}
}

func TestStrHelper(t *testing.T) {
	if got := str("abc"); got != "abc" {
		t.Errorf("str(string) = %q", got)
	}
	if got := str(123); got != "" {
		t.Errorf("str(non-string) = %q, want empty", got)
	}
	if got := str(nil); got != "" {
		t.Errorf("str(nil) = %q, want empty", got)
	}
}
