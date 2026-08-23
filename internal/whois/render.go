package whois

import (
	"fmt"
	"strings"
	"time"
)

// RenderDomain renders a domain object as RFC 3912-style plain text.
// It mirrors the fields traditional WHOIS servers return, derived from the same
// registry data the RDAP endpoint serves.
func RenderDomain(d WhoisDomain) string {
	var b strings.Builder

	write := func(k, v string) {
		if v != "" {
			fmt.Fprintf(&b, "%s: %s\r\n", k, v)
		}
	}

	fmt.Fprintf(&b, "Domain Name: %s\r\n", d.LDHName)
	if d.UnicodeName != "" && d.UnicodeName != d.LDHName {
		write("Internationalized Domain Name", d.UnicodeName)
	}
	write("Registrar", d.Registrar)
	if d.RegistrarID != "" {
		write("Registrar IANA ID", d.RegistrarID)
	}
	write("Registrant Organization", d.Registrant)

	// Contact block (registrant, admin, tech, billing).
	for _, c := range d.Contacts {
		label := roleLabel(c.Role)
		if label == "" {
			continue
		}
		fmt.Fprintf(&b, "\r\n%s\r\n", label)
		if c.Redacted {
			fmt.Fprintf(&b, "   %s: %s\r\n", "Name", "Redacted")
		} else {
			write("   Name", c.Name)
			write("   Organization", c.Org)
			if c.Address != "" {
				for _, line := range strings.Split(c.Address, "\n") {
					write("   Address", strings.TrimSpace(line))
				}
			}
			write("   Email", c.Email)
			write("   Phone", c.Phone)
		}
	}

	if len(d.Nameservers) > 0 {
		fmt.Fprintf(&b, "\r\nName Server:\r\n")
		for _, ns := range d.Nameservers {
			fmt.Fprintf(&b, "   %s\r\n", ns.Name)
			for _, ip := range ns.IPV4 {
				fmt.Fprintf(&b, "      %s\r\n", ip)
			}
		}
	}

	write("DNSSEC", dnssecString(d.DNSSEC))
	write("Creation Date", iso(d.CreatedAt))
	write("Updated Date", iso(d.UpdatedAt))
	write("Expiration Date", iso(d.ExpiresAt))

	write("Status", strings.Join(d.Status, " "))

	// Trailing newline per RFC 3912 (server closes connection after response).
	b.WriteString("\r\n")
	return b.String()
}

func roleLabel(role string) string {
	switch strings.ToLower(role) {
	case "registrant":
		return "Registrant:"
	case "administrative", "admin":
		return "Administrative Contact:"
	case "technical", "tech":
		return "Technical Contact:"
	case "billing":
		return "Billing Contact:"
	case "abuse":
		return "Abuse Contact:"
	default:
		return ""
	}
}

func dnssecString(signed bool) string {
	if signed {
		return "signedDelegation"
	}
	return "unsignedDelegation"
}

func iso(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format("2006-01-02T15:04:05Z")
}
