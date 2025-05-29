# github-deps

A command-line tool to manage GitHub dependency updates, supporting both approve and recreate modes for Dependabot initiaiated pull requests.

## Installation

```bash
go install github.com/promiseofcake/github-deps/cmd/github-approve-deps@latest
```

## Usage

The tool requires a GitHub token with appropriate permissions. Set it in your environment:

```bash
export USER_GITHUB_TOKEN=your_github_token
```

### Basic Usage

```bash
# Approve all passing dependency updates
github-approve-deps --owner=organization --repo=repository

# Approve all updates, including failing ones
github-approve-deps --owner=organization --repo=repository --skip-failing=false

# Recreate all dependency updates
github-approve-deps --owner=organization --repo=repository --recreate
```

### Flags

- `--owner`: GitHub organization or user (required)
- `--repo`: GitHub repository name (required)
- `--recreate`: Whether to recreate PRs instead of approving (default: false)
- `--skip-failing`: Whether to skip processing failing PRs (CI failures) (default: true)

## Examples

```bash
# Approve all passing updates in a repository
github-approve-deps --owner=promiseofcake --repo=github-deps

# Recreate all updates, including failing ones
github-approve-deps --owner=promiseofcake --repo=github-deps --recreate --skip-failing=false
```
