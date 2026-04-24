package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseBind_FullZone(t *testing.T) {
	data := []byte(`
$ORIGIN example.com.
$TTL 3600

@    IN  SOA   ns1.example.com. admin.example.com. 2024010101 3600 900 604800 86400
@    IN  A     192.0.2.1
www  IN  A     192.0.2.2
@    IN  AAAA  2001:db8::1
blog IN  CNAME www.example.com.
@    IN  MX 10 mail1.example.com.
@    IN  MX 20 mail2.example.com.
@    IN  TXT   "v=spf1 include:_spf.google.com ~all"
_sip._tcp IN SRV 10 60 5060 sip.example.com.
sub  IN  NS    ns1.example.com.
@    IN  CAA   0 issue "letsencrypt.org"
`)
	cfg, err := ParseBind(data, ManageFull)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cfg.Zones) != 1 {
		t.Fatalf("expected 1 zone, got %d", len(cfg.Zones))
	}
	zone := cfg.Zones[0]
	if zone.Zone != "example.com" {
		t.Errorf("expected zone example.com, got %s", zone.Zone)
	}
	if zone.Manage != ManageFull {
		t.Errorf("expected manage full, got %s", zone.Manage)
	}

	// Should have all records except SOA
	// A(2) + AAAA(1) + CNAME(1) + MX(2) + TXT(1) + SRV(1) + NS(1) + CAA(1) = 10
	if len(zone.Records) != 10 {
		t.Errorf("expected 10 records, got %d", len(zone.Records))
		for _, r := range zone.Records {
			t.Logf("  %s %s %s", r.Name, r.Type, r.Content)
		}
	}
}

func TestParseBind_RecordValues(t *testing.T) {
	data := []byte(`
$ORIGIN example.com.
$TTL 300

www  IN  A     192.0.2.1
@    IN  MX 10 mail.example.com.
@    IN  TXT   "hello world"
_sip._tcp IN SRV 5 60 5060 sip.example.com.
@    IN  CAA   0 issue "letsencrypt.org"
blog IN  CNAME www.example.com.
`)
	cfg, err := ParseBind(data, ManagePartial)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	records := cfg.Zones[0].Records

	// Find and verify each record
	tests := []struct {
		name     string
		typ      string
		content  string
		ttl      int
		priority int
	}{
		{"www", "A", "192.0.2.1", 300, 0},
		{"@", "MX", "mail.example.com", 300, 10},
		{"@", "TXT", "hello world", 300, 0},
		{"_sip._tcp", "SRV", "60 5060 sip.example.com", 300, 5},
		{"@", "CAA", "0 issue letsencrypt.org", 300, 0},
		{"blog", "CNAME", "www.example.com", 300, 0},
	}

	for _, tt := range tests {
		found := false
		for _, r := range records {
			if r.Name == tt.name && r.Type == tt.typ {
				found = true
				if r.Content != tt.content {
					t.Errorf("%s %s: content = %q, want %q", tt.name, tt.typ, r.Content, tt.content)
				}
				if r.TTL != tt.ttl {
					t.Errorf("%s %s: ttl = %d, want %d", tt.name, tt.typ, r.TTL, tt.ttl)
				}
				if r.Priority != tt.priority {
					t.Errorf("%s %s: priority = %d, want %d", tt.name, tt.typ, r.Priority, tt.priority)
				}
				break
			}
		}
		if !found {
			t.Errorf("record %s %s not found", tt.name, tt.typ)
		}
	}
}

func TestParseBind_ApexRecords(t *testing.T) {
	data := []byte(`
$ORIGIN example.com.
$TTL 3600

example.com.  IN  A  192.0.2.1
@             IN  A  192.0.2.2
`)
	// Both resolve to apex A records — validation rejects duplicate single-value records
	_, err := ParseBind(data, ManagePartial)
	if err == nil {
		t.Fatal("expected error for duplicate apex A records")
	}
}

func TestParseBind_DefaultManageMode(t *testing.T) {
	data := []byte(`
$ORIGIN example.com.
$TTL 3600

www  IN  A  192.0.2.1
`)
	cfg, err := ParseBind(data, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Zones[0].Manage != ManagePartial {
		t.Errorf("expected default manage mode partial, got %s", cfg.Zones[0].Manage)
	}
}

func TestParseBind_NoOrigin(t *testing.T) {
	data := []byte(`
$TTL 3600
www  IN  A  192.0.2.1
`)
	_, err := ParseBind(data, ManageFull)
	if err == nil {
		t.Fatal("expected error for missing origin")
	}
}

func TestParseBind_SOAOnly(t *testing.T) {
	data := []byte(`
$ORIGIN example.com.
$TTL 3600

@  IN  SOA  ns1.example.com. admin.example.com. 2024010101 3600 900 604800 86400
`)
	// SOA is skipped, so the zone has 0 records — this is valid (no records to manage)
	cfg, err := ParseBind(data, ManageFull)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cfg.Zones[0].Records) != 0 {
		t.Errorf("expected 0 records (SOA skipped), got %d", len(cfg.Zones[0].Records))
	}
}

func TestParseBind_RelativeNames(t *testing.T) {
	data := []byte(`
$ORIGIN example.com.
$TTL 3600

sub.domain  IN  A  192.0.2.1
deep.sub.domain  IN  A  192.0.2.2
`)
	cfg, err := ParseBind(data, ManagePartial)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	records := cfg.Zones[0].Records
	if len(records) != 2 {
		t.Fatalf("expected 2 records, got %d", len(records))
	}
	if records[0].Name != "sub.domain" {
		t.Errorf("expected name sub.domain, got %s", records[0].Name)
	}
	if records[1].Name != "deep.sub.domain" {
		t.Errorf("expected name deep.sub.domain, got %s", records[1].Name)
	}
}

func TestLoadBind_FromFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.zone")
	content := []byte(`
$ORIGIN example.com.
$TTL 3600

www  IN  A  192.0.2.1
`)
	if err := os.WriteFile(path, content, 0644); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadBind(path, ManageFull)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cfg.Zones) != 1 {
		t.Fatalf("expected 1 zone, got %d", len(cfg.Zones))
	}
	if cfg.Zones[0].Zone != "example.com" {
		t.Errorf("expected zone example.com, got %s", cfg.Zones[0].Zone)
	}
}

func TestLoadBind_FileNotFound(t *testing.T) {
	_, err := LoadBind("/nonexistent/path/test.zone", ManageFull)
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestRelativeName(t *testing.T) {
	tests := []struct {
		fqdn     string
		origin   string
		expected string
	}{
		{"example.com.", "example.com.", "@"},
		{"www.example.com.", "example.com.", "www"},
		{"sub.domain.example.com.", "example.com.", "sub.domain"},
		{"other.com.", "example.com.", "other.com"},
	}
	for _, tt := range tests {
		got := relativeName(tt.fqdn, tt.origin)
		if got != tt.expected {
			t.Errorf("relativeName(%q, %q) = %q, want %q", tt.fqdn, tt.origin, got, tt.expected)
		}
	}
}
