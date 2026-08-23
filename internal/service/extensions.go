package service

import (
	"github.com/tespio/go-rdap-server/internal/config"
	"github.com/tespio/go-rdap-server/internal/rdap"
)

// extensions carries the resolved, enabled RDAP extension settings so the
// mapping layer can emit extension-specific JSON members and conformance
// identifiers. It is derived once from config at Service construction.
//
// All extensions default to OFF. Enabling one appends its identifier to the
// rdapConformance array and (where applicable) emits the extension's members —
// which is exactly what ICANN conformance tooling will observe, so operators
// should verify their conformance posture after enabling any extension.
type extensions struct {
	ttl0        bool
	ttl0Domain  map[string]int
	ttl0NS      map[string]int
	ttl0Remarks []rdap.Remark

	geofeed1   bool
	geofeedURL string

	cidr0 bool

	reverseSearch bool
}

func extensionsFromConfig(cfg config.RDAPConfig) *extensions {
	e := &extensions{
		ttl0:          cfg.ExtensionsEnabled("ttl0"),
		geofeed1:      cfg.ExtensionsEnabled("geofeed1"),
		cidr0:         cfg.ExtensionsEnabled("cidr0"),
		reverseSearch: cfg.ExtensionsEnabled("reverse_search"),
	}
	if cfg.TTL0 != nil {
		e.ttl0Domain = cfg.TTL0.Domain
		e.ttl0NS = cfg.TTL0.Nameserver
		for _, r := range cfg.TTL0.Remarks {
			e.ttl0Remarks = append(e.ttl0Remarks, rdap.Remark{
				Title:       r.Title,
				Description: r.Description,
			})
		}
	}
	if cfg.Geofeed != nil {
		e.geofeedURL = cfg.Geofeed.URL
	}
	return e
}
