package config

import (
	"fmt"
	"os"
	"strings"

	"github.com/miekg/dns"
)

// LoadBind reads and parses a BIND zone file from the given path.
// The zone name is extracted from the $ORIGIN directive.
// Since BIND files have no manage mode concept, it must be provided.
func LoadBind(path string, manageMode ManageMode) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading bind zone file: %w", err)
	}
	return ParseBind(data, manageMode)
}

// ParseBind parses a BIND zone file from raw bytes.
func ParseBind(data []byte, manageMode ManageMode) (*Config, error) {
	zp := dns.NewZoneParser(strings.NewReader(string(data)), "", "")

	var origin string
	var records []Record

	for rr, ok := zp.Next(); ok; rr, ok = zp.Next() {
		// Capture origin from the parser after first record
		if origin == "" {
			origin = zp.Comment() // won't help, but we extract from header
		}

		rec, err := rrToRecord(rr)
		if err != nil {
			continue // skip unsupported record types
		}
		records = append(records, rec)
	}

	if err := zp.Err(); err != nil {
		return nil, fmt.Errorf("parsing bind zone file: %w", err)
	}

	// Extract origin from the records — all names should be FQDNs under the same zone
	// We need to find origin from $ORIGIN or infer from SOA record
	origin = findOrigin(data)
	if origin == "" {
		return nil, fmt.Errorf("bind zone file must contain an $ORIGIN directive or SOA record")
	}

	// Normalize record names: convert FQDNs to relative names
	for i := range records {
		records[i].Name = relativeName(records[i].Name, origin)
	}

	if manageMode == "" {
		manageMode = ManagePartial
	}

	cfg := &Config{
		Zones: []ZoneConfig{
			{
				Zone:    strings.TrimSuffix(origin, "."),
				Manage:  manageMode,
				Records: records,
			},
		},
	}

	if err := validate(cfg); err != nil {
		return nil, err
	}

	return cfg, nil
}

// findOrigin extracts the zone origin from $ORIGIN directive or SOA record.
func findOrigin(data []byte) string {
	// First try to find $ORIGIN directive
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(strings.ToUpper(line), "$ORIGIN") {
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				return parts[1]
			}
		}
	}

	// Fall back to SOA record owner name
	zp := dns.NewZoneParser(strings.NewReader(string(data)), "", "")
	for rr, ok := zp.Next(); ok; rr, ok = zp.Next() {
		if soa, ok := rr.(*dns.SOA); ok {
			return soa.Hdr.Name
		}
	}

	return ""
}

// relativeName converts an FQDN to a relative name within the zone.
// "www.example.com." in zone "example.com." becomes "www".
// "example.com." in zone "example.com." becomes "@".
func relativeName(fqdn, origin string) string {
	// Ensure both have trailing dots for comparison
	if !strings.HasSuffix(fqdn, ".") {
		fqdn += "."
	}
	if !strings.HasSuffix(origin, ".") {
		origin += "."
	}

	if strings.EqualFold(fqdn, origin) {
		return "@"
	}

	// Strip the origin suffix
	suffix := "." + origin
	if strings.HasSuffix(fqdn, suffix) {
		return strings.TrimSuffix(fqdn, suffix)
	}

	// If it doesn't end with the origin, return as-is without trailing dot
	return strings.TrimSuffix(fqdn, ".")
}

// rrToRecord converts a dns.RR to a config.Record.
func rrToRecord(rr dns.RR) (Record, error) {
	hdr := rr.Header()
	rec := Record{
		Name: hdr.Name,
		Type: dns.TypeToString[hdr.Rrtype],
		TTL:  int(hdr.Ttl),
	}

	switch v := rr.(type) {
	case *dns.A:
		rec.Content = v.A.String()
	case *dns.AAAA:
		rec.Content = v.AAAA.String()
	case *dns.CNAME:
		rec.Content = strings.TrimSuffix(v.Target, ".")
	case *dns.MX:
		rec.Content = strings.TrimSuffix(v.Mx, ".")
		rec.Priority = int(v.Preference)
	case *dns.TXT:
		rec.Content = strings.Join(v.Txt, "")
	case *dns.SRV:
		rec.Content = fmt.Sprintf("%d %d %s", v.Weight, v.Port, strings.TrimSuffix(v.Target, "."))
		rec.Priority = int(v.Priority)
	case *dns.NS:
		rec.Content = strings.TrimSuffix(v.Ns, ".")
	case *dns.CAA:
		rec.Content = fmt.Sprintf("%d %s %s", v.Flag, v.Tag, v.Value)
	case *dns.SOA:
		// Skip SOA records — not managed by dnsync
		return Record{}, fmt.Errorf("skipping SOA record")
	default:
		return Record{}, fmt.Errorf("unsupported record type: %s", dns.TypeToString[hdr.Rrtype])
	}

	return rec, nil
}
