# dependabot-bouncer

A command-line tool to manage GitHub dependency updates, supporting both approve and recreate modes for Dependabot initiated pull requests.

## Features

- Automatically approve or recreate Dependabot pull requests
- Close stale PRs with specific labels based on age
- Flexible deny lists for packages and organizations
- YAML-based configuration file support
- Per-repository configuration overrides
- Command-line flags for one-off operations

## Installation

```bash
go install github.com/promiseofcake/dependabot-bouncer/cmd/dependabot-bouncer@latest
```

## Usage

The tool requires a GitHub token with appropriate permissions. You can provide it via:

- Environment variable: `export USER_GITHUB_TOKEN=your_github_token`
- Command-line flag: `--github-token=your_token`
- Config file: `github-token: your_token`

If no token is configured, the tool falls back to `gh auth token` (requires [GitHub CLI](https://cli.github.com/) with `gh auth login`).

### Commands

```bash
# Approve passing dependency updates
dependabot-bouncer approve owner/repo

# Recreate all dependency updates (including failing ones)
dependabot-bouncer recreate owner/repo

# Check for open Dependabot PRs across multiple repositories
dependabot-bouncer check
dependabot-bouncer check owner1/repo1 owner2/repo2

# Close old PRs with a specific label
dependabot-bouncer close owner/repo --older-than 720h
dependabot-bouncer close owner/repo --older-than 720h --dry-run

# Show help
dependabot-bouncer --help
dependabot-bouncer approve --help
```

### Global Flags

- `--config`: Path to config file (default: `~/.dependabot-bouncer/config.yaml`)
- `--github-token`: GitHub token (overrides env var, config, and `gh auth token` fallback)
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

# Override token for one-off use
dependabot-bouncer approve myorg/payment-api \
  --github-token ghp_differenttoken

# Close PRs with "dependencies" label older than 30 days
dependabot-bouncer close myorg/user-service --older-than 720h

# Close PRs older than 6 months (dry run first)
dependabot-bouncer close myorg/user-service --older-than 4320h --dry-run

# Close PRs with a different label
dependabot-bouncer close myorg/user-service --older-than 720h --label stale
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

- Uses Viper for configuration management
- Automatically loads from `~/.dependabot-bouncer/config.yaml`
- Supports environment variables with `DEPENDABOT_BOUNCER_` prefix
- Command-line flags take precedence over config file


### Command Modes

- **approve**: Only processes PRs with passing CI checks
- **recreate**: Processes all PRs, including those with failing CI
- **close**: Closes PRs matching a label that are older than a specified duration

### Close Command

The `close` command helps clean up stale dependency PRs by closing those older than a specified duration.

**Flags:**
- `--older-than`: Required. Duration threshold using Go's `time.Duration` format
- `--label`: Label to filter PRs by (default: `dependencies`)
- `--dry-run`: Preview which PRs would be closed without closing them

**Duration reference:**
| Duration | Value |
|----------|-------|
| 30 days  | `720h` |
| 90 days  | `2160h` |
| 6 months | `4320h` |
| 1 year   | `8760h` |

### Package Filtering

- Package matching is case-insensitive and supports partial matches
- Organization names are extracted from package paths:
  - NPM scoped: `@datadog/browser-rum` → `datadog`
  - GitHub: `github.com/datadog/datadog-go` → `datadog`
- All denied packages and organizations are skipped with a log message
