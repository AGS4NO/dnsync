# dnsync

A GitHub Action that manages DNS records at [DNSimple](https://dnsimple.com) from a declarative YAML configuration file. Define your DNS records in code, review changes in pull requests, and apply them automatically on merge.

## Features

- **Declarative DNS**: Define all your DNS records in a YAML file or BIND zone file
- **Multi-zone support**: Manage multiple DNS zones from one config file
- **Full zone management**: dnsync owns the entire zone — records not in your config are deleted (except SOA and apex NS)
- **Plan/apply workflow**: Preview changes as PR comments, apply on merge to main
- **Safe by default**: Immutable records (SOA, apex NS) are never deleted

## Quick Start

### 1. Create a DNS config file

Add a `dns.yaml` to your repository:

```yaml
zones:
  - zone: example.com
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

| Field | Type | Description |
|-------|------|-------------|
| `zone` | string | Domain name (required) |
| `records` | list | List of DNS records (required) |

All zones are fully managed. Records in the zone that are not in your config will be deleted on the next apply (except SOA and apex NS records, which are always protected).

**Important**: Make sure your config includes all records you want to keep. Any record not listed will be removed.

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

## BIND Zone File Support

As an alternative to YAML, you can define DNS records using a standard BIND zone file. This is useful if you already maintain BIND zone files or prefer the traditional format.

### BIND zone file example

Create a `dns.zone` file:

```
$ORIGIN example.com.
$TTL 3600

@       IN  A       192.0.2.1
www     IN  A       192.0.2.2
@       IN  AAAA    2001:db8::1
blog    IN  CNAME   www.example.com.
@       IN  MX  10  mail1.example.com.
@       IN  MX  20  mail2.example.com.
@       IN  TXT     "v=spf1 include:_spf.google.com ~all"
_sip._tcp IN SRV 10 60 5060 sip.example.com.
sub     IN  NS      ns1.example.com.
@       IN  CAA     0 issue "letsencrypt.org"
```

### Using BIND format in the workflow

Set `config-format: bind` and point `config-file` to your zone file:

```yaml
- uses: ags4no/dnsync@v0.1.0
  with:
    dnsimple-token: ${{ secrets.DNSIMPLE_TOKEN }}
    dnsimple-account-id: ${{ secrets.DNSIMPLE_ACCOUNT_ID }}
    config-file: dns.zone
    config-format: bind
    mode: ${{ github.event_name == 'push' && 'apply' || 'plan' }}
  env:
    GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}
```

### BIND format details

- The zone name is extracted from the `$ORIGIN` directive (required)
- SOA records are ignored — dnsync does not manage SOA records
- Each BIND file defines a single zone. For multi-zone setups, use separate files with separate action steps, or use the YAML format
- All standard record types are supported: A, AAAA, CNAME, MX, TXT, SRV, NS, CAA

## Historical DNS Queries

dnsync uses git history as its audit trail. Since your DNS config is checked into the repository, you can use git to answer questions about DNS changes over time.

### Using git history for DNS archaeology

```bash
# Show all changes to DNS config
git log --oneline dns.yaml

# Show the DNS config at a specific point in time
git show HEAD~5:dns.yaml

# Show the config on a specific date
git log --until="2026-04-15" -1 --format="%H" | xargs -I{} git show {}:dns.yaml

# Show what changed in a specific commit
git show <commit-sha> -- dns.yaml
```

### AI-assisted DNS management

AI agents (Claude, GitHub Copilot, etc.) can use git history to answer natural language queries about your DNS:

- **"Restore my zone to April 15"** — the agent runs `git log` to find the config at that date, then edits `dns.yaml` to match. You commit and merge to apply.
- **"When was the www record last changed?"** — the agent runs `git log -p dns.yaml` and searches for changes to the www record.
- **"What did the MX records look like before the April 18th change?"** — the agent checks out the config just before that date.

No special audit file is needed — the complete history lives in git.

## Action Inputs

| Input | Required | Default | Description |
|-------|----------|---------|-------------|
| `dnsimple-token` | Yes | | DNSimple API token |
| `dnsimple-account-id` | Yes | | DNSimple account ID |
| `config-file` | No | `dns.yaml` | Path to the config file (YAML or BIND zone file) |
| `config-format` | No | `yaml` | Config file format: `yaml` or `bind` |
| `mode` | No | `plan` | `plan` to preview, `apply` to execute |

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
| `internal/config` | YAML and BIND zone file parsing, validation, default values, error cases, record normalization |
| `internal/diff` | Create/update/delete detection, immutable record protection, multi-value records |
| `internal/plan` | Markdown and text formatting, multi-zone output, edge cases |
| `internal/validate` | Duplicate detection, CNAME conflicts, content format validation (A/AAAA/MX/SRV/CAA/CNAME), TXT normalization |

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
```

**Plan** — preview changes without applying:

```bash
INPUT_MODE=plan ./dnsync
```

**Apply** — execute the changes:

```bash
INPUT_MODE=apply ./dnsync
```

When running locally (outside GitHub Actions), the PR comment posting will be skipped automatically since `GITHUB_REF` is not set. The plan output will still print to stdout.

### Testing with a Specific Config

You can point to any config file, including the test fixtures:

```bash
INPUT_CONFIG_FILE="testdata/full_zone.yaml" INPUT_MODE=plan ./dnsync
```

### Local CLI testing with BIND format

```bash
INPUT_CONFIG_FILE="dns.zone" INPUT_CONFIG_FORMAT="bind" INPUT_MODE=plan ./dnsync
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
│   ├── config/             # Config parsing (YAML and BIND) and validation
│   ├── diff/               # Desired vs live record diffing
│   ├── plan/               # Change plan formatting (markdown, text)
│   ├── validate/           # Pre-validation of changes
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
- **`internal/validate`** — Change validation

Only `internal/dnsimple` and `internal/github` require live API access, and integration testing is handled by the maintainers.

### Development Environment

- Go 1.22+ is required (a devcontainer configuration is included)
- No external services are needed to run unit tests
- The project uses only the Go standard library and three dependencies:
  - `github.com/dnsimple/dnsimple-go` — DNSimple API client
  - `github.com/google/go-github/v60` — GitHub API client
  - `gopkg.in/yaml.v3` — YAML parsing
  - `github.com/miekg/dns` — BIND zone file parsing

### Security

- **Never commit secrets** (API tokens, account IDs) to the repository
- PRs that modify GitHub Actions workflows will receive extra scrutiny
- If you discover a security vulnerability, please report it privately via GitHub Security Advisories rather than opening a public issue

## License

MIT
