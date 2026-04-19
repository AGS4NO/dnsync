// Package plan formats diff changesets into human-readable output
// suitable for PR comments and action logs.
package plan

import (
	"fmt"
	"strings"

	"github.com/ags4no/dnsync/internal/config"
	"github.com/ags4no/dnsync/internal/diff"
)

// CommentMarker is embedded in PR comments so we can find and update them.
const CommentMarker = "<!-- dnsync-plan -->"

// Summary holds the formatted plan for all zones.
type Summary struct {
	Zones      []ZoneSummary
	HasChanges bool
}

// ZoneSummary holds the formatted plan for a single zone.
type ZoneSummary struct {
	Zone       string
	Manage     config.ManageMode
	Changeset  diff.Changeset
	HasChanges bool
}

// NewSummary creates a Summary from a list of changesets.
func NewSummary(changesets []diff.Changeset) Summary {
	s := Summary{}
	for _, cs := range changesets {
		zs := ZoneSummary{
			Zone:       cs.Zone,
			Manage:     cs.Manage,
			Changeset:  cs,
			HasChanges: cs.HasChanges(),
		}
		s.Zones = append(s.Zones, zs)
		if zs.HasChanges {
			s.HasChanges = true
		}
	}
	return s
}

// FormatMarkdown renders the plan as a markdown string for PR comments.
func FormatMarkdown(summary Summary) string {
	var b strings.Builder

	b.WriteString(CommentMarker)
	b.WriteString("\n## DNS Change Plan\n\n")

	if !summary.HasChanges {
		b.WriteString("No DNS changes detected.\n")
		return b.String()
	}

	for _, zs := range summary.Zones {
		b.WriteString(fmt.Sprintf("### %s (%s management)\n\n", zs.Zone, zs.Manage))

		if !zs.HasChanges {
			b.WriteString("No changes.\n\n")
			continue
		}

		b.WriteString("| Action | Name | Type | Value | TTL |\n")
		b.WriteString("|--------|------|------|-------|-----|\n")

		for _, c := range zs.Changeset.Changes {
			switch c.Action {
			case diff.ActionCreate:
				name := displayName(c.Record.NormalizedName())
				b.WriteString(fmt.Sprintf("| **+** Create | %s | %s | `%s` | %d |\n",
					name, c.Record.Type, c.Record.Content, c.Record.TTL))

			case diff.ActionUpdate:
				name := displayName(c.Record.NormalizedName())
				b.WriteString(fmt.Sprintf("| **~** Update | %s | %s | `%s` | %d |\n",
					name, c.Record.Type, c.Record.Content, c.Record.TTL))

			case diff.ActionDelete:
				name := displayName(c.Current.Name)
				b.WriteString(fmt.Sprintf("| **-** Delete | %s | %s | `%s` | %d |\n",
					name, c.Current.Type, c.Current.Content, c.Current.TTL))
			}
		}
		b.WriteString("\n")
	}

	return b.String()
}

// FormatText renders the plan as plain text for action logs.
func FormatText(summary Summary) string {
	var b strings.Builder

	if !summary.HasChanges {
		b.WriteString("No DNS changes detected.\n")
		return b.String()
	}

	for _, zs := range summary.Zones {
		b.WriteString(fmt.Sprintf("Zone: %s (%s)\n", zs.Zone, zs.Manage))

		if !zs.HasChanges {
			b.WriteString("  No changes.\n")
			continue
		}

		for _, c := range zs.Changeset.Changes {
			switch c.Action {
			case diff.ActionCreate:
				b.WriteString(fmt.Sprintf("  + %s %s %s (TTL: %d)\n",
					displayName(c.Record.NormalizedName()), c.Record.Type, c.Record.Content, c.Record.TTL))
			case diff.ActionUpdate:
				b.WriteString(fmt.Sprintf("  ~ %s %s %s -> %s (TTL: %d)\n",
					displayName(c.Record.NormalizedName()), c.Record.Type, c.Current.Content, c.Record.Content, c.Record.TTL))
			case diff.ActionDelete:
				b.WriteString(fmt.Sprintf("  - %s %s %s (TTL: %d)\n",
					displayName(c.Current.Name), c.Current.Type, c.Current.Content, c.Current.TTL))
			}
		}
	}

	return b.String()
}

func displayName(name string) string {
	if name == "" {
		return "@"
	}
	return name
}
