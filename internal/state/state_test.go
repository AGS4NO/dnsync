package state

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ags4no/dnsync/internal/config"
)

func TestNew(t *testing.T) {
	f := New()
	if f.Version != currentVersion {
		t.Errorf("expected version %d, got %d", currentVersion, f.Version)
	}
	if len(f.Zones) != 0 {
		t.Errorf("expected empty zones, got %d", len(f.Zones))
	}
}

func TestParse(t *testing.T) {
	data := []byte(`{
  "version": 1,
  "zones": {
    "example.com": {
      "manage": "partial",
      "records": [
        {"name": "www", "type": "A", "content": "192.0.2.1", "ttl": 300}
      ]
    }
  }
}`)
	f, err := Parse(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if f.Version != 1 {
		t.Errorf("expected version 1, got %d", f.Version)
	}
	z, ok := f.Zones["example.com"]
	if !ok {
		t.Fatal("expected example.com zone")
	}
	if len(z.Records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(z.Records))
	}
	if z.Records[0].Content != "192.0.2.1" {
		t.Errorf("expected content 192.0.2.1, got %s", z.Records[0].Content)
	}
}

func TestParse_EmptyZones(t *testing.T) {
	data := []byte(`{"version": 1}`)
	f, err := Parse(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if f.Zones == nil {
		t.Error("expected non-nil zones map")
	}
}

func TestLoad_FileNotFound(t *testing.T) {
	f, err := Load("/nonexistent/state.json")
	if err != nil {
		t.Fatalf("expected no error for missing file, got: %v", err)
	}
	if f.Version != currentVersion {
		t.Errorf("expected fresh state with version %d", currentVersion)
	}
}

func TestSaveAndLoad(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".dnsync.state.json")

	f := New()
	f.Zones["example.com"] = &Zone{
		Manage: config.ManagePartial,
		Records: []Record{
			{Name: "www", Type: "A", Content: "192.0.2.1", TTL: 300},
			{Name: "api", Type: "A", Content: "192.0.2.2", TTL: 300},
		},
	}

	if err := f.Save(path); err != nil {
		t.Fatalf("save error: %v", err)
	}

	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("load error: %v", err)
	}
	z := loaded.Zones["example.com"]
	if z == nil {
		t.Fatal("expected example.com zone in loaded state")
	}
	if len(z.Records) != 2 {
		t.Errorf("expected 2 records, got %d", len(z.Records))
	}
	// Verify sorted order (api < www)
	if z.Records[0].Name != "api" {
		t.Errorf("expected sorted order, first record should be api, got %s", z.Records[0].Name)
	}
}

func TestUpdateFromConfig(t *testing.T) {
	f := New()
	// Pre-existing zone that will be removed
	f.Zones["old.com"] = &Zone{
		Manage:  config.ManageFull,
		Records: []Record{{Name: "www", Type: "A", Content: "1.2.3.4", TTL: 300}},
	}

	cfg := &config.Config{
		Zones: []config.ZoneConfig{
			{
				Zone:   "example.com",
				Manage: config.ManagePartial,
				Records: []config.Record{
					{Name: "@", Type: "A", Content: "192.0.2.1", TTL: 3600},
					{Name: "www", Type: "CNAME", Content: "example.com", TTL: 3600},
				},
			},
		},
	}

	f.UpdateFromConfig(cfg)

	if _, ok := f.Zones["old.com"]; ok {
		t.Error("expected old.com to be removed from state")
	}
	z, ok := f.Zones["example.com"]
	if !ok {
		t.Fatal("expected example.com in state")
	}
	if len(z.Records) != 2 {
		t.Errorf("expected 2 records, got %d", len(z.Records))
	}
	// @ should be normalized to ""
	if z.Records[0].Name != "" && z.Records[1].Name != "" {
		t.Error("expected apex record name to be normalized to empty string")
	}
}

func TestGetZoneRecords_Exists(t *testing.T) {
	f := New()
	f.Zones["example.com"] = &Zone{
		Records: []Record{
			{Name: "www", Type: "A", Content: "1.2.3.4", TTL: 300},
		},
	}
	recs := f.GetZoneRecords("example.com")
	if len(recs) != 1 {
		t.Errorf("expected 1 record, got %d", len(recs))
	}
}

func TestGetZoneRecords_NotExists(t *testing.T) {
	f := New()
	recs := f.GetZoneRecords("missing.com")
	if recs != nil {
		t.Errorf("expected nil for missing zone, got %v", recs)
	}
}

func TestRecordKey(t *testing.T) {
	r := Record{Name: "www", Type: "A", Content: "1.2.3.4"}
	if got := r.RecordKey(); got != "www/A" {
		t.Errorf("RecordKey() = %q, want %q", got, "www/A")
	}
}

func TestContentKey(t *testing.T) {
	r := Record{Name: "www", Type: "A", Content: "1.2.3.4"}
	if got := r.ContentKey(); got != "www/A/1.2.3.4" {
		t.Errorf("ContentKey() = %q, want %q", got, "www/A/1.2.3.4")
	}
}

func TestSave_DeterministicOrder(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")

	f := New()
	f.Zones["example.com"] = &Zone{
		Manage: config.ManagePartial,
		Records: []Record{
			{Name: "zzz", Type: "A", Content: "1.1.1.1", TTL: 300},
			{Name: "aaa", Type: "A", Content: "2.2.2.2", TTL: 300},
			{Name: "aaa", Type: "MX", Content: "mail.example.com", TTL: 300},
		},
	}

	if err := f.Save(path); err != nil {
		t.Fatal(err)
	}

	data1, _ := os.ReadFile(path)

	// Save again — should produce identical output
	loaded, _ := Load(path)
	if err := loaded.Save(path); err != nil {
		t.Fatal(err)
	}

	data2, _ := os.ReadFile(path)

	if string(data1) != string(data2) {
		t.Error("expected deterministic output across saves")
	}
}
