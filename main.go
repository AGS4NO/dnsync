package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/ags4no/dnsync/internal/config"
	"github.com/ags4no/dnsync/internal/diff"
	dns "github.com/ags4no/dnsync/internal/dnsimple"
	ghclient "github.com/ags4no/dnsync/internal/github"
	"github.com/ags4no/dnsync/internal/plan"
	"github.com/ags4no/dnsync/internal/state"
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
	if mode != "plan" && mode != "apply" {
		return fmt.Errorf("mode must be \"plan\" or \"apply\", got %q", mode)
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

	// Compute changesets for each zone
	var changesets []diff.Changeset
	for _, zone := range cfg.Zones {
		fmt.Printf("Processing zone: %s (%s mode)\n", zone.Zone, zone.Manage)

		live, err := dnsClient.FetchRecords(ctx, zone.Zone)
		if err != nil {
			return fmt.Errorf("fetching live records for %s: %w", zone.Zone, err)
		}

		prevRecords := st.GetZoneRecords(zone.Zone)
		cs := diff.Compute(zone.Zone, zone.Manage, zone.Records, live, prevRecords)
		changesets = append(changesets, cs)
	}

	// Build summary
	summary := plan.NewSummary(changesets)

	switch mode {
	case "plan":
		return runPlan(ctx, summary)
	case "apply":
		return runApply(ctx, dnsClient, summary, changesets, cfg, stateFile)
	}

	return nil
}

func runPlan(ctx context.Context, summary plan.Summary) error {
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
		if err := ghClient.UpsertPlanComment(ctx, prNumber, md); err != nil {
			return fmt.Errorf("posting PR comment: %w", err)
		}
		fmt.Printf("Posted plan comment to PR #%d\n", prNumber)
	}

	if summary.HasChanges {
		fmt.Println("DNS changes detected. Review the plan above.")
	}

	return nil
}

func runApply(ctx context.Context, dnsClient *dns.Client, summary plan.Summary, changesets []diff.Changeset, cfg *config.Config, stateFile string) error {
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

	// Update and save state
	st := state.New()
	st.UpdateFromConfig(cfg)
	if err := st.Save(stateFile); err != nil {
		return fmt.Errorf("saving state: %w", err)
	}
	fmt.Printf("State saved to %s\n", stateFile)

	// Commit and push state file
	if err := commitStateFile(stateFile); err != nil {
		return fmt.Errorf("committing state file: %w", err)
	}

	fmt.Println("All DNS changes applied successfully.")
	return nil
}

func commitStateFile(stateFile string) error {
	absPath, err := filepath.Abs(stateFile)
	if err != nil {
		return fmt.Errorf("resolving state file path: %w", err)
	}

	// Configure git for the GitHub Actions bot
	commands := [][]string{
		{"git", "config", "user.name", "github-actions[bot]"},
		{"git", "config", "user.email", "github-actions[bot]@users.noreply.github.com"},
		{"git", "add", absPath},
	}

	for _, args := range commands {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("running %v: %w", args, err)
		}
	}

	// Check if there are staged changes to commit
	cmd := exec.Command("git", "diff", "--cached", "--quiet")
	if err := cmd.Run(); err != nil {
		// Exit code 1 means there are changes — commit them
		commitCmd := exec.Command("git", "commit", "-m", "chore: update dnsync state [skip ci]")
		commitCmd.Stdout = os.Stdout
		commitCmd.Stderr = os.Stderr
		if err := commitCmd.Run(); err != nil {
			return fmt.Errorf("committing state: %w", err)
		}

		pushCmd := exec.Command("git", "push")
		pushCmd.Stdout = os.Stdout
		pushCmd.Stderr = os.Stderr
		if err := pushCmd.Run(); err != nil {
			return fmt.Errorf("pushing state: %w", err)
		}

		fmt.Println("State file committed and pushed.")
	} else {
		fmt.Println("State file unchanged, nothing to commit.")
	}

	return nil
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
