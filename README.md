# dependabot-bouncer

A command-line tool to manage GitHub dependency updates, supporting both approve and recreate modes for Dependabot initiated pull requests.

## Features

- Automatically approve Dependabot pull requests with passing CI
- Recreate Dependabot pull requests (including those with failing CI)
- Handle merge conflicts and out-of-date branches automatically
- Enable auto-merge with squash strategy on approved PRs
- Flexible deny lists for packages and organizations with wildcard support
- YAML-based configuration file support
- Per-repository configuration overrides
- Command-line flags for one-off operations

## Prerequisites

- [GitHub CLI](https://cli.github.com/) (`gh`) installed and authenticated via `gh auth login`

## Installation

```bash
go install github.com/promiseofcake/dependabot-bouncer/cmd/dependabot-bouncer@latest
```

## Usage

The tool uses the GitHub CLI (`gh`) for all GitHub API operations. Make sure you are authenticated:

```bash
gh auth login
```

### Commands

```bash
# Approve passing dependency updates
dependabot-bouncer approve owner/repo

# Recreate all dependency updates (including failing ones)
dependabot-bouncer recreate owner/repo

# Check for open Dependabot PRs across multiple repositories
dependabot-bouncer check
dependabot-bouncer check owner1/repo1 owner2/repo2

# Show help
dependabot-bouncer --help
dependabot-bouncer approve --help
```

### Global Flags

- `--config`: Path to config file (default: `~/.dependabot-bouncer/config.yaml`)
- `--deny-packages`: Additional packages to deny (can be used multiple times)
- `--deny-orgs`: Additional organizations to deny (can be used multiple times)

## Examples

```bash
# Approve all passing updates
dependabot-bouncer approve myorg/user-service

# Recreate all updates (including failing ones)
dependabot-bouncer recreate myorg/payment-api

# Check multiple repositories for Dependabot PRs
dependabot-bouncer check myorg/user-service myorg/payment-api myorg/gateway-service

# Check repositories from config file
dependabot-bouncer check

# Deny specific packages via command line
dependabot-bouncer approve myorg/user-service \
  --deny-packages github.com/pkg/errors \
  --deny-packages gopkg.in/mgo.v2

# Deny organizations
dependabot-bouncer approve myorg/payment-api \
  --deny-orgs datadog \
  --deny-orgs elastic

# Use custom config file
dependabot-bouncer approve myorg/user-service \
  --config ./my-config.yaml
```

## Configuration

### Configuration File

The tool supports a YAML configuration file at `~/.dependabot-bouncer/config.yaml` for persistent settings.

**Example configuration:**

```yaml
# Global settings apply to all repositories
global:
  denied_packages:
    - github.com/pkg/errors         # Use stdlib errors
    - github.com/dgrijalva/jwt-go   # Unmaintained
    - gopkg.in/mgo.v2               # Old MongoDB driver
    - "*alpha*"                      # No alpha versions
    - "*beta*"                       # No beta versions
    - "*rc*"                         # No release candidates
    - "*/v0"                         # No v0 packages

  denied_orgs:
    - datadog          # Expensive monitoring
    - elastic          # Using OpenSearch

# Repository configurations
# All repos listed here are checked by 'check' command
repositories:
  # Simple tracking (no special config)
  myorg/user-service: {}
  myorg/payment-api: {}

  # Repository with specific overrides
  myorg/legacy-api:
    denied_packages:
      - github.com/gin-gonic/gin@v1   # Need v1.9+
      - github.com/aws/aws-sdk-go     # Use aws-sdk-go-v2
    denied_orgs:
      - hashicorp      # Licensing concerns
    ignored_prs:
      - 123            # Breaking change
      - 456            # Manual review needed
```

See `config.example.yaml` for a complete example.

### Configuration Priority

Settings are merged in the following order (later overrides earlier):

1. Global config from YAML file
2. Repository-specific config from YAML file
3. Command-line flags

All deny lists are merged (not replaced), so command-line flags add to the configured lists.

## Behavior

### Command Modes

- **approve**: Only processes PRs with passing CI checks. For each PR:
  - PRs with merge conflicts (`DIRTY`) are recreated via `@dependabot recreate`
  - PRs behind the base branch (`BEHIND`) are rebased via `@dependabot rebase`
  - PRs not yet approved are approved
  - Auto-merge is enabled with squash strategy
- **recreate**: Processes all PRs regardless of CI status and comments `@dependabot recreate` on each
- **check**: Lists open Dependabot PRs with their CI status and merge state across one or more repositories

### Package Filtering

- Package matching is case-insensitive and supports partial matches
- Wildcard patterns are supported: `*alpha*`, `*beta*`, `*rc*`, `*/v0`
- Version-specific denials: `github.com/gin-gonic/gin@v1`
- Organization names are extracted from package paths:
  - NPM scoped: `@datadog/browser-rum` → `datadog`
  - GitHub: `github.com/datadog/datadog-go` → `datadog`
  - gopkg.in: `gopkg.in/DataDog/dd-trace-go.v1` → `datadog`
- All denied packages and organizations are skipped with a log message
