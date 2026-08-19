package store

import (
	"encoding/json"

	"github.com/tespio/go-rdap-server/internal/domain"
)

// nsJSON is the shape of the JSONB "nameservers" column stored on a domain row.
// It carries full nameserver objects (as produced by the example schema).
type nsJSON struct {
	Handle      string   `json:"handle"`
	LDHName     string   `json:"ldhName"`
	UnicodeName string   `json:"unicodeName"`
	IPV4        []string `json:"ipv4"`
	IPV6        []string `json:"ipv6"`
	Status      []string `json:"status"`
}

// secureDNSJSON is the shape of the JSONB "secure_dns" column.
type secureDNSJSON struct {
	ZoneSigned       bool `json:"zoneSigned"`
	DelegationSigned bool `json:"delegationSigned"`
	MaxSigLife       *int `json:"maxSigLife"`
	DSData           []struct {
		KeyTag     int    `json:"keyTag"`
		Algorithm  int    `json:"algorithm"`
		DigestType int    `json:"digestType"`
		Digest     string `json:"digest"`
	} `json:"dsData"`
	KeyData []struct {
		Flags     int    `json:"flags"`
		Protocol  int    `json:"protocol"`
		Algorithm int    `json:"algorithm"`
		PublicKey string `json:"publicKey"`
	} `json:"keyData"`
}

// parseStatus converts a JSON array of status strings into domain.Status values.
func parseStatus(statusJSON []byte) []domain.Status {
	var vals []string
	if err := json.Unmarshal(statusJSON, &vals); err != nil {
		return nil
	}
	out := make([]domain.Status, 0, len(vals))
	for _, v := range vals {
		out = append(out, domain.Status{Value: v})
	}
	return out
}

// parseNameservers converts a JSON array of nameserver objects into domain.NameServer values.
func parseNameservers(raw []byte) []domain.NameServer {
	var items []nsJSON
	if err := json.Unmarshal(raw, &items); err != nil {
		return nil
	}
	out := make([]domain.NameServer, 0, len(items))
	for _, n := range items {
		out = append(out, domain.NameServer{
			Handle:      n.Handle,
			LDHName:     n.LDHName,
			UnicodeName: n.UnicodeName,
			IPV4:        n.IPV4,
			IPV6:        n.IPV6,
			Status:      parseStatusVal(n.Status),
		})
	}
	return out
}

func parseStatusVal(vals []string) []domain.Status {
	out := make([]domain.Status, 0, len(vals))
	for _, v := range vals {
		out = append(out, domain.Status{Value: v})
	}
	return out
}

// parseSecureDNS converts a JSON secure_dns blob into a domain.SecureDNS.
func parseSecureDNS(sdJSON []byte) *domain.SecureDNS {
	if sdJSON == nil || len(sdJSON) == 0 {
		return nil
	}
	var s secureDNSJSON
	if err := json.Unmarshal(sdJSON, &s); err != nil {
		return nil
	}
	out := &domain.SecureDNS{
		ZoneSigned:       s.ZoneSigned,
		DelegationSigned: s.DelegationSigned,
		MaxSigLife:       s.MaxSigLife,
	}
	for _, ds := range s.DSData {
		out.DSRecords = append(out.DSRecords, domain.DSRecord{
			KeyTag:     ds.KeyTag,
			Algorithm:  ds.Algorithm,
			DigestType: ds.DigestType,
			Digest:     ds.Digest,
		})
	}
	for _, k := range s.KeyData {
		out.KeyRecords = append(out.KeyRecords, domain.KeyRecord{
			Flags:     k.Flags,
			Protocol:  k.Protocol,
			Algorithm: k.Algorithm,
			PublicKey: k.PublicKey,
		})
	}
	return out
}

// statusStrings flattens domain.Status values back into their string values.
func statusStrings(status []domain.Status) []string {
	out := make([]string, 0, len(status))
	for _, s := range status {
		out = append(out, s.Value)
	}
	return out
}

// parseVCardJSON parses a jCard array string into a domain.VCard. It is lenient:
// unknown properties are ignored, and any parse failure returns nil.
func parseVCardJSON(raw string) *domain.VCard {
	var arr []interface{}
	if err := json.Unmarshal([]byte(raw), &arr); err != nil {
		return nil
	}
	if len(arr) < 2 {
		return nil
	}
	// arr[0] should be "vcard"; arr[1] is the array of property arrays.
	propsRaw, ok := arr[1].([]interface{})
	if !ok {
		return nil
	}

	v := &domain.VCard{Version: "4.0"}
	var adr *domain.VCardAddress

	for _, p := range propsRaw {
		prop, ok := p.([]interface{})
		if !ok || len(prop) < 4 {
			continue
		}
		name, _ := prop[0].(string)
		// params at index 1, type at index 2, value at index 3+
		switch name {
		case "fn":
			if s, ok := prop[3].(string); ok {
				v.FullName = s
			}
		case "kind":
			if s, ok := prop[3].(string); ok {
				v.Kind = s
			}
		case "org":
			if s, ok := prop[3].(string); ok {
				v.Organization = s
			}
		case "tel":
			params, _ := prop[1].(map[string]interface{})
			if s, ok := prop[3].(string); ok {
				if t, _ := params["type"].(string); t == "fax" {
					v.FaxTel = s
				} else {
					v.VoiceTel = s
				}
			}
		case "email":
			if s, ok := prop[3].(string); ok {
				v.Email = s
			}
		case "contact-uri":
			if s, ok := prop[3].(string); ok {
				v.ContactURI = s
			}
		case "adr":
			params, _ := prop[1].(map[string]interface{})
			cc, _ := params["cc"].(string)
			adr = &domain.VCardAddress{CountryCode: cc}
			if vals, ok := prop[3].([]interface{}); ok && len(vals) >= 7 {
				adr.POBox = str(vals[0])
				adr.Extended = str(vals[1])
				adr.Street = str(vals[2])
				adr.Locality = str(vals[3])
				adr.Region = str(vals[4])
				adr.PostalCode = str(vals[5])
				adr.CountryName = str(vals[6])
			}
		}
	}

	if adr != nil {
		v.Address = adr
	}
	return v
}

func str(v interface{}) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}
