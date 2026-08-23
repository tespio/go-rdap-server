package service

import (
	"errors"
	"testing"

	"github.com/tespio/go-rdap-server/internal/config"
	"github.com/tespio/go-rdap-server/internal/domain"
	"github.com/tespio/go-rdap-server/internal/store"
)

const testReqURL = "https://rdap.example.com/domain/example.com"

func lookupService() *Service {
	st, err := store.NewMemoryStore(config.StorageConfig{})
	if err != nil {
		panic(err)
	}
	return New(st, config.RDAPConfig{
		BaseURL:          "https://rdap.example.com",
		RegistrarBaseURL: "https://rdap.example.org/rdap/",
		Mode:             "registrar",
		MaxSearchLimit:   100,
	})
}

func TestLookupDomain(t *testing.T) {
	svc := lookupService()

	d, err := svc.LookupDomain("example.com", testReqURL)
	if err != nil {
		t.Fatalf("LookupDomain: %v", err)
	}
	if d.Handle != "EX1-NAME" {
		t.Errorf("handle = %q", d.Handle)
	}
	if d.LDHName != "example.com" {
		t.Errorf("ldhName = %q", d.LDHName)
	}

	// Not-found propagates the store error.
	if _, err := svc.LookupDomain("nope.invalid", testReqURL); err == nil {
		t.Error("expected error for unknown domain")
	}
}

func TestLookupEntity(t *testing.T) {
	svc := lookupService()

	e, err := svc.LookupEntity("2", testReqURL)
	if err != nil {
		t.Fatalf("LookupEntity: %v", err)
	}
	if e.Handle != "2" {
		t.Errorf("handle = %q", e.Handle)
	}

	if _, err := svc.LookupEntity("nope", testReqURL); err == nil {
		t.Error("expected error for unknown entity")
	}
}

func TestLookupNameserver(t *testing.T) {
	svc := lookupService()

	ns, err := svc.LookupNameserver("ns1.example.com", testReqURL)
	if err != nil {
		t.Fatalf("LookupNameserver: %v", err)
	}
	if ns.Handle != "NS1-NAME" {
		t.Errorf("handle = %q", ns.Handle)
	}

	if _, err := svc.LookupNameserver("nope.example", testReqURL); err == nil {
		t.Error("expected error for unknown nameserver")
	}
}

func TestLookupIPNetwork(t *testing.T) {
	svc := lookupService()

	n, err := svc.LookupIPNetwork("8.8.8.0/24", testReqURL)
	if err != nil {
		t.Fatalf("LookupIPNetwork: %v", err)
	}
	if n.Handle != "NET-8-8-8-0-24" {
		t.Errorf("handle = %q", n.Handle)
	}

	if _, err := svc.LookupIPNetwork("1.2.3.0/24", testReqURL); err == nil {
		t.Error("expected error for unknown network")
	}
}

func TestSearchDomainsByName(t *testing.T) {
	svc := lookupService()

	res, err := svc.SearchDomainsByName("example*", 10, testReqURL)
	if err != nil {
		t.Fatalf("SearchDomainsByName: %v", err)
	}
	if len(res) != 1 || res[0].LDHName != "example.com" {
		t.Errorf("results = %+v", res)
	}
}

func TestSearchDomainsByNS(t *testing.T) {
	svc := lookupService()

	res, err := svc.SearchDomainsByNS("ns1.example.com", 10, testReqURL)
	if err != nil {
		t.Fatalf("SearchDomainsByNS: %v", err)
	}
	if len(res) != 1 || res[0].LDHName != "example.com" {
		t.Errorf("results = %+v", res)
	}
}

func TestSearchEntitiesByName(t *testing.T) {
	svc := lookupService()

	res, err := svc.SearchEntitiesByName("REG*", 10, testReqURL)
	if err != nil {
		t.Fatalf("SearchEntitiesByName: %v", err)
	}
	if len(res) != 1 || res[0].Handle != "REG1-NAME" {
		t.Errorf("results = %+v", res)
	}
}

func TestSearchEntitiesByHandle(t *testing.T) {
	svc := lookupService()

	res, err := svc.SearchEntitiesByHandle("888", 10, testReqURL)
	if err != nil {
		t.Fatalf("SearchEntitiesByHandle: %v", err)
	}
	if len(res) != 1 || res[0].Handle != "888" {
		t.Errorf("results = %+v", res)
	}
}

func TestSearchNameserversByName(t *testing.T) {
	svc := lookupService()

	res, err := svc.SearchNameserversByName("ns1*", 10, testReqURL)
	if err != nil {
		t.Fatalf("SearchNameserversByName: %v", err)
	}
	if len(res) != 1 || res[0].Handle != "NS1-NAME" {
		t.Errorf("results = %+v", res)
	}
}

func TestSearchNameserversByIP(t *testing.T) {
	svc := lookupService()

	res, err := svc.SearchNameserversByIP("8.8.8.8", 10, testReqURL)
	if err != nil {
		t.Fatalf("SearchNameserversByIP: %v", err)
	}
	if len(res) != 1 || res[0].Handle != "NS1-NAME" {
		t.Errorf("results = %+v", res)
	}
}

func TestBaseURL(t *testing.T) {
	svc := lookupService()
	if got := svc.BaseURL(); got != "https://rdap.example.com" {
		t.Errorf("BaseURL = %q", got)
	}
}

// failStore is a store.Interface that returns an error from every search,
// letting us exercise the error-propagation branches of the service Search* wrappers.
type failStore struct{}

func (failStore) LookupDomain(name string) (*domain.Domain, error) { return nil, errors.New("fail") }
func (failStore) GetDomainAggregate(name string) (*domain.DomainAggregate, error) {
	return nil, errors.New("fail")
}
func (failStore) LookupContact(handle string) (*domain.Contact, error) {
	return nil, errors.New("fail")
}
func (failStore) LookupNameserver(name string) (*domain.NameServer, error) {
	return nil, errors.New("fail")
}
func (failStore) LookupIPNetwork(cidr string) (*domain.IPNetwork, error) {
	return nil, errors.New("fail")
}
func (failStore) LookupAutnum(asn int) (*domain.Autnum, error) { return nil, errors.New("fail") }
func (failStore) SearchDomainsByName(pattern string, limit int) ([]domain.Domain, error) {
	return nil, errors.New("fail")
}
func (failStore) SearchDomainsByNS(nsName string, limit int) ([]domain.Domain, error) {
	return nil, errors.New("fail")
}
func (failStore) SearchContactsByName(pattern string, limit int) ([]domain.Contact, error) {
	return nil, errors.New("fail")
}
func (failStore) SearchContactsByHandle(pattern string, limit int) ([]domain.Contact, error) {
	return nil, errors.New("fail")
}
func (failStore) SearchNameserversByName(pattern string, limit int) ([]domain.NameServer, error) {
	return nil, errors.New("fail")
}
func (failStore) SearchNameserversByIP(ip string, limit int) ([]domain.NameServer, error) {
	return nil, errors.New("fail")
}
func (failStore) Ping() error  { return nil }
func (failStore) Close() error { return nil }

func TestSearchErrorPropagation(t *testing.T) {
	svc := New(failStore{}, config.RDAPConfig{BaseURL: "https://rdap.example.com", Mode: "registrar"})
	calls := []struct {
		name string
		fn   func() error
	}{
		{"SearchDomainsByName", func() error { _, err := svc.SearchDomainsByName("x", 10, testReqURL); return err }},
		{"SearchDomainsByNS", func() error { _, err := svc.SearchDomainsByNS("x", 10, testReqURL); return err }},
		{"SearchEntitiesByName", func() error { _, err := svc.SearchEntitiesByName("x", 10, testReqURL); return err }},
		{"SearchEntitiesByHandle", func() error { _, err := svc.SearchEntitiesByHandle("x", 10, testReqURL); return err }},
		{"SearchNameserversByName", func() error { _, err := svc.SearchNameserversByName("x", 10, testReqURL); return err }},
		{"SearchNameserversByIP", func() error { _, err := svc.SearchNameserversByIP("x", 10, testReqURL); return err }},
	}
	for _, c := range calls {
		if err := c.fn(); err == nil {
			t.Errorf("%s: expected error", c.name)
		}
	}
}
