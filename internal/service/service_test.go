package service

import (
	"testing"
	"time"

	"github.com/tespio/go-rdap-server/internal/config"
	"github.com/tespio/go-rdap-server/internal/domain"
	"github.com/tespio/go-rdap-server/internal/rdap"
)

func testService() *Service {
	return New(nil, config.RDAPConfig{
		BaseURL:          "https://rdap.example.com",
		RegistrarBaseURL: "https://rdap.example.org/rdap/",
		Mode:             "registrar",
	})
}

func vcardProp(vcard []interface{}, name string) []interface{} {
	arr, _ := vcard[1].([]interface{})
	for _, p := range arr {
		prop, _ := p.([]interface{})
		if len(prop) >= 4 && prop[0] == name {
			return prop
		}
	}
	return nil
}

func fnValue(vcard []interface{}) string {
	if prop := vcardProp(vcard, "fn"); prop != nil {
		if s, ok := prop[3].(string); ok {
			return s
		}
	}
	return ""
}

// TestAggregateWithSparseRegistrarDoesNotFallBackToExample ensures that a
// resolved registrar contact that is real but sparse (no vcard data) is rendered
// as-is and does NOT silently substitute the fake "Example Registrar Inc." text.
func TestAggregateWithSparseRegistrarDoesNotFallBackToExample(t *testing.T) {
	svc := testService()

	agg := &domain.DomainAggregate{
		Domain: &domain.Domain{
			Handle:    "EX1-NAME",
			LDHName:   "example.com",
			Status:    []domain.Status{{Value: "active"}},
			ExpiresAt: time.Now().Add(time.Hour),
			Registrar: "42", // real registrar handle
			Metadata:  domain.Metadata{CreatedAt: time.Now(), UpdatedAt: time.Now()},
		},
		Registrar: &domain.Contact{
			Handle: "42",
			Roles:  []domain.ContactRole{domain.RoleRegistrar},
			Status: []domain.Status{{Value: "active"}},
			// Intentionally no VCard: real but sparse registrar.
		},
		Contacts:    map[string]*domain.Contact{},
		Nameservers: map[string]*domain.NameServer{},
	}

	out := svc.DomainAggregateToRDAP(agg, "https://rdap.example.com/domain/example.com")

	var registrar *rdap.Entity
	for i := range out.Entities {
		if len(out.Entities[i].Roles) > 0 && out.Entities[i].Roles[0] == "registrar" {
			registrar = &out.Entities[i]
			break
		}
	}
	if registrar == nil {
		t.Fatal("expected a registrar entity")
	}
	fn := fnValue(registrar.VCardArray)
	if fn == "Example Registrar Inc." {
		t.Fatalf("sparse registrar fell back to the fake example (fn=%q)", fn)
	}
	if fn != "42" {
		t.Fatalf("sparse registrar fn should be synthesized from the handle, got %q", fn)
	}
	if registrar.Handle != "42" {
		t.Fatalf("registrar handle = %q, want %q", registrar.Handle, "42")
	}
}

// TestAggregateWithNoRegistrarUsesExample confirms the static example is used
// ONLY when there is genuinely no resolved registrar (plain domain search path).
func TestAggregateWithNoRegistrarUsesExample(t *testing.T) {
	svc := testService()

	// agg is nil: the plain domain-search path (no aggregate).
	out := svc.DomainToRDAP(&domain.Domain{
		Handle:    "EX1-NAME",
		LDHName:   "example.com",
		Status:    []domain.Status{{Value: "active"}},
		ExpiresAt: time.Now().Add(time.Hour),
		Registrar: "2",
		Metadata:  domain.Metadata{CreatedAt: time.Now(), UpdatedAt: time.Now()},
	}, "https://rdap.example.com/domain/example.com")

	var registrar *rdap.Entity
	for i := range out.Entities {
		if len(out.Entities[i].Roles) > 0 && out.Entities[i].Roles[0] == "registrar" {
			registrar = &out.Entities[i]
			break
		}
	}
	if registrar == nil {
		t.Fatal("expected a registrar entity")
	}
	if fn := fnValue(registrar.VCardArray); fn != "Example Registrar Inc." {
		t.Fatalf("no-registrar path should use the example, got fn=%q", fn)
	}
}

// TestAggregateWithSparseRegistrantDoesNotFallBackToExample covers the
// partially-redacted registrant case: a resolved registrant whose vcard is empty
// must render as empty, not fall back to the fake "Example Registrant".
func TestAggregateWithSparseRegistrantDoesNotFallBackToExample(t *testing.T) {
	svc := testService()

	agg := &domain.DomainAggregate{
		Domain: &domain.Domain{
			Handle:    "EX1-NAME",
			LDHName:   "example.com",
			Status:    []domain.Status{{Value: "active"}},
			ExpiresAt: time.Now().Add(time.Hour),
			Registrar: "42",
			Contacts: map[domain.ContactRole][]string{
				domain.RoleRegistrant: {"REG-REAL"},
			},
			Metadata: domain.Metadata{CreatedAt: time.Now(), UpdatedAt: time.Now()},
		},
		Registrar: &domain.Contact{Handle: "42", Roles: []domain.ContactRole{domain.RoleRegistrar}},
		Contacts: map[string]*domain.Contact{
			"REG-REAL": {
				Handle: "REG-REAL",
				Roles:  []domain.ContactRole{domain.RoleRegistrant},
				// Intentionally nil VCard: redacted/sparse registrant.
			},
		},
		Nameservers: map[string]*domain.NameServer{},
	}

	out := svc.DomainAggregateToRDAP(agg, "https://rdap.example.com/domain/example.com")

	var registrant *rdap.Entity
	for i := range out.Entities {
		if len(out.Entities[i].Roles) > 0 && out.Entities[i].Roles[0] == "registrant" {
			registrant = &out.Entities[i]
			break
		}
	}
	if registrant == nil {
		t.Fatal("expected a registrant entity")
	}
	fn := fnValue(registrant.VCardArray)
	if fn == "Example Registrant" {
		t.Fatalf("sparse registrant fell back to the fake example (fn=%q)", fn)
	}
	if fn != "REG-REAL" {
		t.Fatalf("sparse registrant fn should be synthesized from the handle, got %q", fn)
	}
	if registrant.Handle != "REG-REAL" {
		t.Fatalf("registrant handle = %q, want REG-REAL", registrant.Handle)
	}
}

// TestVCardToJCardAlwaysHasFN guarantees the serialized vcard always includes the
// REQUIRED jCard "fn" property (RFC 6350 §6.2.1), even for a nil vcard or one
// with no name — so a name-less contact can never produce an fn-less (invalid)
// vcard on a real response path.
func TestVCardToJCardAlwaysHasFN(t *testing.T) {
	cases := []struct {
		name    string
		v       *domain.VCard
		fallback string
		wantFn  string
	}{
		{"nil vcard", nil, "HANDLE-1", "HANDLE-1"},
		{"empty vcard", &domain.VCard{}, "HANDLE-2", "HANDLE-2"},
		{"named vcard", &domain.VCard{FullName: "Real Registrar"}, "HANDLE-3", "Real Registrar"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			jcard := vcardToJCard(tc.v, tc.fallback)
			fn := fnValue(jcard)
			if fn == "" {
				t.Fatalf("vcard emitted without a required fn property")
			}
			if fn != tc.wantFn {
				t.Fatalf("fn = %q, want %q", fn, tc.wantFn)
			}
		})
	}
}

// TestDomainToRDAPRegistrarModeEvents ensures registrar mode emits the
// "registrar expiration" event required by the 2024 profile (-65600).
func TestDomainToRDAPRegistrarModeEvents(t *testing.T) {
	svc := testService() // mode = registrar
	now := time.Now()
	out := svc.DomainToRDAP(&domain.Domain{
		Handle:    "EX1-NAME",
		LDHName:   "example.com",
		Status:    []domain.Status{{Value: "active"}},
		ExpiresAt: now.Add(24 * time.Hour),
		Registrar: "2",
		Metadata:  domain.Metadata{CreatedAt: now.Add(-time.Hour), UpdatedAt: now},
	}, "https://rdap.example.com/domain/example.com")

	actions := make(map[string]bool)
	for _, e := range out.Events {
		actions[e.EventAction] = true
	}
	for _, want := range []string{"registration", "last changed", "expiration", "last update of RDAP database", "registrar expiration"} {
		if !actions[want] {
			t.Fatalf("registrar mode missing event %q; got %v", want, actions)
		}
	}
}

// TestDomainToRDAPRegistryModeOmitsRegistrant ensures registry mode does NOT
// emit a registrant entity or the registrar-expiration event.
func TestDomainToRDAPRegistryModeOmitsRegistrant(t *testing.T) {
	svc := New(nil, config.RDAPConfig{
		BaseURL:          "https://rdap.example.com",
		RegistrarBaseURL: "https://rdap.example.org/rdap/",
		Mode:             "registry",
	})
	now := time.Now()
	out := svc.DomainToRDAP(&domain.Domain{
		Handle:    "EX1-NAME",
		LDHName:   "example.com",
		Status:    []domain.Status{{Value: "active"}},
		ExpiresAt: now.Add(24 * time.Hour),
		Registrar: "2",
		Metadata:  domain.Metadata{CreatedAt: now.Add(-time.Hour), UpdatedAt: now},
	}, "https://rdap.example.com/domain/example.com")

	for _, e := range out.Entities {
		for _, r := range e.Roles {
			if r == "registrant" {
				t.Fatal("registry mode must not emit a registrant entity")
			}
		}
	}
	for _, e := range out.Events {
		if e.EventAction == "registrar expiration" {
			t.Fatal("registry mode must not emit a registrar expiration event")
		}
	}
}

// TestEntityToRDAP verifies the contact -> entity mapping (roles, public IDs,
// self link, events).
func TestEntityToRDAP(t *testing.T) {
	svc := testService()
	now := time.Now()
	out := svc.EntityToRDAP(&domain.Contact{
		Handle: "2",
		Roles:  []domain.ContactRole{domain.RoleRegistrar},
		Status: []domain.Status{{Value: "active"}},
		PublicIDs: []domain.PublicID{
			{Type: "IANA Registrar ID", Identifier: "2"},
		},
		Metadata: domain.Metadata{CreatedAt: now.Add(-time.Hour), UpdatedAt: now},
	}, "https://rdap.example.com/entity/2")

	if out.ObjectClassName != "entity" {
		t.Fatalf("objectClassName = %q, want entity", out.ObjectClassName)
	}
	if out.Handle != "2" {
		t.Fatalf("handle = %q, want 2", out.Handle)
	}
	if len(out.Roles) != 1 || out.Roles[0] != "registrar" {
		t.Fatalf("roles = %v, want [registrar]", out.Roles)
	}
	if len(out.PublicIDs) != 1 || out.PublicIDs[0].Identifier != "2" {
		t.Fatalf("publicIds = %v, want IANA Registrar ID 2", out.PublicIDs)
	}
	// self link
	if len(out.Links) != 1 || out.Links[0].Rel != "self" {
		t.Fatalf("expected a self link, got %v", out.Links)
	}
	if out.Links[0].Href != "https://rdap.example.com/entity/2" {
		t.Fatalf("self href = %q", out.Links[0].Href)
	}
}

// TestNameserverToRDAP verifies the nameserver -> mapping (IPs, self link).
func TestNameserverToRDAP(t *testing.T) {
	svc := testService()
	now := time.Now()
	out := svc.NameserverToRDAP(&domain.NameServer{
		Handle:      "NS1-NAME",
		LDHName:     "ns1.example.com",
		UnicodeName: "ns1.example.com",
		IPV4:        []string{"8.8.8.8"},
		IPV6:        []string{"2001:4860:4860::8888"},
		Status:      []domain.Status{{Value: "associated"}},
		Metadata:    domain.Metadata{CreatedAt: now.Add(-time.Hour), UpdatedAt: now},
	}, "https://rdap.example.com/nameserver/ns1.example.com")

	if out.LDHName != "ns1.example.com" {
		t.Fatalf("ldhName = %q", out.LDHName)
	}
	if out.IPAddresses == nil || len(out.IPAddresses.V4) != 1 || out.IPAddresses.V4[0] != "8.8.8.8" {
		t.Fatalf("v4 addresses = %+v", out.IPAddresses)
	}
	if out.IPAddresses.V6[0] != "2001:4860:4860::8888" {
		t.Fatalf("v6 addresses = %+v", out.IPAddresses.V6)
	}
	if out.Links[0].Href != "https://rdap.example.com/nameserver/ns1.example.com" {
		t.Fatalf("self href = %q", out.Links[0].Href)
	}
}

// TestIPNetworkToRDAP verifies the IP network mapping.
func TestIPNetworkToRDAP(t *testing.T) {
	svc := testService()
	now := time.Now()
	out := svc.IPNetworkToRDAP(&domain.IPNetwork{
		Handle:       "NET-8-8-8-0-24",
		StartAddress: "8.8.8.0",
		EndAddress:   "8.8.8.255",
		IPVersion:    "v4",
		CIDR:         []string{"8.8.8.0/24"},
		Name:         "GOOGLE",
		Type:         "ALLOCATED",
		Country:      "US",
		Status:       []domain.Status{{Value: "active"}},
		Metadata:     domain.Metadata{CreatedAt: now.Add(-time.Hour), UpdatedAt: now},
	}, "https://rdap.example.com/ip/8.8.8.0/24")

	if out.ObjectClassName != "ip network" {
		t.Fatalf("objectClassName = %q", out.ObjectClassName)
	}
	if out.StartAddress != "8.8.8.0" || out.EndAddress != "8.8.8.255" {
		t.Fatalf("address range = %s-%s", out.StartAddress, out.EndAddress)
	}
	if len(out.CIDR) != 1 || out.CIDR[0] != "8.8.8.0/24" {
		t.Fatalf("cidr = %v", out.CIDR)
	}
	if out.Country != "US" {
		t.Fatalf("country = %q", out.Country)
	}
	if out.Links[0].Href != "https://rdap.example.com/ip/8.8.8.0/24" {
		t.Fatalf("self href = %q", out.Links[0].Href)
	}
}

// TestNoticeOptionsFromConfig verifies config -> rdap.NoticeOptions conversion
// (ToS customization + custom notices).
func TestNoticeOptionsFromConfig(t *testing.T) {
	opts := NoticeOptionsFromConfig(config.RDAPConfig{
		ToS: &config.ToSConfig{
			Title:       "My ToS",
			Description: []string{"Provided by Example Registrar, Inc."},
			URL:         "https://example.com/terms",
		},
		CustomNotices: []config.CustomNoticeConfig{
			{Title: "Data Policy", Description: []string{"Policy text"}, URL: "https://example.com/policy", Rel: "privacy-policy"},
		},
	})

	if opts.ToSTitle != "My ToS" {
		t.Fatalf("ToSTitle = %q", opts.ToSTitle)
	}
	if len(opts.ToSDescription) != 1 || opts.ToSDescription[0] != "Provided by Example Registrar, Inc." {
		t.Fatalf("ToSDescription = %v", opts.ToSDescription)
	}
	if opts.ToSURL != "https://example.com/terms" {
		t.Fatalf("ToSURL = %q", opts.ToSURL)
	}
	if len(opts.Custom) != 1 || opts.Custom[0].Rel != "privacy-policy" {
		t.Fatalf("custom notices = %+v", opts.Custom)
	}
}

// TestVCardToJCardRendersAllProperties verifies a full VCard maps to all expected
// jCard properties (fn, org, adr, tel, email) with correct shapes.
func TestVCardToJCardRendersAllProperties(t *testing.T) {
	jcard := vcardToJCard(&domain.VCard{
		FullName:     "Alice Anderson",
		Kind:         "individual",
		Organization: "Anderson LLC",
		Address: &domain.VCardAddress{
			CountryCode: "US", Street: "123 Elm", Locality: "Springfield", Region: "IL", PostalCode: "62701",
		},
		VoiceTel: "tel:+1-217-555-0132",
		Email:    "alice@example.com",
	}, "HANDLE")

	if fn := fnValue(jcard); fn != "Alice Anderson" {
		t.Fatalf("fn = %q", fn)
	}
	if prop := vcardProp(jcard, "org"); prop == nil || prop[3] != "Anderson LLC" {
		t.Fatalf("org missing/incorrect: %+v", prop)
	}
	if prop := vcardProp(jcard, "adr"); prop == nil {
		t.Fatal("adr missing")
	} else {
		adr := prop[3].([]interface{})
		if len(adr) != 7 {
			t.Fatalf("adr must have 7 elements, got %d", len(adr))
		}
		if adr[2] != "123 Elm" || adr[3] != "Springfield" {
			t.Fatalf("adr street/locality = %v", adr)
		}
	}
	if prop := vcardProp(jcard, "tel"); prop == nil || prop[3] != "tel:+1-217-555-0132" {
		t.Fatalf("tel missing/incorrect: %+v", prop)
	}
	if prop := vcardProp(jcard, "email"); prop == nil || prop[3] != "alice@example.com" {
		t.Fatalf("email missing/incorrect: %+v", prop)
	}
}
