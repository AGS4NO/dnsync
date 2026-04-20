# dnsync

A GitHub Action that manages DNS records at [DNSimple](https://dnsimple.com) from a declarative YAML configuration file. Define your DNS records in code, review changes in pull requests, and apply them automatically on merge.

## Features

- **Declarative DNS**: Define all your DNS records in a single YAML file
- **Multi-zone support**: Manage multiple DNS zones from one config file
- **Full or partial management**: Choose whether dnsync owns the entire zone or only manages specific records
- **Plan/apply workflow**: Preview changes as PR comments, apply on merge to main
- **Safe defaults**: Partial management mode by default, immutable records (SOA, apex NS) are never deleted

## Quick Start

### 1. Create a DNS config file

Add a `dns.yaml` to your repository:

```yaml
zones:
  - zone: example.com
    manage: full
    records:
      - name: "@"
        type: A
        content: 192.0.2.1
        ttl: 3600
      - name: www
        type: CNAME
        content: example.com
        ttl: 3600
      - name: "@"
        type: MX
        content: mail.example.com
        ttl: 3600
        priority: 10

  - zone: staging.example.com
    manage: partial
    records:
      - name: api
        type: A
        content: 203.0.113.5
        ttl: 300
```

### 2. Set up the GitHub Action workflow

Create `.github/workflows/dns.yml`:

```yaml
name: DNS Management

on:
  pull_request:
    paths: [dns.yaml]
  push:
    branches: [main]
    paths: [dns.yaml]

permissions:
  contents: write       # Required for committing state file after apply
  pull-requests: write

jobs:
  dns:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - uses: ags4no/dnsync@v0.1.0
        with:
          dnsimple-token: ${{ secrets.DNSIMPLE_TOKEN }}
          dnsimple-account-id: ${{ secrets.DNSIMPLE_ACCOUNT_ID }}
          mode: ${{ github.event_name == 'push' && 'apply' || 'plan' }}
        env:
          GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}
```

### 3. Add secrets

In your repository settings, add:
- `DNSIMPLE_TOKEN`: Your DNSimple API token
- `DNSIMPLE_ACCOUNT_ID`: Your DNSimple account ID

## Configuration Reference

### Top-level

| Field | Type | Description |
|-------|------|-------------|
| `zones` | list | List of zone configurations (required) |

### Zone Configuration

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `zone` | string | | Domain name (required) |
| `manage` | string | `partial` | Management mode: `full` or `partial` |
| `records` | list | | List of DNS records (required) |

### Management Modes

| Mode | Records in file & zone | Records only in file | Records only in zone |
|------|----------------------|---------------------|---------------------|
| `full` | Update if different | Create | **Delete** |
| `partial` | Update if different | Create | Leave alone (unless previously managed) |

#### Full mode

Use `full` when dnsync should be the **single source of truth** for the entire zone. Any record in the zone that is not in your config file will be deleted on the next apply (except SOA and apex NS records, which are always protected).

This is the right choice when:
- dnsync is the only tool managing this zone
- You want strict enforcement — no manual edits should persist
- You want a complete, auditable record of every DNS entry in git

**Warning**: If other tools or team members manage records in this zone outside of dnsync, `full` mode will delete their records.

#### Partial mode

Use `partial` when dnsync should only manage **specific records** in the zone, leaving everything else untouched. dnsync tracks which records it has previously applied via a [state file](#state-tracking), so it can distinguish between:

- **Records it manages** — created, updated, or deleted based on your config
- **Records it doesn't manage** — left completely alone, even if dnsync has never seen them

This is the right choice when:
- Other tools or team members also manage records in this zone
- You only want to automate a subset of your DNS records
- You want a safer default that won't accidentally delete anything unexpected

When you remove a record from your config in partial mode, dnsync checks the state file and deletes it if it was previously managed. Records that were never managed by dnsync are never touched.

On the first run (no state file), partial mode will only create and update — never delete — until the state file is established.

### Record Configuration

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `name` | string | No | Record name. Use `@` or omit for zone apex |
| `type` | string | Yes | Record type (A, AAAA, CNAME, MX, TXT, SRV, NS, etc.) |
| `content` | string | Yes | Record value |
| `ttl` | int | No | Time to live in seconds |
| `priority` | int | No | Priority (for MX, SRV records) |

### Records with Priority (MX, SRV)

MX and SRV records use a separate `priority` field — do **not** include the priority value inside `content`.

**MX records** — `content` is the mail server hostname, `priority` is separate:

```yaml
records:
  - name: "@"
    type: MX
    content: mail.example.com            # just the hostname
    ttl: 3600
    priority: 10
```

**SRV records** — `content` is `"weight port target"` (space-separated), `priority` is separate:

```yaml
records:
  - name: _sip._tcp
    type: SRV
    content: "60 5060 sip.example.com"   # weight, port, target
    ttl: 3600
    priority: 10
```

This matches how DNSimple's API handles these record types.

### Multi-value Records

Record types that support multiple values for the same name (MX, TXT, SRV, NS) can be specified multiple times:

```yaml
records:
  - name: "@"
    type: MX
    content: mail1.example.com
    priority: 10
  - name: "@"
    type: MX
    content: mail2.example.com
    priority: 20
```

## State Tracking

dnsync uses a state file (`.dnsync.state.json`) to track which records it has previously applied. The state file is automatically committed and pushed to the repo after each successful `apply` run. **You should commit this file to your repo** and not add it to `.gitignore`.

The state file is primarily used by [partial mode](#partial-mode) to distinguish between records dnsync manages and records it should leave alone. In full mode, the state file is maintained for consistency but is not required for correct behavior.

## Audit Log

dnsync maintains an audit log (`.dnsync.audit.json`) that records the full history of DNS changes. After each `apply` or `reconcile`, an entry is appended containing:

- **Timestamp** — when the changes were applied
- **Changes** — every create, update, and delete with old and new values
- **Zone snapshot** — the complete state of the zone after the changes

The audit log is committed to the repo alongside the state file, providing a git-tracked history of your DNS infrastructure.

### AI-Powered Zone Management

The audit log is designed to be read by AI agents (Claude, GitHub Copilot, etc.) to answer natural language queries about your DNS history. Example prompts:

- **"Restore my zone to 2026-04-15"** — the agent reads the audit log, finds the snapshot at that date, and edits `dns.yaml` to match that state. You then commit and merge to apply.
- **"When was the last time the www record was updated?"** — the agent searches the audit log for changes affecting the `www` record and reports the timestamps and details.
- **"Show me all changes made to dnsync.net in the last month"** — the agent filters entries by date range and summarizes the changes.
- **"What did the MX records look like before the April 18th change?"** — the agent finds the snapshot just before that date and reports the MX records.

The audit file uses a self-documenting JSON format with a `_description` field explaining its purpose, so AI agents can understand the file without additional context.

### Audit Log Format

```json
{
  "_description": "dnsync audit log — records all DNS changes...",
  "entries": [
    {
      "timestamp": "2026-04-19T15:30:00Z",
      "action": "apply",
      "zones": {
        "dnsync.net": {
          "manage": "partial",
          "changes": [
            {
              "action": "update",
              "name": "www",
              "type": "A",
              "content": "192.0.2.2",
              "ttl": 3600,
              "old_content": "192.0.2.1",
              "old_ttl": 3600
            }
          ],
          "snapshot": [
            {"name": "www", "type": "A", "content": "192.0.2.2", "ttl": 3600},
            {"name": "test", "type": "TXT", "content": "dnsync-managed-record", "ttl": 3600}
          ]
        }
      }
    }
  ]
}
```

## Action Inputs

| Input | Required | Default | Description |
|-------|----------|---------|-------------|
| `dnsimple-token` | Yes | | DNSimple API token |
| `dnsimple-account-id` | Yes | | DNSimple account ID |
| `config-file` | No | `dns.yaml` | Path to the config file |
| `mode` | No | `plan` | `plan` to preview, `apply` to execute, `reconcile` to clean up orphans |
| `state-file` | No | `.dnsync.state.json` | Path to the state tracking file |
| `audit-file` | No | `.dnsync.audit.json` | Path to the audit log file |

## Testing

### Prerequisites

- Go 1.22+ (run inside devcontainer if Go is not installed locally)
- No external services needed for unit tests

### Running Unit Tests

```bash
# Run all tests
go test ./...

# Run tests with verbose output
go test -v ./...

# Run tests for a specific package
go test -v ./internal/config/
go test -v ./internal/diff/
go test -v ./internal/plan/
go test -v ./internal/state/
```

### Test Coverage

```bash
# Generate coverage report
go test -coverprofile=coverage.out ./...

# View coverage in browser
go tool cover -html=coverage.out

# View coverage summary
go tool cover -func=coverage.out
```

### What's Tested

| Package | What's covered |
|---------|---------------|
| `internal/config` | YAML parsing, validation, default values, error cases, record normalization |
| `internal/diff` | Create/update/delete detection, full vs partial mode, state-based deletion in partial mode, immutable record protection, multi-value records |
| `internal/plan` | Markdown and text formatting, multi-zone output, edge cases |
| `internal/state` | State file load/save, config-to-state conversion, deterministic serialization, missing file handling |
| `internal/validate` | Duplicate detection, CNAME conflicts, content format validation (A/AAAA/MX/SRV/CAA/CNAME), TXT normalization |
| `internal/audit` | Audit log load/save, apply/reconcile recording, snapshot building, record history queries, snapshot-at-time queries, snapshot-to-config conversion |

### Local CLI Testing

You can build and run dnsync as a standalone CLI to test against a real DNSimple account without GitHub Actions.

**Build the binary:**

```bash
go build -o dnsync .
```

**Set required environment variables:**

```bash
export INPUT_DNSIMPLE_TOKEN="your-api-token"
export INPUT_DNSIMPLE_ACCOUNT_ID="your-account-id"
export INPUT_CONFIG_FILE="dns.yaml"
export INPUT_STATE_FILE=".dnsync.state.json"
```

**Plan** — preview changes without applying:

```bash
INPUT_MODE=plan ./dnsync
```

**Apply** — execute the changes:

```bash
INPUT_MODE=apply ./dnsync
```

**Reconcile** — find and remove orphaned records from previous failed runs:

```bash
INPUT_MODE=reconcile ./dnsync
```

When running locally (outside GitHub Actions), the PR comment posting will be skipped automatically since `GITHUB_REF` is not set. The plan output will still print to stdout.

Note: the git commit/push of the state file will also run locally. To avoid this, you can manually save the state file and skip the commit by hitting `Ctrl+C` after "State saved" prints, or temporarily modify the state file path to a throwaway location:

```bash
INPUT_STATE_FILE="/tmp/dnsync-test-state.json" INPUT_MODE=apply ./dnsync
```

### Testing with a Specific Config

You can point to any config file, including the test fixtures:

```bash
INPUT_CONFIG_FILE="testdata/partial_zone.yaml" INPUT_MODE=plan ./dnsync
```

### Docker Build Test

```bash
docker build -t dnsync .
docker run --rm \
  -e INPUT_DNSIMPLE_TOKEN="your-api-token" \
  -e INPUT_DNSIMPLE_ACCOUNT_ID="your-account-id" \
  -e INPUT_CONFIG_FILE="dns.yaml" \
  -e INPUT_MODE="plan" \
  -v $(pwd)/dns.yaml:/dns.yaml \
  dnsync
```

## Project Structure

```
dnsync/
├── action.yml              # GitHub Action metadata
├── Dockerfile              # Container action image
├── main.go                 # Entrypoint and orchestration
├── internal/
│   ├── config/             # YAML config parsing and validation
│   ├── diff/               # Desired vs live record diffing
│   ├── plan/               # Change plan formatting (markdown, text)
│   ├── dnsimple/           # DNSimple API client wrapper
│   └── github/             # GitHub PR comment management
└── testdata/               # Sample config files for testing
```

## Contributing

Contributions are welcome! Here's how to get started:

### Getting Started

1. **Fork the repository** and clone your fork
2. **Create a branch** for your changes:
   ```bash
   git checkout -b my-feature
   ```
3. **Make your changes** and add tests
4. **Run the test suite** to make sure everything passes:
   ```bash
   go test -v ./...
   ```
5. **Open a pull request** against `main`

### Pull Request Process

- All PRs require the `unit-tests` status check to pass before merging
- PRs from outside collaborators require maintainer approval before workflows run — this is a security measure since the CI environment has access to DNS credentials
- Keep PRs focused — one feature or fix per PR
- Add or update tests for any new functionality
- Update documentation (README, CLAUDE.md) if your change affects usage or architecture

### What You Can Work On Without DNS Access

Most of the codebase can be developed and tested without a DNSimple account:

- **`internal/config`** — YAML parsing and validation
- **`internal/diff`** — Record diffing logic (fully unit tested with mock data)
- **`internal/plan`** — Plan formatting (markdown and text output)
- **`internal/state`** — State file management
- **`internal/validate`** — Change validation
- **`internal/audit`** — Audit log and history queries

Only `internal/dnsimple` and `internal/github` require live API access, and integration testing is handled by the maintainers.

### Development Environment

- Go 1.22+ is required (a devcontainer configuration is included)
- No external services are needed to run unit tests
- The project uses only the Go standard library and three dependencies:
  - `github.com/dnsimple/dnsimple-go` — DNSimple API client
  - `github.com/google/go-github/v60` — GitHub API client
  - `gopkg.in/yaml.v3` — YAML parsing

### Security

- **Never commit secrets** (API tokens, account IDs) to the repository
- PRs that modify GitHub Actions workflows will receive extra scrutiny
- If you discover a security vulnerability, please report it privately via GitHub Security Advisories rather than opening a public issue

## License

MIT
