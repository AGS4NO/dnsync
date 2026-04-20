package audit

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ags4no/dnsync/internal/config"
	"github.com/ags4no/dnsync/internal/diff"
)

func TestNew(t *testing.T) {
	log := New()
	if len(log.Entries) != 0 {
		t.Errorf("expected empty entries, got %d", len(log.Entries))
	}
	if log.Description == "" {
		t.Error("expected description to be set")
	}
}

func TestLoad_FileNotFound(t *testing.T) {
	log, err := Load("/nonexistent/audit.json")
	if err != nil {
		t.Fatalf("expected no error for missing file, got: %v", err)
	}
	if len(log.Entries) != 0 {
		t.Errorf("expected empty log for missing file")
	}
}

func TestSaveAndLoad(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "audit.json")

	log := New()
	log.Entries = append(log.Entries, Entry{
		Timestamp: time.Date(2026, 4, 19, 12, 0, 0, 0, time.UTC),
		Action:    "apply",
		Zones: map[string]ZoneEntry{
			"example.com": {
				Manage: config.ManagePartial,
				Changes: []ChangeRecord{
					{Action: "create", Name: "www", Type: "A", Content: "192.0.2.1", TTL: 300},
				},
				Snapshot: []SnapshotRecord{
					{Name: "www", Type: "A", Content: "192.0.2.1", TTL: 300},
				},
			},
		},
	})

	if err := log.Save(path); err != nil {
		t.Fatalf("save error: %v", err)
	}

	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("load error: %v", err)
	}
	if len(loaded.Entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(loaded.Entries))
	}
	if loaded.Entries[0].Action != "apply" {
		t.Errorf("expected action apply, got %s", loaded.Entries[0].Action)
	}
	ze := loaded.Entries[0].Zones["example.com"]
	if len(ze.Changes) != 1 {
		t.Errorf("expected 1 change, got %d", len(ze.Changes))
	}
	if len(ze.Snapshot) != 1 {
		t.Errorf("expected 1 snapshot record, got %d", len(ze.Snapshot))
	}
}

func TestRecordApply(t *testing.T) {
	log := New()

	changesets := []diff.Changeset{
		{
			Zone:   "example.com",
			Manage: config.ManagePartial,
			Changes: []diff.Change{
				{
					Action: diff.ActionCreate,
					Zone:   "example.com",
					Record: config.Record{Name: "www", Type: "A", Content: "192.0.2.1", TTL: 300},
				},
				{
					Action:  diff.ActionUpdate,
					Zone:    "example.com",
					Record:  config.Record{Name: "api", Type: "A", Content: "192.0.2.2", TTL: 300},
					LiveID:  2,
					Current: &diff.LiveRecord{ID: 2, Name: "api", Type: "A", Content: "192.0.2.1", TTL: 600},
				},
			},
		},
	}
	liveByZone := map[string][]diff.LiveRecord{
		"example.com": {
			{ID: 2, Name: "api", Type: "A", Content: "192.0.2.1", TTL: 600},
			{ID: 3, Name: "other", Type: "A", Content: "10.0.0.1", TTL: 3600},
		},
	}
	cfg := &config.Config{
		Zones: []config.ZoneConfig{
			{Zone: "example.com", Manage: config.ManagePartial},
		},
	}

	log.RecordApply(changesets, liveByZone, cfg)

	if len(log.Entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(log.Entries))
	}
	entry := log.Entries[0]
	if entry.Action != "apply" {
		t.Errorf("expected action apply, got %s", entry.Action)
	}

	ze := entry.Zones["example.com"]
	if len(ze.Changes) != 2 {
		t.Fatalf("expected 2 changes, got %d", len(ze.Changes))
	}

	// Check update has old_content
	for _, ch := range ze.Changes {
		if ch.Action == "update" && ch.OldContent == "" {
			t.Error("expected old_content on update change")
		}
	}

	// Snapshot should have: www (created), api (updated), other (unchanged)
	if len(ze.Snapshot) != 3 {
		t.Errorf("expected 3 snapshot records, got %d", len(ze.Snapshot))
	}
}

func TestRecordApply_DeleteRemovesFromSnapshot(t *testing.T) {
	log := New()

	changesets := []diff.Changeset{
		{
			Zone:   "example.com",
			Manage: config.ManageFull,
			Changes: []diff.Change{
				{
					Action:  diff.ActionDelete,
					Zone:    "example.com",
					LiveID:  1,
					Current: &diff.LiveRecord{ID: 1, Name: "old", Type: "A", Content: "192.0.2.99", TTL: 3600},
				},
			},
		},
	}
	liveByZone := map[string][]diff.LiveRecord{
		"example.com": {
			{ID: 1, Name: "old", Type: "A", Content: "192.0.2.99", TTL: 3600},
			{ID: 2, Name: "keep", Type: "A", Content: "192.0.2.1", TTL: 300},
		},
	}
	cfg := &config.Config{
		Zones: []config.ZoneConfig{
			{Zone: "example.com", Manage: config.ManageFull},
		},
	}

	log.RecordApply(changesets, liveByZone, cfg)

	ze := log.Entries[0].Zones["example.com"]
	// Snapshot should only have "keep", not "old"
	if len(ze.Snapshot) != 1 {
		t.Fatalf("expected 1 snapshot record after delete, got %d", len(ze.Snapshot))
	}
	if ze.Snapshot[0].Name != "keep" {
		t.Errorf("expected keep record in snapshot, got %s", ze.Snapshot[0].Name)
	}
}

func TestFindRecordHistory(t *testing.T) {
	log := New()

	// Two entries with changes to "www A"
	log.Entries = []Entry{
		{
			Timestamp: time.Date(2026, 4, 15, 10, 0, 0, 0, time.UTC),
			Action:    "apply",
			Zones: map[string]ZoneEntry{
				"example.com": {
					Changes: []ChangeRecord{
						{Action: "create", Name: "www", Type: "A", Content: "192.0.2.1", TTL: 300},
					},
				},
			},
		},
		{
			Timestamp: time.Date(2026, 4, 18, 14, 0, 0, 0, time.UTC),
			Action:    "apply",
			Zones: map[string]ZoneEntry{
				"example.com": {
					Changes: []ChangeRecord{
						{Action: "update", Name: "www", Type: "A", Content: "192.0.2.2", OldContent: "192.0.2.1", TTL: 300},
						{Action: "create", Name: "api", Type: "A", Content: "10.0.0.1", TTL: 300},
					},
				},
			},
		},
	}

	history := log.FindRecordHistory("example.com", "www", "A")

	if len(history) != 2 {
		t.Fatalf("expected 2 history entries for www A, got %d", len(history))
	}
	if history[0].Change.Action != "create" {
		t.Errorf("expected first entry to be create, got %s", history[0].Change.Action)
	}
	if history[1].Change.Action != "update" {
		t.Errorf("expected second entry to be update, got %s", history[1].Change.Action)
	}
}

func TestFindRecordHistory_NoResults(t *testing.T) {
	log := New()
	log.Entries = []Entry{
		{
			Timestamp: time.Date(2026, 4, 15, 10, 0, 0, 0, time.UTC),
			Action:    "apply",
			Zones: map[string]ZoneEntry{
				"example.com": {
					Changes: []ChangeRecord{
						{Action: "create", Name: "www", Type: "A", Content: "192.0.2.1"},
					},
				},
			},
		},
	}

	history := log.FindRecordHistory("example.com", "api", "A")
	if len(history) != 0 {
		t.Errorf("expected no history for api A, got %d", len(history))
	}
}

func TestFindSnapshotAt(t *testing.T) {
	log := New()
	log.Entries = []Entry{
		{
			Timestamp: time.Date(2026, 4, 15, 10, 0, 0, 0, time.UTC),
			Action:    "apply",
			Zones: map[string]ZoneEntry{
				"example.com": {
					Snapshot: []SnapshotRecord{
						{Name: "www", Type: "A", Content: "192.0.2.1", TTL: 300},
					},
				},
			},
		},
		{
			Timestamp: time.Date(2026, 4, 18, 14, 0, 0, 0, time.UTC),
			Action:    "apply",
			Zones: map[string]ZoneEntry{
				"example.com": {
					Snapshot: []SnapshotRecord{
						{Name: "www", Type: "A", Content: "192.0.2.2", TTL: 300},
						{Name: "api", Type: "A", Content: "10.0.0.1", TTL: 300},
					},
				},
			},
		},
	}

	// Query at April 16 — should get the April 15 snapshot
	snapshot, ts, found := log.FindSnapshotAt("example.com", time.Date(2026, 4, 16, 0, 0, 0, 0, time.UTC))
	if !found {
		t.Fatal("expected to find a snapshot")
	}
	if len(snapshot) != 1 {
		t.Fatalf("expected 1 record in April 15 snapshot, got %d", len(snapshot))
	}
	if snapshot[0].Content != "192.0.2.1" {
		t.Errorf("expected 192.0.2.1, got %s", snapshot[0].Content)
	}
	if !ts.Equal(time.Date(2026, 4, 15, 10, 0, 0, 0, time.UTC)) {
		t.Errorf("expected timestamp 2026-04-15, got %s", ts)
	}

	// Query at April 19 — should get the April 18 snapshot
	snapshot, _, found = log.FindSnapshotAt("example.com", time.Date(2026, 4, 19, 0, 0, 0, 0, time.UTC))
	if !found {
		t.Fatal("expected to find a snapshot")
	}
	if len(snapshot) != 2 {
		t.Fatalf("expected 2 records in April 18 snapshot, got %d", len(snapshot))
	}
}

func TestFindSnapshotAt_BeforeAnyEntry(t *testing.T) {
	log := New()
	log.Entries = []Entry{
		{
			Timestamp: time.Date(2026, 4, 15, 10, 0, 0, 0, time.UTC),
			Action:    "apply",
			Zones:     map[string]ZoneEntry{"example.com": {}},
		},
	}

	_, _, found := log.FindSnapshotAt("example.com", time.Date(2026, 4, 14, 0, 0, 0, 0, time.UTC))
	if found {
		t.Error("expected no snapshot before first entry")
	}
}

func TestSnapshotToConfig(t *testing.T) {
	snapshot := []SnapshotRecord{
		{Name: "", Type: "SOA", Content: "ns1.dnsimple.com admin.example.com", TTL: 3600},
		{Name: "", Type: "NS", Content: "ns1.dnsimple.com", TTL: 3600},
		{Name: "", Type: "A", Content: "192.0.2.1", TTL: 3600},
		{Name: "www", Type: "CNAME", Content: "example.com", TTL: 3600},
		{Name: "", Type: "MX", Content: "mail.example.com", TTL: 3600, Priority: 10},
	}

	zc := SnapshotToConfig("example.com", config.ManagePartial, snapshot)

	if zc.Zone != "example.com" {
		t.Errorf("expected zone example.com, got %s", zc.Zone)
	}
	if zc.Manage != config.ManagePartial {
		t.Errorf("expected partial manage, got %s", zc.Manage)
	}
	// Should exclude SOA and apex NS, leaving 3 records
	if len(zc.Records) != 3 {
		t.Fatalf("expected 3 records (no SOA/NS), got %d", len(zc.Records))
	}

	// Check @ is used for apex records
	for _, r := range zc.Records {
		if r.Type == "A" && r.Name != "@" {
			t.Errorf("expected apex A record to have name @, got %s", r.Name)
		}
	}
}

func TestSave_Deterministic(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "audit.json")

	log := New()
	log.Entries = append(log.Entries, Entry{
		Timestamp: time.Date(2026, 4, 19, 12, 0, 0, 0, time.UTC),
		Action:    "apply",
		Zones: map[string]ZoneEntry{
			"example.com": {
				Snapshot: []SnapshotRecord{
					{Name: "www", Type: "A", Content: "192.0.2.1", TTL: 300},
				},
			},
		},
	})

	if err := log.Save(path); err != nil {
		t.Fatal(err)
	}
	data1, _ := os.ReadFile(path)

	loaded, _ := Load(path)
	if err := loaded.Save(path); err != nil {
		t.Fatal(err)
	}
	data2, _ := os.ReadFile(path)

	if string(data1) != string(data2) {
		t.Error("expected deterministic output across saves")
	}
}
