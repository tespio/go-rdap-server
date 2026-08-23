package store

import (
	"context"
	"os"
	"testing"

	"github.com/tespio/go-rdap-server/internal/config"
)

// helper for gated Postgres tests: skip when RDAP_TEST_DSN is unset.
func newTestPostgresStore(t *testing.T) *PostgresStore {
	t.Helper()
	dsn := os.Getenv("RDAP_TEST_DSN")
	if dsn == "" {
		t.Skip("RDAP_TEST_DSN not set; skipping Postgres integration test")
	}
	st, err := NewPostgresStore(config.StorageConfig{DSN: dsn})
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	return st
}

func TestPostgresStoreLookups(t *testing.T) {
	st := newTestPostgresStore(t)
	ctx := context.Background()

	// Domain lookup (seed: example.com / EX1-NAME).
	d, err := st.LookupDomain("example.com")
	if err != nil {
		t.Fatalf("LookupDomain: %v", err)
	}
	if d.Handle != "EX1-NAME" || d.LDHName != "example.com" {
		t.Errorf("domain = %+v", d)
	}
	if len(d.Status) == 0 || d.Status[0].Value != "active" {
		t.Errorf("status = %+v", d.Status)
	}
	if d.Registrar != "2" {
		t.Errorf("registrar = %q", d.Registrar)
	}

	// Aggregate.
	agg, err := st.GetDomainAggregate("example.com")
	if err != nil {
		t.Fatalf("GetDomainAggregate: %v", err)
	}
	if agg.Domain == nil || agg.Domain.Handle != "EX1-NAME" {
		t.Fatalf("aggregate domain = %+v", agg.Domain)
	}
	if agg.Registrar == nil || agg.Registrar.Handle != "2" {
		t.Errorf("aggregate registrar = %+v", agg.Registrar)
	}
	if len(agg.Contacts) == 0 {
		t.Errorf("aggregate contacts empty")
	}

	// Unknown domain errors.
	if _, err := st.LookupDomain("nope.invalid"); err == nil {
		t.Error("expected error for unknown domain")
	}
	if _, err := st.GetDomainAggregate("nope.invalid"); err == nil {
		t.Error("expected error for unknown aggregate")
	}
	_ = ctx
}

func TestPostgresStoreContactsAndNameservers(t *testing.T) {
	st := newTestPostgresStore(t)

	c, err := st.LookupContact("2")
	if err != nil {
		t.Fatalf("LookupContact: %v", err)
	}
	if c.Handle != "2" {
		t.Errorf("contact handle = %q", c.Handle)
	}
	if len(c.Roles) == 0 {
		t.Errorf("roles empty: %+v", c.Roles)
	}
	if _, err := st.LookupContact("nonexistent"); err == nil {
		t.Error("expected error for unknown contact")
	}

	ns, err := st.LookupNameserver("ns1.example.com")
	if err != nil {
		t.Fatalf("LookupNameserver: %v", err)
	}
	if ns.Handle != "NS1-NAME" {
		t.Errorf("nameserver = %+v", ns)
	}
	if len(ns.IPV4) != 1 || ns.IPV4[0] != "8.8.8.8" {
		t.Errorf("ipv4 = %v", ns.IPV4)
	}
	if _, err := st.LookupNameserver("nonexistent.example"); err == nil {
		t.Error("expected error for unknown nameserver")
	}
}

func TestPostgresStoreIPNetworksAndAutnums(t *testing.T) {
	st := newTestPostgresStore(t)
	ctx := context.Background()

	// Seed has no ip_networks/autnums, so insert fixtures to test lookups.
	cleanup := func() {
		_, _ = st.pool.Exec(ctx, "DELETE FROM ip_networks WHERE handle=$1", "TEST-NET")
		_, _ = st.pool.Exec(ctx, "DELETE FROM autnums WHERE handle=$1", "AS99999")
	}
	cleanup()
	defer cleanup()

	_, err := st.pool.Exec(ctx, `
		INSERT INTO ip_networks (handle, start_address, end_address, ip_version, cidr, name, type, country, status)
		VALUES ($1, '10.0.0.0', '10.0.0.255', 'v4', ARRAY['10.0.0.0/24'], 'TEST-NET', 'ALLOCATED', 'US', '["active"]')
	`, "TEST-NET")
	if err != nil {
		t.Fatalf("insert ip_network: %v", err)
	}
	_, err = st.pool.Exec(ctx, `
		INSERT INTO autnums (handle, start_asn, end_asn, name, type, country, status)
		VALUES ($1, 99999, 99999, 'TEST-ASN', 'ALLOCATED', 'US', '["active"]')
	`, "AS99999")
	if err != nil {
		t.Fatalf("insert autnum: %v", err)
	}

	n, err := st.LookupIPNetwork("10.0.0.0/24")
	if err != nil {
		t.Fatalf("LookupIPNetwork: %v", err)
	}
	if n.Name != "TEST-NET" || n.IPVersion != "v4" {
		t.Errorf("network = %+v", n)
	}
	if _, err := st.LookupIPNetwork("192.168.0.0/24"); err == nil {
		t.Error("expected error for unknown network")
	}

	a, err := st.LookupAutnum(99999)
	if err != nil {
		t.Fatalf("LookupAutnum: %v", err)
	}
	if a.Name != "TEST-ASN" {
		t.Errorf("autnum = %+v", a)
	}
	if _, err := st.LookupAutnum(1); err == nil {
		t.Error("expected error for unknown autnum")
	}
}

func TestPostgresStoreSearches(t *testing.T) {
	st := newTestPostgresStore(t)

	// Domains by name (wildcard -> %).
	domains, err := st.SearchDomainsByName("example*", 10)
	if err != nil {
		t.Fatalf("SearchDomainsByName: %v", err)
	}
	if len(domains) != 1 || domains[0].LDHName != "example.com" {
		t.Errorf("domain results = %+v", domains)
	}

	// Domains by nameserver.
	byNS, err := st.SearchDomainsByNS("ns1.example.com", 10)
	if err != nil {
		t.Fatalf("SearchDomainsByNS: %v", err)
	}
	if len(byNS) != 1 || byNS[0].LDHName != "example.com" {
		t.Errorf("byNS results = %+v", byNS)
	}

	// Contacts by name/handle.
	contacts, err := st.SearchContactsByName("REG1*", 10)
	if err != nil {
		t.Fatalf("SearchContactsByName: %v", err)
	}
	if len(contacts) != 1 || contacts[0].Handle != "REG1-NAME" {
		t.Errorf("contacts = %+v", contacts)
	}

	byHandle, err := st.SearchContactsByHandle("888", 10)
	if err != nil {
		t.Fatalf("SearchContactsByHandle: %v", err)
	}
	if len(byHandle) != 1 || byHandle[0].Handle != "888" {
		t.Errorf("byHandle = %+v", byHandle)
	}

	// Nameservers by name/IP.
	ns, err := st.SearchNameserversByName("ns1*", 10)
	if err != nil {
		t.Fatalf("SearchNameserversByName: %v", err)
	}
	if len(ns) != 1 || ns[0].Handle != "NS1-NAME" {
		t.Errorf("ns = %+v", ns)
	}

	byIP, err := st.SearchNameserversByIP("8.8.8.8", 10)
	if err != nil {
		t.Fatalf("SearchNameserversByIP: %v", err)
	}
	if len(byIP) != 1 || byIP[0].Handle != "NS1-NAME" {
		t.Errorf("byIP = %+v", byIP)
	}
}

func TestPostgresStorePingClose(t *testing.T) {
	dsn := os.Getenv("RDAP_TEST_DSN")
	if dsn == "" {
		t.Skip("RDAP_TEST_DSN not set; skipping Postgres integration test")
	}
	st, err := NewPostgresStore(config.StorageConfig{DSN: dsn})
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	if err := st.Ping(); err != nil {
		t.Errorf("Ping: %v", err)
	}
	if err := st.Close(); err != nil {
		t.Errorf("Close: %v", err)
	}
}
