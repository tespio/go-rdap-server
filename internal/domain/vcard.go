package domain

// VCard is a structured jCard (RFC 6350/7095) representation of contact data.
// It mirrors the wire format so that the RDAP serializer can emit it directly,
// but lives in the domain layer so the registry model owns contact structure.
type VCard struct {
	// Version is the vCard version, typically "4.0".
	Version string `json:"version"`
	// FullName is the vCard "fn" (formatted name).
	FullName string `json:"fn,omitempty"`
	// Kind is the vCard "kind" (individual, org, group, location).
	Kind string `json:"kind,omitempty"`
	// Organization is the vCard "org".
	Organization string `json:"org,omitempty"`
	// Address is the structured vCard "adr". The 7 elements are:
	// [po box, extended, street, locality, region, postal code, country].
	Address *VCardAddress `json:"adr,omitempty"`
	// VoiceTel is the vCard "tel" with type voice.
	VoiceTel string `json:"voice_tel,omitempty"`
	// FaxTel is the vCard "tel" with type fax.
	FaxTel string `json:"fax_tel,omitempty"`
	// Email is the vCard "email".
	Email string `json:"email,omitempty"`
	// ContactURI is the vCard "contact-uri" (a web-based contact form), used in
	// lieu of an email for privacy redaction per RFC 9537.
	ContactURI string `json:"contact_uri,omitempty"`
}

// VCardAddress is the structured vCard address.
type VCardAddress struct {
	// CountryCode is the two-letter country code (cc param).
	CountryCode string `json:"cc,omitempty"`
	// POBox is the post-office box element (index 0).
	POBox string `json:"po_box,omitempty"`
	// Extended is the extended address element (index 1).
	Extended string `json:"extended,omitempty"`
	// Street is the street element (index 2).
	Street string `json:"street,omitempty"`
	// Locality is the city/locality element (index 3).
	Locality string `json:"locality,omitempty"`
	// Region is the state/region element (index 4).
	Region string `json:"region,omitempty"`
	// PostalCode is the postal code element (index 5).
	PostalCode string `json:"postal_code,omitempty"`
	// CountryName is the full country name element (index 6).
	CountryName string `json:"country_name,omitempty"`
}

// Redaction describes a withheld field on a contact, following RFC 9537.
type Redaction struct {
	// NameType is the ICANN-registered redaction name type (e.g. "Registrant Name").
	NameType string `json:"name_type,omitempty"`
	// NameDescription is a free-form redaction name description.
	NameDescription string `json:"name_description,omitempty"`
	// Method is the RFC 9537 redaction method: removal, emptyValue, partialValue,
	// or replacementValue.
	Method string `json:"method"`
	// PrePath is the JSONPath to the field in the pre-redacted response.
	PrePath string `json:"pre_path,omitempty"`
	// PostPath is the JSONPath to the field in the post-redacted response.
	PostPath string `json:"post_path,omitempty"`
	// ReplacementPath is the JSONPath to the replacement field (replacementValue).
	ReplacementPath string `json:"replacement_path,omitempty"`
	// PathLang is the path expression language (default "jsonpath").
	PathLang string `json:"path_lang,omitempty"`
	// Reason is an optional human-readable reason for the redaction.
	Reason string `json:"reason,omitempty"`
}
