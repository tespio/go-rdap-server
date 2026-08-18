package rdap

import "time"

// RDAP JSON response types per RFC 9083.

type Conformance struct {
	Conformance []string `json:"rdapConformance,omitempty"`
}

type Common struct {
	ObjectClassName string      `json:"objectClassName,omitempty"`
	Handle          string      `json:"handle,omitempty"`
	Links           []Link      `json:"links,omitempty"`
	Events          []Event     `json:"events,omitempty"`
	Notices         []Notice    `json:"notices,omitempty"`
	Remarks         []Remark    `json:"remarks,omitempty"`
	Port43          string      `json:"port43,omitempty"`
	Entities        []Entity    `json:"entities,omitempty"`
	Status          []string    `json:"status,omitempty"`
}

type Link struct {
	Value     string `json:"value,omitempty"`
	Rel       string `json:"rel,omitempty"`
	Href      string `json:"href,omitempty"`
	Type      string `json:"type,omitempty"`
	Title     string `json:"title,omitempty"`
}

type Event struct {
	EventAction string `json:"eventAction,omitempty"`
	EventDate   string `json:"eventDate,omitempty"`
	EventActor  string `json:"eventActor,omitempty"`
}

type Notice struct {
	Title       string   `json:"title,omitempty"`
	Type        string   `json:"type,omitempty"`
	Description []string `json:"description,omitempty"`
	Links       []Link   `json:"links,omitempty"`
}

type Remark struct {
	Title       string   `json:"title,omitempty"`
	Type        string   `json:"type,omitempty"`
	Description []string `json:"description,omitempty"`
	Links       []Link   `json:"links,omitempty"`
}

type PublicID struct {
	Type  string `json:"type,omitempty"`
	Identifier string `json:"identifier,omitempty"`
}

type Entity struct {
	Common
	VCardArray     []interface{} `json:"vcardArray,omitempty"`
	Roles          []string      `json:"roles,omitempty"`
	PublicIDs      []PublicID    `json:"publicIds,omitempty"`
	Networks       []IPNetwork   `json:"networks,omitempty"`
	AsEventActor   bool          `json:"asEventActor,omitempty"`
}

type IPNetwork struct {
	Common
	StartAddress string   `json:"startAddress,omitempty"`
	EndAddress   string   `json:"endAddress,omitempty"`
	IPVersion    string   `json:"ipVersion,omitempty"`
	CIDR         []string `json:"cidr,omitempty"`
	Name         string   `json:"name,omitempty"`
	Type         string   `json:"type,omitempty"`
	Country      string   `json:"country,omitempty"`
	ParentHandle string   `json:"parentHandle,omitempty"`
}

type Domain struct {
	Common
	Conformance
	LDHName      string        `json:"ldhName,omitempty"`
	UnicodeName  string        `json:"unicodeName,omitempty"`
	Variants     []Variant     `json:"variants,omitempty"`
	Nameservers  []Nameserver  `json:"nameservers,omitempty"`
	SecureDNS    *SecureDNS    `json:"secureDNS,omitempty"`
	Network      *IPNetwork    `json:"network,omitempty"`
	Notices      []Notice      `json:"notices,omitempty"`
	PublicIDs    []PublicID    `json:"publicIds,omitempty"`
}

type Nameserver struct {
	Common
	LDHName     string   `json:"ldhName,omitempty"`
	UnicodeName string   `json:"unicodeName,omitempty"`
	IPAddresses *IPAddrSet `json:"ipAddresses,omitempty"`
}

type Variant struct {
	Relation     []string        `json:"relation,omitempty"`
	IDNTable     string          `json:"idnTable,omitempty"`
	VariantNames []VariantName   `json:"variantNames,omitempty"`
}

type VariantName struct {
	LDHName     string `json:"ldhName,omitempty"`
	UnicodeName string `json:"unicodeName,omitempty"`
}

type IPAddrSet struct {
	V4 []string `json:"v4,omitempty"`
	V6 []string `json:"v6,omitempty"`
}

type SecureDNS struct {
	ZoneSigned      bool           `json:"zoneSigned"`
	DelegationSigned bool          `json:"delegationSigned"`
	MaxSigLife      *int           `json:"maxSigLife,omitempty"`
	DSData          []DSData       `json:"dsData,omitempty"`
	KeyData         []KeyData      `json:"keyData,omitempty"`
}

type DSData struct {
	KeyTag     int    `json:"keyTag,omitempty"`
	Algorithm  int    `json:"algorithm,omitempty"`
	DigestType int    `json:"digestType,omitempty"`
	Digest     string `json:"digest,omitempty"`
	Events     []Event `json:"events,omitempty"`
	Links      []Link  `json:"links,omitempty"`
}

type KeyData struct {
	Flags    int      `json:"flags,omitempty"`
	Protocol int      `json:"protocol,omitempty"`
	Algorithm int     `json:"algorithm,omitempty"`
	PublicKey string  `json:"publicKey,omitempty"`
	Events   []Event  `json:"events,omitempty"`
	Links    []Link   `json:"links,omitempty"`
}

type DomainSearchResult struct {
	Conformance
	DomainSearchResults []Domain `json:"domainSearchResults,omitempty"`
	Notices             []Notice `json:"notices,omitempty"`
}

type EntitySearchResult struct {
	Conformance
	EntitySearchResults []Entity `json:"entitySearchResults,omitempty"`
	Notices             []Notice `json:"notices,omitempty"`
}

type NameserverSearchResult struct {
	Conformance
	NameserverSearchResults []Nameserver `json:"nameserverSearchResults,omitempty"`
	Notices                 []Notice     `json:"notices,omitempty"`
}

type Help struct {
	Conformance
	Notices     []Notice `json:"notices,omitempty"`
}

// Top-level response wrappers for objects that can be embedded.
// These add rdapConformance and notices only at the topmost JSON level.
type EntityResponse struct {
	Entity
	Conformance
	Notices []Notice `json:"notices,omitempty"`
}

type NameserverResponse struct {
	Nameserver
	Conformance
	Notices []Notice `json:"notices,omitempty"`
}

type IPNetworkResponse struct {
	IPNetwork
	Conformance
	Notices []Notice `json:"notices,omitempty"`
}

type AutnumResponse struct {
	Autnum
	Conformance
	Notices []Notice `json:"notices,omitempty"`
}

type Autnum struct {
	Common
	StartAutnum int      `json:"startAutnum,omitempty"`
	EndAutnum   int      `json:"endAutnum,omitempty"`
	Name        string   `json:"name,omitempty"`
	Type        string   `json:"type,omitempty"`
	Country     string   `json:"country,omitempty"`
}

type ErrorResponse struct {
	Conformance
	ErrorCode    int      `json:"errorCode,omitempty"`
	Title        string   `json:"title,omitempty"`
	Description  []string `json:"description,omitempty"`
	Notices      []Notice `json:"notices,omitempty"`
}

// DomainRecord is the internal storage model for a domain.
type DomainRecord struct {
	Handle     string    `json:"handle"`
	LDHName    string    `json:"ldh_name"`
	UnicodeName string   `json:"unicode_name"`
	TLD        string    `json:"tld"`
	Status     []string  `json:"status"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
	ExpiresAt  time.Time `json:"expires_at"`
	Registrant string    `json:"registrant"`
	Admin      string    `json:"admin"`
	Tech       string    `json:"tech"`
	Billing    string    `json:"billing"`
	Nameservers []NameserverRecord `json:"nameservers"`
	SecureDNS  *SecureDNSRecord    `json:"secure_dns"`
}

type NameserverRecord struct {
	Handle     string   `json:"handle"`
	LDHName    string   `json:"ldh_name"`
	UnicodeName string  `json:"unicode_name"`
	IPV4       []string `json:"ipv4"`
	IPV6       []string `json:"ipv6"`
	Status     []string `json:"status"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

type SecureDNSRecord struct {
	ZoneSigned       bool       `json:"zone_signed"`
	DelegationSigned bool       `json:"delegation_signed"`
	MaxSigLife       *int       `json:"max_sig_life"`
	DSRecords        []DSRecord `json:"ds_records"`
	KeyRecords       []KeyRecord `json:"key_records"`
}

type DSRecord struct {
	KeyTag     int    `json:"key_tag"`
	Algorithm  int    `json:"algorithm"`
	DigestType int    `json:"digest_type"`
	Digest     string `json:"digest"`
}

type KeyRecord struct {
	Flags     int    `json:"flags"`
	Protocol  int    `json:"protocol"`
	Algorithm int    `json:"algorithm"`
	PublicKey string `json:"public_key"`
}

type EntityRecord struct {
	Handle      string    `json:"handle"`
	VCardJSON   string    `json:"vcard_json"`
	Roles       []string  `json:"roles"`
	Status      []string  `json:"status"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	PublicIDs   []PublicID `json:"public_ids"`
}

type IPNetworkRecord struct {
	Handle       string    `json:"handle"`
	StartAddress string    `json:"start_address"`
	EndAddress   string    `json:"end_address"`
	IPVersion    string    `json:"ip_version"`
	CIDR         []string  `json:"cidr"`
	Name         string    `json:"name"`
	Type         string    `json:"type"`
	Country      string    `json:"country"`
	Status       []string  `json:"status"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}
