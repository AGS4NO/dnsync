package main

import (
	"context"
	"fmt"
	"os"

	"github.com/ags4no/dnsync/internal/config"
	"github.com/ags4no/dnsync/internal/diff"
	dns "github.com/ags4no/dnsync/internal/dnsimple"
	ghclient "github.com/ags4no/dnsync/internal/github"
	"github.com/ags4no/dnsync/internal/plan"
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
	configFormat := getEnv("INPUT_CONFIG_FORMAT", "yaml")
	dnsimpleToken := os.Getenv("INPUT_DNSIMPLE_TOKEN")
	dnsimpleAccountID := os.Getenv("INPUT_DNSIMPLE_ACCOUNT_ID")

	if dnsimpleToken == "" {
		return fmt.Errorf("INPUT_DNSIMPLE_TOKEN is required")
	}
	if dnsimpleAccountID == "" {
		return fmt.Errorf("INPUT_DNSIMPLE_ACCOUNT_ID is required")
	}
	if mode != "plan" && mode != "apply" {
		return fmt.Errorf("mode must be \"plan\" or \"apply\", got %q", mode)
	}
	if configFormat != "yaml" && configFormat != "bind" {
		return fmt.Errorf("config-format must be \"yaml\" or \"bind\", got %q", configFormat)
	}

	// Load config
	var (
		cfg *config.Config
		err error
	)
	switch configFormat {
	case "bind":
		cfg, err = config.LoadBind(configFile)
	default:
		cfg, err = config.Load(configFile)
	}
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	// Initialize DNSimple client
	dnsClient := dns.NewClient(dnsimpleToken, dnsimpleAccountID)

	// Fetch live records and compute changesets for each zone
	var changesets []diff.Changeset
	liveByZone := make(map[string][]diff.LiveRecord)

	for _, zone := range cfg.Zones {
		fmt.Printf("Processing zone: %s\n", zone.Zone)

		live, err := dnsClient.FetchRecords(ctx, zone.Zone)
		if err != nil {
			return fmt.Errorf("fetching live records for %s: %w", zone.Zone, err)
		}

		liveByZone[zone.Zone] = live
		cs := diff.Compute(zone.Zone, zone.Records, live)
		changesets = append(changesets, cs)
	}

	// Build summary
	summary := plan.NewSummary(configFile, changesets)

	// Run validation against live zone state
	validation := validate.Changesets(changesets, liveByZone)
	if validation.HasIssues() {
		fmt.Print(validation.FormatText())
	}

	switch mode {
	case "plan":
		return runPlan(ctx, summary, validation)
	case "apply":
		return runApply(ctx, dnsClient, summary, changesets, validation)
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

		marker := plan.CommentMarker(summary.ConfigFile)
		if err := ghClient.UpsertPlanComment(ctx, prNumber, md, marker); err != nil {
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

func runApply(ctx context.Context, dnsClient *dns.Client, summary plan.Summary, changesets []diff.Changeset, validation validate.Result) error {
	// Block apply if there are validation errors
	if validation.HasErrors() {
		return fmt.Errorf("validation failed — cannot apply changes with errors")
	}

	if !summary.HasChanges {
		fmt.Println("No DNS changes to apply.")
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

	fmt.Println("All DNS changes applied successfully.")
	return nil
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
