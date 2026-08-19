package rdap

import (
	"fmt"
	"net"
	"net/netip"
	"regexp"
	"strings"
	"time"
	"unicode"

	"golang.org/x/net/idna"
)

var (
	domainNameRe = regexp.MustCompile(`^(xn--[a-z0-9-]+|[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?)`)
	ipv4Re       = regexp.MustCompile(`^(\d{1,3}\.){3}\d{1,3}$`)
	ipv6Re       = regexp.MustCompile(`^[0-9a-fA-F:]+$`)
)

type LookupType int

const (
	LookupDomain LookupType = iota
	LookupEntity
	LookupNameserver
	LookupIP
	LookupAutnum
	LookupHelp
)

type SearchType int

const (
	SearchDomainByName SearchType = iota
	SearchDomainByNS
	SearchEntityByName
	SearchEntityByHandle
	SearchNameserverByName
	SearchNameserverByIP
)

func ValidateDomainName(name string) error {
	if len(name) == 0 {
		return fmt.Errorf("domain name is empty")
	}
	if len(name) > 253 {
		return fmt.Errorf("domain name exceeds 253 characters")
	}
	labels := strings.Split(strings.TrimSuffix(name, "."), ".")
	if len(labels) < 2 {
		return fmt.Errorf("domain name must have at least two labels")
	}
	for _, label := range labels {
		if len(label) == 0 {
			return fmt.Errorf("empty label in domain name")
		}
		if len(label) > 63 {
			return fmt.Errorf("label exceeds 63 characters: %s", label)
		}
	}
	return nil
}

func ValidateLDHName(name string) bool {
	if len(name) == 0 || len(name) > 253 {
		return false
	}
	labels := strings.Split(strings.TrimSuffix(name, "."), ".")
	for _, label := range labels {
		if len(label) == 0 || len(label) > 63 {
			return false
		}
		for i, r := range label {
			if i == 0 || i == len(label)-1 {
				if r == '-' {
					return false
				}
			}
			if !((r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-') {
				return false
			}
		}
	}
	return true
}

func NormalizeDomainName(name string) (ldh, unicode string, err error) {
	name = strings.TrimSpace(name)
	name = strings.ToLower(name)
	name = strings.TrimSuffix(name, ".")

	// If already ASCII, no conversion needed
	if isASCII(name) {
		if !ValidateLDHName(name) {
			return "", "", fmt.Errorf("invalid LDH domain name: %s", name)
		}
		return name, name, nil
	}

	// Convert unicode to punycode
	ascii, err := idna.ToASCII(name)
	if err != nil {
		return "", "", fmt.Errorf("idna conversion failed: %w", err)
	}

	unicode, err = idna.ToUnicode(name)
	if err != nil {
		return "", "", fmt.Errorf("idna to unicode failed: %w", err)
	}

	if !ValidateLDHName(ascii) {
		return "", "", fmt.Errorf("invalid domain after IDNA conversion: %s", ascii)
	}

	return ascii, unicode, nil
}

func isASCII(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] > unicode.MaxASCII {
			return false
		}
	}
	return true
}

func ExtractTLD(name string) string {
	name = strings.TrimSuffix(name, ".")
	labels := strings.Split(name, ".")
	if len(labels) < 2 {
		return ""
	}
	return labels[len(labels)-1]
}

func IsTLDSupported(tld string, supportedTLDs []string) bool {
	tld = strings.ToLower(tld)
	for _, supported := range supportedTLDs {
		if strings.EqualFold(supported, tld) {
			return true
		}
	}
	return false
}

func ValidateIP(s string, version int) bool {
	addr, err := netip.ParseAddr(s)
	if err != nil {
		return false
	}
	if version == 4 && !addr.Is4() && !addr.Is4In6() {
		return false
	}
	if version == 6 && !addr.Is6() {
		return false
	}
	return true
}

func ParseCIDR(s string) (*net.IPNet, error) {
	_, cidr, err := net.ParseCIDR(s)
	if err != nil {
		return nil, fmt.Errorf("invalid CIDR: %w", err)
	}
	return cidr, nil
}

func FormatTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}

func NewConformance() Conformance {
	return Conformance{
		Conformance: []string{
			"rdap_level_0",
		},
	}
}

func NewConformance2019() Conformance {
	return Conformance{
		Conformance: []string{
			"rdap_level_0",
			"icann_rdap_technical_implementation_guide_0",
			"icann_rdap_response_profile_0",
		},
	}
}

func NewConformance2024() Conformance {
	return Conformance{
		Conformance: []string{
			"rdap_level_0",
			"icann_rdap_technical_implementation_guide_0",
			"icann_rdap_response_profile_0",
			"icann_rdap_technical_implementation_guide_1",
			"icann_rdap_response_profile_1",
		},
	}
}

// NoticeOptions carries optional registrar/registry customization for notices.
// The ICANN-mandated notices (Status Codes, RDDS Inaccuracy Complaint Form) are
// always emitted regardless; this only customizes the Terms of Service notice
// and appends registrar-specific notices.
type NoticeOptions struct {
	// ToSTitle overrides the Terms of Service notice title.
	ToSTitle string
	// ToSDescription is the Terms of Service notice body text.
	ToSDescription []string
	// ToSURL is the terms-of-service link target. If empty, links to /help.
	ToSURL string
	// Custom is an optional list of registrar/registry-specific notices.
	Custom []CustomNotice
}

// CustomNotice is a registrar/registry-specific notice appended to responses.
type CustomNotice struct {
	Title       string
	Description []string
	URL         string
	Rel         string
}

func NewNotices(baseURL string, opts *NoticeOptions) []Notice {
	return []Notice{
		buildToSNotice(baseURL, baseURL, opts),
		{
			Title:       "Rate Limiting",
			Description: []string{"Access to this RDAP server is rate-limited. Excessive queries may be throttled."},
			Links: []Link{{
				Value: baseURL,
				Rel:   "rate-limit-policy",
				Href:  fmt.Sprintf("%s/help", baseURL),
				Type:  "application/rdap+json",
			}},
		},
	}
}

func NewNoticesWithICANN(requestURL, baseURL string, opts *NoticeOptions) []Notice {
	notices := []Notice{
		buildToSNotice(requestURL, baseURL, opts),
		{
			Title:       "Rate Limiting",
			Description: []string{"Access to this RDAP server is rate-limited. Excessive queries may be throttled."},
			Links: []Link{{
				Value: requestURL,
				Rel:   "service",
				Href:  fmt.Sprintf("%s/help", baseURL),
				Type:  "application/rdap+json",
			}},
		},
		{
			Title:       "Status Codes",
			Description: []string{"For more information on domain status codes, please visit https://icann.org/epp"},
			Links: []Link{{
				Value: requestURL,
				Rel:   "glossary",
				Href:  "https://icann.org/epp",
				Type:  "text/html",
			}},
		},
		{
			Title:       "RDDS Inaccuracy Complaint Form",
			Description: []string{"URL of the ICANN RDDS Inaccuracy Complaint Form: https://icann.org/wicf"},
			Links: []Link{{
				Value: requestURL,
				Rel:   "help",
				Href:  "https://icann.org/wicf",
				Type:  "text/html",
			}},
		},
	}

	if opts != nil {
		for _, c := range opts.Custom {
			if c.URL == "" {
				c.URL = fmt.Sprintf("%s/help", baseURL)
			}
			rel := c.Rel
			if rel == "" {
				rel = "terms-of-service"
			}
			notices = append(notices, Notice{
				Title:       c.Title,
				Description: c.Description,
				Links: []Link{{
					Value: requestURL,
					Rel:   rel,
					Href:  c.URL,
					Type:  "text/html",
				}},
			})
		}
	}

	return notices
}

// buildToSNotice returns the Terms of Service notice, honoring customization.
func buildToSNotice(requestURL, baseURL string, opts *NoticeOptions) Notice {
	title := "Terms of Service"
	desc := []string{"Use of this RDAP server is subject to the registrar's terms of service."}
	href := fmt.Sprintf("%s/help", baseURL)

	if opts != nil {
		if opts.ToSTitle != "" {
			title = opts.ToSTitle
		}
		if len(opts.ToSDescription) > 0 {
			desc = opts.ToSDescription
		}
		if opts.ToSURL != "" {
			href = opts.ToSURL
		}
	}

	return Notice{
		Title:       title,
		Description: desc,
		Links: []Link{{
			Value: requestURL,
			Rel:   "terms-of-service",
			Href:  href,
			Type:  "text/html",
		}},
	}
}

func NewHelp(baseURL string, opts *NoticeOptions) Help {
	return Help{
		Conformance: NewConformance(),
		Notices:     NewNotices(baseURL, opts),
	}
}
