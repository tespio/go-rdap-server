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
