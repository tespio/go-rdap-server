package rdap

import (
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
}
