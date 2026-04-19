package validate

import (
	"testing"

	"github.com/ags4no/dnsync/internal/config"
	"github.com/ags4no/dnsync/internal/diff"
)

func TestValidate_DuplicateCreate(t *testing.T) {
	cs := diff.Changeset{
		Zone:   "example.com",
		Manage: config.ManagePartial,
		Changes: []diff.Change{
			{
				Action: diff.ActionCreate,
				Zone:   "example.com",
				Record: config.Record{Name: "test", Type: "TXT", Content: "hello", TTL: 3600},
			},
		},
	}
	live := map[string][]diff.LiveRecord{
		"example.com": {
			{ID: 1, Name: "test", Type: "TXT", Content: "\"hello\"", TTL: 3600},
		},
	}

	result := Changesets([]diff.Changeset{cs}, live)

	if !result.HasErrors() {
		t.Error("expected error for duplicate create")
	}
	found := false
	for _, i := range result.Issues {
		if i.Severity == SeverityError && i.Record == "test TXT" {
			found = true
		}
	}
	if !found {
		t.Error("expected duplicate record error for test TXT")
	}
}

func TestValidate_CNAMEConflict(t *testing.T) {
	cs := diff.Changeset{
		Zone:   "example.com",
		Manage: config.ManagePartial,
		Changes: []diff.Change{
			{
				Action: diff.ActionCreate,
				Zone:   "example.com",
				Record: config.Record{Name: "www", Type: "CNAME", Content: "example.com", TTL: 3600},
			},
		},
	}
	live := map[string][]diff.LiveRecord{
		"example.com": {
			{ID: 1, Name: "www", Type: "A", Content: "192.0.2.1", TTL: 3600},
		},
	}

	result := Changesets([]diff.Changeset{cs}, live)

	if !result.HasErrors() {
		t.Error("expected error for CNAME conflict")
	}
}

func TestValidate_RecordAtCNAMEName(t *testing.T) {
	cs := diff.Changeset{
		Zone:   "example.com",
		Manage: config.ManagePartial,
		Changes: []diff.Change{
			{
				Action: diff.ActionCreate,
				Zone:   "example.com",
				Record: config.Record{Name: "www", Type: "A", Content: "192.0.2.1", TTL: 3600},
			},
		},
	}
	live := map[string][]diff.LiveRecord{
		"example.com": {
			{ID: 1, Name: "www", Type: "CNAME", Content: "other.example.com", TTL: 3600},
		},
	}

	result := Changesets([]diff.Changeset{cs}, live)

	if !result.HasErrors() {
		t.Error("expected error for creating record at CNAME name")
	}
}

func TestValidate_InvalidIPv4(t *testing.T) {
	cs := diff.Changeset{
		Zone:   "example.com",
		Manage: config.ManagePartial,
		Changes: []diff.Change{
			{
				Action: diff.ActionCreate,
				Zone:   "example.com",
				Record: config.Record{Name: "www", Type: "A", Content: "not-an-ip", TTL: 3600},
			},
		},
	}
	live := map[string][]diff.LiveRecord{"example.com": {}}

	result := Changesets([]diff.Changeset{cs}, live)

	if !result.HasErrors() {
		t.Error("expected error for invalid IPv4")
	}
}

func TestValidate_IPv6InARecord(t *testing.T) {
	cs := diff.Changeset{
		Zone:   "example.com",
		Manage: config.ManagePartial,
		Changes: []diff.Change{
			{
				Action: diff.ActionCreate,
				Zone:   "example.com",
				Record: config.Record{Name: "www", Type: "A", Content: "2001:db8::1", TTL: 3600},
			},
		},
	}
	live := map[string][]diff.LiveRecord{"example.com": {}}

	result := Changesets([]diff.Changeset{cs}, live)

	if !result.HasErrors() {
		t.Error("expected error for IPv6 in A record")
	}
}

func TestValidate_InvalidIPv6(t *testing.T) {
	cs := diff.Changeset{
		Zone:   "example.com",
		Manage: config.ManagePartial,
		Changes: []diff.Change{
			{
				Action: diff.ActionCreate,
				Zone:   "example.com",
				Record: config.Record{Name: "www", Type: "AAAA", Content: "192.0.2.1", TTL: 3600},
			},
		},
	}
	live := map[string][]diff.LiveRecord{"example.com": {}}

	result := Changesets([]diff.Changeset{cs}, live)

	if !result.HasErrors() {
		t.Error("expected error for IPv4 in AAAA record")
	}
}

func TestValidate_MXWithIPContent(t *testing.T) {
	cs := diff.Changeset{
		Zone:   "example.com",
		Manage: config.ManagePartial,
		Changes: []diff.Change{
			{
				Action: diff.ActionCreate,
				Zone:   "example.com",
				Record: config.Record{Name: "", Type: "MX", Content: "192.0.2.1", TTL: 3600, Priority: 10},
			},
		},
	}
	live := map[string][]diff.LiveRecord{"example.com": {}}

	result := Changesets([]diff.Changeset{cs}, live)

	if !result.HasErrors() {
		t.Error("expected error for MX with IP content")
	}
}

func TestValidate_MXWithoutPriority(t *testing.T) {
	cs := diff.Changeset{
		Zone:   "example.com",
		Manage: config.ManagePartial,
		Changes: []diff.Change{
			{
				Action: diff.ActionCreate,
				Zone:   "example.com",
				Record: config.Record{Name: "", Type: "MX", Content: "mail.example.com", TTL: 3600},
			},
		},
	}
	live := map[string][]diff.LiveRecord{"example.com": {}}

	result := Changesets([]diff.Changeset{cs}, live)

	if !result.HasIssues() {
		t.Error("expected warning for MX without priority")
	}
	if result.HasErrors() {
		t.Error("missing priority should be a warning, not an error")
	}
}

func TestValidate_SRVBadContent(t *testing.T) {
	cs := diff.Changeset{
		Zone:   "example.com",
		Manage: config.ManagePartial,
		Changes: []diff.Change{
			{
				Action: diff.ActionCreate,
				Zone:   "example.com",
				Record: config.Record{Name: "_sip._tcp", Type: "SRV", Content: "10 60 5060 sip.example.com", TTL: 3600, Priority: 10},
			},
		},
	}
	live := map[string][]diff.LiveRecord{"example.com": {}}

	result := Changesets([]diff.Changeset{cs}, live)

	if !result.HasErrors() {
		t.Error("expected error for SRV content with 4 parts (priority should be separate)")
	}
}

func TestValidate_SRVValidContent(t *testing.T) {
	cs := diff.Changeset{
		Zone:   "example.com",
		Manage: config.ManagePartial,
		Changes: []diff.Change{
			{
				Action: diff.ActionCreate,
				Zone:   "example.com",
				Record: config.Record{Name: "_sip._tcp", Type: "SRV", Content: "60 5060 sip.example.com", TTL: 3600, Priority: 10},
			},
		},
	}
	live := map[string][]diff.LiveRecord{"example.com": {}}

	result := Changesets([]diff.Changeset{cs}, live)

	if result.HasErrors() {
		t.Errorf("expected no errors for valid SRV, got: %s", result.FormatText())
	}
}

func TestValidate_CAABadContent(t *testing.T) {
	cs := diff.Changeset{
		Zone:   "example.com",
		Manage: config.ManagePartial,
		Changes: []diff.Change{
			{
				Action: diff.ActionCreate,
				Zone:   "example.com",
				Record: config.Record{Name: "", Type: "CAA", Content: "letsencrypt.org", TTL: 3600},
			},
		},
	}
	live := map[string][]diff.LiveRecord{"example.com": {}}

	result := Changesets([]diff.Changeset{cs}, live)

	if !result.HasErrors() {
		t.Error("expected error for CAA content missing flag and tag")
	}
}

func TestValidate_CNAMEWithIP(t *testing.T) {
	cs := diff.Changeset{
		Zone:   "example.com",
		Manage: config.ManagePartial,
		Changes: []diff.Change{
			{
				Action: diff.ActionCreate,
				Zone:   "example.com",
				Record: config.Record{Name: "www", Type: "CNAME", Content: "192.0.2.1", TTL: 3600},
			},
		},
	}
	live := map[string][]diff.LiveRecord{"example.com": {}}

	result := Changesets([]diff.Changeset{cs}, live)

	if !result.HasErrors() {
		t.Error("expected error for CNAME with IP content")
	}
}

func TestValidate_FullModeDeleteWarning(t *testing.T) {
	cs := diff.Changeset{
		Zone:   "example.com",
		Manage: config.ManageFull,
		Changes: []diff.Change{
			{
				Action: diff.ActionDelete,
				Zone:   "example.com",
				LiveID: 1,
				Current: &diff.LiveRecord{
					ID: 1, Name: "old", Type: "A", Content: "192.0.2.99", TTL: 3600,
				},
			},
		},
	}
	live := map[string][]diff.LiveRecord{"example.com": {}}

	result := Changesets([]diff.Changeset{cs}, live)

	if !result.HasIssues() {
		t.Error("expected warning for full mode delete")
	}
	if result.HasErrors() {
		t.Error("full mode delete should be a warning, not an error")
	}
}

func TestValidate_NoIssues(t *testing.T) {
	cs := diff.Changeset{
		Zone:   "example.com",
		Manage: config.ManagePartial,
		Changes: []diff.Change{
			{
				Action: diff.ActionCreate,
				Zone:   "example.com",
				Record: config.Record{Name: "www", Type: "A", Content: "192.0.2.1", TTL: 3600},
			},
		},
	}
	live := map[string][]diff.LiveRecord{"example.com": {}}

	result := Changesets([]diff.Changeset{cs}, live)

	if result.HasIssues() {
		t.Errorf("expected no issues, got: %s", result.FormatText())
	}
}

func TestResult_FormatText_NoIssues(t *testing.T) {
	r := Result{}
	txt := r.FormatText()
	if txt != "Validation passed: no issues found.\n" {
		t.Errorf("unexpected output: %s", txt)
	}
}

func TestResult_FormatMarkdown_NoIssues(t *testing.T) {
	r := Result{}
	md := r.FormatMarkdown()
	if md != "" {
		t.Errorf("expected empty markdown for no issues, got: %s", md)
	}
}
