# dnsync

A GitHub Action written in Go that manages DNS records at DNSimple based on a declarative config file (YAML or BIND zone file).

## Project Goals

- Declarative DNS management: define DNS records in YAML or BIND zone files, apply them via GitHub Actions
- Support multiple zones in a single YAML config file
- Full zone management: dnsync owns the entire zone, deleting records not in config (except SOA and apex NS)
- Plan/apply workflow: post change plans as PR comments, apply on merge to main
- Safe by default: protect immutable records (SOA, NS at apex)
- Use git history for DNS archaeology and AI-assisted restoration (no separate audit file)

## Milestones

### M1 — Core Engine [complete]
- [x] Project scaffolding and Go module
- [x] Config parsing and validation (`internal/config`)
- [x] Diff engine (`internal/diff`)
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

### M3 — Validation & Content Handling [complete]
- [x] Pre-validation of changes in plan mode (`internal/validate`)
- [x] TXT and CAA content normalization (DNSimple quote handling)
- [x] Multi-value record type handling (MX, TXT, SRV, NS)

### M4 — BIND Zone File Support [complete]
- [x] BIND zone file parser (`internal/config/bind.go`)
- [x] Separate test zones: `dnsync.net` (YAML) and `zonefile.dnsync.net` (BIND)

### M5 — Documentation [complete]
- [x] README with usage, config reference, and testing procedures
- [x] Git history guidance for AI-assisted DNS management

## Architecture

```
main.go → config.Load()
                ↓
        dnsimple.Fetch() → diff.Compute(desired, live)
                                  ↓
                    validate.Changesets(changes, live)
                                  ↓
              plan.Format() / dnsimple.Apply()
                   ↓
           github.PostComment()
```

## Development

- Language: Go 1.22+
- Go commands must be run inside a devcontainer (Go is not installed on the host)
- Run tests: `go test ./...`
- Build: `go build -o dnsync .`

## Test Zones

- `dnsync.net` — managed via `dns.yaml` (YAML format)
- `zonefile.dnsync.net` — managed via `dns.zone` (BIND format)

These are separate zones at DNSimple to avoid conflicts between the two config formats.

## Key Design Decisions

- Records are matched by `(name, type)` tuple
- SOA and NS records at the zone apex are never deleted
- PR comments use a hidden HTML marker (`<!-- dnsync-plan -->`) to update in place
- Config supports multiple zones in a single YAML file
- TXT and CAA content is normalized (strip quotes) before comparison — DNSimple wraps these in quotes
- Multi-value record types (MX, TXT, SRV, NS) create new records on content mismatch instead of updating
- Git history of `dns.yaml`/`dns.zone` serves as the audit trail — no separate state or audit files needed
