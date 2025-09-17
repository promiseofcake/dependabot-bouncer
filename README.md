# dependabot-bouncer

A command-line tool to manage GitHub dependency updates, supporting both approve and recreate modes for Dependabot initiated pull requests.

## Features

- Automatically approve or recreate Dependabot pull requests
- Flexible deny lists for packages and organizations
- YAML-based configuration file support
- Per-repository configuration overrides
- Command-line flags for one-off operations

## Installation

```bash
go install github.com/promiseofcake/dependabot-bouncer/cmd/github-approve-deps@latest
```

## Usage

The tool requires a GitHub token with appropriate permissions. You can provide it via:

- Environment variable: `export USER_GITHUB_TOKEN=your_github_token`
- Command-line flag: `--github-token=your_token`
- Config file: `github-token: your_token`

### Commands

```bash
# Approve passing dependency updates
github-approve-deps approve owner/repo

# Recreate all dependency updates (including failing ones)
github-approve-deps recreate owner/repo

# Check for open Dependabot PRs across multiple repositories
github-approve-deps check
github-approve-deps check owner1/repo1 owner2/repo2

# Show help
github-approve-deps --help
github-approve-deps approve --help
```

### Global Flags

- `--config`: Path to config file (default: `~/.github-approve-deps/config.yaml`)
- `--github-token`: GitHub token (overrides env var and config)
- `--deny-packages`: Additional packages to deny (can be used multiple times)
- `--deny-orgs`: Additional organizations to deny (can be used multiple times)

## Examples

```bash
# Approve all passing updates
github-approve-deps approve myorg/user-service

# Recreate all updates (including failing ones)
github-approve-deps recreate myorg/payment-api

# Check multiple repositories for Dependabot PRs
github-approve-deps check myorg/user-service myorg/payment-api myorg/gateway-service

# Check repositories from config file
github-approve-deps check

# Deny specific packages via command line
github-approve-deps approve myorg/user-service \
  --deny-packages github.com/pkg/errors \
  --deny-packages gopkg.in/mgo.v2

# Deny organizations
github-approve-deps approve myorg/payment-api \
  --deny-orgs datadog \
  --deny-orgs elastic

# Use custom config file
github-approve-deps approve myorg/user-service \
  --config ./my-config.yaml

# Override token for one-off use
github-approve-deps approve myorg/payment-api \
  --github-token ghp_differenttoken
```

## Configuration

### Configuration File

The tool supports a YAML configuration file at `~/.github-approve-deps/config.yaml` for persistent settings.

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

### Command Modes

- **approve**: Only processes PRs with passing CI checks
- **recreate**: Processes all PRs, including those with failing CI

### Package Filtering

- Package matching is case-insensitive and supports partial matches
- Organization names are extracted from package paths:
  - NPM scoped: `@datadog/browser-rum` → `datadog`
  - GitHub: `github.com/datadog/datadog-go` → `datadog`
- All denied packages and organizations are skipped with a log message

### Configuration

- Uses Viper for configuration management
- Automatically loads from `~/.dependabot-bouncer/config.yaml`
- Supports environment variables with `GITHUB_DEPS_` prefix
- Command-line flags take precedence over config file
