package store

import (
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/rdap-server/rdap/internal/config"
	"github.com/rdap-server/rdap/internal/rdap"
)

type MemoryStore struct {
	mu             sync.RWMutex
	domains        map[string]*rdap.DomainRecord
	entities       map[string]*rdap.EntityRecord
	nameservers    map[string]*rdap.NameserverRecord
	ipNetworks     map[string]*rdap.IPNetworkRecord
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
		domains:        make(map[string]*rdap.DomainRecord),
		entities:       make(map[string]*rdap.EntityRecord),
		nameservers:    make(map[string]*rdap.NameserverRecord),
		ipNetworks:     make(map[string]*rdap.IPNetworkRecord),
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
	s.entities["2"] = &rdap.EntityRecord{
		Handle:    "2",
		Roles:     []string{"registrar"},
		Status:    []string{"active"},
		CreatedAt: now.Add(-365 * 24 * time.Hour),
		UpdatedAt: now,
	}

	s.entities["888"] = &rdap.EntityRecord{
		Handle:    "888",
		Roles:     []string{"technical"},
		Status:    []string{"active"},
		CreatedAt: now.Add(-365 * 24 * time.Hour),
		UpdatedAt: now,
	}

	// Seed nameservers (EPP ROID format: <local-id>-<registered repository ID>)
	s.nameservers["NS1-NAME"] = &rdap.NameserverRecord{
		Handle:      "NS1-NAME",
		LDHName:     "ns1.example.com",
		UnicodeName: "ns1.example.com",
		IPV4:        []string{"8.8.8.8"},
		IPV6:        []string{"2001:4860:4860::8888"},
		Status:      []string{"associated"},
		CreatedAt:   now.Add(-365 * 24 * time.Hour),
		UpdatedAt:   now,
	}

	s.nameservers["NS2-NAME"] = &rdap.NameserverRecord{
		Handle:      "NS2-NAME",
		LDHName:     "ns2.example.com",
		UnicodeName: "ns2.example.com",
		IPV4:        []string{"1.1.1.1"},
		IPV6:        []string{"2606:4700:4700::1111"},
		Status:      []string{"associated"},
		CreatedAt:   now.Add(-365 * 24 * time.Hour),
		UpdatedAt:   now,
	}

	// Seed example domains
	exampleDomain := &rdap.DomainRecord{
		Handle:      "EX1-NAME",
		LDHName:     "example.com",
		UnicodeName: "example.com",
		TLD:         "com",
		Status:      []string{"active"},
		CreatedAt:   now.Add(-365 * 24 * time.Hour),
		UpdatedAt:   now,
		ExpiresAt:   now.Add(365 * 24 * time.Hour),
		Registrant:  "2",
		Admin:       "888",
		Tech:        "888",
		Nameservers: []rdap.NameserverRecord{
			*s.nameservers["NS1-NAME"],
			*s.nameservers["NS2-NAME"],
		},
	}
	s.domains["example.com"] = exampleDomain
	s.domainByNS["ns1.example.com"] = append(s.domainByNS["ns1.example.com"], "example.com")
	s.domainByNS["ns2.example.com"] = append(s.domainByNS["ns2.example.com"], "example.com")
}

func (s *MemoryStore) LookupDomain(name string) (*rdap.DomainRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	name = strings.ToLower(strings.TrimSuffix(name, "."))
	record, ok := s.domains[name]
	if !ok {
		return nil, fmt.Errorf("domain not found: %s", name)
	}
	return record, nil
}

func (s *MemoryStore) LookupEntity(handle string) (*rdap.EntityRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	record, ok := s.entities[handle]
	if !ok {
		return nil, fmt.Errorf("entity not found: %s", handle)
	}
	return record, nil
}

func (s *MemoryStore) LookupNameserver(name string) (*rdap.NameserverRecord, error) {
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

func (s *MemoryStore) LookupIPNetwork(cidr string) (*rdap.IPNetworkRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	record, ok := s.ipNetworks[cidr]
	if !ok {
		return nil, fmt.Errorf("IP network not found: %s", cidr)
	}
	return record, nil
}

func (s *MemoryStore) SearchDomainsByName(pattern string, limit int) ([]rdap.DomainRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	pattern = strings.ToLower(pattern)
	pattern = strings.ReplaceAll(pattern, "*", ".*")
	pattern = strings.ReplaceAll(pattern, "?", ".")
	re, err := compileGlob(pattern)
	if err != nil {
		return nil, err
	}

	var results []rdap.DomainRecord
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

func (s *MemoryStore) SearchDomainsByNS(nsName string, limit int) ([]rdap.DomainRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	nsName = strings.ToLower(nsName)
	domainNames := s.domainByNS[nsName]
	if domainNames == nil {
		return nil, nil
	}

	var results []rdap.DomainRecord
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

func (s *MemoryStore) SearchEntitiesByName(pattern string, limit int) ([]rdap.EntityRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	pattern = strings.ToLower(pattern)
	re, err := compileGlob(pattern)
	if err != nil {
		return nil, err
	}

	var results []rdap.EntityRecord
	for _, e := range s.entities {
		if re.MatchString(strings.ToLower(e.Handle)) {
			results = append(results, *e)
			if limit > 0 && len(results) >= limit {
				break
			}
		}
	}
	return results, nil
}

func (s *MemoryStore) SearchEntitiesByHandle(pattern string, limit int) ([]rdap.EntityRecord, error) {
	return s.SearchEntitiesByName(pattern, limit)
}

func (s *MemoryStore) SearchNameserversByName(pattern string, limit int) ([]rdap.NameserverRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	pattern = strings.ToLower(pattern)
	re, err := compileGlob(pattern)
	if err != nil {
		return nil, err
	}

	var results []rdap.NameserverRecord
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

func (s *MemoryStore) SearchNameserversByIP(ip string, limit int) ([]rdap.NameserverRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var results []rdap.NameserverRecord
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
