package store

import (
	"testing"

	"github.com/tespio/go-rdap-server/internal/domain"
)

func TestParseStatus(t *testing.T) {
	status := parseStatus([]byte(`["active","clientTransferProhibited"]`))
	if len(status) != 2 {
		t.Fatalf("expected 2 statuses, got %d", len(status))
	}
	if status[0].Value != "active" || status[1].Value != "clientTransferProhibited" {
		t.Errorf("unexpected statuses: %+v", status)
	}

	if got := parseStatus([]byte(`not-json`)); got != nil {
		t.Errorf("invalid JSON should return nil, got %+v", got)
	}
}

func TestParseStatusVal(t *testing.T) {
	got := parseStatusVal([]string{"active", "associated"})
	if len(got) != 2 || got[0].Value != "active" || got[1].Value != "associated" {
		t.Fatalf("unexpected: %+v", got)
	}
	if got := parseStatusVal(nil); len(got) != 0 {
		t.Errorf("nil input should return empty slice, got %+v", got)
	}
}

func TestParseNameservers(t *testing.T) {
	raw := `[
		{"handle":"NS1-NAME","ldhName":"ns1.example.com","unicodeName":"ns1.example.com","ipv4":["8.8.8.8"],"ipv6":["2001:4860:4860::8888"],"status":["associated"]},
		{"handle":"NS2-NAME","ldhName":"ns2.example.com","unicodeName":"ns2.example.com","ipv4":["1.1.1.1"],"ipv6":["2606:4700:4700::1111"],"status":["associated"]}
	]`
	ns := parseNameservers([]byte(raw))
	if len(ns) != 2 {
		t.Fatalf("expected 2 nameservers, got %d", len(ns))
	}
	if ns[0].Handle != "NS1-NAME" || ns[0].LDHName != "ns1.example.com" {
		t.Errorf("nameserver[0] = %+v", ns[0])
	}
	if len(ns[0].IPV4) != 1 || ns[0].IPV4[0] != "8.8.8.8" {
		t.Errorf("nameserver[0] IPv4 = %v", ns[0].IPV4)
	}
	if len(ns[0].Status) != 1 || ns[0].Status[0].Value != "associated" {
		t.Errorf("nameserver[0] status = %+v", ns[0].Status)
	}

	if got := parseNameservers([]byte(`nope`)); got != nil {
		t.Errorf("invalid JSON should return nil, got %+v", got)
	}
}

func TestParseSecureDNS(t *testing.T) {
	raw := `{
		"zoneSigned":true,
		"delegationSigned":true,
		"maxSigLife":172800,
		"dsData":[{"keyTag":2371,"algorithm":13,"digestType":2,"digest":"aabb"}],
		"keyData":[{"flags":257,"protocol":3,"algorithm":13,"publicKey":"AAA="}]
	}`
	sd := parseSecureDNS([]byte(raw))
	if sd == nil {
		t.Fatal("expected non-nil SecureDNS")
	}
	if !sd.ZoneSigned || !sd.DelegationSigned {
		t.Errorf("zone/delegation signed = %v/%v", sd.ZoneSigned, sd.DelegationSigned)
	}
	if sd.MaxSigLife == nil || *sd.MaxSigLife != 172800 {
		t.Errorf("maxSigLife = %v", sd.MaxSigLife)
	}
	if len(sd.DSRecords) != 1 || sd.DSRecords[0].KeyTag != 2371 || sd.DSRecords[0].Digest != "aabb" {
		t.Errorf("DSRecords = %+v", sd.DSRecords)
	}
	if len(sd.KeyRecords) != 1 || sd.KeyRecords[0].Flags != 257 || sd.KeyRecords[0].PublicKey != "AAA=" {
		t.Errorf("KeyRecords = %+v", sd.KeyRecords)
	}

	// Nil / empty input returns nil.
	if got := parseSecureDNS(nil); got != nil {
		t.Errorf("nil input should return nil, got %+v", got)
	}
	if got := parseSecureDNS([]byte("")); got != nil {
		t.Errorf("empty input should return nil, got %+v", got)
	}
	// Invalid JSON returns nil.
	if got := parseSecureDNS([]byte("junk")); got != nil {
		t.Errorf("invalid JSON should return nil, got %+v", got)
	}
}

func TestStatusStrings(t *testing.T) {
	got := statusStrings([]domain.Status{{Value: "active"}, {Value: "ok"}})
	if len(got) != 2 || got[0] != "active" || got[1] != "ok" {
		t.Fatalf("unexpected: %v", got)
	}
	if got := statusStrings(nil); len(got) != 0 {
		t.Errorf("nil should return empty, got %v", got)
	}
}

func TestParseVCardJSON(t *testing.T) {
	raw := `["vcard", [
		["version", {}, "text", "4.0"],
		["fn", {}, "text", "Example Registrant"],
		["kind", {}, "text", "individual"],
		["org", {}, "text", "Example Organization"],
		["tel", {"type":"voice"}, "uri", "tel:+1-217-555-0132"],
		["tel", {"type":"fax"}, "uri", "tel:+1-217-555-0199"],
		["email", {}, "text", "registrant@example.com"],
		["contact-uri", {}, "uri", "https://rdap.example.com/contact/1"],
		["adr", {"cc":"US"}, "text", ["", "", "123 Elm Street", "Springfield", "IL", "62701", "United States"]]
	]]`
	v := parseVCardJSON(raw)
	if v == nil {
		t.Fatal("expected non-nil VCard")
	}
	if v.FullName != "Example Registrant" || v.Kind != "individual" || v.Organization != "Example Organization" {
		t.Errorf("name/kind/org = %q/%q/%q", v.FullName, v.Kind, v.Organization)
	}
	if v.VoiceTel != "tel:+1-217-555-0132" || v.FaxTel != "tel:+1-217-555-0199" {
		t.Errorf("tel = voice:%q fax:%q", v.VoiceTel, v.FaxTel)
	}
	if v.Email != "registrant@example.com" || v.ContactURI != "https://rdap.example.com/contact/1" {
		t.Errorf("email/uri = %q/%q", v.Email, v.ContactURI)
	}
	if v.Address == nil {
		t.Fatal("expected address")
	}
	if v.Address.CountryCode != "US" || v.Address.Street != "123 Elm Street" || v.Address.Locality != "Springfield" ||
		v.Address.Region != "IL" || v.Address.PostalCode != "62701" || v.Address.CountryName != "United States" {
		t.Errorf("address = %+v", v.Address)
	}

	// Invalid inputs return nil.
	for _, bad := range []string{"not-json", `["vcard"]`, `["vcard", {}]`, `[123]`} {
		if got := parseVCardJSON(bad); got != nil {
			t.Errorf("parseVCardJSON(%q) should be nil, got %+v", bad, got)
		}
	}
}
