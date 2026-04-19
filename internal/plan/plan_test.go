package plan

import (
	"strings"
	"testing"

	"github.com/ags4no/dnsync/internal/config"
	"github.com/ags4no/dnsync/internal/diff"
)

func TestFormatMarkdown_NoChanges(t *testing.T) {
	s := NewSummary([]diff.Changeset{
		{Zone: "example.com", Manage: config.ManagePartial},
	})

	md := FormatMarkdown(s)

	if !strings.Contains(md, CommentMarker) {
		t.Error("expected comment marker")
	}
	if !strings.Contains(md, "No DNS changes detected") {
		t.Error("expected no changes message")
	}
}

func TestFormatMarkdown_WithChanges(t *testing.T) {
	cs := diff.Changeset{
		Zone:   "example.com",
		Manage: config.ManageFull,
		Changes: []diff.Change{
			{
				Action: diff.ActionCreate,
				Zone:   "example.com",
				Record: config.Record{Name: "www", Type: "A", Content: "192.0.2.1", TTL: 300},
			},
			{
				Action: diff.ActionUpdate,
				Zone:   "example.com",
				Record: config.Record{Name: "api", Type: "A", Content: "192.0.2.2", TTL: 600},
				LiveID: 1,
				Current: &diff.LiveRecord{
					ID: 1, Name: "api", Type: "A", Content: "192.0.2.1", TTL: 300,
				},
			},
			{
				Action: diff.ActionDelete,
				Zone:   "example.com",
				LiveID: 2,
				Current: &diff.LiveRecord{
					ID: 2, Name: "old", Type: "CNAME", Content: "legacy.example.com", TTL: 3600,
				},
			},
		},
	}

	s := NewSummary([]diff.Changeset{cs})
	md := FormatMarkdown(s)

	if !strings.Contains(md, "example.com (full management)") {
		t.Error("expected zone header with manage mode")
	}
	if !strings.Contains(md, "**+** Create") {
		t.Error("expected create action")
	}
	if !strings.Contains(md, "**~** Update") {
		t.Error("expected update action")
	}
	if !strings.Contains(md, "**-** Delete") {
		t.Error("expected delete action")
	}
	if !strings.Contains(md, "`192.0.2.1`") {
		t.Error("expected content in backticks")
	}
}

func TestFormatMarkdown_ApexRecordDisplaysAt(t *testing.T) {
	cs := diff.Changeset{
		Zone:   "example.com",
		Manage: config.ManagePartial,
		Changes: []diff.Change{
			{
				Action: diff.ActionCreate,
				Zone:   "example.com",
				Record: config.Record{Name: "@", Type: "A", Content: "192.0.2.1", TTL: 300},
			},
		},
	}

	s := NewSummary([]diff.Changeset{cs})
	md := FormatMarkdown(s)

	if !strings.Contains(md, "| @ |") {
		t.Error("expected @ display for apex record")
	}
}

func TestFormatMarkdown_MultiZone(t *testing.T) {
	changesets := []diff.Changeset{
		{
			Zone:   "example.com",
			Manage: config.ManageFull,
			Changes: []diff.Change{
				{Action: diff.ActionCreate, Record: config.Record{Name: "www", Type: "A", Content: "1.2.3.4", TTL: 300}},
			},
		},
		{
			Zone:   "other.org",
			Manage: config.ManagePartial,
		},
	}

	s := NewSummary(changesets)
	md := FormatMarkdown(s)

	if !strings.Contains(md, "### example.com") {
		t.Error("expected example.com header")
	}
	if !strings.Contains(md, "### other.org") {
		t.Error("expected other.org header")
	}
	if !strings.Contains(md, "No changes.") {
		t.Error("expected no changes for other.org")
	}
}

func TestFormatText_WithChanges(t *testing.T) {
	cs := diff.Changeset{
		Zone:   "example.com",
		Manage: config.ManageFull,
		Changes: []diff.Change{
			{
				Action: diff.ActionCreate,
				Record: config.Record{Name: "www", Type: "A", Content: "192.0.2.1", TTL: 300},
			},
			{
				Action:  diff.ActionUpdate,
				Record:  config.Record{Name: "api", Type: "A", Content: "192.0.2.2", TTL: 300},
				Current: &diff.LiveRecord{Name: "api", Type: "A", Content: "192.0.2.1", TTL: 300},
			},
			{
				Action:  diff.ActionDelete,
				Current: &diff.LiveRecord{Name: "old", Type: "A", Content: "192.0.2.99", TTL: 3600},
			},
		},
	}

	s := NewSummary([]diff.Changeset{cs})
	txt := FormatText(s)

	if !strings.Contains(txt, "+ www A 192.0.2.1") {
		t.Error("expected create line")
	}
	if !strings.Contains(txt, "~ api A 192.0.2.1 -> 192.0.2.2") {
		t.Error("expected update line")
	}
	if !strings.Contains(txt, "- old A 192.0.2.99") {
		t.Error("expected delete line")
	}
}

func TestFormatText_NoChanges(t *testing.T) {
	s := NewSummary(nil)
	txt := FormatText(s)

	if !strings.Contains(txt, "No DNS changes") {
		t.Error("expected no changes message")
	}
}

func TestNewSummary_HasChanges(t *testing.T) {
	s := NewSummary([]diff.Changeset{
		{Zone: "a.com"},
		{Zone: "b.com", Changes: []diff.Change{{Action: diff.ActionCreate}}},
	})

	if !s.HasChanges {
		t.Error("expected HasChanges true")
	}
	if s.Zones[0].HasChanges {
		t.Error("expected zone a.com to have no changes")
	}
	if !s.Zones[1].HasChanges {
		t.Error("expected zone b.com to have changes")
	}
}
