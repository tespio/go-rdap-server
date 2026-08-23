package store

import (
	"encoding/json"
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/tespio/go-rdap-server/internal/domain"
)

// fakeScanner implements domainRowScanner and mysqlRowScanner by assigning
// provided values into the destination pointers.
type fakeScanner struct {
	values []interface{}
	err    error
}

func (f *fakeScanner) Scan(dest ...interface{}) error {
	if f.err != nil {
		return f.err
	}
	if len(dest) != len(f.values) {
		return fmt.Errorf("scan: expected %d dests, got %d", len(f.values), len(dest))
	}
	for i, d := range dest {
		if err := assign(d, f.values[i]); err != nil {
			return err
		}
	}
	return nil
}

func assign(dest interface{}, val interface{}) error {
	rv := reflect.ValueOf(dest)
	if rv.Kind() != reflect.Ptr || rv.IsNil() {
		return fmt.Errorf("dest is not a non-nil pointer")
	}
	target := rv.Elem()
	if val == nil {
		switch target.Kind() {
		case reflect.Ptr, reflect.Interface, reflect.Slice, reflect.Map:
			target.Set(reflect.Zero(target.Type()))
		default:
			return fmt.Errorf("cannot assign nil to %s", target.Type())
		}
		return nil
	}
	src := reflect.ValueOf(val)
	if !src.Type().AssignableTo(target.Type()) {
		return fmt.Errorf("cannot assign %s to %s", src.Type(), target.Type())
	}
	target.Set(src)
	return nil
}

func jb(v interface{}) []byte {
	b, _ := json.Marshal(v)
	return b
}

func TestScanDomainRow(t *testing.T) {
	created := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	updated := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	expires := time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)
	reg := "REG1-NAME"
	tech := "888"

	fs := &fakeScanner{values: []interface{}{
		"EX1-NAME", "example.com", "example.com", "com",
		jb([]string{"active"}), created, updated, expires,
		&reg, nil, &tech, nil,
		jb([]map[string]interface{}{{"handle": "NS1-NAME", "ldhName": "ns1.example.com", "status": []string{"associated"}}}),
		jb(map[string]interface{}{"zoneSigned": false}),
	}}
	d, err := scanDomainRow(fs)
	if err != nil {
		t.Fatalf("scanDomainRow: %v", err)
	}
	if d.Handle != "EX1-NAME" || d.TLD != "com" {
		t.Errorf("domain = %+v", d)
	}
	if len(d.Status) != 1 || d.Status[0].Value != "active" {
		t.Errorf("status = %+v", d.Status)
	}
	if d.Registrar != "REG1-NAME" || d.ExpiresAt != expires {
		t.Errorf("registrar/expiry = %q/%v", d.Registrar, d.ExpiresAt)
	}
	if c := d.Contacts[domain.RoleRegistrant]; len(c) != 1 || c[0] != "REG1-NAME" {
		t.Errorf("registrant contacts = %v", c)
	}
	if c := d.Contacts[domain.RoleTechnical]; len(c) != 1 || c[0] != "888" {
		t.Errorf("tech contacts = %v", c)
	}
	if len(d.Nameservers) != 1 || d.Nameservers[0].Handle != "NS1-NAME" {
		t.Errorf("nameservers = %+v", d.Nameservers)
	}
	if d.SecureDNS == nil {
		t.Error("secureDNS is nil")
	}

	// Scan error propagates.
	fs = &fakeScanner{err: fmt.Errorf("boom")}
	if _, err := scanDomainRow(fs); err == nil {
		t.Error("expected scan error")
	}
}

func TestScanContactRow(t *testing.T) {
	created := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	updated := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	vcard := `["vcard", [["fn", {}, "text", "Example Registrar Inc."]]]`

	fs := &fakeScanner{values: []interface{}{
		"2", &vcard, jb([]string{"registrar"}), jb([]string{"active"}), created, updated,
		jb([]map[string]string{{"type": "IANA Registrar ID", "identifier": "2"}}),
	}}
	c, err := scanContactRow(fs)
	if err != nil {
		t.Fatalf("scanContactRow: %v", err)
	}
	if c.Handle != "2" {
		t.Errorf("handle = %q", c.Handle)
	}
	if len(c.Roles) != 1 || c.Roles[0] != domain.RoleRegistrar {
		t.Errorf("roles = %+v", c.Roles)
	}
	if c.VCard == nil || c.VCard.FullName != "Example Registrar Inc." {
		t.Errorf("vcard = %+v", c.VCard)
	}
	if len(c.PublicIDs) != 1 || c.PublicIDs[0].Identifier != "2" {
		t.Errorf("public ids = %+v", c.PublicIDs)
	}

	// Nil vcard is tolerated.
	fs = &fakeScanner{values: []interface{}{"X", nil, jb([]string{"registrant"}), jb([]string{"active"}), created, updated, jb([]interface{}{})}}
	c, err = scanContactRow(fs)
	if err != nil {
		t.Fatalf("scanContactRow(nil vcard): %v", err)
	}
	if c.VCard != nil {
		t.Errorf("expected nil vcard, got %+v", c.VCard)
	}

	fs = &fakeScanner{err: fmt.Errorf("boom")}
	if _, err := scanContactRow(fs); err == nil {
		t.Error("expected scan error")
	}
}

func TestScanNameserverRow(t *testing.T) {
	created := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	updated := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	fs := &fakeScanner{values: []interface{}{
		"NS1-NAME", "ns1.example.com", "ns1.example.com",
		jb([]string{"8.8.8.8"}), jb([]string{"2001:4860:4860::8888"}), jb([]string{"associated"}),
		created, updated,
	}}
	n, err := scanNameserverRow(fs)
	if err != nil {
		t.Fatalf("scanNameserverRow: %v", err)
	}
	if n.Handle != "NS1-NAME" || len(n.IPV4) != 1 || n.IPV4[0] != "8.8.8.8" {
		t.Errorf("nameserver = %+v", n)
	}
	if len(n.IPV6) != 1 || n.IPV6[0] != "2001:4860:4860::8888" {
		t.Errorf("ipv6 = %v", n.IPV6)
	}

	fs = &fakeScanner{err: fmt.Errorf("boom")}
	if _, err := scanNameserverRow(fs); err == nil {
		t.Error("expected scan error")
	}
}

func TestScanMySQLDomainRow(t *testing.T) {
	created := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	updated := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	expires := time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)
	billing := "BILL-1"

	fs := &fakeScanner{values: []interface{}{
		"EX1-NAME", "example.com", "example.com", "com",
		jb([]string{"active"}), created, updated, expires,
		nil, nil, nil, &billing,
		nil, jb(map[string]interface{}{"delegationSigned": true}),
	}}
	d, err := scanMySQLDomainRow(fs)
	if err != nil {
		t.Fatalf("scanMySQLDomainRow: %v", err)
	}
	if d.Handle != "EX1-NAME" {
		t.Errorf("handle = %q", d.Handle)
	}
	if d.SecureDNS == nil || !d.SecureDNS.DelegationSigned {
		t.Errorf("secureDNS = %+v", d.SecureDNS)
	}
	if c := d.Contacts[domain.RoleBilling]; len(c) != 1 || c[0] != "BILL-1" {
		t.Errorf("billing contacts = %v", c)
	}

	fs = &fakeScanner{err: fmt.Errorf("boom")}
	if _, err := scanMySQLDomainRow(fs); err == nil {
		t.Error("expected scan error")
	}
}

func TestScanMySQLContactRow(t *testing.T) {
	created := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	updated := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	vcard := `["vcard", [["fn", {}, "text", "Abuse Contact"]]]`

	fs := &fakeScanner{values: []interface{}{
		"ABUSE-NAME", &vcard, jb([]string{"abuse"}), jb([]string{"active"}), created, updated, jb([]interface{}{}),
	}}
	c, err := scanMySQLContactRow(fs)
	if err != nil {
		t.Fatalf("scanMySQLContactRow: %v", err)
	}
	if c.Handle != "ABUSE-NAME" || len(c.Roles) != 1 || c.Roles[0] != domain.RoleAbuse {
		t.Errorf("contact = %+v", c)
	}
	if c.VCard == nil || c.VCard.FullName != "Abuse Contact" {
		t.Errorf("vcard = %+v", c.VCard)
	}

	fs = &fakeScanner{err: fmt.Errorf("boom")}
	if _, err := scanMySQLContactRow(fs); err == nil {
		t.Error("expected scan error")
	}
}

func TestScanMySQLNameserverRow(t *testing.T) {
	created := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	updated := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	fs := &fakeScanner{values: []interface{}{
		"NS2-NAME", "ns2.example.com", "ns2.example.com",
		jb([]string{"1.1.1.1"}), jb([]string{"2606:4700:4700::1111"}), jb([]string{"associated"}),
		created, updated,
	}}
	n, err := scanMySQLNameserverRow(fs)
	if err != nil {
		t.Fatalf("scanMySQLNameserverRow: %v", err)
	}
	if n.Handle != "NS2-NAME" || len(n.IPV4) != 1 || n.IPV4[0] != "1.1.1.1" {
		t.Errorf("nameserver = %+v", n)
	}

	fs = &fakeScanner{err: fmt.Errorf("boom")}
	if _, err := scanMySQLNameserverRow(fs); err == nil {
		t.Error("expected scan error")
	}
}

func TestPopulateDomainContacts(t *testing.T) {
	d := &domain.Domain{}
	reg, admin, tech, billing := "R", "A", "T", "B"
	populateDomainContacts(d, &reg, &admin, &tech, &billing)
	if d.Registrar != "R" {
		t.Errorf("registrar = %q", d.Registrar)
	}
	for role, want := range map[domain.ContactRole]string{
		domain.RoleRegistrant:     "R",
		domain.RoleAdministrative: "A",
		domain.RoleTechnical:      "T",
		domain.RoleBilling:        "B",
	} {
		if got := d.Contacts[role]; len(got) != 1 || got[0] != want {
			t.Errorf("contacts[%s] = %v, want [%s]", role, got, want)
		}
	}

	// All nil -> empty map, no registrar.
	d2 := &domain.Domain{}
	populateDomainContacts(d2, nil, nil, nil, nil)
	if d2.Registrar != "" || len(d2.Contacts) != 0 {
		t.Errorf("empty contacts = %+v", d2)
	}
}

func TestPatternToSQL(t *testing.T) {
	if got := patternToSQL("example*"); got != "example%" {
		t.Errorf("patternToSQL(example*) = %q", got)
	}
	if got := patternToSQL("ns?1"); got != "ns_1" {
		t.Errorf("patternToSQL(ns?1) = %q", got)
	}
}
