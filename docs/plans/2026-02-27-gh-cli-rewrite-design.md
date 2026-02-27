# Rewrite: Shell Out to gh CLI

## Problem

The codebase grew complex managing GitHub API interactions: raw GraphQL for auto-merge, dual CI state detection (commit statuses vs check runs), mergeable state polling, sentinel errors. The `gh` CLI already handles all of these edge cases. We should use it.

## Solution

Replace all `go-github` API calls with `gh` CLI commands. Drop the `go-github` dependency entirely. The tool becomes a thin orchestration layer around `gh` with deny list logic on top.

## Scope

3 commands: `approve`, `recreate`, `check`. The `close` command is dropped for now.

Auth is handled entirely by `gh auth` — no token flags or env vars.

## What Gets Deleted

- `EnableAutoMerge`, `MergePullRequest`, `GetPRMergeableState`, `RebasePullRequest`, `getCIState`
- `ApprovePullRequests`, `RebasePullRequests`, `RecreatePullRequests`
- `GetDependencyUpdates`, `GetDependabotPRs`, `GetDependabotPRsWithDenyList`
- `GetOldLabeledPRs`, `ClosePullRequests`, `formatDuration`
- `ErrPRClean`, `GraphQLURL`, `dependabotUserID`, `NewGithubClient`
- `resolveGitHubToken`, `httpClient()`
- `go-github/v72` and all transitive deps

## What Stays

- `extractPackageInfo`, `isDenied` + all tests
- Data types: `DependencyUpdateQuery`, `PRInfo` (simplified)
- Deny list config (viper), `buildDenyLists`, `parseRepo`, `removeDuplicates`
- Cobra CLI structure

## Data Flow

### Listing PRs

Single `gh pr list` call per repo:

```bash
gh pr list --repo owner/repo --base main \
  --json number,title,url,author,mergeStateStatus,statusCheckRollup \
  --limit 100
```

Filter in Go:
1. `author.login == "app/dependabot"`
2. `extractPackageInfo` + `isDenied` (existing logic)
3. CI filtering (approve mode): all `statusCheckRollup` entries completed with success/skipped/neutral
4. `mergeStateStatus`: `BEHIND` triggers rebase, `CLEAN` merges directly

### Approve Flow (per PR)

1. `gh pr review --approve --repo owner/repo NUMBER`
2. If `mergeStateStatus == "BEHIND"`: `gh pr comment --repo owner/repo NUMBER --body "@dependabot rebase"`
3. `gh pr merge --auto --squash --repo owner/repo NUMBER`

`gh pr merge --auto` handles clean-vs-pending: merges immediately if ready, enables auto-merge if waiting on checks.

### Recreate Flow (per PR)

1. `gh pr comment --repo owner/repo NUMBER --body "@dependabot recreate"`

### Check Flow

Same `gh pr list` call, display PR info with status. No actions.

## Error Handling

Each `gh` command runs per-PR. On failure, log a warning with stderr and continue to the next PR.

## Testing

Deny list tests stay as-is. JSON parsing tested with static fixtures. Actual `gh` calls are integration-level (not mocked).
