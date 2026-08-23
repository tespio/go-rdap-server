package rdap

import (
	"strings"
	"testing"
	"time"
)

func TestNormalizeDomainNameASCII(t *testing.T) {
	ldh, unicode, err := NormalizeDomainName("Example.COM")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ldh != "example.com" || unicode != "example.com" {
		t.Fatalf("got %q/%q, want example.com/example.com", ldh, unicode)
	}
}

func TestNormalizeDomainNameIDN(t *testing.T) {
	// bücher.com -> xn--bcher-kva.com
	ldh, unicode, err := NormalizeDomainName("bücher.com")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ldh != "xn--bcher-kva.com" {
		t.Fatalf("ldh = %q, want xn--bcher-kva.com", ldh)
	}
	if unicode != "bücher.com" {
		t.Fatalf("unicode = %q, want bücher.com", unicode)
	}
}

func TestNormalizeDomainNameTrailingDot(t *testing.T) {
	ldh, _, err := NormalizeDomainName("example.com.")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ldh != "example.com" {
		t.Fatalf("ldh = %q, want example.com", ldh)
	}
}

func TestNormalizeDomainNameInvalid(t *testing.T) {
	for _, bad := range []string{"", "-bad.com", "bad-.com", "exa mple.com", "a..b"} {
		if _, _, err := NormalizeDomainName(bad); err == nil {
			t.Fatalf("expected error for %q", bad)
		}
	}
}

func TestFormatTime(t *testing.T) {
	if got := FormatTime(time.Time{}); got != "" {
		t.Fatalf("zero time should be empty, got %q", got)
	}
	ts := time.Date(2026, 8, 19, 12, 0, 0, 0, time.UTC)
	if got := FormatTime(ts); got != "2026-08-19T12:00:00Z" {
		t.Fatalf("FormatTime = %q", got)
	}
	// Non-UTC input should be converted to UTC.
	local := time.Date(2026, 8, 19, 14, 0, 0, 0, time.FixedZone("+2", 2*3600))
	if got := FormatTime(local); got != "2026-08-19T12:00:00Z" {
		t.Fatalf("FormatTime(non-UTC) = %q, want 2026-08-19T12:00:00Z", got)
	}
}

func TestConformanceVersions(t *testing.T) {
	if c := NewConformance(); len(c.Conformance) != 1 || c.Conformance[0] != "rdap_level_0" {
		t.Fatalf("NewConformance = %v", c.Conformance)
	}
	has := func(confs []string, want string) bool {
		for _, c := range confs {
			if c == want {
				return true
			}
		}
		return false
	}
	c2019 := NewConformance2019()
	if !has(c2019.Conformance, "icann_rdap_response_profile_0") || !has(c2019.Conformance, "icann_rdap_technical_implementation_guide_0") {
		t.Fatalf("2019 conformance missing profile/TIG 0: %v", c2019.Conformance)
	}
	c2024 := NewConformance2024()
	if !has(c2024.Conformance, "icann_rdap_response_profile_1") || !has(c2024.Conformance, "icann_rdap_technical_implementation_guide_1") {
		t.Fatalf("2024 conformance missing profile/TIG 1: %v", c2024.Conformance)
	}
}

func TestNewNoticesWithICANNPreservesMandatory(t *testing.T) {
	notices := NewNoticesWithICANN("https://rdap.example.com/domain/example.com", "https://rdap.example.com", nil)
	titles := map[string]bool{}
	for _, n := range notices {
		titles[n.Title] = true
	}
	for _, want := range []string{"Terms of Service", "Status Codes", "RDDS Inaccuracy Complaint Form"} {
		if !titles[want] {
			t.Fatalf("mandatory notice %q missing; got %v", want, titles)
		}
	}
}

func TestNewNoticesWithICANNCustomToSAndNotices(t *testing.T) {
	opts := &NoticeOptions{
		ToSTitle:       "My ToS",
		ToSDescription: []string{"Provided by Example Registrar, Inc."},
		ToSURL:         "https://example.com/terms",
		Custom: []CustomNotice{
			{Title: "Data Policy", Description: []string{"Policy"}, URL: "https://example.com/policy", Rel: "privacy-policy"},
		},
	}
	notices := NewNoticesWithICANN("https://rdap.example.com/domain/example.com", "https://rdap.example.com", opts)

	var tosFound, customFound bool
	for _, n := range notices {
		switch n.Title {
		case "My ToS":
			tosFound = true
			if len(n.Description) != 1 || n.Description[0] != "Provided by Example Registrar, Inc." {
				t.Fatalf("ToS description = %v", n.Description)
			}
			if n.Links[0].Href != "https://example.com/terms" {
				t.Fatalf("ToS href = %q", n.Links[0].Href)
			}
			if n.Links[0].Rel != "terms-of-service" {
				t.Fatalf("ToS rel = %q", n.Links[0].Rel)
			}
		case "Data Policy":
			customFound = true
			if n.Links[0].Rel != "privacy-policy" {
				t.Fatalf("custom rel = %q", n.Links[0].Rel)
			}
		}
	}
	if !tosFound {
		t.Fatal("custom ToS notice not present")
	}
	if !customFound {
		t.Fatal("custom notice not present")
	}
}

func TestNewHelpNoticesGenericToSAndRateLimits(t *testing.T) {
	// Help should use a generic ToS even when a domain-specific ToS is configured.
	opts := &NoticeOptions{
		ToSDescription: []string{"Registration data for example.com ..."},
	}
	rate := RateLimitInfo{Enabled: true, Requests: 100, Window: time.Minute, Burst: 50}
	notices := NewHelpNotices("https://rdap.example.com", opts, rate, SearchInfo{Enabled: true})

	var tosDesc, rateDesc string
	for _, n := range notices {
		if n.Title == "Terms of Service" && len(n.Description) > 0 {
			tosDesc = n.Description[0]
		}
		if n.Title == "Rate Limiting" && len(n.Description) > 0 {
			rateDesc = n.Description[0]
		}
	}
	// Generic ToS, not the domain-specific one.
	if tosDesc == "Registration data for example.com ..." {
		t.Fatalf("help used domain-specific ToS text: %q", tosDesc)
	}
	if tosDesc == "" {
		t.Fatal("help ToS description empty")
	}
	// Rate limit should be documented.
	if rateDesc == "" || rateDesc == "Access to this RDAP server is rate-limited. Excessive queries may be throttled." {
		t.Fatalf("help rate-limit notice does not document the limit: %q", rateDesc)
	}
}

func TestNewHelpNoticesSearchDisabled(t *testing.T) {
	// When searches are disabled, /help must advertise that fact.
	notices := NewHelpNotices("https://rdap.example.com", nil, RateLimitInfo{}, SearchInfo{Enabled: false})
	found := false
	for _, n := range notices {
		if n.Title == "Search Disabled" && len(n.Description) > 0 {
			found = true
		}
	}
	if !found {
		t.Fatal("expected a 'Search Disabled' notice when searches are off")
	}

	// When searches are enabled, no such notice.
	notices = NewHelpNotices("https://rdap.example.com", nil, RateLimitInfo{}, SearchInfo{Enabled: true})
	for _, n := range notices {
		if n.Title == "Search Disabled" {
			t.Fatal("unexpected 'Search Disabled' notice when searches are enabled")
		}
	}
}

func TestExtractTLD(t *testing.T) {
	cases := map[string]string{
		"example.com":  "com",
		"a.b.co.uk":    "uk",
		"example":      "",
		"example.com.": "com",
	}
	for in, want := range cases {
		if got := ExtractTLD(in); got != want {
			t.Fatalf("ExtractTLD(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestValidateIP(t *testing.T) {
	if !ValidateIP("8.8.8.8", 4) {
		t.Error("8.8.8.8 should be valid IPv4")
	}
	if ValidateIP("8.8.8.8", 6) {
		t.Error("8.8.8.8 should not be valid IPv6")
	}
	if !ValidateIP("2001:4860:4860::8888", 6) {
		t.Error("2001:4860:4860::8888 should be valid IPv6")
	}
	if ValidateIP("not-an-ip", 4) {
		t.Error("not-an-ip should be invalid")
	}
	// IPv4-mapped IPv6 is accepted as v4.
	if !ValidateIP("::ffff:8.8.8.8", 4) {
		t.Error("::ffff:8.8.8.8 should be valid IPv4 (4-in-6)")
	}
}

func TestValidateDomainName(t *testing.T) {
	for _, ok := range []string{"example.com", "a.b.co.uk", "xn--bcher-kva.com"} {
		if err := ValidateDomainName(ok); err != nil {
			t.Errorf("ValidateDomainName(%q): %v", ok, err)
		}
	}
	for _, bad := range []string{"", "singlelabel", "a..b", ".leading", "trailing.", strings.Repeat("a", 254), strings.Repeat("a", 64) + ".com"} {
		if err := ValidateDomainName(bad); err == nil {
			t.Errorf("ValidateDomainName(%q): expected error", bad)
		}
	}
}

func TestValidateLDHName(t *testing.T) {
	if !ValidateLDHName("example.com") {
		t.Error("example.com should be valid LDH")
	}
	for _, bad := range []string{"", "-bad.com", "bad-.com", "exa mple.com", "a..b", strings.Repeat("a", 254)} {
		if ValidateLDHName(bad) {
			t.Errorf("ValidateLDHName(%q): expected false", bad)
		}
	}
}

func TestIsTLDSupported(t *testing.T) {
	if !IsTLDSupported("com", []string{"com", "net"}) {
		t.Error("com should be supported")
	}
	if !IsTLDSupported("COM", []string{"com", "net"}) {
		t.Error("COM should match case-insensitively")
	}
	if IsTLDSupported("org", []string{"com", "net"}) {
		t.Error("org should not be supported")
	}
	if IsTLDSupported("com", nil) {
		t.Error("empty TLD list should support nothing")
	}
}

func TestParseCIDR(t *testing.T) {
	cidr, err := ParseCIDR("8.8.8.0/24")
	if err != nil {
		t.Fatalf("ParseCIDR: %v", err)
	}
	if cidr.String() != "8.8.8.0/24" {
		t.Errorf("cidr = %q", cidr.String())
	}
	if _, err := ParseCIDR("not-a-cidr"); err == nil {
		t.Error("expected error for invalid CIDR")
	}
}

func TestNewNotices(t *testing.T) {
	notices := NewNotices("https://rdap.example.com", nil)
	titles := map[string]bool{}
	for _, n := range notices {
		titles[n.Title] = true
	}
	if !titles["Terms of Service"] {
		t.Error("missing ToS notice")
	}
	if !titles["Rate Limiting"] {
		t.Error("missing Rate Limiting notice")
	}
}

func TestNewHelp(t *testing.T) {
	h := NewHelp("https://rdap.example.com", nil, RateLimitInfo{}, SearchInfo{Enabled: false})
	if len(h.Conformance.Conformance) == 0 {
		t.Error("help conformance empty")
	}
	found := false
	for _, n := range h.Notices {
		if n.Title == "Search Disabled" {
			found = true
		}
	}
	if !found {
		t.Error("NewHelp with disabled search should include Search Disabled notice")
	}
}

func TestNewHelpNoticesCustomAndBurst(t *testing.T) {
	// Custom notice with empty URL and empty rel -> defaults applied.
	opts := &NoticeOptions{Custom: []CustomNotice{
		{Title: "Data Policy", Description: []string{"Policy"}},
	}}
	notices := NewHelpNotices("https://rdap.example.com", opts, RateLimitInfo{
		Enabled: true, Requests: 100, Window: 0, Burst: 50, // zero window -> defaults to a minute
	}, SearchInfo{Enabled: true})

	var customFound bool
	for _, n := range notices {
		if n.Title == "Data Policy" {
			customFound = true
			if n.Links[0].Href != "https://rdap.example.com/help" {
				t.Errorf("custom href = %q (expected default /help)", n.Links[0].Href)
			}
			if n.Links[0].Rel != "terms-of-service" {
				t.Errorf("custom rel = %q (expected default)", n.Links[0].Rel)
			}
		}
	}
	if !customFound {
		t.Error("custom notice not present")
	}

	// burstSuffix with a positive burst.
	if got := burstSuffix(50); got != ", with a burst allowance of 50" {
		t.Errorf("burstSuffix(50) = %q", got)
	}
	if got := burstSuffix(0); got != "" {
		t.Errorf("burstSuffix(0) = %q", got)
	}
}

func TestNewNoticesWithICANNCustomNoURL(t *testing.T) {
	opts := &NoticeOptions{Custom: []CustomNotice{
		{Title: "Custom", Description: []string{"Desc"}},
	}}
	notices := NewNoticesWithICANN("https://rdap.example.com/domain/example.com", "https://rdap.example.com", opts)
	for _, n := range notices {
		if n.Title == "Custom" {
			if n.Links[0].Href != "https://rdap.example.com/help" {
				t.Errorf("custom href = %q, want default /help", n.Links[0].Href)
			}
			if n.Links[0].Rel != "terms-of-service" {
				t.Errorf("custom rel = %q, want default", n.Links[0].Rel)
			}
		}
	}
}

func TestNormalizeDomainNameIDNError(t *testing.T) {
	// A label that is far too long should fail IDNA conversion.
	long := strings.Repeat("é", 64) + ".com"
	if _, _, err := NormalizeDomainName(long); err == nil {
		t.Error("expected error for over-long unicode label")
	}
}
