# Auto-Merge Migration Design

## Problem

GitHub deprecated Dependabot comment commands (`@dependabot merge`, `@dependabot squash and merge`, etc.) on January 27, 2026. The `approve` command relied on `@dependabot merge` to both approve and trigger merging in one step. This no longer works.

Commands NOT deprecated: `@dependabot rebase`, `@dependabot recreate` — these are unaffected.

## Solution

Replace `@dependabot merge` with two native GitHub API calls:

1. **Approve the PR** via REST API (same as today, just change the review body)
2. **Enable auto-merge** via GraphQL API (`enablePullRequestAutoMerge` mutation, squash strategy)

GitHub then handles waiting for required checks to pass and merging automatically.

## Prerequisites

Repos must have auto-merge enabled in repository settings. The tool will log an error and continue if auto-merge is not available for a given PR.

## Changes

### `internal/scm/deps.go`

Add `NodeID string` to `DependencyUpdateRequest`. The GraphQL mutation requires the PR's global node ID, not the integer PR number.

### `internal/scm/github.go`

1. **`GetDependencyUpdates`**: Capture `p.GetNodeID()` when building `DependencyUpdateRequest` structs.
2. **`ApprovePullRequests`**: Change review body from `@dependabot merge` to `"Approved by dependabot-bouncer"`.
3. **New `EnableAutoMerge` method**: Raw HTTP POST to `https://api.github.com/graphql` with the `enablePullRequestAutoMerge` mutation (merge method: `SQUASH`). Iterates over requests, logs errors per-PR but does not fail the batch.

The `githubClient` struct gains a `token` field (needed for the raw GraphQL call's Authorization header).

### `cmd/dependabot-bouncer/commands.go`

In `runDependencyUpdate` (the approve path), after approving each PR:

1. **Check mergeable state** via `GetPRMergeableState`
2. If **"behind"**: tell dependabot to rebase (`@dependabot rebase`), then enable auto-merge so it merges once CI passes on the rebased branch
3. If **"clean"** (auto-merge returns `ErrPRClean`): merge immediately via `MergePullRequest` (squash)
4. Otherwise: enable auto-merge and let GitHub handle the rest

### `internal/scm/github_test.go`

Test the new `EnableAutoMerge` method using an HTTP test server that verifies the GraphQL request payload.

## CI State Detection

The tool checks CI status using both the commit status API and the check runs API (`getCIState` helper). This is necessary because:
- Traditional CI (CircleCI, Jenkins) reports via commit statuses
- GitHub Actions reports via check runs only
- A repo using only GitHub Actions would show `"pending"` with zero statuses via the commit status API alone

## Unchanged

- `RecreatePullRequests` (`@dependabot recreate`) — not deprecated
- `ClosePullRequests` — uses REST API directly
- All deny list logic, config, CLI structure

## GraphQL Mutation

```graphql
mutation($pullRequestId: ID!, $mergeMethod: PullRequestMergeMethod!) {
  enablePullRequestAutoMerge(input: {
    pullRequestId: $pullRequestId
    mergeMethod: $mergeMethod
  }) {
    pullRequest {
      autoMergeRequest {
        enabledAt
      }
    }
  }
}
```

Called with `pullRequestId` = PR node ID, `mergeMethod` = `SQUASH`.

## Error Handling

- If enabling auto-merge fails with "clean status", fall back to direct merge (PR is already ready)
- If enabling auto-merge fails for other reasons (e.g., repo setting not enabled, insufficient permissions), log the error for that PR and continue processing the rest of the batch
- If rebase or direct merge fails, log a warning and continue
