// Package config handles parsing and validation of the dnsync YAML configuration.
package config

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// Config is the top-level configuration structure.
type Config struct {
	Zones []ZoneConfig `yaml:"zones"`
}

// ZoneConfig represents a single DNS zone and its desired records.
// All zones are fully managed: records not in the config will be deleted
// (except immutable records like SOA and apex NS).
type ZoneConfig struct {
	Zone    string   `yaml:"zone"`
	Records []Record `yaml:"records"`
}

// Record represents a single DNS record.
type Record struct {
	Name     string `yaml:"name"`
	Type     string `yaml:"type"`
	Content  string `yaml:"content"`
	TTL      int    `yaml:"ttl"`
	Priority int    `yaml:"priority,omitempty"`
}

// RecordKey returns the matching key for a record.
func (r Record) RecordKey() string {
	return r.NormalizedName() + "/" + strings.ToUpper(r.Type)
}

// NormalizedName returns the record name, converting "@" to empty string
// to match DNSimple's convention for apex records.
func (r Record) NormalizedName() string {
	if r.Name == "@" {
		return ""
	}
	return r.Name
}

// Load reads and parses a dnsync config file from the given path.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading config file: %w", err)
	}
	return Parse(data)
}

// Parse parses a dnsync config from raw YAML bytes.
func Parse(data []byte) (*Config, error) {
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}
	if err := validate(&cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func validate(cfg *Config) error {
	if len(cfg.Zones) == 0 {
		return fmt.Errorf("config must define at least one zone")
	}

	seen := make(map[string]bool)
	for i, z := range cfg.Zones {
		if z.Zone == "" {
			return fmt.Errorf("zone[%d]: zone name is required", i)
		}
		if seen[z.Zone] {
			return fmt.Errorf("zone[%d]: duplicate zone %q", i, z.Zone)
		}
		seen[z.Zone] = true

		recordKeys := make(map[string]bool)
		for j, r := range z.Records {
			if r.Type == "" {
				return fmt.Errorf("zone[%d] %q record[%d]: type is required", i, z.Zone, j)
			}
			if r.Content == "" {
				return fmt.Errorf("zone[%d] %q record[%d]: content is required", i, z.Zone, j)
			}
			if r.TTL < 0 {
				return fmt.Errorf("zone[%d] %q record[%d]: ttl must be non-negative", i, z.Zone, j)
			}
			key := r.RecordKey()
			if r.Type != "MX" && r.Type != "TXT" && r.Type != "SRV" && r.Type != "NS" {
				if recordKeys[key] {
					return fmt.Errorf("zone[%d] %q record[%d]: duplicate record %s", i, z.Zone, j, key)
				}
			}
			recordKeys[key] = true
		}
	}
	return nil
}
