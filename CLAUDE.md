# dnsync

A GitHub Action written in Go that manages DNS records at DNSimple based on a declarative YAML config file.

## Project Goals

- Declarative DNS management: define DNS records in YAML, apply them via GitHub Actions
- Support multiple zones in a single config file
- Two management modes per zone: `full` (own the entire zone) and `partial` (only manage declared records)
- Plan/apply workflow: post change plans as PR comments, apply on merge to main
- Safe by default: protect immutable records (SOA, NS at apex), require explicit mode for deletions

## Milestones

### M1 — Core Engine (current)
- [x] Project scaffolding and Go module
- [ ] Config parsing and validation (`internal/config`)
- [ ] Diff engine with full/partial mode support (`internal/diff`)
- [ ] Plan formatting as markdown (`internal/plan`)
- [ ] DNSimple API client wrapper (`internal/dnsimple`)
- [ ] GitHub PR comment integration (`internal/github`)
- [ ] Main entrypoint orchestration (`main.go`)
- [ ] Unit tests for all packages
- [ ] Test fixtures in `testdata/`

### M2 — GitHub Action Packaging
- [ ] `action.yml` with inputs/outputs
- [ ] `Dockerfile` for container action
- [ ] End-to-end workflow example

### M3 — Documentation
- [ ] README with usage, config reference, and testing procedures
- [ ] Example `dns.yaml` configs

## Architecture

```
main.go → config.Load() → dnsimple.Fetch() → diff.Compute() → plan.Format() / dnsimple.Apply()
                                                                  ↓
                                                          github.PostComment()
```

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
