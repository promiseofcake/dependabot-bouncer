# Auto-Merge Migration Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Replace deprecated `@dependabot merge` with GitHub's native approve + auto-merge API flow.

**Architecture:** The approve command will make two API calls per PR: (1) REST approval review, (2) GraphQL `enablePullRequestAutoMerge` mutation. The `githubClient` struct stores the token for the raw GraphQL call. Errors enabling auto-merge are logged per-PR without failing the batch.

**Tech Stack:** Go 1.25, `google/go-github/v72` (REST), raw `net/http` (GraphQL)

**Design doc:** `docs/plans/2026-02-27-auto-merge-migration-design.md`

---

### Task 1: Add NodeID to DependencyUpdateRequest

**Files:**
- Modify: `internal/scm/deps.go:11-17`

**Step 1: Add NodeID field**

In `internal/scm/deps.go`, add `NodeID` to `DependencyUpdateRequest`:

```go
type DependencyUpdateRequest struct {
	Owner             string
	Repo              string
	PullRequestNumber int
	NodeID            string // GraphQL global ID for the PR
	Title             string // PR title for logging
	PackageName       string // Extracted package name
}
```

**Step 2: Run tests to verify nothing breaks**

Run: `go test ./...`
Expected: All existing tests pass (this field is just additive).

**Step 3: Commit**

```bash
git add internal/scm/deps.go
git commit -m "Add NodeID field to DependencyUpdateRequest"
```

---

### Task 2: Store token on githubClient and populate NodeID

**Files:**
- Modify: `internal/scm/github.go:19-27` (struct + constructor)
- Modify: `internal/scm/github.go:231-237` (first request builder in GetDependencyUpdates)
- Modify: `internal/scm/github.go:251-257` (second request builder in GetDependencyUpdates)

**Step 1: Add token field to githubClient and store it in constructor**

In `internal/scm/github.go`, change the struct and constructor:

```go
type githubClient struct {
	client *github.Client
	token  string
}

func NewGithubClient(client *http.Client, token string) *githubClient {
	return &githubClient{
		client: github.NewClient(client).WithAuthToken(token),
		token:  token,
	}
}
```

**Step 2: Populate NodeID in GetDependencyUpdates**

In `GetDependencyUpdates`, both places where `DependencyUpdateRequest` is built (lines 231-237 and 251-257), add `NodeID: p.GetNodeID()`:

First occurrence (skipFailing=true path, ~line 231):
```go
reqs = append(reqs, DependencyUpdateRequest{
	Owner:             q.Owner,
	Repo:              q.Repo,
	PullRequestNumber: p.GetNumber(),
	NodeID:            p.GetNodeID(),
	Title:             title,
	PackageName:       packageName,
})
```

Second occurrence (skipFailing=false path, ~line 251):
```go
reqs = append(reqs, DependencyUpdateRequest{
	Owner:             q.Owner,
	Repo:              q.Repo,
	PullRequestNumber: p.GetNumber(),
	NodeID:            p.GetNodeID(),
	Title:             title,
	PackageName:       packageName,
})
```

**Step 3: Run tests**

Run: `go test ./...`
Expected: All existing tests pass.

**Step 4: Commit**

```bash
git add internal/scm/github.go
git commit -m "Store token on githubClient and populate NodeID in requests"
```

---

### Task 3: Write failing test for EnableAutoMerge

**Files:**
- Modify: `internal/scm/github_test.go` (add test at end of file)

**Step 1: Write the test**

Add to `internal/scm/github_test.go`:

```go
func TestEnableAutoMerge(t *testing.T) {
	tests := []struct {
		name           string
		responseBody   string
		responseStatus int
		expectError    bool
	}{
		{
			name: "successful auto-merge enable",
			responseBody: `{
				"data": {
					"enablePullRequestAutoMerge": {
						"pullRequest": {
							"autoMergeRequest": {
								"enabledAt": "2026-01-01T00:00:00Z"
							}
						}
					}
				}
			}`,
			responseStatus: 200,
			expectError:    false,
		},
		{
			name: "auto-merge not allowed on repo",
			responseBody: `{
				"data": null,
				"errors": [
					{
						"message": "Pull request is not in the correct state to enable auto-merge"
					}
				]
			}`,
			responseStatus: 200,
			expectError:    true,
		},
		{
			name:           "server error",
			responseBody:   `{"message": "Internal Server Error"}`,
			responseStatus: 500,
			expectError:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var receivedBody map[string]interface{}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// Verify request
				if r.Method != "POST" {
					t.Errorf("expected POST, got %s", r.Method)
				}
				if r.Header.Get("Authorization") != "Bearer test-token" {
					t.Errorf("expected Bearer test-token, got %s", r.Header.Get("Authorization"))
				}

				body, _ := io.ReadAll(r.Body)
				json.Unmarshal(body, &receivedBody)

				w.WriteHeader(tt.responseStatus)
				w.Write([]byte(tt.responseBody))
			}))
			defer server.Close()

			c := NewGithubClient(http.DefaultClient, "test-token")
			ctx := context.Background()

			err := c.EnableAutoMerge(ctx, server.URL, DependencyUpdateRequest{
				Owner:             "owner",
				Repo:              "repo",
				PullRequestNumber: 42,
				NodeID:            "PR_abc123",
				Title:             "Bump foo from 1.0 to 2.0",
				PackageName:       "foo",
			})

			if tt.expectError && err == nil {
				t.Error("expected error, got nil")
			}
			if !tt.expectError && err != nil {
				t.Errorf("expected no error, got %v", err)
			}

			// Verify the GraphQL query was sent correctly
			if receivedBody != nil {
				query, ok := receivedBody["query"].(string)
				if !ok || !strings.Contains(query, "enablePullRequestAutoMerge") {
					t.Error("request body missing enablePullRequestAutoMerge mutation")
				}
				variables, ok := receivedBody["variables"].(map[string]interface{})
				if !ok {
					t.Fatal("request body missing variables")
				}
				if variables["pullRequestId"] != "PR_abc123" {
					t.Errorf("expected pullRequestId PR_abc123, got %v", variables["pullRequestId"])
				}
				if variables["mergeMethod"] != "SQUASH" {
					t.Errorf("expected mergeMethod SQUASH, got %v", variables["mergeMethod"])
				}
			}
		})
	}
}
```

You'll also need to add these imports to the test file's import block: `"context"`, `"encoding/json"`, `"io"`, `"net/http"`, `"net/http/httptest"`, `"strings"`.

**Step 2: Run test to verify it fails**

Run: `go test ./internal/scm/ -run TestEnableAutoMerge -v`
Expected: FAIL — `EnableAutoMerge` method doesn't exist yet.

**Step 3: Commit**

```bash
git add internal/scm/github_test.go
git commit -m "Add failing test for EnableAutoMerge"
```

---

### Task 4: Implement EnableAutoMerge

**Files:**
- Modify: `internal/scm/github.go` (add imports + new method)

**Step 1: Add imports**

Add `"bytes"`, `"encoding/json"`, and `"io"` to the import block in `github.go`.

**Step 2: Add EnableAutoMerge method**

Add after `ApprovePullRequests` (after line 288):

```go
const graphQLURL = "https://api.github.com/graphql"

// EnableAutoMerge enables auto-merge on a pull request using GitHub's GraphQL API.
func (g *githubClient) EnableAutoMerge(ctx context.Context, graphqlEndpoint string, req DependencyUpdateRequest) error {
	query := `mutation($pullRequestId: ID!, $mergeMethod: PullRequestMergeMethod!) {
		enablePullRequestAutoMerge(input: {pullRequestId: $pullRequestId, mergeMethod: $mergeMethod}) {
			pullRequest {
				autoMergeRequest {
					enabledAt
				}
			}
		}
	}`

	payload := map[string]interface{}{
		"query": query,
		"variables": map[string]string{
			"pullRequestId": req.NodeID,
			"mergeMethod":   "SQUASH",
		},
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal GraphQL request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", graphqlEndpoint, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	httpReq.Header.Set("Authorization", "Bearer "+g.token)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		return fmt.Errorf("GraphQL request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("GraphQL request returned status %d: %s", resp.StatusCode, string(respBody))
	}

	// Check for GraphQL-level errors
	var result struct {
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return fmt.Errorf("failed to parse response: %w", err)
	}
	if len(result.Errors) > 0 {
		return fmt.Errorf("GraphQL error: %s", result.Errors[0].Message)
	}

	return nil
}
```

**Step 3: Run test to verify it passes**

Run: `go test ./internal/scm/ -run TestEnableAutoMerge -v`
Expected: PASS — all three sub-tests pass.

**Step 4: Run full test suite**

Run: `go test ./...`
Expected: All tests pass.

**Step 5: Commit**

```bash
git add internal/scm/github.go
git commit -m "Implement EnableAutoMerge via raw GraphQL"
```

---

### Task 5: Update ApprovePullRequests to remove @dependabot merge

**Files:**
- Modify: `internal/scm/github.go:265-288`

**Step 1: Change the review body**

In `ApprovePullRequests`, change line 266 from:

```go
approveMessage := `@dependabot merge`
```

to:

```go
approveMessage := `Approved by dependabot-bouncer`
```

**Step 2: Run full test suite**

Run: `go test ./...`
Expected: All tests pass.

**Step 3: Commit**

```bash
git add internal/scm/github.go
git commit -m "Replace @dependabot merge with neutral approval message"
```

---

### Task 6: Call EnableAutoMerge from the approve command

**Files:**
- Modify: `cmd/dependabot-bouncer/commands.go:271-275`

**Step 1: Add EnableAutoMerge call after ApprovePullRequests**

Replace the approve branch in `runDependencyUpdate` (lines 271-275):

```go
	if recreate {
		err = c.RecreatePullRequests(ctx, updates)
	} else {
		err = c.ApprovePullRequests(ctx, updates)
		if err != nil {
			return fmt.Errorf("failed to approve pull requests: %w", err)
		}

		// Enable auto-merge on each approved PR
		for _, u := range updates {
			if amErr := c.EnableAutoMerge(ctx, scm.GraphQLURL, u); amErr != nil {
				log.Printf("Warning: failed to enable auto-merge on PR #%d: %v\n", u.PullRequestNumber, amErr)
			} else {
				log.Printf("Enabled auto-merge on PR #%d: %s\n", u.PullRequestNumber, u.Title)
			}
		}
		return nil
	}
```

Note: `GraphQLURL` needs to be exported from the `scm` package. Change `const graphQLURL` to `const GraphQLURL` in `github.go`.

**Step 2: Build to verify compilation**

Run: `go build ./...`
Expected: Builds successfully.

**Step 3: Run full test suite**

Run: `go test ./...`
Expected: All tests pass.

**Step 4: Commit**

```bash
git add internal/scm/github.go cmd/dependabot-bouncer/commands.go
git commit -m "Wire up EnableAutoMerge in approve command flow"
```

---

### Task 7: Verify and clean up

**Step 1: Run full test suite one final time**

Run: `go test -v ./...`
Expected: All tests pass.

**Step 2: Build the binary**

Run: `go build -o /dev/null ./cmd/dependabot-bouncer/`
Expected: Builds cleanly with no warnings.

**Step 3: Review all changes since main**

Run: `git diff main...HEAD --stat` and `git log main..HEAD --oneline`
Verify: All changes align with the design doc.

---

## Post-Implementation Fixes

### Fix: CI state detection for GitHub Actions repos

**Problem:** `GetCombinedStatus` only returns legacy commit statuses. Repos using GitHub Actions (check runs) reported `"pending"` with zero statuses, causing `skipFailing` to filter out every PR.

**Fix:** Added `getCIState()` helper that checks commit statuses first, falls back to `Checks.ListCheckRunsForRef` if no statuses exist. Used in `GetDependencyUpdates`, `GetDependabotPRs`, and `GetDependabotPRsWithDenyList`.

### Fix: Handle PRs already in clean status

**Problem:** `enablePullRequestAutoMerge` returns a GraphQL error ("Pull request is in clean status") when all checks have already passed — auto-merge is for queuing, not for already-mergeable PRs.

**Fix:** Added `ErrPRClean` sentinel error. When detected, fall back to `MergePullRequest` (squash merge via REST API).

### Fix: Rebase out-of-date PRs before enabling auto-merge

**Problem:** PRs behind `main` (`mergeable_state: "behind"`) need a rebase before CI can validate against the latest base.

**Fix:** Added `GetPRMergeableState` and `RebasePullRequest` methods. The approve flow now checks each PR's state — if behind, it tells dependabot to rebase (`@dependabot rebase`) and enables auto-merge so the PR merges automatically once CI passes on the rebased branch.

### Codebase cleanup

- Deduplicated `GetDependencyUpdates` (collapsed two near-identical branches)
- Fixed raw pointer dereference `*p.Number` → `p.GetNumber()`
- Removed garbage comment, duplicate const
- Extracted `parseRepo` and `buildDenyLists` helpers
- `interface{}` → `any`
- Added `-race` flag to CI test step
