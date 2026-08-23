package whois

import (
	"context"
	"strings"

	"github.com/tespio/go-rdap-server/internal/domain"
	"github.com/tespio/go-rdap-server/internal/store"
)

// StoreLookup returns a LookupFunc backed by an RDAP store. It resolves a
// domain via the store's transactional aggregate and adapts it to the WHOIS
// renderer's shape.
func StoreLookup(st store.Interface) LookupFunc {
	return func(ctx context.Context, name string) (WhoisDomain, error) {
		agg, err := st.GetDomainAggregate(strings.ToLower(strings.TrimSuffix(name, ".")))
		if err != nil {
			return WhoisDomain{}, ErrNotFound
		}
		return domainToWhois(agg), nil
	}
}

// domainToWhois adapts a domain aggregate to the WHOIS renderer shape.
func domainToWhois(agg *domain.DomainAggregate) WhoisDomain {
	d := agg.Domain
	out := WhoisDomain{
		LDHName:     d.LDHName,
		UnicodeName: d.UnicodeName,
		Status:      statusStrings(d.Status),
		CreatedAt:   d.Metadata.CreatedAt,
		UpdatedAt:   d.Metadata.UpdatedAt,
		ExpiresAt:   d.ExpiresAt,
		DNSSEC:      d.SecureDNS != nil && d.SecureDNS.DelegationSigned,
	}

	// Registrar from the resolved aggregate.
	if agg.Registrar != nil {
		out.Registrar = contactDisplayName(agg.Registrar)
		if len(agg.Registrar.PublicIDs) > 0 {
			out.RegistrarID = agg.Registrar.PublicIDs[0].Identifier
		}
	}

	// Contacts by role.
	for role, handles := range d.Contacts {
		for _, h := range handles {
			if c, ok := agg.Contacts[h]; ok {
				out.Contacts = append(out.Contacts, contactToWhois(role, c))
			}
		}
	}
	// Registrant name for the top-level field.
	for _, c := range out.Contacts {
		if strings.EqualFold(c.Role, "registrant") && c.Name != "" {
			out.Registrant = c.Name
			break
		}
	}

	// Nameservers.
	for _, ns := range d.Nameservers {
		wn := WhoisNameserver{Name: ns.LDHName, IPV4: ns.IPV4, IPV6: ns.IPV6}
		out.Nameservers = append(out.Nameservers, wn)
	}

	return out
}

func contactToWhois(role domain.ContactRole, c *domain.Contact) WhoisContact {
	vc := c.VCard
	wc := WhoisContact{
		Role:     string(role),
		Redacted: c.Privacy != "" && c.Privacy != domain.PrivacyPublic,
	}
	if vc != nil {
		wc.Name = vc.FullName
		wc.Org = vc.Organization
		wc.Email = vc.Email
		if wc.Email == "" {
			wc.Email = vc.ContactURI
		}
		wc.Phone = vc.VoiceTel
		if vc.Address != nil {
			var parts []string
			for _, p := range []string{vc.Address.Street, vc.Address.Locality, vc.Address.Region, vc.Address.PostalCode, vc.Address.CountryName} {
				if p != "" {
					parts = append(parts, p)
				}
			}
			wc.Address = strings.Join(parts, ", ")
		}
	}
	if wc.Name == "" {
		wc.Name = c.Handle
	}
	return wc
}

func contactDisplayName(c *domain.Contact) string {
	if c.VCard != nil && c.VCard.FullName != "" {
		return c.VCard.FullName
	}
	if c.VCard != nil && c.VCard.Organization != "" {
		return c.VCard.Organization
	}
	return c.Handle
}

func statusStrings(status []domain.Status) []string {
	out := make([]string, 0, len(status))
	for _, s := range status {
		if s.Value != "" {
			out = append(out, s.Value)
		}
	}
	return out
}
