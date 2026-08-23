package whois

import (
	"strings"
	"testing"
	"time"
)

func TestRenderDomain(t *testing.T) {
	d := WhoisDomain{
		LDHName:     "example.com",
		UnicodeName: "example.com",
		Status:      []string{"active"},
		Registrar:   "Example Registrar Inc.",
		RegistrarID: "2",
		Registrant:  "Example Registrant",
		Contacts: []WhoisContact{
			{Role: "registrant", Name: "Example Registrant", Org: "Example Organization",
				Email: "registrant@example.com", Phone: "tel:+1-217-555-0132",
				Address: "123 Elm Street, Springfield, IL, 62701"},
			{Role: "technical", Name: "Example Technical", Email: "tech@example.com"},
		},
		Nameservers: []WhoisNameserver{
			{Name: "ns1.example.com", IPV4: []string{"8.8.8.8"}},
			{Name: "ns2.example.com", IPV6: []string{"2606:4700:4700::1111"}},
		},
		CreatedAt: time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC),
		UpdatedAt: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC),
		ExpiresAt: time.Date(2027, 1, 2, 3, 4, 5, 0, time.UTC),
		DNSSEC:    false,
	}

	out := RenderDomain(d)
	for _, want := range []string{
		"Domain Name: example.com",
		"Registrar: Example Registrar Inc.",
		"Registrar IANA ID: 2",
		"Registrant Organization: Example Registrant",
		"Registrant:",
		"Example Registrant",
		"registrant@example.com",
		"Name Server:",
		"ns1.example.com",
		"8.8.8.8",
		"ns2.example.com",
		"DNSSEC: unsignedDelegation",
		"Creation Date: 2025-01-02T03:04:05Z",
		"Expiration Date: 2027-01-02T03:04:05Z",
		"Status: active",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("rendered WHOIS missing %q\n---\n%s", want, out)
		}
	}
}

func TestRenderDomainRedacted(t *testing.T) {
	d := WhoisDomain{
		LDHName: "example.com",
		Contacts: []WhoisContact{
			{Role: "registrant", Name: "Should Not Show", Redacted: true},
		},
	}
	out := RenderDomain(d)
	if strings.Contains(out, "Should Not Show") {
		t.Error("redacted contact name should not appear")
	}
	if !strings.Contains(out, "Redacted") {
		t.Error("expected Redacted marker")
	}
}

func TestRenderDomainDNSSECSigned(t *testing.T) {
	d := WhoisDomain{LDHName: "example.com", DNSSEC: true}
	if !strings.Contains(RenderDomain(d), "DNSSEC: signedDelegation") {
		t.Error("expected signedDelegation")
	}
}

func TestRoleLabel(t *testing.T) {
	cases := map[string]string{
		"registrant": "Registrant:",
		"admin":      "Administrative Contact:",
		"technical":  "Technical Contact:",
		"billing":    "Billing Contact:",
		"abuse":      "Abuse Contact:",
		"other":      "",
	}
	for in, want := range cases {
		if got := roleLabel(in); got != want {
			t.Errorf("roleLabel(%q) = %q, want %q", in, got, want)
		}
	}
}
