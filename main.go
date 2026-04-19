package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/ags4no/dnsync/internal/config"
	"github.com/ags4no/dnsync/internal/diff"
	dns "github.com/ags4no/dnsync/internal/dnsimple"
	ghclient "github.com/ags4no/dnsync/internal/github"
	"github.com/ags4no/dnsync/internal/plan"
	"github.com/ags4no/dnsync/internal/state"
	"github.com/ags4no/dnsync/internal/validate"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	ctx := context.Background()

	// Read action inputs from environment
	mode := getEnv("INPUT_MODE", "plan")
	configFile := getEnv("INPUT_CONFIG_FILE", "dns.yaml")
	stateFile := getEnv("INPUT_STATE_FILE", ".dnsync.state.json")
	dnsimpleToken := os.Getenv("INPUT_DNSIMPLE_TOKEN")
	dnsimpleAccountID := os.Getenv("INPUT_DNSIMPLE_ACCOUNT_ID")

	if dnsimpleToken == "" {
		return fmt.Errorf("INPUT_DNSIMPLE_TOKEN is required")
	}
	if dnsimpleAccountID == "" {
		return fmt.Errorf("INPUT_DNSIMPLE_ACCOUNT_ID is required")
	}
	if mode != "plan" && mode != "apply" && mode != "reconcile" {
		return fmt.Errorf("mode must be \"plan\", \"apply\", or \"reconcile\", got %q", mode)
	}

	// Load config
	cfg, err := config.Load(configFile)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	// Load previous state
	st, err := state.Load(stateFile)
	if err != nil {
		return fmt.Errorf("loading state: %w", err)
	}

	// Initialize DNSimple client
	dnsClient := dns.NewClient(dnsimpleToken, dnsimpleAccountID)

	// Fetch live records and compute changesets for each zone
	var changesets []diff.Changeset
	liveByZone := make(map[string][]diff.LiveRecord)

	for _, zone := range cfg.Zones {
		fmt.Printf("Processing zone: %s (%s mode)\n", zone.Zone, zone.Manage)

		live, err := dnsClient.FetchRecords(ctx, zone.Zone)
		if err != nil {
			return fmt.Errorf("fetching live records for %s: %w", zone.Zone, err)
		}

		liveByZone[zone.Zone] = live
		prevRecords := st.GetZoneRecords(zone.Zone)
		cs := diff.Compute(zone.Zone, zone.Manage, zone.Records, live, prevRecords)
		changesets = append(changesets, cs)
	}

	// Build summary
	summary := plan.NewSummary(changesets)

	// Run validation against live zone state
	validation := validate.Changesets(changesets, liveByZone)
	if validation.HasIssues() {
		fmt.Print(validation.FormatText())
	}

	switch mode {
	case "plan":
		return runPlan(ctx, summary, validation)
	case "apply":
		return runApply(ctx, dnsClient, summary, changesets, cfg, stateFile, validation)
	case "reconcile":
		return runReconcile(ctx, dnsClient, cfg, st, liveByZone, stateFile)
	}

	return nil
}

func runPlan(ctx context.Context, summary plan.Summary, validation validate.Result) error {
	// Print to action log
	fmt.Print(plan.FormatText(summary))

	// Post PR comment if in a PR context
	prNumber, err := ghclient.GetPRNumber()
	if err != nil {
		fmt.Printf("Not in a PR context, skipping comment: %v\n", err)
	} else {
		ghClient, err := ghclient.NewClientFromEnv()
		if err != nil {
			return fmt.Errorf("initializing GitHub client: %w", err)
		}

		md := plan.FormatMarkdown(summary)
		// Append validation issues to PR comment
		md += validation.FormatMarkdown()

		if err := ghClient.UpsertPlanComment(ctx, prNumber, md); err != nil {
			return fmt.Errorf("posting PR comment: %w", err)
		}
		fmt.Printf("Posted plan comment to PR #%d\n", prNumber)
	}

	// Fail the plan if there are validation errors
	if validation.HasErrors() {
		return fmt.Errorf("validation failed — fix errors before applying")
	}

	if summary.HasChanges {
		fmt.Println("DNS changes detected. Review the plan above.")
	}

	return nil
}

func runApply(ctx context.Context, dnsClient *dns.Client, summary plan.Summary, changesets []diff.Changeset, cfg *config.Config, stateFile string, validation validate.Result) error {
	// Block apply if there are validation errors
	if validation.HasErrors() {
		return fmt.Errorf("validation failed — cannot apply changes with errors")
	}

	if !summary.HasChanges {
		fmt.Println("No DNS changes to apply.")
		// Still save state to establish tracking
		st := state.New()
		st.UpdateFromConfig(cfg)
		if err := st.Save(stateFile); err != nil {
			return fmt.Errorf("saving state: %w", err)
		}
		fmt.Printf("State saved to %s\n", stateFile)
		return nil
	}

	fmt.Print(plan.FormatText(summary))
	fmt.Println("\nApplying changes...")

	for _, cs := range changesets {
		if !cs.HasChanges() {
			continue
		}
		fmt.Printf("Applying %d changes to %s...\n", len(cs.Changes), cs.Zone)
		if err := dnsClient.ApplyChanges(ctx, cs.Changes); err != nil {
			return fmt.Errorf("applying changes to %s: %w", cs.Zone, err)
		}
		fmt.Printf("Successfully applied changes to %s\n", cs.Zone)
	}

	// Update and save state
	st := state.New()
	st.UpdateFromConfig(cfg)
	if err := st.Save(stateFile); err != nil {
		return fmt.Errorf("saving state: %w", err)
	}
	fmt.Printf("State saved to %s\n", stateFile)

	fmt.Println("All DNS changes applied successfully.")
	return nil
}

func runReconcile(ctx context.Context, dnsClient *dns.Client, cfg *config.Config, st *state.File, liveByZone map[string][]diff.LiveRecord, stateFile string) error {
	fmt.Println("Reconciling: finding orphaned records from previous runs...")

	var orphans []diff.Change

	for _, zone := range cfg.Zones {
		live := liveByZone[zone.Zone]
		prevRecords := st.GetZoneRecords(zone.Zone)

		// Build sets of desired and state-tracked content keys
		desiredKeys := make(map[string]bool)
		for _, r := range zone.Records {
			desiredKeys[r.NormalizedName()+"/"+r.Type+"/"+r.Content] = true
		}
		stateKeys := make(map[string]bool)
		for _, r := range prevRecords {
			stateKeys[r.ContentKey()] = true
		}

		// Find live records that match desired content but aren't in state
		// (orphans from failed prior runs that created records without saving state)
		for _, lr := range live {
			contentKey := lr.Name + "/" + lr.Type + "/" + normalizeTXT(lr.Type, lr.Content)

			// Skip if it's in the desired config (will be managed normally)
			if desiredKeys[contentKey] {
				continue
			}
			// Skip if already tracked in state (will be handled by diff engine)
			if stateKeys[contentKey] {
				continue
			}
			// Skip immutable records
			if lr.Type == "SOA" || (lr.Type == "NS" && lr.Name == "") {
				continue
			}

			// Check if this looks like an orphan — same name/type as a desired record
			// but with different (stale) content
			for _, r := range zone.Records {
				if r.NormalizedName() == lr.Name && strings.ToUpper(r.Type) == strings.ToUpper(lr.Type) && r.Content != normalizeTXT(lr.Type, lr.Content) {
					orphans = append(orphans, diff.Change{
						Action:  diff.ActionDelete,
						Zone:    zone.Zone,
						LiveID:  lr.ID,
						Current: &diff.LiveRecord{ID: lr.ID, Name: lr.Name, Type: lr.Type, Content: lr.Content, TTL: lr.TTL, Priority: lr.Priority},
					})
					break
				}
			}
		}
	}

	if len(orphans) == 0 {
		fmt.Println("No orphaned records found.")
		return nil
	}

	fmt.Printf("Found %d orphaned record(s):\n", len(orphans))
	for _, o := range orphans {
		fmt.Printf("  - %s %s %s (ID: %d)\n", o.Current.Name, o.Current.Type, o.Current.Content, o.Current.ID)
	}

	fmt.Println("Removing orphaned records...")
	if err := dnsClient.ApplyChanges(ctx, orphans); err != nil {
		return fmt.Errorf("removing orphaned records: %w", err)
	}

	// Update state after reconcile
	st.UpdateFromConfig(cfg)
	if err := st.Save(stateFile); err != nil {
		return fmt.Errorf("saving state: %w", err)
	}

	fmt.Printf("State saved to %s\n", stateFile)
	fmt.Println("Reconciliation complete.")
	return nil
}

func normalizeTXT(recordType, content string) string {
	if recordType == "TXT" {
		if len(content) >= 2 && content[0] == '"' && content[len(content)-1] == '"' {
			content = content[1 : len(content)-1]
		}
	}
	return content
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
