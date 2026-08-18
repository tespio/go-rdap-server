package handlers

import (
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/rdap-server/rdap/internal/config"
	"github.com/rdap-server/rdap/internal/rdap"
	"github.com/rdap-server/rdap/internal/store"
)

type Handler struct {
	store store.Interface
	cfg   config.RDAPConfig
	port  int
}

func New(s store.Interface, cfg config.RDAPConfig, serverPort int) *Handler {
	return &Handler{store: s, cfg: cfg, port: serverPort}
}

func (h *Handler) RegisterRoutes(r chi.Router) {
	r.Get("/help", h.Help)
	r.Head("/help", h.Help)
	r.Get("/", h.Help)

	r.Get("/domain/{domainName}", h.LookupDomain)
	r.Head("/domain/{domainName}", h.LookupDomain)
	r.Get("/entity/{handle}", h.LookupEntity)
	r.Head("/entity/{handle}", h.LookupEntity)
	r.Get("/nameserver/{name}", h.LookupNameserver)
	r.Head("/nameserver/{name}", h.LookupNameserver)
	r.Get("/ip/{network}", h.LookupIPNetwork)
	r.Head("/ip/{network}", h.LookupIPNetwork)
	r.Get("/autnum/{asn}", h.LookupAutnum)
	r.Head("/autnum/{asn}", h.LookupAutnum)

	// Search endpoints
	r.Get("/domains", h.SearchDomains)
	r.Head("/domains", h.SearchDomains)
	r.Get("/entities", h.SearchEntities)
	r.Head("/entities", h.SearchEntities)
	r.Get("/nameservers", h.SearchNameservers)
	r.Head("/nameservers", h.SearchNameservers)
}

func (h *Handler) requestURL(r *http.Request) string {
	scheme := "http"
	if r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https") {
		scheme = "https"
	}
	host := r.Host
	if _, _, err := net.SplitHostPort(host); err != nil {
		// Host header has no explicit port; add the configured port unless it is
		// the default for the scheme (80/443).
		port := h.port
		if !(scheme == "http" && port == 80) && !(scheme == "https" && port == 443) {
			host = net.JoinHostPort(host, strconv.Itoa(port))
		}
	}
	u := url.URL{Scheme: scheme, Host: host, Path: r.URL.Path}
	if r.URL.RawQuery != "" {
		u.RawQuery = r.URL.RawQuery
	}
	return u.String()
}

func (h *Handler) Help(w http.ResponseWriter, r *http.Request) {
	help := rdap.NewHelp(h.cfg.BaseURL)
	writeJSON(w, http.StatusOK, help)
}

func (h *Handler) LookupDomain(w http.ResponseWriter, r *http.Request) {
	domainName := chi.URLParam(r, "domainName")
	if domainName == "" {
		writeError(w, http.StatusBadRequest, 400, "Invalid domain name", "Domain name parameter is required")
		return
	}

	ldhName, _, err := rdap.NormalizeDomainName(domainName)
	if err != nil {
		writeError(w, http.StatusBadRequest, 400, "Invalid domain name", err.Error())
		return
	}

	record, err := h.store.LookupDomain(ldhName)
	if err != nil {
		writeError(w, http.StatusNotFound, 404, "Domain not found", fmt.Sprintf("No domain found for: %s", domainName))
		return
	}

	reqURL := h.requestURL(r)
	resp := domainRecordToRDAP(record, h.cfg.BaseURL, reqURL, h.cfg.RegistrarBaseURL, h.cfg.Mode)
	writeJSON(w, http.StatusOK, resp)
}

func (h *Handler) LookupEntity(w http.ResponseWriter, r *http.Request) {
	handle := chi.URLParam(r, "handle")
	if handle == "" {
		writeError(w, http.StatusBadRequest, 400, "Invalid entity handle", "Entity handle parameter is required")
		return
	}

	record, err := h.store.LookupEntity(handle)
	if err != nil {
		writeError(w, http.StatusNotFound, 404, "Entity not found", fmt.Sprintf("No entity found for handle: %s", handle))
		return
	}

	reqURL := h.requestURL(r)
	resp := rdap.EntityResponse{
		Entity:       entityRecordToRDAP(record, h.cfg.BaseURL),
		Conformance:  rdap.NewConformance(),
		Notices:      rdap.NewNoticesWithICANN(reqURL, h.cfg.BaseURL),
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *Handler) LookupNameserver(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	if name == "" {
		writeError(w, http.StatusBadRequest, 400, "Invalid nameserver name", "Nameserver name parameter is required")
		return
	}

	record, err := h.store.LookupNameserver(strings.ToLower(name))
	if err != nil {
		writeError(w, http.StatusNotFound, 404, "Nameserver not found", fmt.Sprintf("No nameserver found for: %s", name))
		return
	}

	reqURL := h.requestURL(r)
	resp := rdap.NameserverResponse{
		Nameserver:   nameserverRecordToRDAP(record, h.cfg.BaseURL),
		Conformance:  rdap.NewConformance(),
		Notices:      rdap.NewNoticesWithICANN(reqURL, h.cfg.BaseURL),
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *Handler) LookupIPNetwork(w http.ResponseWriter, r *http.Request) {
	network := chi.URLParam(r, "network")
	if network == "" {
		writeError(w, http.StatusBadRequest, 400, "Invalid network", "Network parameter is required")
		return
	}

	_, err := netip.ParsePrefix(network)
	if err != nil {
		if _, err := netip.ParseAddr(network); err != nil {
			writeError(w, http.StatusBadRequest, 400, "Invalid network", "Must be a valid IP address or CIDR notation")
			return
		}
		if strings.Contains(network, ":") {
			network += "/128"
		} else {
			network += "/32"
		}
	}

	record, err := h.store.LookupIPNetwork(network)
	if err != nil {
		writeError(w, http.StatusNotFound, 404, "IP network not found", fmt.Sprintf("No IP network found for: %s", network))
		return
	}

	reqURL := h.requestURL(r)
	resp := rdap.IPNetworkResponse{
		IPNetwork:    ipNetworkRecordToRDAP(record, h.cfg.BaseURL),
		Conformance:  rdap.NewConformance(),
		Notices:      rdap.NewNoticesWithICANN(reqURL, h.cfg.BaseURL),
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *Handler) LookupAutnum(w http.ResponseWriter, r *http.Request) {
	asnStr := chi.URLParam(r, "asn")
	if asnStr == "" {
		writeError(w, http.StatusBadRequest, 400, "Invalid ASN", "ASN parameter is required")
		return
	}

	asn, err := strconv.ParseUint(asnStr, 10, 32)
	if err != nil {
		writeError(w, http.StatusBadRequest, 400, "Invalid ASN", "ASN must be a valid 32-bit unsigned integer")
		return
	}

	resp := rdap.AutnumResponse{
		Autnum: rdap.Autnum{
			Common: rdap.Common{
				ObjectClassName: "autnum",
				Handle:          fmt.Sprintf("AS%d", asn),
				Status:          []string{"active"},
			},
			StartAutnum: int(asn),
			EndAutnum:   int(asn),
			Name:        fmt.Sprintf("AS%d", asn),
		},
		Conformance: rdap.NewConformance(),
		Notices:     rdap.NewNoticesWithICANN(h.requestURL(r), h.cfg.BaseURL),
	}

	writeJSON(w, http.StatusOK, resp)
}

func (h *Handler) SearchDomains(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	namePattern := q.Get("name")
	nsPattern := q.Get("nsLdhName")

	limit := h.cfg.MaxSearchLimit
	if l := q.Get("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 && parsed <= limit {
			limit = parsed
		}
	}

	if namePattern != "" && nsPattern != "" {
		writeError(w, http.StatusBadRequest, 400, "Ambiguous search", "Cannot specify both 'name' and 'nsLdhName' parameters")
		return
	}

	var records []rdap.DomainRecord
	var err error

	if namePattern != "" {
		records, err = h.store.SearchDomainsByName(namePattern, limit)
	} else if nsPattern != "" {
		records, err = h.store.SearchDomainsByNS(nsPattern, limit)
	} else {
		writeError(w, http.StatusBadRequest, 400, "Missing search parameter", "Either 'name' or 'nsLdhName' parameter is required")
		return
	}

	if err != nil {
		writeError(w, http.StatusInternalServerError, 500, "Search failed", err.Error())
		return
	}

	results := make([]rdap.Domain, 0, len(records))
	for _, record := range records {
		results = append(results, domainRecordToRDAP(&record, h.cfg.BaseURL, h.requestURL(r), h.cfg.RegistrarBaseURL, h.cfg.Mode))
	}

	resp := rdap.DomainSearchResult{
		Conformance:         rdap.NewConformance(),
		DomainSearchResults: results,
		Notices:             rdap.NewNoticesWithICANN(h.requestURL(r), h.cfg.BaseURL),
	}

	writeJSON(w, http.StatusOK, resp)
}

func (h *Handler) SearchEntities(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	fnPattern := q.Get("fn")
	handlePattern := q.Get("handle")

	limit := h.cfg.MaxSearchLimit
	if l := q.Get("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 && parsed <= limit {
			limit = parsed
		}
	}

	if fnPattern != "" && handlePattern != "" {
		writeError(w, http.StatusBadRequest, 400, "Ambiguous search", "Cannot specify both 'fn' and 'handle' parameters")
		return
	}

	var records []rdap.EntityRecord
	var err error

	if fnPattern != "" {
		records, err = h.store.SearchEntitiesByName(fnPattern, limit)
	} else if handlePattern != "" {
		records, err = h.store.SearchEntitiesByHandle(handlePattern, limit)
	} else {
		writeError(w, http.StatusBadRequest, 400, "Missing search parameter", "Either 'fn' or 'handle' parameter is required")
		return
	}

	if err != nil {
		writeError(w, http.StatusInternalServerError, 500, "Search failed", err.Error())
		return
	}

	results := make([]rdap.Entity, 0, len(records))
	for _, record := range records {
		results = append(results, entityRecordToRDAP(&record, h.cfg.BaseURL))
	}

	resp := rdap.EntitySearchResult{
		Conformance:         rdap.NewConformance(),
		EntitySearchResults: results,
		Notices:             rdap.NewNoticesWithICANN(h.requestURL(r), h.cfg.BaseURL),
	}

	writeJSON(w, http.StatusOK, resp)
}

func (h *Handler) SearchNameservers(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	namePattern := q.Get("name")
	ip := q.Get("ip")

	limit := h.cfg.MaxSearchLimit
	if l := q.Get("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 && parsed <= limit {
			limit = parsed
		}
	}

	if namePattern != "" && ip != "" {
		writeError(w, http.StatusBadRequest, 400, "Ambiguous search", "Cannot specify both 'name' and 'ip' parameters")
		return
	}

	var records []rdap.NameserverRecord
	var err error

	if namePattern != "" {
		records, err = h.store.SearchNameserversByName(namePattern, limit)
	} else if ip != "" {
		records, err = h.store.SearchNameserversByIP(ip, limit)
	} else {
		writeError(w, http.StatusBadRequest, 400, "Missing search parameter", "Either 'name' or 'ip' parameter is required")
		return
	}

	if err != nil {
		writeError(w, http.StatusInternalServerError, 500, "Search failed", err.Error())
		return
	}

	results := make([]rdap.Nameserver, 0, len(records))
	for _, record := range records {
		results = append(results, nameserverRecordToRDAP(&record, h.cfg.BaseURL))
	}

	resp := rdap.NameserverSearchResult{
		Conformance:              rdap.NewConformance(),
		NameserverSearchResults: results,
		Notices:                  rdap.NewNoticesWithICANN(h.requestURL(r), h.cfg.BaseURL),
	}

	writeJSON(w, http.StatusOK, resp)
}

func domainRecordToRDAP(record *rdap.DomainRecord, baseURL, requestURL, registrarBaseURL, mode string) rdap.Domain {
	nameservers := make([]rdap.Nameserver, len(record.Nameservers))
	for i, ns := range record.Nameservers {
		nameservers[i] = rdap.Nameserver{
			Common: rdap.Common{
				ObjectClassName: "nameserver",
				Handle:          ns.Handle,
				Status:          ns.Status,
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

	events := []rdap.Event{
		{EventAction: "registration", EventDate: rdap.FormatTime(record.CreatedAt)},
		{EventAction: "last changed", EventDate: rdap.FormatTime(record.UpdatedAt)},
		{EventAction: "expiration", EventDate: rdap.FormatTime(record.ExpiresAt)},
		{EventAction: "last update of RDAP database", EventDate: rdap.FormatTime(record.UpdatedAt)},
	}
	if mode == "registrar" {
		// Required by the 2024 gTLD Response Profile for registrar servers (-65600).
		events = append(events, rdap.Event{EventAction: "registrar expiration", EventDate: rdap.FormatTime(record.ExpiresAt)})
	}

	// Registrar entity with vcardArray, publicIds matching IANA registrar ID.
	// The about link points at the registrar's IANA-registered RDAP base URL.
	registrarEntity := rdap.Entity{
		Common: rdap.Common{
			ObjectClassName: "entity",
			Handle:          record.Registrant,
			Status:          []string{"active"},
			Links: []rdap.Link{{
				Value: registrarBaseURL,
				Rel:   "about",
				Href:  registrarBaseURL,
				Type:  "application/rdap+json",
			}},
		},
		Roles: []string{"registrar"},
		VCardArray: []interface{}{
			"vcard",
			[]interface{}{
				[]interface{}{"version", map[string]interface{}{}, "text", "4.0"},
				[]interface{}{"fn", map[string]interface{}{}, "text", "Example Registrar Inc."},
				[]interface{}{"adr", map[string]interface{}{"cc": "US"}, "text", []interface{}{"", "", "123 Maple Ave", "Los Angeles", "CA", "90210", ""}},
			},
		},
		PublicIDs: []rdap.PublicID{
			{Type: "IANA Registrar ID", Identifier: record.Registrant},
		},
	}

	// Abuse entity inside registrar entity with tel and email
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

	domainEntities := []rdap.Entity{registrarEntity}
	if mode == "registrar" {
		// Registrant data is served by registrars; required by the 2024 gTLD
		// Response Profile section 2.7.2 for registrar servers (-63000).
		domainEntities = append(domainEntities, registrantEntity)
	}

	domain := rdap.Domain{
		Common: rdap.Common{
			ObjectClassName: "domain",
			Handle:          record.Handle,
			Status:          record.Status,
			Events:          events,
			Entities:        domainEntities,
			Links: []rdap.Link{{
				Value: requestURL,
				Rel:   "self",
				Href:  fmt.Sprintf("%s/domain/%s", baseURL, record.LDHName),
				Type:  "application/rdap+json",
			}, {
				Value: requestURL,
				Rel:   "related",
				Href:  fmt.Sprintf("%s/domain/%s", strings.TrimRight(registrarBaseURL, "/"), record.LDHName),
				Type:  "application/rdap+json",
			}},
			Port43: "",
		},
		LDHName:     record.LDHName,
		UnicodeName: record.UnicodeName,
		Nameservers: nameservers,
		SecureDNS:   secureDNS,
	}

	domain.Conformance = rdap.NewConformance2024()
	domain.Notices = rdap.NewNoticesWithICANN(requestURL, baseURL)

	return domain
}

func entityRecordToRDAP(record *rdap.EntityRecord, baseURL string) rdap.Entity {
	return rdap.Entity{
		Common: rdap.Common{
			ObjectClassName: "entity",
			Handle:          record.Handle,
			Status:          record.Status,
			Events: []rdap.Event{
				{EventAction: "registration", EventDate: rdap.FormatTime(record.CreatedAt)},
				{EventAction: "last changed", EventDate: rdap.FormatTime(record.UpdatedAt)},
			},
			Links: []rdap.Link{{
				Value: fmt.Sprintf("%s/entity/%s", baseURL, record.Handle),
				Rel:   "self",
				Href:  fmt.Sprintf("%s/entity/%s", baseURL, record.Handle),
				Type:  "application/rdap+json",
			}},
		},
		Roles:     record.Roles,
		PublicIDs: record.PublicIDs,
	}
}

func nameserverRecordToRDAP(record *rdap.NameserverRecord, baseURL string) rdap.Nameserver {
	return rdap.Nameserver{
		Common: rdap.Common{
			ObjectClassName: "nameserver",
			Handle:          record.Handle,
			Status:          record.Status,
			Events: []rdap.Event{
				{EventAction: "registration", EventDate: rdap.FormatTime(record.CreatedAt)},
				{EventAction: "last changed", EventDate: rdap.FormatTime(record.UpdatedAt)},
			},
			Links: []rdap.Link{{
				Value: fmt.Sprintf("%s/nameserver/%s", baseURL, record.LDHName),
				Rel:   "self",
				Href:  fmt.Sprintf("%s/nameserver/%s", baseURL, record.LDHName),
				Type:  "application/rdap+json",
			}},
		},
		LDHName:     record.LDHName,
		UnicodeName: record.UnicodeName,
		IPAddresses: &rdap.IPAddrSet{
			V4: record.IPV4,
			V6: record.IPV6,
		},
	}
}

func ipNetworkRecordToRDAP(record *rdap.IPNetworkRecord, baseURL string) rdap.IPNetwork {
	return rdap.IPNetwork{
		Common: rdap.Common{
			ObjectClassName: "ip network",
			Handle:          record.Handle,
			Status:          record.Status,
			Events: []rdap.Event{
				{EventAction: "registration", EventDate: rdap.FormatTime(record.CreatedAt)},
				{EventAction: "last changed", EventDate: rdap.FormatTime(record.UpdatedAt)},
			},
			Links: []rdap.Link{{
				Value: fmt.Sprintf("%s/ip/%s", baseURL, record.CIDR[0]),
				Rel:   "self",
				Href:  fmt.Sprintf("%s/ip/%s", baseURL, record.CIDR[0]),
				Type:  "application/rdap+json",
			}},
		},
		StartAddress: record.StartAddress,
		EndAddress:   record.EndAddress,
		IPVersion:    record.IPVersion,
		CIDR:         record.CIDR,
		Name:         record.Name,
		Type:         record.Type,
		Country:      record.Country,
		ParentHandle: "",
	}
}
