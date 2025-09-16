# github-deps

A command-line tool to manage GitHub dependency updates, supporting both approve and recreate modes for Dependabot initiated pull requests.

## Features

- Automatically approve or recreate Dependabot pull requests
- Flexible deny lists for packages and organizations
- YAML-based configuration file support
- Per-repository configuration overrides
- Command-line flags for one-off operations

## Installation

```bash
go install github.com/promiseofcake/github-deps/cmd/github-approve-deps@latest
```

## Usage

The tool requires a GitHub token with appropriate permissions. You can provide it via:
- Environment variable: `export USER_GITHUB_TOKEN=your_github_token`
- Command-line flag: `--github-token=your_token`
- Config file: `github-token: your_token`

### Commands

```bash
# Approve passing dependency updates
github-approve-deps approve owner repo

# Recreate all dependency updates (including failing ones)
github-approve-deps recreate owner repo

# Show help
github-approve-deps --help
github-approve-deps approve --help
```

### Global Flags

- `--config`: Path to config file (default: `~/.github-deps/config.yaml`)
- `--github-token`: GitHub token (overrides env var and config)
- `--deny-packages`: Additional packages to deny (can be used multiple times)
- `--deny-orgs`: Additional organizations to deny (can be used multiple times)

## Examples

```bash
# Approve all passing updates
github-approve-deps approve promiseofcake github-deps

# Recreate all updates (including failing ones)
github-approve-deps recreate promiseofcake github-deps

# Deny specific packages via command line
github-approve-deps approve promiseofcake github-deps \
  --deny-packages lodash \
  --deny-packages moment

# Deny organizations
github-approve-deps approve promiseofcake github-deps \
  --deny-orgs datadog \
  --deny-orgs sentry

# Use custom config file
github-approve-deps approve promiseofcake github-deps \
  --config ./my-config.yaml

# Override token for one-off use
github-approve-deps approve promiseofcake github-deps \
  --github-token ghp_differenttoken
```

## Configuration

### Configuration File

The tool supports a YAML configuration file at `~/.github-deps/config.yaml` for persistent settings.

**Example configuration:**

```yaml
# Global settings apply to all repositories
global:
  denied_packages:
    - lodash           # Security and bundle size concerns
    - moment           # Deprecated, use date-fns or dayjs
    - jquery           # Prefer modern vanilla JS
    
  denied_orgs:
    - datadog          # Expensive monitoring
    - sentry           # Using alternative

# Repository-specific configurations
repositories:
  myorg/frontend-app:
    denied_packages:
      - bootstrap@3     # Old version
      - angular         # This is a React project
    denied_orgs:
      - analytics-corp  # Different provider
    ignored_prs:
      - 123            # Known issues
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
- Automatically loads from `~/.github-deps/config.yaml`
- Supports environment variables with `GITHUB_DEPS_` prefix
- Command-line flags take precedence over config file
