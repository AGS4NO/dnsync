// Package diff computes the changes needed to reconcile desired DNS records
// with the live state of a zone.
package diff

import (
	"strings"

	"github.com/ags4no/dnsync/internal/config"
	"github.com/ags4no/dnsync/internal/state"
)

// Action represents the type of change to apply.
type Action string

const (
	ActionCreate Action = "create"
	ActionUpdate Action = "update"
	ActionDelete Action = "delete"
)

// LiveRecord represents a DNS record as it exists in the DNS provider.
type LiveRecord struct {
	ID       int64
	Name     string
	Type     string
	Content  string
	TTL      int
	Priority int
}

// RecordKey returns the matching key for a live record.
func (r LiveRecord) RecordKey() string {
	return r.Name + "/" + strings.ToUpper(r.Type)
}

// ContentKey returns a key including content for multi-value matching.
func (r LiveRecord) ContentKey() string {
	return r.Name + "/" + strings.ToUpper(r.Type) + "/" + r.Content
}

// Change represents a single DNS record change.
type Change struct {
	Action  Action
	Zone    string
	Record  config.Record // desired state (empty for deletes)
	LiveID  int64         // ID of the existing record (0 for creates)
	Current *LiveRecord   // current state (nil for creates)
}

// Changeset holds all changes for a single zone.
type Changeset struct {
	Zone    string
	Manage  config.ManageMode
	Changes []Change
}

// HasChanges returns true if there are any changes to apply.
func (cs Changeset) HasChanges() bool {
	return len(cs.Changes) > 0
}

// immutableTypes are record types that should never be deleted in full mode.
var immutableTypes = map[string]bool{
	"SOA": true,
}

// isImmutable returns true if a live record should never be deleted.
func isImmutable(r LiveRecord) bool {
	if immutableTypes[strings.ToUpper(r.Type)] {
		return true
	}
	// NS records at the zone apex are immutable
	if strings.ToUpper(r.Type) == "NS" && r.Name == "" {
		return true
	}
	return false
}

// Compute calculates the changeset needed to reconcile desired records with live records.
// previousState is used in partial mode to detect records removed from config that should
// be deleted. Pass nil if there is no previous state.
func Compute(zone string, manage config.ManageMode, desired []config.Record, live []LiveRecord, previousState []state.Record) Changeset {
	cs := Changeset{
		Zone:   zone,
		Manage: manage,
	}

	// Build lookup maps for live records.
	// For multi-value types (MX, TXT, SRV, NS), multiple records can share a key.
	type liveEntry struct {
		record  LiveRecord
		matched bool
	}
	liveByKey := make(map[string][]*liveEntry)
	liveByContentKey := make(map[string]*liveEntry)
	for _, lr := range live {
		key := lr.RecordKey()
		entry := &liveEntry{record: lr}
		liveByKey[key] = append(liveByKey[key], entry)
		liveByContentKey[lr.ContentKey()] = entry
	}

	// Process desired records — find creates and updates
	for _, dr := range desired {
		key := dr.RecordKey()
		entries := liveByKey[key]

		if len(entries) == 0 {
			// No matching live record — create
			cs.Changes = append(cs.Changes, Change{
				Action: ActionCreate,
				Zone:   zone,
				Record: dr,
			})
			continue
		}

		// Find an exact content match or an unmatched entry to update
		var exactMatch *liveEntry
		var updateCandidate *liveEntry

		for _, e := range entries {
			if e.record.Content == dr.Content {
				exactMatch = e
				break
			}
			if !e.matched {
				updateCandidate = e
			}
		}

		if exactMatch != nil {
			exactMatch.matched = true
			// Check if TTL or priority differ
			if needsUpdate(dr, exactMatch.record) {
				cs.Changes = append(cs.Changes, Change{
					Action:  ActionUpdate,
					Zone:    zone,
					Record:  dr,
					LiveID:  exactMatch.record.ID,
					Current: &exactMatch.record,
				})
			}
		} else if updateCandidate != nil && !isMultiValueType(dr.Type) {
			// Content differs and this is a single-value type — update in place
			updateCandidate.matched = true
			cs.Changes = append(cs.Changes, Change{
				Action:  ActionUpdate,
				Zone:    zone,
				Record:  dr,
				LiveID:  updateCandidate.record.ID,
				Current: &updateCandidate.record,
			})
		} else {
			// Multi-value type with new content, or all entries already matched — create
			cs.Changes = append(cs.Changes, Change{
				Action: ActionCreate,
				Zone:   zone,
				Record: dr,
			})
		}
	}

	// Determine which unmatched live records to delete
	switch manage {
	case config.ManageFull:
		// Delete all unmatched live records (except immutable)
		for _, entries := range liveByKey {
			for _, e := range entries {
				if !e.matched && !isImmutable(e.record) {
					cs.Changes = append(cs.Changes, Change{
						Action:  ActionDelete,
						Zone:    zone,
						LiveID:  e.record.ID,
						Current: &e.record,
					})
				}
			}
		}

	case config.ManagePartial:
		// Only delete records that were previously managed (in state) but
		// are no longer in the config. This detects intentional removals.
		if previousState != nil {
			// Build a set of desired content keys
			desiredContentKeys := make(map[string]bool)
			for _, dr := range desired {
				desiredContentKeys[dr.NormalizedName()+"/"+strings.ToUpper(dr.Type)+"/"+dr.Content] = true
			}

			for _, sr := range previousState {
				contentKey := sr.ContentKey()
				// Record was in state but is no longer in config — delete it
				if !desiredContentKeys[contentKey] {
					if entry, ok := liveByContentKey[contentKey]; ok && !entry.matched && !isImmutable(entry.record) {
						entry.matched = true
						cs.Changes = append(cs.Changes, Change{
							Action:  ActionDelete,
							Zone:    zone,
							LiveID:  entry.record.ID,
							Current: &entry.record,
						})
					}
				}
			}
		}
	}

	return cs
}

// isMultiValueType returns true for record types that can have multiple
// records with the same name (e.g., MX, TXT, SRV, NS). For these types,
// a content mismatch should result in a create, not an update.
func isMultiValueType(recordType string) bool {
	switch strings.ToUpper(recordType) {
	case "MX", "TXT", "SRV", "NS":
		return true
	}
	return false
}

func needsUpdate(desired config.Record, live LiveRecord) bool {
	if desired.TTL != 0 && desired.TTL != live.TTL {
		return true
	}
	if desired.Priority != 0 && desired.Priority != live.Priority {
		return true
	}
	return false
}
