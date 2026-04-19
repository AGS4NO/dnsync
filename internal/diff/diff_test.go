package diff

import (
	"testing"

	"github.com/ags4no/dnsync/internal/config"
)

func TestCompute_CreateNew(t *testing.T) {
	desired := []config.Record{
		{Name: "www", Type: "A", Content: "192.0.2.1", TTL: 300},
	}
	live := []LiveRecord{}

	cs := Compute("example.com", config.ManagePartial, desired, live)

	if len(cs.Changes) != 1 {
		t.Fatalf("expected 1 change, got %d", len(cs.Changes))
	}
	if cs.Changes[0].Action != ActionCreate {
		t.Errorf("expected create, got %s", cs.Changes[0].Action)
	}
	if cs.Changes[0].Record.Content != "192.0.2.1" {
		t.Errorf("expected content 192.0.2.1, got %s", cs.Changes[0].Record.Content)
	}
}

func TestCompute_NoChanges(t *testing.T) {
	desired := []config.Record{
		{Name: "www", Type: "A", Content: "192.0.2.1", TTL: 300},
	}
	live := []LiveRecord{
		{ID: 1, Name: "www", Type: "A", Content: "192.0.2.1", TTL: 300},
	}

	cs := Compute("example.com", config.ManagePartial, desired, live)

	if cs.HasChanges() {
		t.Errorf("expected no changes, got %d", len(cs.Changes))
	}
}

func TestCompute_UpdateContent(t *testing.T) {
	desired := []config.Record{
		{Name: "www", Type: "A", Content: "192.0.2.2", TTL: 300},
	}
	live := []LiveRecord{
		{ID: 1, Name: "www", Type: "A", Content: "192.0.2.1", TTL: 300},
	}

	cs := Compute("example.com", config.ManagePartial, desired, live)

	if len(cs.Changes) != 1 {
		t.Fatalf("expected 1 change, got %d", len(cs.Changes))
	}
	if cs.Changes[0].Action != ActionUpdate {
		t.Errorf("expected update, got %s", cs.Changes[0].Action)
	}
	if cs.Changes[0].LiveID != 1 {
		t.Errorf("expected LiveID 1, got %d", cs.Changes[0].LiveID)
	}
}

func TestCompute_UpdateTTL(t *testing.T) {
	desired := []config.Record{
		{Name: "www", Type: "A", Content: "192.0.2.1", TTL: 600},
	}
	live := []LiveRecord{
		{ID: 1, Name: "www", Type: "A", Content: "192.0.2.1", TTL: 300},
	}

	cs := Compute("example.com", config.ManagePartial, desired, live)

	if len(cs.Changes) != 1 {
		t.Fatalf("expected 1 change, got %d", len(cs.Changes))
	}
	if cs.Changes[0].Action != ActionUpdate {
		t.Errorf("expected update, got %s", cs.Changes[0].Action)
	}
}

func TestCompute_FullMode_DeletesUnmanaged(t *testing.T) {
	desired := []config.Record{
		{Name: "www", Type: "A", Content: "192.0.2.1", TTL: 300},
	}
	live := []LiveRecord{
		{ID: 1, Name: "www", Type: "A", Content: "192.0.2.1", TTL: 300},
		{ID: 2, Name: "old", Type: "A", Content: "192.0.2.99", TTL: 3600},
	}

	cs := Compute("example.com", config.ManageFull, desired, live)

	if len(cs.Changes) != 1 {
		t.Fatalf("expected 1 change, got %d", len(cs.Changes))
	}
	if cs.Changes[0].Action != ActionDelete {
		t.Errorf("expected delete, got %s", cs.Changes[0].Action)
	}
	if cs.Changes[0].LiveID != 2 {
		t.Errorf("expected LiveID 2, got %d", cs.Changes[0].LiveID)
	}
}

func TestCompute_PartialMode_IgnoresUnmanaged(t *testing.T) {
	desired := []config.Record{
		{Name: "www", Type: "A", Content: "192.0.2.1", TTL: 300},
	}
	live := []LiveRecord{
		{ID: 1, Name: "www", Type: "A", Content: "192.0.2.1", TTL: 300},
		{ID: 2, Name: "old", Type: "A", Content: "192.0.2.99", TTL: 3600},
	}

	cs := Compute("example.com", config.ManagePartial, desired, live)

	if cs.HasChanges() {
		t.Errorf("expected no changes in partial mode, got %d", len(cs.Changes))
	}
}

func TestCompute_FullMode_ProtectsSOA(t *testing.T) {
	desired := []config.Record{}
	live := []LiveRecord{
		{ID: 1, Name: "", Type: "SOA", Content: "ns1.dnsimple.com admin.example.com", TTL: 3600},
	}

	cs := Compute("example.com", config.ManageFull, desired, live)

	if cs.HasChanges() {
		t.Errorf("SOA should not be deleted, got %d changes", len(cs.Changes))
	}
}

func TestCompute_FullMode_ProtectsApexNS(t *testing.T) {
	desired := []config.Record{}
	live := []LiveRecord{
		{ID: 1, Name: "", Type: "NS", Content: "ns1.dnsimple.com", TTL: 3600},
	}

	cs := Compute("example.com", config.ManageFull, desired, live)

	if cs.HasChanges() {
		t.Errorf("apex NS should not be deleted, got %d changes", len(cs.Changes))
	}
}

func TestCompute_FullMode_AllowsNonApexNSDelete(t *testing.T) {
	desired := []config.Record{}
	live := []LiveRecord{
		{ID: 1, Name: "sub", Type: "NS", Content: "ns1.other.com", TTL: 3600},
	}

	cs := Compute("example.com", config.ManageFull, desired, live)

	if len(cs.Changes) != 1 {
		t.Fatalf("expected 1 change, got %d", len(cs.Changes))
	}
	if cs.Changes[0].Action != ActionDelete {
		t.Errorf("expected delete for non-apex NS, got %s", cs.Changes[0].Action)
	}
}

func TestCompute_MultipleMXRecords(t *testing.T) {
	desired := []config.Record{
		{Name: "", Type: "MX", Content: "mail1.example.com", TTL: 3600, Priority: 10},
		{Name: "", Type: "MX", Content: "mail2.example.com", TTL: 3600, Priority: 20},
	}
	live := []LiveRecord{
		{ID: 1, Name: "", Type: "MX", Content: "mail1.example.com", TTL: 3600, Priority: 10},
	}

	cs := Compute("example.com", config.ManagePartial, desired, live)

	if len(cs.Changes) != 1 {
		t.Fatalf("expected 1 change (create mail2), got %d", len(cs.Changes))
	}
	if cs.Changes[0].Action != ActionCreate {
		t.Errorf("expected create, got %s", cs.Changes[0].Action)
	}
}

func TestCompute_MixedChanges(t *testing.T) {
	desired := []config.Record{
		{Name: "www", Type: "A", Content: "192.0.2.2", TTL: 300},       // update
		{Name: "new", Type: "CNAME", Content: "example.com", TTL: 3600}, // create
	}
	live := []LiveRecord{
		{ID: 1, Name: "www", Type: "A", Content: "192.0.2.1", TTL: 300},
		{ID: 2, Name: "old", Type: "A", Content: "192.0.2.99", TTL: 3600},
	}

	cs := Compute("example.com", config.ManageFull, desired, live)

	creates, updates, deletes := 0, 0, 0
	for _, c := range cs.Changes {
		switch c.Action {
		case ActionCreate:
			creates++
		case ActionUpdate:
			updates++
		case ActionDelete:
			deletes++
		}
	}
	if creates != 1 {
		t.Errorf("expected 1 create, got %d", creates)
	}
	if updates != 1 {
		t.Errorf("expected 1 update, got %d", updates)
	}
	if deletes != 1 {
		t.Errorf("expected 1 delete, got %d", deletes)
	}
}
