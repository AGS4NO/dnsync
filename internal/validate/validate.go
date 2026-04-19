// Package validate pre-validates planned DNS changes against the live zone state
// to catch errors during plan mode before they reach apply.
package validate

import (
	"fmt"
	"net"
	"strings"

	"github.com/ags4no/dnsync/internal/diff"
)

// Severity indicates how critical a validation issue is.
type Severity string

const (
	SeverityError   Severity = "error"
	SeverityWarning Severity = "warning"
)

// Issue represents a single validation problem.
type Issue struct {
	Severity Severity
	Zone     string
	Record   string // human-readable record identifier (e.g., "www A")
	Message  string
}

func (i Issue) String() string {
	prefix := "ERROR"
	if i.Severity == SeverityWarning {
		prefix = "WARN"
	}
	return fmt.Sprintf("[%s] %s: %s — %s", prefix, i.Zone, i.Record, i.Message)
}

// Result holds all validation issues found.
type Result struct {
	Issues []Issue
}

// HasErrors returns true if any issues are errors (not just warnings).
func (r Result) HasErrors() bool {
	for _, i := range r.Issues {
		if i.Severity == SeverityError {
			return true
		}
	}
	return false
}

// HasIssues returns true if there are any issues at all.
func (r Result) HasIssues() bool {
	return len(r.Issues) > 0
}

// FormatText returns a human-readable summary of all issues.
func (r Result) FormatText() string {
	if !r.HasIssues() {
		return "Validation passed: no issues found.\n"
	}
	var b strings.Builder
	errors, warnings := 0, 0
	for _, i := range r.Issues {
		if i.Severity == SeverityError {
			errors++
		} else {
			warnings++
		}
		b.WriteString(i.String())
		b.WriteString("\n")
	}
	b.WriteString(fmt.Sprintf("\nValidation: %d error(s), %d warning(s)\n", errors, warnings))
	return b.String()
}

// FormatMarkdown returns a markdown-formatted summary for PR comments.
func (r Result) FormatMarkdown() string {
	if !r.HasIssues() {
		return ""
	}
	var b strings.Builder
	b.WriteString("\n### Validation Issues\n\n")
	b.WriteString("| Severity | Zone | Record | Issue |\n")
	b.WriteString("|----------|------|--------|-------|\n")
	for _, i := range r.Issues {
		icon := "**X**"
		if i.Severity == SeverityWarning {
			icon = "**!**"
		}
		b.WriteString(fmt.Sprintf("| %s | %s | %s | %s |\n", icon, i.Zone, i.Record, i.Message))
	}
	b.WriteString("\n")
	return b.String()
}

// Changesets validates a list of changesets against live zone records.
func Changesets(changesets []diff.Changeset, liveByZone map[string][]diff.LiveRecord) Result {
	var result Result

	for _, cs := range changesets {
		live := liveByZone[cs.Zone]
		issues := validateChangeset(cs, live)
		result.Issues = append(result.Issues, issues...)
	}

	return result
}

func validateChangeset(cs diff.Changeset, live []diff.LiveRecord) []Issue {
	var issues []Issue

	// Build a set of live records for quick lookup
	type liveKey struct {
		Name    string
		Type    string
		Content string
	}
	liveSet := make(map[liveKey]bool)
	liveCNAMEs := make(map[string]bool)    // names that have a CNAME
	liveNonCNAMEs := make(map[string]bool) // names that have a non-CNAME record
	for _, lr := range live {
		liveSet[liveKey{lr.Name, strings.ToUpper(lr.Type), normalizeTXT(lr.Type, lr.Content)}] = true
		if strings.ToUpper(lr.Type) == "CNAME" {
			liveCNAMEs[lr.Name] = true
		} else if strings.ToUpper(lr.Type) != "SOA" && strings.ToUpper(lr.Type) != "NS" {
			liveNonCNAMEs[lr.Name] = true
		}
	}

	for _, ch := range cs.Changes {
		switch ch.Action {
		case diff.ActionCreate:
			r := ch.Record
			recordID := fmt.Sprintf("%s %s", displayName(r.NormalizedName()), r.Type)

			// Check if this exact record already exists in the zone
			key := liveKey{r.NormalizedName(), strings.ToUpper(r.Type), normalizeTXT(r.Type, r.Content)}
			if liveSet[key] {
				issues = append(issues, Issue{
					Severity: SeverityError,
					Zone:     cs.Zone,
					Record:   recordID,
					Message:  fmt.Sprintf("record already exists in zone with content `%s` — possible orphan from a previous run", r.Content),
				})
			}

			// Validate CNAME conflicts
			if strings.ToUpper(r.Type) == "CNAME" && liveNonCNAMEs[r.NormalizedName()] {
				issues = append(issues, Issue{
					Severity: SeverityError,
					Zone:     cs.Zone,
					Record:   recordID,
					Message:  "CNAME cannot coexist with other record types at the same name",
				})
			}
			if strings.ToUpper(r.Type) != "CNAME" && liveCNAMEs[r.NormalizedName()] {
				issues = append(issues, Issue{
					Severity: SeverityError,
					Zone:     cs.Zone,
					Record:   recordID,
					Message:  fmt.Sprintf("cannot create %s record — a CNAME already exists at this name", r.Type),
				})
			}

			// Validate content format
			issues = append(issues, validateContent(cs.Zone, recordID, r.Type, r.Content, r.Priority)...)

		case diff.ActionUpdate:
			r := ch.Record
			recordID := fmt.Sprintf("%s %s", displayName(r.NormalizedName()), r.Type)
			issues = append(issues, validateContent(cs.Zone, recordID, r.Type, r.Content, r.Priority)...)

		case diff.ActionDelete:
			// Warn when deleting in full mode to make deletions visible
			if cs.Manage == "full" && ch.Current != nil {
				recordID := fmt.Sprintf("%s %s", displayName(ch.Current.Name), ch.Current.Type)
				issues = append(issues, Issue{
					Severity: SeverityWarning,
					Zone:     cs.Zone,
					Record:   recordID,
					Message:  fmt.Sprintf("will be deleted (full management mode) — content: `%s`", ch.Current.Content),
				})
			}
		}
	}

	return issues
}

func validateContent(zone, recordID, recordType, content string, priority int) []Issue {
	var issues []Issue
	rType := strings.ToUpper(recordType)

	switch rType {
	case "A":
		ip := net.ParseIP(content)
		if ip == nil || ip.To4() == nil {
			issues = append(issues, Issue{
				Severity: SeverityError,
				Zone:     zone,
				Record:   recordID,
				Message:  fmt.Sprintf("invalid IPv4 address: `%s`", content),
			})
		}

	case "AAAA":
		ip := net.ParseIP(content)
		if ip == nil || ip.To4() != nil {
			issues = append(issues, Issue{
				Severity: SeverityError,
				Zone:     zone,
				Record:   recordID,
				Message:  fmt.Sprintf("invalid IPv6 address: `%s`", content),
			})
		}

	case "MX":
		if priority <= 0 {
			issues = append(issues, Issue{
				Severity: SeverityWarning,
				Zone:     zone,
				Record:   recordID,
				Message:  "MX record has no priority set",
			})
		}
		if net.ParseIP(content) != nil {
			issues = append(issues, Issue{
				Severity: SeverityError,
				Zone:     zone,
				Record:   recordID,
				Message:  "MX content must be a hostname, not an IP address",
			})
		}

	case "SRV":
		parts := strings.Fields(content)
		if len(parts) != 3 {
			issues = append(issues, Issue{
				Severity: SeverityError,
				Zone:     zone,
				Record:   recordID,
				Message:  fmt.Sprintf("SRV content must be \"weight port target\", got `%s`", content),
			})
		}
		if priority <= 0 {
			issues = append(issues, Issue{
				Severity: SeverityWarning,
				Zone:     zone,
				Record:   recordID,
				Message:  "SRV record has no priority set",
			})
		}

	case "CNAME", "ALIAS":
		if net.ParseIP(content) != nil {
			issues = append(issues, Issue{
				Severity: SeverityError,
				Zone:     zone,
				Record:   recordID,
				Message:  fmt.Sprintf("%s content must be a hostname, not an IP address", rType),
			})
		}

	case "CAA":
		parts := strings.Fields(content)
		if len(parts) < 3 {
			issues = append(issues, Issue{
				Severity: SeverityError,
				Zone:     zone,
				Record:   recordID,
				Message:  fmt.Sprintf("CAA content must be \"flag tag value\", got `%s`", content),
			})
		}
	}

	return issues
}

func displayName(name string) string {
	if name == "" {
		return "@"
	}
	return name
}

// normalizeTXT strips surrounding quotes from TXT content for comparison.
func normalizeTXT(recordType, content string) string {
	if strings.ToUpper(recordType) == "TXT" {
		content = strings.TrimPrefix(content, "\"")
		content = strings.TrimSuffix(content, "\"")
	}
	return content
}
