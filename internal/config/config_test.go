package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParse_ValidMultiZone(t *testing.T) {
	data := []byte(`
zones:
  - zone: example.com
    manage: full
    records:
      - name: "@"
        type: A
        content: 192.0.2.1
        ttl: 3600
      - name: www
        type: CNAME
        content: example.com
        ttl: 3600
  - zone: other.org
    manage: partial
    records:
      - name: api
        type: A
        content: 203.0.113.5
        ttl: 300
`)
	cfg, err := Parse(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cfg.Zones) != 2 {
		t.Fatalf("expected 2 zones, got %d", len(cfg.Zones))
	}
	if cfg.Zones[0].Zone != "example.com" {
		t.Errorf("expected zone example.com, got %s", cfg.Zones[0].Zone)
	}
	if cfg.Zones[0].Manage != ManageFull {
		t.Errorf("expected manage full, got %s", cfg.Zones[0].Manage)
	}
	if len(cfg.Zones[0].Records) != 2 {
		t.Errorf("expected 2 records, got %d", len(cfg.Zones[0].Records))
	}
	if cfg.Zones[1].Manage != ManagePartial {
		t.Errorf("expected manage partial, got %s", cfg.Zones[1].Manage)
	}
}

func TestParse_DefaultManageMode(t *testing.T) {
	data := []byte(`
zones:
  - zone: example.com
    records:
      - name: www
        type: A
        content: 192.0.2.1
        ttl: 300
`)
	cfg, err := Parse(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Zones[0].Manage != ManagePartial {
		t.Errorf("expected default manage mode partial, got %s", cfg.Zones[0].Manage)
	}
}

func TestParse_NoZones(t *testing.T) {
	data := []byte(`zones: []`)
	_, err := Parse(data)
	if err == nil {
		t.Fatal("expected error for empty zones")
	}
}

func TestParse_DuplicateZone(t *testing.T) {
	data := []byte(`
zones:
  - zone: example.com
    records:
      - name: www
        type: A
        content: 192.0.2.1
  - zone: example.com
    records:
      - name: api
        type: A
        content: 192.0.2.2
`)
	_, err := Parse(data)
	if err == nil {
		t.Fatal("expected error for duplicate zone")
	}
}

func TestParse_MissingType(t *testing.T) {
	data := []byte(`
zones:
  - zone: example.com
    records:
      - name: www
        content: 192.0.2.1
`)
	_, err := Parse(data)
	if err == nil {
		t.Fatal("expected error for missing type")
	}
}

func TestParse_MissingContent(t *testing.T) {
	data := []byte(`
zones:
  - zone: example.com
    records:
      - name: www
        type: A
`)
	_, err := Parse(data)
	if err == nil {
		t.Fatal("expected error for missing content")
	}
}

func TestParse_InvalidManageMode(t *testing.T) {
	data := []byte(`
zones:
  - zone: example.com
    manage: yolo
    records:
      - name: www
        type: A
        content: 192.0.2.1
`)
	_, err := Parse(data)
	if err == nil {
		t.Fatal("expected error for invalid manage mode")
	}
}

func TestParse_DuplicateRecordSingleValue(t *testing.T) {
	data := []byte(`
zones:
  - zone: example.com
    records:
      - name: www
        type: A
        content: 192.0.2.1
      - name: www
        type: A
        content: 192.0.2.2
`)
	_, err := Parse(data)
	if err == nil {
		t.Fatal("expected error for duplicate A record")
	}
}

func TestParse_DuplicateMXAllowed(t *testing.T) {
	data := []byte(`
zones:
  - zone: example.com
    records:
      - name: "@"
        type: MX
        content: mail1.example.com
        priority: 10
      - name: "@"
        type: MX
        content: mail2.example.com
        priority: 20
`)
	_, err := Parse(data)
	if err != nil {
		t.Fatalf("MX duplicates should be allowed: %v", err)
	}
}

func TestRecord_NormalizedName(t *testing.T) {
	tests := []struct {
		name     string
		expected string
	}{
		{"@", ""},
		{"www", "www"},
		{"sub.domain", "sub.domain"},
		{"", ""},
	}
	for _, tt := range tests {
		r := Record{Name: tt.name, Type: "A", Content: "1.2.3.4"}
		if got := r.NormalizedName(); got != tt.expected {
			t.Errorf("NormalizedName(%q) = %q, want %q", tt.name, got, tt.expected)
		}
	}
}

func TestRecord_RecordKey(t *testing.T) {
	r := Record{Name: "www", Type: "cname", Content: "example.com"}
	if got := r.RecordKey(); got != "www/CNAME" {
		t.Errorf("RecordKey() = %q, want %q", got, "www/CNAME")
	}
}

func TestLoad_FromFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "dns.yaml")
	content := []byte(`
zones:
  - zone: example.com
    manage: full
    records:
      - name: www
        type: A
        content: 192.0.2.1
        ttl: 300
`)
	if err := os.WriteFile(path, content, 0644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cfg.Zones) != 1 {
		t.Fatalf("expected 1 zone, got %d", len(cfg.Zones))
	}
}

func TestLoad_FileNotFound(t *testing.T) {
	_, err := Load("/nonexistent/path/dns.yaml")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}
