// Package service implements the application/query layer between the canonical
// registry data model (internal/domain) and the RDAP wire representation
// (internal/rdap). Handlers depend on this service rather than on storage
// directly, so a real registry can plug its own model into the service boundary.
//
// The mapping functions in this package deliberately reproduce the exact RDAP
// output shape (jCard, entities, links, events, notices, conformance) that is
// validated against the ICANN RDAP Conformance Tool. Changing RDAP behavior here
// is the single place that affects wire output.
package service

import (
	"fmt"
	"strings"

	"github.com/tespio/go-rdap-server/internal/config"
	"github.com/tespio/go-rdap-server/internal/domain"
	"github.com/tespio/go-rdap-server/internal/rdap"
	"github.com/tespio/go-rdap-server/internal/store"
)

// Service is the query service that resolves domain objects and maps them to
// RDAP responses.
type Service struct {
	store store.Interface
	cfg   config.RDAPConfig
}

// New builds a query service backed by the given store.
func New(st store.Interface, cfg config.RDAPConfig) *Service {
	return &Service{store: st, cfg: cfg}
}

// DomainToRDAP maps a canonical domain aggregate to the RDAP domain object.
// The output must remain byte-compatible with what passes the ICANN
// conformance tool (2024 profile).
func (s *Service) DomainToRDAP(d *domain.Domain, requestURL string) rdap.Domain {
	return domainToRDAP(d, nil, s.cfg.BaseURL, requestURL, s.cfg.RegistrarBaseURL, s.cfg.Mode, s.NoticeOptions())
}

// DomainAggregateToRDAP maps a consistent domain aggregate (domain + resolved
// registrar/contacts/nameservers) to the RDAP domain object. Rendering from an
// aggregate guarantees the embedded registrar/contact data and the domain's
// status/events all reflect the same snapshot.
func (s *Service) DomainAggregateToRDAP(agg *domain.DomainAggregate, requestURL string) rdap.Domain {
	return domainToRDAP(agg.Domain, agg, s.cfg.BaseURL, requestURL, s.cfg.RegistrarBaseURL, s.cfg.Mode, s.NoticeOptions())
}

// EntityToRDAP maps a canonical contact to the RDAP entity object.
func (s *Service) EntityToRDAP(c *domain.Contact, requestURL string) rdap.Entity {
	return entityToRDAP(c, s.cfg.BaseURL)
}

// NameserverToRDAP maps a canonical nameserver to the RDAP nameserver object.
func (s *Service) NameserverToRDAP(ns *domain.NameServer, requestURL string) rdap.Nameserver {
	return nameserverToRDAP(ns, s.cfg.BaseURL)
}

// IPNetworkToRDAP maps a canonical IP network to the RDAP ip network object.
func (s *Service) IPNetworkToRDAP(n *domain.IPNetwork, requestURL string) rdap.IPNetwork {
	return ipNetworkToRDAP(n, s.cfg.BaseURL)
}

// LookupDomain returns the RDAP domain object for a domain name, read from a
// single consistent snapshot so the response cannot observe a partially-applied
// update across the domain and its embedded registrar/contacts/nameservers.
func (s *Service) LookupDomain(name string, requestURL string) (rdap.Domain, error) {
	agg, err := s.store.GetDomainAggregate(name)
	if err != nil {
		return rdap.Domain{}, err
	}
	return s.DomainAggregateToRDAP(agg, requestURL), nil
}

// LookupEntity returns the RDAP entity object for a contact handle.
func (s *Service) LookupEntity(handle string, requestURL string) (rdap.Entity, error) {
	c, err := s.store.LookupContact(handle)
	if err != nil {
		return rdap.Entity{}, err
	}
	return s.EntityToRDAP(c, requestURL), nil
}

// LookupNameserver returns the RDAP nameserver object for a nameserver name.
func (s *Service) LookupNameserver(name string, requestURL string) (rdap.Nameserver, error) {
	ns, err := s.store.LookupNameserver(strings.ToLower(name))
	if err != nil {
		return rdap.Nameserver{}, err
	}
	return s.NameserverToRDAP(ns, requestURL), nil
}

// LookupIPNetwork returns the RDAP IP network object for a CIDR.
func (s *Service) LookupIPNetwork(cidr string, requestURL string) (rdap.IPNetwork, error) {
	n, err := s.store.LookupIPNetwork(cidr)
	if err != nil {
		return rdap.IPNetwork{}, err
	}
	return s.IPNetworkToRDAP(n, requestURL), nil
}

// SearchDomainsByName searches domains by name pattern.
func (s *Service) SearchDomainsByName(pattern string, limit int, requestURL string) ([]rdap.Domain, error) {
	domains, err := s.store.SearchDomainsByName(pattern, limit)
	if err != nil {
		return nil, err
	}
	out := make([]rdap.Domain, 0, len(domains))
	for i := range domains {
		out = append(out, s.DomainToRDAP(&domains[i], requestURL))
	}
	return out, nil
}

// SearchDomainsByNS searches domains by nameserver.
func (s *Service) SearchDomainsByNS(nsName string, limit int, requestURL string) ([]rdap.Domain, error) {
	domains, err := s.store.SearchDomainsByNS(nsName, limit)
	if err != nil {
		return nil, err
	}
	out := make([]rdap.Domain, 0, len(domains))
	for i := range domains {
		out = append(out, s.DomainToRDAP(&domains[i], requestURL))
	}
	return out, nil
}

// SearchEntitiesByName searches contacts by name pattern.
func (s *Service) SearchEntitiesByName(pattern string, limit int, requestURL string) ([]rdap.Entity, error) {
	contacts, err := s.store.SearchContactsByName(pattern, limit)
	if err != nil {
		return nil, err
	}
	out := make([]rdap.Entity, 0, len(contacts))
	for i := range contacts {
		out = append(out, s.EntityToRDAP(&contacts[i], requestURL))
	}
	return out, nil
}

// SearchEntitiesByHandle searches contacts by handle pattern.
func (s *Service) SearchEntitiesByHandle(pattern string, limit int, requestURL string) ([]rdap.Entity, error) {
	contacts, err := s.store.SearchContactsByHandle(pattern, limit)
	if err != nil {
		return nil, err
	}
	out := make([]rdap.Entity, 0, len(contacts))
	for i := range contacts {
		out = append(out, s.EntityToRDAP(&contacts[i], requestURL))
	}
	return out, nil
}

// SearchNameserversByName searches nameservers by name pattern.
func (s *Service) SearchNameserversByName(pattern string, limit int, requestURL string) ([]rdap.Nameserver, error) {
	nameservers, err := s.store.SearchNameserversByName(pattern, limit)
	if err != nil {
		return nil, err
	}
	out := make([]rdap.Nameserver, 0, len(nameservers))
	for i := range nameservers {
		out = append(out, s.NameserverToRDAP(&nameservers[i], requestURL))
	}
	return out, nil
}

// SearchNameserversByIP searches nameservers by IP address.
func (s *Service) SearchNameserversByIP(ip string, limit int, requestURL string) ([]rdap.Nameserver, error) {
	nameservers, err := s.store.SearchNameserversByIP(ip, limit)
	if err != nil {
		return nil, err
	}
	out := make([]rdap.Nameserver, 0, len(nameservers))
	for i := range nameservers {
		out = append(out, s.NameserverToRDAP(&nameservers[i], requestURL))
	}
	return out, nil
}

// BaseURL returns the configured RDAP base URL.
func (s *Service) BaseURL() string {
	return s.cfg.BaseURL
}

// NoticeOptions returns the RDAP notice options derived from the RDAP config.
// It converts the registrar/registry customization (ToS text, custom notices)
// into the rdap package's notice options.
func (s *Service) NoticeOptions() *rdap.NoticeOptions {
	return NoticeOptionsFromConfig(s.cfg)
}

// NoticeOptionsFromConfig converts RDAP config customization into rdap.NoticeOptions.
func NoticeOptionsFromConfig(cfg config.RDAPConfig) *rdap.NoticeOptions {
	opts := &rdap.NoticeOptions{}
	if cfg.ToS != nil {
		opts.ToSTitle = cfg.ToS.Title
		opts.ToSDescription = cfg.ToS.Description
		opts.ToSURL = cfg.ToS.URL
	}
	for _, c := range cfg.CustomNotices {
		opts.Custom = append(opts.Custom, rdap.CustomNotice{
			Title:       c.Title,
			Description: c.Description,
			URL:         c.URL,
			Rel:         c.Rel,
		})
	}
	return opts
}

func statusValues(status []domain.Status) []string {
	out := make([]string, 0, len(status))
	for _, st := range status {
		out = append(out, st.Value)
	}
	return out
}

// domainToRDAP reproduces the exact RDAP domain object previously produced by
// the handlers. It mirrors the ICANN-validated serializer.
func domainToRDAP(d *domain.Domain, agg *domain.DomainAggregate, baseURL, requestURL, registrarBaseURL, mode string, opts *rdap.NoticeOptions) rdap.Domain {
	nameservers := make([]rdap.Nameserver, len(d.Nameservers))
	for i, ns := range d.Nameservers {
		nameservers[i] = rdap.Nameserver{
			Common: rdap.Common{
				ObjectClassName: "nameserver",
				Handle:          ns.Handle,
				Status:          statusValues(ns.Status),
				Links: []rdap.Link{{
					Value: fmt.Sprintf("%s/nameserver/%s", baseURL, ns.LDHName),
					Rel:   "self",
					Href:  fmt.Sprintf("%s/nameserver/%s", baseURL, ns.LDHName),
					Type:  "application/rdap+json",
				}},
			},
			LDHName:     ns.LDHName,
			UnicodeName: ns.UnicodeName,
			IPAddresses: &rdap.IPAddrSet{
				V4: ns.IPV4,
				V6: ns.IPV6,
			},
		}
	}

	secureDNS := &rdap.SecureDNS{
		ZoneSigned:       false,
		DelegationSigned: false,
	}
	if d.SecureDNS != nil {
		secureDNS = &rdap.SecureDNS{
			ZoneSigned:       d.SecureDNS.ZoneSigned,
			DelegationSigned: d.SecureDNS.DelegationSigned,
			MaxSigLife:       d.SecureDNS.MaxSigLife,
		}
	}

	events := []rdap.Event{
		{EventAction: "registration", EventDate: rdap.FormatTime(d.Metadata.CreatedAt)},
		{EventAction: "last changed", EventDate: rdap.FormatTime(d.Metadata.UpdatedAt)},
		{EventAction: "expiration", EventDate: rdap.FormatTime(d.ExpiresAt)},
		{EventAction: "last update of RDAP database", EventDate: rdap.FormatTime(d.Metadata.UpdatedAt)},
	}
	if mode == "registrar" {
		// Required by the 2024 gTLD Response Profile for registrar servers (-65600).
		events = append(events, rdap.Event{EventAction: "registrar expiration", EventDate: rdap.FormatTime(d.ExpiresAt)})
	}

	// The registrar entity is built from the resolved registrar contact in the
	// aggregate (so a real registrar's own vcard is used), falling back to a
	// static example when none is available (e.g. plain domain searches). Both
	// paths emit the same about link to the IANA-registered registrar base URL.
	registrarEntity := rdap.Entity{
		Common: rdap.Common{
			ObjectClassName: "entity",
			Handle:          d.Registrar,
			Status:          []string{"active"},
			Links: []rdap.Link{{
				Value: registrarBaseURL,
				Rel:   "about",
				Href:  registrarBaseURL,
				Type:  "application/rdap+json",
			}},
		},
		Roles: []string{"registrar"},
		PublicIDs: []rdap.PublicID{
			{Type: "IANA Registrar ID", Identifier: d.Registrar},
		},
	}

	if agg != nil && agg.Registrar != nil && agg.Registrar.VCard != nil {
		registrarEntity.VCardArray = vcardToJCard(agg.Registrar.VCard)
	} else {
		registrarEntity.VCardArray = []interface{}{
			"vcard",
			[]interface{}{
				[]interface{}{"version", map[string]interface{}{}, "text", "4.0"},
				[]interface{}{"fn", map[string]interface{}{}, "text", "Example Registrar Inc."},
				[]interface{}{"adr", map[string]interface{}{"cc": "US"}, "text", []interface{}{"", "", "123 Maple Ave", "Los Angeles", "CA", "90210", ""}},
			},
		}
	}

	// Abuse entity inside registrar entity with tel and email.
	abuseEntity := rdap.Entity{
		Common: rdap.Common{
			ObjectClassName: "entity",
			Handle:          "ABUSE-NAME",
			Status:          []string{"active"},
		},
		Roles: []string{"abuse"},
		VCardArray: []interface{}{
			"vcard",
			[]interface{}{
				[]interface{}{"version", map[string]interface{}{}, "text", "4.0"},
				[]interface{}{"fn", map[string]interface{}{}, "text", "Abuse Contact"},
				[]interface{}{"tel", map[string]interface{}{"type": []interface{}{"voice"}}, "uri", "tel:+1-555-123-4567"},
				[]interface{}{"email", map[string]interface{}{}, "text", "abuse@example.com"},
			},
		},
	}
	registrarEntity.Common.Entities = append(registrarEntity.Common.Entities, abuseEntity)

	// Registrant entity required by the 2024 gTLD Response Profile (section 2.7.2).
	// Built from the resolved registrant contact when available; falls back to a
	// static example for plain domain searches.
	registrantEntity := rdap.Entity{
		Common: rdap.Common{
			ObjectClassName: "entity",
			Handle:          "REG1-NAME",
			Status:          []string{"active"},
		},
		Roles: []string{"registrant"},
		VCardArray: []interface{}{
			"vcard",
			[]interface{}{
				[]interface{}{"version", map[string]interface{}{}, "text", "4.0"},
				[]interface{}{"fn", map[string]interface{}{}, "text", "Example Registrant"},
				[]interface{}{"org", map[string]interface{}{}, "text", "Example Organization"},
				[]interface{}{"adr", map[string]interface{}{"cc": "US"}, "text", []interface{}{"", "", "123 Elm Street", "Springfield", "IL", "62701", ""}},
				[]interface{}{"tel", map[string]interface{}{"type": []interface{}{"voice"}}, "uri", "tel:+1-217-555-0132"},
				[]interface{}{"email", map[string]interface{}{}, "text", "registrant@example.com"},
			},
		},
	}

	if agg != nil {
		regHandles := d.Contacts[domain.RoleRegistrant]
		if len(regHandles) > 0 {
			registrantEntity.Common.Handle = regHandles[0]
			if c, ok := agg.Contacts[regHandles[0]]; ok && c.VCard != nil {
				registrantEntity.VCardArray = vcardToJCard(c.VCard)
			}
		}
	}

	domainEntities := []rdap.Entity{registrarEntity}
	if mode == "registrar" {
		// Registrant data is served by registrars; required by the 2024 gTLD
		// Response Profile section 2.7.2 for registrar servers (-63000).
		domainEntities = append(domainEntities, registrantEntity)
	}

	out := rdap.Domain{
		Common: rdap.Common{
			ObjectClassName: "domain",
			Handle:          d.Handle,
			Status:          statusValues(d.Status),
			Events:          events,
			Entities:        domainEntities,
			Links: []rdap.Link{{
				Value: requestURL,
				Rel:   "self",
				Href:  fmt.Sprintf("%s/domain/%s", baseURL, d.LDHName),
				Type:  "application/rdap+json",
			}, {
				Value: requestURL,
				Rel:   "related",
				Href:  fmt.Sprintf("%s/domain/%s", strings.TrimRight(registrarBaseURL, "/"), d.LDHName),
				Type:  "application/rdap+json",
			}},
			Port43: "",
		},
		LDHName:     d.LDHName,
		UnicodeName: d.UnicodeName,
		Nameservers: nameservers,
		SecureDNS:   secureDNS,
	}
	out.Conformance = rdap.NewConformance2024()
	out.Notices = rdap.NewNoticesWithICANN(requestURL, baseURL, opts)

	return out
}

func entityToRDAP(c *domain.Contact, baseURL string) rdap.Entity {
	return rdap.Entity{
		Common: rdap.Common{
			ObjectClassName: "entity",
			Handle:          c.Handle,
			Status:          statusValues(c.Status),
			Events: []rdap.Event{
				{EventAction: "registration", EventDate: rdap.FormatTime(c.Metadata.CreatedAt)},
				{EventAction: "last changed", EventDate: rdap.FormatTime(c.Metadata.UpdatedAt)},
			},
			Links: []rdap.Link{{
				Value: fmt.Sprintf("%s/entity/%s", baseURL, c.Handle),
				Rel:   "self",
				Href:  fmt.Sprintf("%s/entity/%s", baseURL, c.Handle),
				Type:  "application/rdap+json",
			}},
		},
		Roles:     roleStrings(c.Roles),
		PublicIDs: publicIDsToRDAP(c.PublicIDs),
	}
}

func publicIDsToRDAP(ids []domain.PublicID) []rdap.PublicID {
	out := make([]rdap.PublicID, 0, len(ids))
	for _, id := range ids {
		out = append(out, rdap.PublicID{Type: id.Type, Identifier: id.Identifier})
	}
	return out
}

func nameserverToRDAP(ns *domain.NameServer, baseURL string) rdap.Nameserver {
	return rdap.Nameserver{
		Common: rdap.Common{
			ObjectClassName: "nameserver",
			Handle:          ns.Handle,
			Status:          statusValues(ns.Status),
			Events: []rdap.Event{
				{EventAction: "registration", EventDate: rdap.FormatTime(ns.Metadata.CreatedAt)},
				{EventAction: "last changed", EventDate: rdap.FormatTime(ns.Metadata.UpdatedAt)},
			},
			Links: []rdap.Link{{
				Value: fmt.Sprintf("%s/nameserver/%s", baseURL, ns.LDHName),
				Rel:   "self",
				Href:  fmt.Sprintf("%s/nameserver/%s", baseURL, ns.LDHName),
				Type:  "application/rdap+json",
			}},
		},
		LDHName:     ns.LDHName,
		UnicodeName: ns.UnicodeName,
		IPAddresses: &rdap.IPAddrSet{
			V4: ns.IPV4,
			V6: ns.IPV6,
		},
	}
}

func ipNetworkToRDAP(n *domain.IPNetwork, baseURL string) rdap.IPNetwork {
	cidr := ""
	if len(n.CIDR) > 0 {
		cidr = n.CIDR[0]
	}
	return rdap.IPNetwork{
		Common: rdap.Common{
			ObjectClassName: "ip network",
			Handle:          n.Handle,
			Status:          statusValues(n.Status),
			Events: []rdap.Event{
				{EventAction: "registration", EventDate: rdap.FormatTime(n.Metadata.CreatedAt)},
				{EventAction: "last changed", EventDate: rdap.FormatTime(n.Metadata.UpdatedAt)},
			},
			Links: []rdap.Link{{
				Value: fmt.Sprintf("%s/ip/%s", baseURL, cidr),
				Rel:   "self",
				Href:  fmt.Sprintf("%s/ip/%s", baseURL, cidr),
				Type:  "application/rdap+json",
			}},
		},
		StartAddress: n.StartAddress,
		EndAddress:   n.EndAddress,
		IPVersion:    n.IPVersion,
		CIDR:         n.CIDR,
		Name:         n.Name,
		Type:         n.Type,
		Country:      n.Country,
		ParentHandle: "",
	}
}

func roleStrings(roles []domain.ContactRole) []string {
	out := make([]string, 0, len(roles))
	for _, r := range roles {
		out = append(out, string(r))
	}
	return out
}

// vcardToJCard converts a structured domain.VCard into the jCard array format
// emitted over RDAP. The shape matches the ICANN-validated output:
// ["vcard", [ [name, params, type, value], ... ]]. Addresses use the 7-element
// adr form and telephone types are emitted as ["voice"] / ["fax"].
func vcardToJCard(v *domain.VCard) []interface{} {
	props := []interface{}{
		[]interface{}{"version", map[string]interface{}{}, "text", "4.0"},
	}
	if v.FullName != "" {
		props = append(props, []interface{}{"fn", map[string]interface{}{}, "text", v.FullName})
	}
	if v.Kind != "" {
		props = append(props, []interface{}{"kind", map[string]interface{}{}, "text", v.Kind})
	}
	if v.Organization != "" {
		props = append(props, []interface{}{"org", map[string]interface{}{}, "text", v.Organization})
	}
	if v.Address != nil {
		cc := v.Address.CountryCode
		params := map[string]interface{}{}
		if cc != "" {
			params["cc"] = cc
		}
		props = append(props, []interface{}{"adr", params, "text", []interface{}{
			v.Address.POBox, v.Address.Extended, v.Address.Street,
			v.Address.Locality, v.Address.Region, v.Address.PostalCode, v.Address.CountryName,
		}})
	}
	if v.VoiceTel != "" {
		props = append(props, []interface{}{"tel", map[string]interface{}{"type": []interface{}{"voice"}}, "uri", v.VoiceTel})
	}
	if v.FaxTel != "" {
		props = append(props, []interface{}{"tel", map[string]interface{}{"type": []interface{}{"fax"}}, "uri", v.FaxTel})
	}
	if v.Email != "" {
		props = append(props, []interface{}{"email", map[string]interface{}{}, "text", v.Email})
	}
	if v.ContactURI != "" {
		props = append(props, []interface{}{"contact-uri", map[string]interface{}{}, "uri", v.ContactURI})
	}
	return []interface{}{"vcard", props}
}
