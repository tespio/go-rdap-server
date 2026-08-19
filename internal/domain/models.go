// Package domain defines the canonical registry data model. It is the
// authoritative representation of registration data, independent of both the
// storage layer and the RDAP wire format. Storage adapters (internal/store)
// populate these aggregates, and the query service (internal/service) maps them
// to RDAP responses.
//
// The types here are intentionally richer than the RDAP wire records: they carry
// registry metadata (version, source-of-truth timestamps, history, privacy
// state, registrar relationships, audit) that a real registry needs but that is
// not necessarily exposed over RDAP.
package domain

import "time"

// ObjectKind identifies the type of a registry object.
type ObjectKind string

const (
	KindDomain      ObjectKind = "domain"
	KindContact     ObjectKind = "entity"
	KindNameserver  ObjectKind = "nameserver"
	KindIPNetwork   ObjectKind = "ip_network"
	KindAutnum      ObjectKind = "autnum"
)

// Metadata carries source-of-truth, versioning, and audit information common to
// all registry objects. This is what lets a real registry reconcile its own
// model with this one, track change lineage, and reproduce historical state.
type Metadata struct {
	// Version is a monotonically increasing version number for this object.
	Version int64 `json:"version"`
	// CreatedAt is the source-of-truth creation timestamp.
	CreatedAt time.Time `json:"created_at"`
	// UpdatedAt is the source-of-truth last-modification timestamp.
	UpdatedAt time.Time `json:"updated_at"`
	// UpdatedBy identifies the actor/process that last changed the object.
	UpdatedBy string `json:"updated_by,omitempty"`
	// Source identifies the upstream source of truth (e.g. "epp", "rdap", "srs").
	Source string `json:"source,omitempty"`
	// History holds prior versions/events of this object (see HistoryEntry).
	History []HistoryEntry `json:"history,omitempty"`
	// Audit is an optional free-form audit log for this object.
	Audit []AuditEntry `json:"audit,omitempty"`
}

// HistoryEntry records a previous state of an object, enabling "as-of" queries.
type HistoryEntry struct {
	// Version is the version this entry represents.
	Version int64 `json:"version"`
	// ChangedAt is when this version was recorded.
	ChangedAt time.Time `json:"changed_at"`
	// Action describes what changed (e.g. "create", "update", "status", "delete").
	Action string `json:"action,omitempty"`
	// Actor is who/what performed the change.
	Actor string `json:"actor,omitempty"`
	// Snapshot is an opaque serialized snapshot of the object at this version.
	Snapshot string `json:"snapshot,omitempty"`
}

// AuditEntry is a single audit-trail record for an object.
type AuditEntry struct {
	Timestamp time.Time `json:"timestamp"`
	Actor     string    `json:"actor,omitempty"`
	Action    string    `json:"action,omitempty"`
	Detail    string    `json:"detail,omitempty"`
}

// Status is a registry status value (RFC 5731 statuses for domains/hosts,
// or entity/network statuses). It can carry a rationale.
type Status struct {
	Value   string `json:"value"`
	Reason  string `json:"reason,omitempty"`
	Actor   string `json:"actor,omitempty"`
}

// Event is a registry lifecycle event (registration, last changed, expiration,
// transfer, etc.), distinct from the RDAP event wire type.
type Event struct {
	Action string    `json:"action"`
	Date   time.Time `json:"date"`
	Actor  string    `json:"actor,omitempty"`
}

// PrivacyState describes how contact data is disclosed.
type PrivacyState string

const (
	PrivacyPublic    PrivacyState = "public"
	PrivacyRedacted  PrivacyState = "redacted"
	PrivacyPrivate   PrivacyState = "private"
	PrivacyProxy     PrivacyState = "proxy"
)

// ContactRole enumerates the roles a contact can play on a domain.
type ContactRole string

const (
	RoleRegistrant     ContactRole = "registrant"
	RoleAdministrative ContactRole = "administrative"
	RoleTechnical      ContactRole = "technical"
	RoleBilling        ContactRole = "billing"
	RoleRegistrar      ContactRole = "registrar"
	RoleAbuse          ContactRole = "abuse"
	RoleReseller       ContactRole = "reseller"
)

// Contact is a registry contact/entity. It may be a registrar, registrant,
// technical/administrative/billing contact, or an abuse contact.
type Contact struct {
	Handle string `json:"handle"`
	// Roles this contact plays.
	Roles []ContactRole `json:"roles"`
	// Status of the contact.
	Status []Status `json:"status"`
	// VCard is the structured contact data. See internal/domain/vcard for the
	// concrete structure; it is stored as a flexible JSON value so that any
	// jCard representation can be carried.
	VCard *VCard `json:"vcard,omitempty"`
	// PublicIDs holds external identifiers, e.g. IANA Registrar ID.
	PublicIDs []PublicID `json:"public_ids,omitempty"`
	// Privacy indicates how the contact's data is disclosed.
	Privacy PrivacyState `json:"privacy,omitempty"`
	// RegistrarID is the IANA registrar identifier when this is a registrar.
	RegistrarID string `json:"registrar_id,omitempty"`
	// RegistrarBaseURL is the IANA-registered RDAP base URL for this registrar.
	RegistrarBaseURL string `json:"registrar_base_url,omitempty"`
	// Nested entities (e.g. abuse contact under a registrar).
	Entities []*Contact `json:"entities,omitempty"`
	Metadata Metadata  `json:"metadata"`
}

// PublicID is an external identifier (IANA Registrar ID, etc.).
type PublicID struct {
	Type       string `json:"type"`
	Identifier string `json:"identifier"`
}

// NameServer is a registry nameserver (host).
type NameServer struct {
	Handle      string      `json:"handle"`
	LDHName     string      `json:"ldh_name"`
	UnicodeName string      `json:"unicode_name"`
	IPV4        []string    `json:"ipv4,omitempty"`
	IPV6        []string    `json:"ipv6,omitempty"`
	Status      []Status    `json:"status"`
	Metadata    Metadata    `json:"metadata"`
}

// DSRecord is a DNSSEC Delegation Signer record.
type DSRecord struct {
	KeyTag     int    `json:"key_tag"`
	Algorithm  int    `json:"algorithm"`
	DigestType int    `json:"digest_type"`
	Digest     string `json:"digest"`
}

// KeyRecord is a DNSSEC DNSKEY record.
type KeyRecord struct {
	Flags     int    `json:"flags"`
	Protocol  int    `json:"protocol"`
	Algorithm int    `json:"algorithm"`
	PublicKey string `json:"public_key"`
}

// SecureDNS describes the DNSSEC state of a domain.
type SecureDNS struct {
	ZoneSigned       bool        `json:"zone_signed"`
	DelegationSigned bool        `json:"delegation_signed"`
	MaxSigLife       *int        `json:"max_sig_life,omitempty"`
	DSRecords        []DSRecord  `json:"ds_records,omitempty"`
	KeyRecords       []KeyRecord `json:"key_records,omitempty"`
}

// Domain is the canonical registry domain aggregate. It carries the full set of
// contacts, nameservers, statuses, events, DNSSEC state, and registry metadata.
type Domain struct {
	Handle      string        `json:"handle"`
	LDHName     string        `json:"ldh_name"`
	UnicodeName string        `json:"unicode_name"`
	TLD         string        `json:"tld"`
	Status      []Status      `json:"status"`
	ExpiresAt   time.Time     `json:"expires_at"`
	// Contacts keyed by role. A role may map to multiple handles.
	Contacts map[ContactRole][]string `json:"contacts"`
	// Nameservers attached to the domain, with their full data.
	Nameservers []NameServer `json:"nameservers"`
	SecureDNS   *SecureDNS  `json:"secure_dns,omitempty"`
	// Registrar is the handle of the sponsoring registrar (IANA Registrar ID or
	// internal handle).
	Registrar string    `json:"registrar"`
	Metadata  Metadata  `json:"metadata"`
}

// DomainAggregate is a domain plus its resolved related objects, all read from a
// single transactional snapshot. Rendering an RDAP response from one aggregate
// guarantees internal consistency: the domain's status/events and its embedded
// registrar, contacts, and nameservers all reflect the same moment in time, so a
// concurrent transfer/renewal/delete can never produce a torn response (e.g. a
// domain that says "transferred to Registrar B" but still carries Registrar A's
// contact data).
type DomainAggregate struct {
	Domain *Domain
	// Registrar is the resolved sponsoring registrar contact (if available).
	Registrar *Contact
	// Contacts maps contact handle -> resolved contact.
	Contacts map[string]*Contact
	// Nameservers maps nameserver handle -> resolved nameserver.
	Nameservers map[string]*NameServer
}

// IPNetwork is a registry IP network (address block).
type IPNetwork struct {
	Handle       string    `json:"handle"`
	StartAddress string    `json:"start_address"`
	EndAddress   string    `json:"end_address"`
	IPVersion    string    `json:"ip_version"`
	CIDR         []string  `json:"cidr"`
	Name         string    `json:"name,omitempty"`
	Type         string    `json:"type,omitempty"`
	Country      string    `json:"country,omitempty"`
	Status       []Status  `json:"status"`
	ParentHandle string    `json:"parent_handle,omitempty"`
	Metadata     Metadata  `json:"metadata"`
}

// Autnum is a registry autonomous system number.
type Autnum struct {
	Handle     string   `json:"handle"`
	StartASN   uint32   `json:"start_asn"`
	EndASN     uint32   `json:"end_asn"`
	Name       string   `json:"name,omitempty"`
	Type       string   `json:"type,omitempty"`
	Country    string   `json:"country,omitempty"`
	Status     []Status `json:"status"`
	Metadata   Metadata `json:"metadata"`
}
