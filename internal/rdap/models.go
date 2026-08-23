package rdap

// RDAP JSON response types per RFC 9083.

type Conformance struct {
	Conformance []string `json:"rdapConformance,omitempty"`
}

type Common struct {
	ObjectClassName string   `json:"objectClassName,omitempty"`
	Handle          string   `json:"handle,omitempty"`
	Links           []Link   `json:"links,omitempty"`
	Events          []Event  `json:"events,omitempty"`
	Notices         []Notice `json:"notices,omitempty"`
	Remarks         []Remark `json:"remarks,omitempty"`
	Port43          string   `json:"port43,omitempty"`
	Entities        []Entity `json:"entities,omitempty"`
	Status          []string `json:"status,omitempty"`
}

type Link struct {
	Value string `json:"value,omitempty"`
	Rel   string `json:"rel,omitempty"`
	Href  string `json:"href,omitempty"`
	Type  string `json:"type,omitempty"`
	Title string `json:"title,omitempty"`
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
	Type       string `json:"type,omitempty"`
	Identifier string `json:"identifier,omitempty"`
}

type Entity struct {
	Common
	VCardArray   []interface{} `json:"vcardArray,omitempty"`
	Roles        []string      `json:"roles,omitempty"`
	PublicIDs    []PublicID    `json:"publicIds,omitempty"`
	Networks     []IPNetwork   `json:"networks,omitempty"`
	AsEventActor bool          `json:"asEventActor,omitempty"`
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
	// CIDR0CIDRs is emitted when the cidr0 extension is enabled (NRO RDAP CIDR).
	CIDR0CIDRs []CIDR0 `json:"cidr0_cidrs,omitempty"`
}

// CIDR0 is one entry of the cidr0 extension: a single CIDR notation expressed
// as a prefix address and length (NRO RDAP CIDR).
type CIDR0 struct {
	V4Prefix string `json:"v4prefix,omitempty"`
	V6Prefix string `json:"v6prefix,omitempty"`
	Length   int    `json:"length"`
}

type Domain struct {
	Common
	Conformance
	LDHName     string       `json:"ldhName,omitempty"`
	UnicodeName string       `json:"unicodeName,omitempty"`
	Variants    []Variant    `json:"variants,omitempty"`
	Nameservers []Nameserver `json:"nameservers,omitempty"`
	SecureDNS   *SecureDNS   `json:"secureDNS,omitempty"`
	Network     *IPNetwork   `json:"network,omitempty"`
	Notices     []Notice     `json:"notices,omitempty"`
	PublicIDs   []PublicID   `json:"publicIds,omitempty"`
	// TTL0Data is emitted when the ttl0 extension is enabled (draft-ietf-regext-rdap-ttl-extension).
	TTL0Data *TTL0Data `json:"ttl0_data,omitempty"`
}

type Nameserver struct {
	Common
	LDHName     string     `json:"ldhName,omitempty"`
	UnicodeName string     `json:"unicodeName,omitempty"`
	IPAddresses *IPAddrSet `json:"ipAddresses,omitempty"`
	// TTL0Data is emitted when the ttl0 extension is enabled.
	TTL0Data *TTL0Data `json:"ttl0_data,omitempty"`
}

// TTL0Data is the ttl0 extension payload mapping DNS record type mnemonics to
// TTL values, with optional remarks (draft-ietf-regext-rdap-ttl-extension-11).
type TTL0Data struct {
	Values  map[string]int `json:"values"`
	Remarks []Remark       `json:"remarks,omitempty"`
}

type Variant struct {
	Relation     []string      `json:"relation,omitempty"`
	IDNTable     string        `json:"idnTable,omitempty"`
	VariantNames []VariantName `json:"variantNames,omitempty"`
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
	ZoneSigned       bool      `json:"zoneSigned"`
	DelegationSigned bool      `json:"delegationSigned"`
	MaxSigLife       *int      `json:"maxSigLife,omitempty"`
	DSData           []DSData  `json:"dsData,omitempty"`
	KeyData          []KeyData `json:"keyData,omitempty"`
}

type DSData struct {
	KeyTag     int     `json:"keyTag,omitempty"`
	Algorithm  int     `json:"algorithm,omitempty"`
	DigestType int     `json:"digestType,omitempty"`
	Digest     string  `json:"digest,omitempty"`
	Events     []Event `json:"events,omitempty"`
	Links      []Link  `json:"links,omitempty"`
}

type KeyData struct {
	Flags     int     `json:"flags,omitempty"`
	Protocol  int     `json:"protocol,omitempty"`
	Algorithm int     `json:"algorithm,omitempty"`
	PublicKey string  `json:"publicKey,omitempty"`
	Events    []Event `json:"events,omitempty"`
	Links     []Link  `json:"links,omitempty"`
}

type DomainSearchResult struct {
	Conformance
	DomainSearchResults []Domain `json:"domainSearchResults,omitempty"`
	Notices             []Notice `json:"notices,omitempty"`
	// ReverseSearchPropertiesMapping is present on reverse search responses
	// (RFC 9536 §5).
	ReverseSearchPropertiesMapping []ReverseSearchPropertiesMapping `json:"reverse_search_properties_mapping,omitempty"`
}

type EntitySearchResult struct {
	Conformance
	EntitySearchResults []Entity `json:"entitySearchResults,omitempty"`
	Notices             []Notice `json:"notices,omitempty"`
	// ReverseSearchPropertiesMapping is present on reverse search responses
	// (RFC 9536 §5).
	ReverseSearchPropertiesMapping []ReverseSearchPropertiesMapping `json:"reverse_search_properties_mapping,omitempty"`
}

type NameserverSearchResult struct {
	Conformance
	NameserverSearchResults []Nameserver `json:"nameserverSearchResults,omitempty"`
	Notices                 []Notice     `json:"notices,omitempty"`
	// ReverseSearchPropertiesMapping is present on reverse search responses
	// (RFC 9536 §5).
	ReverseSearchPropertiesMapping []ReverseSearchPropertiesMapping `json:"reverse_search_properties_mapping,omitempty"`
}

type Help struct {
	Conformance
	Notices []Notice `json:"notices,omitempty"`
	// ReverseSearchProperties advertises the reverse searches the server
	// supports (RFC 9536). Emitted when the reverse_search extension is enabled.
	ReverseSearchProperties []ReverseSearchProperty `json:"reverse_search_properties,omitempty"`
}

// ReverseSearchProperty describes one supported reverse search (RFC 9536 §4).
type ReverseSearchProperty struct {
	SearchableResourceType string `json:"searchableResourceType"`
	RelatedResourceType    string `json:"relatedResourceType"`
	Property               string `json:"property"`
}

// ReverseSearchPropertiesMapping is included at the top of a reverse search
// response to detail the JSONPath mapping applied for each query property
// (RFC 9536 §5).
type ReverseSearchPropertiesMapping struct {
	Property     string `json:"property"`
	PropertyPath string `json:"propertyPath"`
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
	StartAutnum uint32 `json:"startAutnum,omitempty"`
	EndAutnum   uint32 `json:"endAutnum,omitempty"`
	Name        string `json:"name,omitempty"`
	Type        string `json:"type,omitempty"`
	Country     string `json:"country,omitempty"`
}

type ErrorResponse struct {
	Conformance
	ErrorCode   int      `json:"errorCode,omitempty"`
	Title       string   `json:"title,omitempty"`
	Description []string `json:"description,omitempty"`
	Notices     []Notice `json:"notices,omitempty"`
}
