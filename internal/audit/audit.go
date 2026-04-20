// Package audit maintains a historical log of all DNS changes applied by dnsync.
//
// The audit log (.dnsync.audit.json) is an append-only file that records:
//   - Every apply/reconcile action with a timestamp
//   - The specific changes made (creates, updates, deletes)
//   - A complete snapshot of the zone after the changes were applied
//
// This enables:
//   - Restoring a zone to any previous point in time
//   - Querying the history of a specific record
//   - Understanding when and why DNS changes were made
//
// The file is designed to be read by AI agents for natural language queries
// such as "restore my zone to 2026-04-15" or "when was the www record last changed?"
package audit

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/ags4no/dnsync/internal/config"
	"github.com/ags4no/dnsync/internal/diff"
)

// Log is the top-level audit log structure.
type Log struct {
	// Description explains the purpose of this file for AI agents reading it.
	Description string  `json:"_description"`
	Entries     []Entry `json:"entries"`
}

// Entry represents a single apply or reconcile event.
type Entry struct {
	Timestamp time.Time            `json:"timestamp"`
	Action    string               `json:"action"` // "apply" or "reconcile"
	Zones     map[string]ZoneEntry `json:"zones"`
}

// ZoneEntry holds the changes and resulting snapshot for a single zone.
type ZoneEntry struct {
	Manage   config.ManageMode `json:"manage"`
	Changes  []ChangeRecord    `json:"changes,omitempty"`
	Snapshot []SnapshotRecord  `json:"snapshot"`
}

// ChangeRecord describes a single DNS change that was applied.
type ChangeRecord struct {
	Action     string `json:"action"` // "create", "update", "delete"
	Name       string `json:"name"`
	Type       string `json:"type"`
	Content    string `json:"content"`
	TTL        int    `json:"ttl,omitempty"`
	Priority   int    `json:"priority,omitempty"`
	OldContent string `json:"old_content,omitempty"` // for updates
	OldTTL     int    `json:"old_ttl,omitempty"`     // for updates
}

// SnapshotRecord represents a DNS record in the zone at a point in time.
type SnapshotRecord struct {
	Name     string `json:"name"`
	Type     string `json:"type"`
	Content  string `json:"content"`
	TTL      int    `json:"ttl"`
	Priority int    `json:"priority,omitempty"`
}

const description = "dnsync audit log — records all DNS changes with timestamps and zone snapshots. " +
	"Use this file to restore zones to a previous state or query record change history. " +
	"Each entry contains the changes applied and a complete snapshot of the zone after those changes."

// New creates an empty audit log.
func New() *Log {
	return &Log{
		Description: description,
		Entries:     []Entry{},
	}
}

// Load reads an audit log from disk. Returns an empty log if the file doesn't exist.
func Load(path string) (*Log, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return New(), nil
		}
		return nil, fmt.Errorf("reading audit log: %w", err)
	}
	return Parse(data)
}

// Parse parses an audit log from raw JSON bytes.
func Parse(data []byte) (*Log, error) {
	var log Log
	if err := json.Unmarshal(data, &log); err != nil {
		return nil, fmt.Errorf("parsing audit log: %w", err)
	}
	return &log, nil
}

// Save writes the audit log to disk.
func (l *Log) Save(path string) error {
	l.Description = description
	data, err := json.MarshalIndent(l, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling audit log: %w", err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("writing audit log: %w", err)
	}
	return nil
}

// RecordApply appends an entry for an apply action.
func (l *Log) RecordApply(changesets []diff.Changeset, liveByZone map[string][]diff.LiveRecord, cfg *config.Config) {
	l.recordEntry("apply", changesets, liveByZone, cfg)
}

// RecordReconcile appends an entry for a reconcile action.
func (l *Log) RecordReconcile(changes []diff.Change, liveByZone map[string][]diff.LiveRecord, cfg *config.Config) {
	// Group changes by zone
	byZone := make(map[string][]diff.Change)
	for _, c := range changes {
		byZone[c.Zone] = append(byZone[c.Zone], c)
	}

	// Build changesets from grouped changes
	var changesets []diff.Changeset
	for _, zone := range cfg.Zones {
		cs := diff.Changeset{
			Zone:    zone.Zone,
			Manage:  zone.Manage,
			Changes: byZone[zone.Zone],
		}
		changesets = append(changesets, cs)
	}

	l.recordEntry("reconcile", changesets, liveByZone, cfg)
}

func (l *Log) recordEntry(action string, changesets []diff.Changeset, liveByZone map[string][]diff.LiveRecord, cfg *config.Config) {
	entry := Entry{
		Timestamp: time.Now().UTC(),
		Action:    action,
		Zones:     make(map[string]ZoneEntry),
	}

	// Build a manage mode lookup from config
	manageByZone := make(map[string]config.ManageMode)
	for _, z := range cfg.Zones {
		manageByZone[z.Zone] = z.Manage
	}

	for _, cs := range changesets {
		ze := ZoneEntry{
			Manage: manageByZone[cs.Zone],
		}

		// Record changes
		for _, ch := range cs.Changes {
			cr := changeToRecord(ch)
			ze.Changes = append(ze.Changes, cr)
		}

		// Build snapshot from live records (these reflect state BEFORE our changes,
		// so we need to apply the changes to get the post-apply snapshot)
		ze.Snapshot = buildSnapshot(liveByZone[cs.Zone], cs.Changes)

		entry.Zones[cs.Zone] = ze
	}

	l.Entries = append(l.Entries, entry)
}

func changeToRecord(ch diff.Change) ChangeRecord {
	cr := ChangeRecord{
		Action: string(ch.Action),
	}

	switch ch.Action {
	case diff.ActionCreate:
		cr.Name = displayName(ch.Record.NormalizedName())
		cr.Type = ch.Record.Type
		cr.Content = ch.Record.Content
		cr.TTL = ch.Record.TTL
		cr.Priority = ch.Record.Priority
	case diff.ActionUpdate:
		cr.Name = displayName(ch.Record.NormalizedName())
		cr.Type = ch.Record.Type
		cr.Content = ch.Record.Content
		cr.TTL = ch.Record.TTL
		cr.Priority = ch.Record.Priority
		if ch.Current != nil {
			cr.OldContent = ch.Current.Content
			cr.OldTTL = ch.Current.TTL
		}
	case diff.ActionDelete:
		if ch.Current != nil {
			cr.Name = displayName(ch.Current.Name)
			cr.Type = ch.Current.Type
			cr.Content = ch.Current.Content
			cr.TTL = ch.Current.TTL
			cr.Priority = ch.Current.Priority
		}
	}

	return cr
}

// buildSnapshot constructs the post-apply zone state by applying changes to the live records.
func buildSnapshot(live []diff.LiveRecord, changes []diff.Change) []SnapshotRecord {
	// Start with live records indexed by ID
	byID := make(map[int64]diff.LiveRecord)
	for _, lr := range live {
		byID[lr.ID] = lr
	}

	// Apply changes
	var created []SnapshotRecord
	for _, ch := range changes {
		switch ch.Action {
		case diff.ActionDelete:
			delete(byID, ch.LiveID)
		case diff.ActionUpdate:
			if lr, ok := byID[ch.LiveID]; ok {
				lr.Content = ch.Record.Content
				lr.TTL = ch.Record.TTL
				if ch.Record.Priority > 0 {
					lr.Priority = ch.Record.Priority
				}
				byID[ch.LiveID] = lr
			}
		case diff.ActionCreate:
			created = append(created, SnapshotRecord{
				Name:     ch.Record.NormalizedName(),
				Type:     ch.Record.Type,
				Content:  ch.Record.Content,
				TTL:      ch.Record.TTL,
				Priority: ch.Record.Priority,
			})
		}
	}

	// Build snapshot from remaining live records + created records
	var snapshot []SnapshotRecord
	for _, lr := range byID {
		snapshot = append(snapshot, SnapshotRecord{
			Name:     lr.Name,
			Type:     lr.Type,
			Content:  normalizeContent(lr.Type, lr.Content),
			TTL:      lr.TTL,
			Priority: lr.Priority,
		})
	}
	snapshot = append(snapshot, created...)

	// Sort for deterministic output
	sort.Slice(snapshot, func(i, j int) bool {
		if snapshot[i].Name != snapshot[j].Name {
			return snapshot[i].Name < snapshot[j].Name
		}
		if snapshot[i].Type != snapshot[j].Type {
			return snapshot[i].Type < snapshot[j].Type
		}
		return snapshot[i].Content < snapshot[j].Content
	})

	return snapshot
}

// FindRecordHistory returns all changes for a specific record name and type across all entries.
func (l *Log) FindRecordHistory(zone, name, recordType string) []RecordHistoryEntry {
	var history []RecordHistoryEntry

	normalizedName := name
	if name == "@" {
		normalizedName = "@"
	}

	for _, entry := range l.Entries {
		ze, ok := entry.Zones[zone]
		if !ok {
			continue
		}
		for _, ch := range ze.Changes {
			if ch.Name == normalizedName && strings.EqualFold(ch.Type, recordType) {
				history = append(history, RecordHistoryEntry{
					Timestamp: entry.Timestamp,
					Action:    entry.Action,
					Change:    ch,
				})
			}
		}
	}

	return history
}

// RecordHistoryEntry pairs a change with its timestamp for history queries.
type RecordHistoryEntry struct {
	Timestamp time.Time    `json:"timestamp"`
	Action    string       `json:"action"`
	Change    ChangeRecord `json:"change"`
}

// FindSnapshotAt returns the zone snapshot closest to (but not after) the given time.
func (l *Log) FindSnapshotAt(zone string, at time.Time) ([]SnapshotRecord, time.Time, bool) {
	var bestSnapshot []SnapshotRecord
	var bestTime time.Time
	found := false

	for _, entry := range l.Entries {
		if entry.Timestamp.After(at) {
			break
		}
		ze, ok := entry.Zones[zone]
		if !ok {
			continue
		}
		bestSnapshot = ze.Snapshot
		bestTime = entry.Timestamp
		found = true
	}

	return bestSnapshot, bestTime, found
}

// SnapshotToConfig converts a zone snapshot to a config.ZoneConfig that can be
// written to dns.yaml for restoring a zone to a previous state.
func SnapshotToConfig(zone string, manage config.ManageMode, snapshot []SnapshotRecord) config.ZoneConfig {
	zc := config.ZoneConfig{
		Zone:   zone,
		Manage: manage,
	}
	for _, sr := range snapshot {
		name := sr.Name
		if name == "" {
			name = "@"
		}
		// Skip SOA and apex NS records — these shouldn't be in the config
		if sr.Type == "SOA" || (sr.Type == "NS" && sr.Name == "") {
			continue
		}
		zc.Records = append(zc.Records, config.Record{
			Name:     name,
			Type:     sr.Type,
			Content:  sr.Content,
			TTL:      sr.TTL,
			Priority: sr.Priority,
		})
	}
	return zc
}

func displayName(name string) string {
	if name == "" {
		return "@"
	}
	return name
}

// normalizeContent strips quotes that DNSimple adds to TXT and CAA records.
func normalizeContent(recordType, content string) string {
	switch strings.ToUpper(recordType) {
	case "TXT":
		content = strings.TrimPrefix(content, "\"")
		content = strings.TrimSuffix(content, "\"")
	case "CAA":
		content = strings.ReplaceAll(content, "\"", "")
	}
	return content
}
