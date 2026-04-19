// Package state manages the dnsync state file, which tracks what records
// dnsync has previously applied. This enables partial mode to detect records
// that were removed from config and should be deleted.
package state

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"

	"github.com/ags4no/dnsync/internal/config"
)

// File represents the persisted dnsync state.
type File struct {
	Version int                `json:"version"`
	Zones   map[string]*Zone   `json:"zones"`
}

// Zone holds the state for a single DNS zone.
type Zone struct {
	Manage  config.ManageMode `json:"manage"`
	Records []Record          `json:"records"`
}

// Record represents a previously-applied DNS record in state.
type Record struct {
	Name     string `json:"name"`
	Type     string `json:"type"`
	Content  string `json:"content"`
	TTL      int    `json:"ttl"`
	Priority int    `json:"priority,omitempty"`
}

// RecordKey returns the matching key for a state record.
func (r Record) RecordKey() string {
	return r.Name + "/" + r.Type
}

// ContentKey returns a key that includes content, for multi-value record matching.
func (r Record) ContentKey() string {
	return r.Name + "/" + r.Type + "/" + r.Content
}

const currentVersion = 1

// New creates an empty state file.
func New() *File {
	return &File{
		Version: currentVersion,
		Zones:   make(map[string]*Zone),
	}
}

// Load reads a state file from disk. Returns an empty state if the file doesn't exist.
func Load(path string) (*File, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return New(), nil
		}
		return nil, fmt.Errorf("reading state file: %w", err)
	}
	return Parse(data)
}

// Parse parses a state file from raw JSON bytes.
func Parse(data []byte) (*File, error) {
	var f File
	if err := json.Unmarshal(data, &f); err != nil {
		return nil, fmt.Errorf("parsing state file: %w", err)
	}
	if f.Zones == nil {
		f.Zones = make(map[string]*Zone)
	}
	return &f, nil
}

// Save writes the state file to disk.
func (f *File) Save(path string) error {
	// Sort records within each zone for deterministic output
	for _, z := range f.Zones {
		sort.Slice(z.Records, func(i, j int) bool {
			if z.Records[i].Name != z.Records[j].Name {
				return z.Records[i].Name < z.Records[j].Name
			}
			if z.Records[i].Type != z.Records[j].Type {
				return z.Records[i].Type < z.Records[j].Type
			}
			return z.Records[i].Content < z.Records[j].Content
		})
	}

	data, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling state: %w", err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("writing state file: %w", err)
	}
	return nil
}

// UpdateFromConfig updates the state to reflect the current config.
// Call this after a successful apply.
func (f *File) UpdateFromConfig(cfg *config.Config) {
	f.Version = currentVersion
	// Remove zones no longer in config
	configZones := make(map[string]bool)
	for _, z := range cfg.Zones {
		configZones[z.Zone] = true
	}
	for name := range f.Zones {
		if !configZones[name] {
			delete(f.Zones, name)
		}
	}
	// Update zones from config
	for _, z := range cfg.Zones {
		zone := &Zone{
			Manage:  z.Manage,
			Records: make([]Record, len(z.Records)),
		}
		for i, r := range z.Records {
			zone.Records[i] = Record{
				Name:     r.NormalizedName(),
				Type:     r.Type,
				Content:  r.Content,
				TTL:      r.TTL,
				Priority: r.Priority,
			}
		}
		f.Zones[z.Zone] = zone
	}
}

// GetZoneRecords returns the previously-managed records for a zone.
// Returns nil if the zone has no prior state.
func (f *File) GetZoneRecords(zone string) []Record {
	z, ok := f.Zones[zone]
	if !ok {
		return nil
	}
	return z.Records
}
