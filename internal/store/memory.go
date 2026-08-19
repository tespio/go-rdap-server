package store

import (
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/tespio/go-rdap-server/internal/config"
	"github.com/tespio/go-rdap-server/internal/domain"
)

// MemoryStore is an in-memory implementation of the storage interface, seeded
// with sample registry data. It produces canonical domain.Model objects.
type MemoryStore struct {
	mu             sync.RWMutex
	domains        map[string]*domain.Domain
	contacts       map[string]*domain.Contact
	nameservers    map[string]*domain.NameServer
	ipNetworks     map[string]*domain.IPNetwork
	autnums        map[int]*domain.Autnum
	domainByNS     map[string][]string
	domainByEntity map[string][]string
	cacheTTL       time.Duration
	seeded         bool
}

func NewMemoryStore(cfg config.StorageConfig) (*MemoryStore, error) {
	ttl := 5 * time.Minute
	if cfg.CacheTTL != "" {
		parsed, err := time.ParseDuration(cfg.CacheTTL)
		if err == nil {
			ttl = parsed
		}
	}

	s := &MemoryStore{
		domains:        make(map[string]*domain.Domain),
		contacts:       make(map[string]*domain.Contact),
		nameservers:    make(map[string]*domain.NameServer),
		ipNetworks:     make(map[string]*domain.IPNetwork),
		autnums:        make(map[int]*domain.Autnum),
		domainByNS:     make(map[string][]string),
		domainByEntity: make(map[string][]string),
		cacheTTL:       ttl,
	}

	s.seed()
	return s, nil
}

func (s *MemoryStore) seed() {
	if s.seeded {
		return
	}
	s.seeded = true

	now := time.Now()

	// Seed example registrar (IANA Registrar ID 2 = Network Solutions)
	s.contacts["2"] = &domain.Contact{
		Handle: "2",
		Roles:  []domain.ContactRole{domain.RoleRegistrar},
		Status: []domain.Status{{Value: "active"}},
		VCard: &domain.VCard{
			Version:      "4.0",
			FullName:     "Example Registrar Inc.",
			Kind:         "org",
			Organization: "Example Registrar Inc.",
			Address: &domain.VCardAddress{
				CountryCode: "US",
				Street:      "123 Maple Ave",
				Locality:    "Los Angeles",
				Region:      "CA",
				PostalCode:  "90210",
			},
		},
		PublicIDs:        []domain.PublicID{{Type: "IANA Registrar ID", Identifier: "2"}},
		RegistrarID:      "2",
		RegistrarBaseURL: "https://rdap.example.org/rdap/",
		Entities: []*domain.Contact{
			{
				Handle: "ABUSE-NAME",
				Roles:  []domain.ContactRole{domain.RoleAbuse},
				Status: []domain.Status{{Value: "active"}},
				VCard: &domain.VCard{
					Version:  "4.0",
					FullName: "Abuse Contact",
					VoiceTel: "tel:+1-555-123-4567",
					Email:    "abuse@example.com",
				},
			},
		},
		Metadata: domain.Metadata{
			Version:   1,
			CreatedAt: now.Add(-365 * 24 * time.Hour),
			UpdatedAt: now,
			Source:    "seed",
		},
	}

	s.contacts["888"] = &domain.Contact{
		Handle: "888",
		Roles:  []domain.ContactRole{domain.RoleTechnical},
		Status: []domain.Status{{Value: "active"}},
		VCard: &domain.VCard{
			Version:  "4.0",
			FullName: "Example Technical Contact",
			Email:    "tech@example.com",
		},
		Metadata: domain.Metadata{
			Version:   1,
			CreatedAt: now.Add(-365 * 24 * time.Hour),
			UpdatedAt: now,
			Source:    "seed",
		},
	}

	s.contacts["REG1-NAME"] = &domain.Contact{
		Handle: "REG1-NAME",
		Roles:  []domain.ContactRole{domain.RoleRegistrant},
		Status: []domain.Status{{Value: "active"}},
		VCard: &domain.VCard{
			Version:      "4.0",
			FullName:     "Example Registrant",
			Kind:         "individual",
			Organization: "Example Organization",
			Address: &domain.VCardAddress{
				CountryCode: "US",
				Street:      "123 Elm Street",
				Locality:    "Springfield",
				Region:      "IL",
				PostalCode:  "62701",
			},
			VoiceTel: "tel:+1-217-555-0132",
			Email:    "registrant@example.com",
		},
		Metadata: domain.Metadata{
			Version:   1,
			CreatedAt: now.Add(-365 * 24 * time.Hour),
			UpdatedAt: now,
			Source:    "seed",
		},
	}

	// Seed nameservers (EPP ROID format: <local-id>-<registered repository ID>)
	s.nameservers["NS1-NAME"] = &domain.NameServer{
		Handle:      "NS1-NAME",
		LDHName:     "ns1.example.com",
		UnicodeName: "ns1.example.com",
		IPV4:        []string{"8.8.8.8"},
		IPV6:        []string{"2001:4860:4860::8888"},
		Status:      []domain.Status{{Value: "associated"}},
		Metadata: domain.Metadata{
			Version:   1,
			CreatedAt: now.Add(-365 * 24 * time.Hour),
			UpdatedAt: now,
			Source:    "seed",
		},
	}

	s.nameservers["NS2-NAME"] = &domain.NameServer{
		Handle:      "NS2-NAME",
		LDHName:     "ns2.example.com",
		UnicodeName: "ns2.example.com",
		IPV4:        []string{"1.1.1.1"},
		IPV6:        []string{"2606:4700:4700::1111"},
		Status:      []domain.Status{{Value: "associated"}},
		Metadata: domain.Metadata{
			Version:   1,
			CreatedAt: now.Add(-365 * 24 * time.Hour),
			UpdatedAt: now,
			Source:    "seed",
		},
	}

	// Seed example domains
	exampleDomain := &domain.Domain{
		Handle:      "EX1-NAME",
		LDHName:     "example.com",
		UnicodeName: "example.com",
		TLD:         "com",
		Status:      []domain.Status{{Value: "active"}},
		ExpiresAt:   now.Add(365 * 24 * time.Hour),
		Contacts: map[domain.ContactRole][]string{
			domain.RoleRegistrant: {"REG1-NAME"},
			domain.RoleTechnical:  {"888"},
			domain.RoleRegistrar:  {"2"},
		},
		Nameservers: []domain.NameServer{
			*s.nameservers["NS1-NAME"],
			*s.nameservers["NS2-NAME"],
		},
		Registrar: "2",
		SecureDNS:   &domain.SecureDNS{ZoneSigned: false, DelegationSigned: false},
		Metadata: domain.Metadata{
			Version:   1,
			CreatedAt: now.Add(-365 * 24 * time.Hour),
			UpdatedAt: now,
			Source:    "seed",
		},
	}
	s.domains["example.com"] = exampleDomain
	s.domainByNS["ns1.example.com"] = append(s.domainByNS["ns1.example.com"], "example.com")
	s.domainByNS["ns2.example.com"] = append(s.domainByNS["ns2.example.com"], "example.com")
	s.domainByEntity["REG1-NAME"] = append(s.domainByEntity["REG1-NAME"], "example.com")

	// Seed IP network (8.8.8.0/24)
	s.ipNetworks["8.8.8.0/24"] = &domain.IPNetwork{
		Handle:       "NET-8-8-8-0-24",
		StartAddress: "8.8.8.0",
		EndAddress:   "8.8.8.255",
		IPVersion:    "v4",
		CIDR:         []string{"8.8.8.0/24"},
		Name:         "GOOGLE",
		Type:         "ALLOCATED",
		Country:      "US",
		Status:       []domain.Status{{Value: "active"}},
		Metadata: domain.Metadata{
			Version:   1,
			CreatedAt: now.Add(-365 * 24 * time.Hour),
			UpdatedAt: now,
			Source:    "seed",
		},
	}

	// Seed autnum 15169
	s.autnums[15169] = &domain.Autnum{
		Handle:   "AS15169",
		StartASN: 15169,
		EndASN:   15169,
		Name:     "GOOGLE",
		Type:     "DIRECT ALLOCATION",
		Country:  "US",
		Status:   []domain.Status{{Value: "active"}},
		Metadata: domain.Metadata{
			Version:   1,
			CreatedAt: now.Add(-365 * 24 * time.Hour),
			UpdatedAt: now,
			Source:    "seed",
		},
	}
}

func (s *MemoryStore) LookupDomain(name string) (*domain.Domain, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	name = strings.ToLower(strings.TrimSuffix(name, "."))
	record, ok := s.domains[name]
	if !ok {
		return nil, fmt.Errorf("domain not found: %s", name)
	}
	return record, nil
}

// GetDomainAggregate resolves a domain plus its registrar, contacts, and
// nameservers while holding a single read lock, so the returned aggregate is a
// consistent snapshot. In the memory store all objects live in maps, so this is
// purely a lock-scoping guarantee against any future concurrent writes.
func (s *MemoryStore) GetDomainAggregate(name string) (*domain.DomainAggregate, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	name = strings.ToLower(strings.TrimSuffix(name, "."))
	d, ok := s.domains[name]
	if !ok {
		return nil, fmt.Errorf("domain not found: %s", name)
	}

	agg := &domain.DomainAggregate{
		Domain:      d,
		Contacts:    map[string]*domain.Contact{},
		Nameservers: map[string]*domain.NameServer{},
	}

	if reg, ok := s.contacts[d.Registrar]; ok {
		agg.Registrar = reg
		agg.Contacts[d.Registrar] = reg
	}

	for role, handles := range d.Contacts {
		_ = role
		for _, h := range handles {
			if c, ok := s.contacts[h]; ok {
				agg.Contacts[h] = c
			}
		}
	}

	for _, ns := range d.Nameservers {
		if n, ok := s.nameservers[ns.Handle]; ok {
			agg.Nameservers[ns.Handle] = n
		}
	}

	return agg, nil
}

func (s *MemoryStore) LookupContact(handle string) (*domain.Contact, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	record, ok := s.contacts[handle]
	if !ok {
		return nil, fmt.Errorf("entity not found: %s", handle)
	}
	return record, nil
}

func (s *MemoryStore) LookupNameserver(name string) (*domain.NameServer, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	name = strings.ToLower(strings.TrimSuffix(name, "."))
	for _, record := range s.nameservers {
		if strings.EqualFold(record.LDHName, name) || strings.EqualFold(record.UnicodeName, name) {
			return record, nil
		}
	}
	return nil, fmt.Errorf("nameserver not found: %s", name)
}

func (s *MemoryStore) LookupIPNetwork(cidr string) (*domain.IPNetwork, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	record, ok := s.ipNetworks[cidr]
	if !ok {
		return nil, fmt.Errorf("IP network not found: %s", cidr)
	}
	return record, nil
}

func (s *MemoryStore) LookupAutnum(asn int) (*domain.Autnum, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	record, ok := s.autnums[asn]
	if !ok {
		return nil, fmt.Errorf("autnum not found: %d", asn)
	}
	return record, nil
}

func (s *MemoryStore) SearchDomainsByName(pattern string, limit int) ([]domain.Domain, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	pattern = strings.ToLower(pattern)
	pattern = strings.ReplaceAll(pattern, "*", ".*")
	pattern = strings.ReplaceAll(pattern, "?", ".")
	re, err := compileGlob(pattern)
	if err != nil {
		return nil, err
	}

	var results []domain.Domain
	for _, d := range s.domains {
		if re.MatchString(d.LDHName) {
			results = append(results, *d)
			if limit > 0 && len(results) >= limit {
				break
			}
		}
	}
	return results, nil
}

func (s *MemoryStore) SearchDomainsByNS(nsName string, limit int) ([]domain.Domain, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	nsName = strings.ToLower(nsName)
	domainNames := s.domainByNS[nsName]
	if domainNames == nil {
		return nil, nil
	}

	var results []domain.Domain
	for _, dn := range domainNames {
		if d, ok := s.domains[dn]; ok {
			results = append(results, *d)
			if limit > 0 && len(results) >= limit {
				break
			}
		}
	}
	return results, nil
}

func (s *MemoryStore) SearchContactsByName(pattern string, limit int) ([]domain.Contact, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	pattern = strings.ToLower(pattern)
	re, err := compileGlob(pattern)
	if err != nil {
		return nil, err
	}

	var results []domain.Contact
	for _, e := range s.contacts {
		if re.MatchString(strings.ToLower(e.Handle)) {
			results = append(results, *e)
			if limit > 0 && len(results) >= limit {
				break
			}
		}
	}
	return results, nil
}

func (s *MemoryStore) SearchContactsByHandle(pattern string, limit int) ([]domain.Contact, error) {
	return s.SearchContactsByName(pattern, limit)
}

func (s *MemoryStore) SearchNameserversByName(pattern string, limit int) ([]domain.NameServer, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	pattern = strings.ToLower(pattern)
	re, err := compileGlob(pattern)
	if err != nil {
		return nil, err
	}

	var results []domain.NameServer
	for _, n := range s.nameservers {
		if re.MatchString(n.LDHName) {
			results = append(results, *n)
			if limit > 0 && len(results) >= limit {
				break
			}
		}
	}
	return results, nil
}

func (s *MemoryStore) SearchNameserversByIP(ip string, limit int) ([]domain.NameServer, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var results []domain.NameServer
	for _, n := range s.nameservers {
		for _, v4 := range n.IPV4 {
			if v4 == ip {
				results = append(results, *n)
				break
			}
		}
		if limit > 0 && len(results) >= limit {
			break
		}
		for _, v6 := range n.IPV6 {
			if v6 == ip {
				results = append(results, *n)
				break
			}
		}
		if limit > 0 && len(results) >= limit {
			break
		}
	}
	return results, nil
}

func (s *MemoryStore) Ping() error {
	return nil
}

func (s *MemoryStore) Close() error {
	return nil
}

func compileGlob(pattern string) (*regexp.Regexp, error) {
	reStr := "^" + strings.ReplaceAll(strings.ReplaceAll(pattern, ".", "\\."), "*", ".*") + "$"
	re, err := regexp.Compile(reStr)
	if err != nil {
		return nil, fmt.Errorf("invalid pattern: %w", err)
	}
	return re, nil
}
