package store

import (
	"context"
	"os"
	"testing"

	"github.com/tespio/go-rdap-server/internal/config"
)

// newTestMySQLStore opens a MySQL-backed store for integration tests. It is
// gated behind RDAP_TEST_MYSQL_DSN (e.g. "root@tcp(127.0.0.1:3306)/rdap?parseTime=true&charset=utf8mb4").
func newTestMySQLStore(t *testing.T) *MySQLStore {
	t.Helper()
	dsn := os.Getenv("RDAP_TEST_MYSQL_DSN")
	if dsn == "" {
		t.Skip("RDAP_TEST_MYSQL_DSN not set; skipping MySQL integration test")
	}
	st, err := NewMySQLStore(config.StorageConfig{DSN: dsn})
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	return st
}

func TestMySQLStoreLookups(t *testing.T) {
	st := newTestMySQLStore(t)

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
	if len(d.Nameservers) != 2 {
		t.Errorf("nameservers = %d, want 2", len(d.Nameservers))
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

	if _, err := st.LookupDomain("nope.invalid"); err == nil {
		t.Error("expected error for unknown domain")
	}
}

func TestMySQLStoreContactsAndNameservers(t *testing.T) {
	st := newTestMySQLStore(t)

	c, err := st.LookupContact("2")
	if err != nil {
		t.Fatalf("LookupContact: %v", err)
	}
	if c.Handle != "2" || len(c.Roles) == 0 {
		t.Errorf("contact = %+v", c)
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

func TestMySQLStoreIPNetworksAndAutnums(t *testing.T) {
	st := newTestMySQLStore(t)
	ctx := context.Background()

	cleanup := func() {
		_, _ = st.db.ExecContext(ctx, "DELETE FROM ip_networks WHERE handle IN ('TEST-NET','TEST-NET6')")
		_, _ = st.db.ExecContext(ctx, "DELETE FROM autnums WHERE handle=?", "AS99999")
	}
	cleanup()
	defer cleanup()

	// IPv4 network fixture (10.0.0.0/24).
	_, err := st.db.ExecContext(ctx, `
		INSERT INTO ip_networks (handle, start_address, end_address, ip_version, start_ip, end_ip, cidr, name, type, country, status)
		VALUES (?, ?, ?, 'v4', INET_ATON('10.0.0.0'), INET_ATON('10.0.0.255'), ?, 'TEST-NET', 'ALLOCATED', 'US', '["active"]')
		ON DUPLICATE KEY UPDATE name='TEST-NET'
	`, "TEST-NET", "10.0.0.0", "10.0.0.255", `["10.0.0.0/24"]`)
	if err != nil {
		t.Fatalf("insert ip_network: %v", err)
	}

	// IPv6 network fixture (2001:db8::/32).
	_, err = st.db.ExecContext(ctx, `
		INSERT INTO ip_networks (handle, start_address, end_address, ip_version, start_ip6, end_ip6, cidr, name, type, country, status)
		VALUES (?, ?, ?, 'v6', UNHEX('20010db8000000000000000000000000'), UNHEX('20010db8ffffffffffffffffffffffff'), ?, 'TEST-NET6', 'ALLOCATED', 'US', '["active"]')
		ON DUPLICATE KEY UPDATE name='TEST-NET6'
	`, "TEST-NET6", "2001:db8::", "2001:db8::ffff:ffff:ffff:ffff", `["2001:db8::/32"]`)
	if err != nil {
		t.Fatalf("insert ip_network v6: %v", err)
	}

	_, err = st.db.ExecContext(ctx, `
		INSERT INTO autnums (handle, start_asn, end_asn, name, type, country, status)
		VALUES (?, 99999, 99999, 'TEST-ASN', 'ALLOCATED', 'US', '["active"]')
		ON DUPLICATE KEY UPDATE name='TEST-ASN'
	`, "AS99999")
	if err != nil {
		t.Fatalf("insert autnum: %v", err)
	}

	// IPv4 lookup.
	n, err := st.LookupIPNetwork("10.0.0.0/24")
	if err != nil {
		t.Fatalf("LookupIPNetwork v4: %v", err)
	}
	if n.Name != "TEST-NET" || n.IPVersion != "v4" {
		t.Errorf("network = %+v", n)
	}
	if len(n.CIDR) != 1 || n.CIDR[0] != "10.0.0.0/24" {
		t.Errorf("cidr = %v", n.CIDR)
	}

	// IPv6 lookup.
	n6, err := st.LookupIPNetwork("2001:db8::/32")
	if err != nil {
		t.Fatalf("LookupIPNetwork v6: %v", err)
	}
	if n6.Name != "TEST-NET6" || n6.IPVersion != "v6" {
		t.Errorf("network v6 = %+v", n6)
	}

	if _, err := st.LookupIPNetwork("192.168.0.0/24"); err == nil {
		t.Error("expected error for unknown network")
	}
	if _, err := st.LookupIPNetwork("not-a-cidr"); err == nil {
		t.Error("expected error for invalid cidr")
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

func TestMySQLStoreSearches(t *testing.T) {
	st := newTestMySQLStore(t)

	domains, err := st.SearchDomainsByName("example*", 10)
	if err != nil {
		t.Fatalf("SearchDomainsByName: %v", err)
	}
	if len(domains) != 1 || domains[0].LDHName != "example.com" {
		t.Errorf("domain results = %+v", domains)
	}

	byNS, err := st.SearchDomainsByNS("ns1.example.com", 10)
	if err != nil {
		t.Fatalf("SearchDomainsByNS: %v", err)
	}
	if len(byNS) != 1 || byNS[0].LDHName != "example.com" {
		t.Errorf("byNS results = %+v", byNS)
	}

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

func TestMySQLStorePingClose(t *testing.T) {
	dsn := os.Getenv("RDAP_TEST_MYSQL_DSN")
	if dsn == "" {
		t.Skip("RDAP_TEST_MYSQL_DSN not set; skipping MySQL integration test")
	}
	st, err := NewMySQLStore(config.StorageConfig{DSN: dsn})
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
