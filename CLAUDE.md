# dnsync

A GitHub Action written in Go that manages DNS records at DNSimple based on a declarative YAML config file.

## Project Goals

- Declarative DNS management: define DNS records in YAML, apply them via GitHub Actions
- Support multiple zones in a single config file
- Two management modes per zone: `full` (own the entire zone) and `partial` (only manage declared records)
- Plan/apply workflow: post change plans as PR comments, apply on merge to main
- Safe by default: protect immutable records (SOA, NS at apex), require explicit mode for deletions
- Historical audit log for zone change tracking and AI-assisted time-travel restoration

## Milestones

### M1 — Core Engine [complete]
- [x] Project scaffolding and Go module
- [x] Config parsing and validation (`internal/config`)
- [x] Diff engine with full/partial mode support (`internal/diff`)
- [x] Plan formatting as markdown (`internal/plan`)
- [x] DNSimple API client wrapper (`internal/dnsimple`)
- [x] GitHub PR comment integration (`internal/github`)
- [x] Main entrypoint orchestration (`main.go`)
- [x] Unit tests for all packages
- [x] Test fixtures in `testdata/`

### M2 — GitHub Action Packaging [complete]
- [x] `action.yml` with inputs/outputs
- [x] `Dockerfile` for container action
- [x] End-to-end workflow example

### M3 — State & Validation [complete]
- [x] State file tracking for partial mode deletions (`internal/state`)
- [x] Pre-validation of changes in plan mode (`internal/validate`)
- [x] Reconcile mode for cleaning up orphaned records
- [x] TXT and CAA content normalization (DNSimple quote handling)
- [x] Multi-value record type handling (MX, TXT, SRV, NS)

### M4 — Audit Log [complete]
- [x] Audit log with timestamped entries and zone snapshots (`internal/audit`)
- [x] Record change history queries
- [x] Snapshot-at-time queries for zone restoration
- [x] Snapshot-to-config conversion for generating dns.yaml

### M5 — Documentation [complete]
- [x] README with usage, config reference, and testing procedures
- [x] Management mode documentation (full vs partial)
- [x] Audit log documentation with AI prompting examples

## Architecture

```
main.go → config.Load() → state.Load() → audit.Load()
                                              ↓
        dnsimple.Fetch() → diff.Compute(desired, live, prevState)
                                              ↓
                              validate.Changesets(changes, live)
                                              ↓
                    plan.Format() / dnsimple.Apply() → state.Save() + audit.Save()
                         ↓
                 github.PostComment()
```

State and audit files are committed by the GitHub Actions workflow (not the Go binary).

## Development

- Language: Go 1.22+
- Go commands must be run inside a devcontainer (Go is not installed on the host)
- Run tests: `go test ./...`
- Build: `go build -o dnsync .`

## Key Design Decisions

- Records are matched by `(name, type)` tuple
- In `full` mode, SOA and NS records at the zone apex are never deleted
- PR comments use a hidden HTML marker (`<!-- dnsync-plan -->`) to update in place
- Config supports multiple zones with independent management modes
- State file (`.dnsync.state.json`) tracks previously managed records for partial mode deletion
- State and audit files are committed by the workflow step, not the Go binary (Docker containers don't have git access to the workspace)
- State matching uses content keys (`name/type/content`) for multi-value record support
- TXT and CAA content is normalized (strip quotes) before comparison — DNSimple wraps these in quotes
- Multi-value record types (MX, TXT, SRV, NS) create new records on content mismatch instead of updating
- Audit log (`.dnsync.audit.json`) records every apply/reconcile with full zone snapshots for time-travel queries
