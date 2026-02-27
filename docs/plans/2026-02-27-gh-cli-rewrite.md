# gh CLI Rewrite Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Replace all go-github API calls with `gh` CLI commands, dropping the go-github dependency entirely.

**Architecture:** `internal/scm/github.go` is rewritten as a thin wrapper that shells out to `gh`. The deny list logic (`extractPackageInfo`, `isDenied`) stays as-is. `commands.go` is simplified: no token management, no HTTP client. The `close` command is removed.

**Tech Stack:** Go 1.25, `gh` CLI, cobra, viper

**Design doc:** `docs/plans/2026-02-27-gh-cli-rewrite-design.md`

---

### Task 1: Add gh JSON types and ListDependabotPRs function

**Files:**
- Modify: `internal/scm/github.go` (gut and rewrite)
- Modify: `internal/scm/deps.go` (simplify types)

**Step 1: Rewrite `deps.go` — remove unused types, simplify**

Replace the entire file with:

```go
package scm

// DependencyUpdateQuery holds parameters for listing and filtering Dependabot PRs.
type DependencyUpdateQuery struct {
	Owner          string
	Repo           string
	IgnoredPRs     []int
	DeniedPackages []string
	DeniedOrgs     []string
}

// PRInfo contains information about a Dependabot pull request.
type PRInfo struct {
	Number           int
	Title            string
	URL              string
	MergeStateStatus string // BEHIND, BLOCKED, CLEAN, DIRTY, DRAFT, HAS_HOOKS, UNKNOWN, UNSTABLE
	CIStatus         string // success, failure, pending
	PackageName      string
	Skipped          bool
	SkipReason       string
}
```

Drop `DependencyUpdateRequest` (no longer needed — `PRInfo` carries everything). Drop `ClosePRInfo` (close command removed). Drop `NodeID` (no more GraphQL).

**Step 2: Rewrite `github.go` — delete everything above `extractPackageInfo`, replace with gh wrapper**

Delete the entire file contents from the top through line 194 (everything before `extractPackageInfo`). Replace with this new header and `ListDependabotPRs` function:

```go
package scm

import (
	"encoding/json"
	"fmt"
	"log"
	"os/exec"
	"regexp"
	"strings"
)

// ghPR is the JSON shape returned by `gh pr list --json ...`.
type ghPR struct {
	Number           int    `json:"number"`
	Title            string `json:"title"`
	URL              string `json:"url"`
	MergeStateStatus string `json:"mergeStateStatus"`
	Author           struct {
		Login string `json:"login"`
	} `json:"author"`
	StatusCheckRollup []struct {
		Status     string `json:"status"`
		Conclusion string `json:"conclusion"`
	} `json:"statusCheckRollup"`
}

// ListDependabotPRs lists open Dependabot PRs for a repo using the gh CLI.
// It filters by author, deny lists, ignored PRs, and optionally by CI status.
func ListDependabotPRs(q DependencyUpdateQuery, skipFailing bool) ([]PRInfo, error) {
	out, err := exec.Command("gh", "pr", "list",
		"--repo", q.Owner+"/"+q.Repo,
		"--base", "main",
		"--json", "number,title,url,author,mergeStateStatus,statusCheckRollup",
		"--limit", "100",
	).Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return nil, fmt.Errorf("gh pr list failed: %s", string(exitErr.Stderr))
		}
		return nil, fmt.Errorf("gh pr list failed: %w", err)
	}

	var pulls []ghPR
	if err := json.Unmarshal(out, &pulls); err != nil {
		return nil, fmt.Errorf("failed to parse gh output: %w", err)
	}

	excluded := make(map[int]bool)
	for _, p := range q.IgnoredPRs {
		excluded[p] = true
	}

	var prs []PRInfo
	for _, p := range pulls {
		if p.Author.Login != "app/dependabot" {
			continue
		}
		if excluded[p.Number] {
			continue
		}

		title := p.Title
		packageName, orgName := extractPackageInfo(title)

		if isDenied(packageName, orgName, q.DeniedPackages, q.DeniedOrgs) {
			log.Printf("Skipping denied package: %s (org: %s) - PR #%d: %s\n", packageName, orgName, p.Number, title)
			continue
		}

		ci := ciStatus(p.StatusCheckRollup)
		if skipFailing && ci != "success" {
			continue
		}

		prs = append(prs, PRInfo{
			Number:           p.Number,
			Title:            title,
			URL:              p.URL,
			MergeStateStatus: p.MergeStateStatus,
			CIStatus:         ci,
			PackageName:      packageName,
		})
	}

	return prs, nil
}

// ciStatus determines overall CI status from the statusCheckRollup array.
// Returns "success" if all checks completed with success/skipped/neutral,
// "failure" if any check failed, "pending" if checks are still running or empty.
func ciStatus(checks []struct {
	Status     string `json:"status"`
	Conclusion string `json:"conclusion"`
}) string {
	if len(checks) == 0 {
		return "pending"
	}
	for _, c := range checks {
		if c.Status != "COMPLETED" {
			return "pending"
		}
		switch c.Conclusion {
		case "SUCCESS", "SKIPPED", "NEUTRAL":
			continue
		default:
			return "failure"
		}
	}
	return "success"
}
```

Keep `extractPackageInfo` and `isDenied` exactly as they are (lines 44–194 in the current file).

**Step 3: Delete everything after `isDenied` in `github.go`**

Delete from line 196 (the `getCIState` function) through the end of the file (line 679). This removes:
- `getCIState`, `GetDependencyUpdates`, `ApprovePullRequests`
- `ErrPRClean`, `EnableAutoMerge`, `GetPRMergeableState`
- `RebasePullRequest`, `MergePullRequest`
- `RebasePullRequests`, `RecreatePullRequests`
- `GetDependabotPRs`, `GetDependabotPRsWithDenyList`
- `GetOldLabeledPRs`, `ClosePullRequests`, `formatDuration`

**Step 4: Run tests**

Run: `go test ./internal/scm/ -run 'TestExtractPackageInfo|TestIsDenied|TestRealWorldDenials|TestIsDeniedCaseInsensitive|TestWildcardPatterns' -v`
Expected: All 5 existing test suites pass. `TestEnableAutoMerge` should now fail to compile (that's expected — we'll delete it next task).

**Step 5: Commit**

```bash
git add internal/scm/deps.go internal/scm/github.go
git commit -m "Replace go-github with gh CLI wrapper for listing PRs"
```

---

### Task 2: Add gh action functions (approve, merge, rebase, recreate, comment)

**Files:**
- Modify: `internal/scm/github.go` (add action functions after `ListDependabotPRs`)

**Step 1: Add the action functions**

Add these after the `ciStatus` function, before `extractPackageInfo`:

```go
// ApprovePR approves a pull request.
func ApprovePR(owner, repo string, number int) error {
	return ghCommand("approve PR", "gh", "pr", "review", "--approve",
		"--repo", owner+"/"+repo, fmt.Sprintf("%d", number))
}

// AutoMergePR enables auto-merge (squash) on a pull request.
// If the PR is already clean, gh merges it immediately.
func AutoMergePR(owner, repo string, number int) error {
	return ghCommand("auto-merge PR", "gh", "pr", "merge", "--auto", "--squash",
		"--repo", owner+"/"+repo, fmt.Sprintf("%d", number))
}

// RebasePR tells Dependabot to rebase a pull request.
func RebasePR(owner, repo string, number int) error {
	return ghCommand("rebase PR", "gh", "pr", "comment",
		"--repo", owner+"/"+repo, fmt.Sprintf("%d", number),
		"--body", "@dependabot rebase")
}

// RecreatePR tells Dependabot to recreate a pull request.
func RecreatePR(owner, repo string, number int) error {
	return ghCommand("recreate PR", "gh", "pr", "comment",
		"--repo", owner+"/"+repo, fmt.Sprintf("%d", number),
		"--body", "@dependabot recreate")
}

// ghCommand runs a gh CLI command and returns a descriptive error on failure.
func ghCommand(desc string, args ...string) error {
	cmd := exec.Command(args[0], args[1:]...)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to %s: %s", desc, strings.TrimSpace(string(out)))
	}
	return nil
}
```

**Step 2: Build to verify compilation**

Run: `go build ./...`
Expected: Build succeeds (commands.go will fail — that's Task 3).

Actually, `commands.go` still references the old API. We need to fix that before building. Skip building here — Task 3 handles it.

**Step 3: Commit**

```bash
git add internal/scm/github.go
git commit -m "Add gh CLI action functions for approve, merge, rebase, recreate"
```

---

### Task 3: Rewrite commands.go to use the new scm API

**Files:**
- Modify: `cmd/dependabot-bouncer/commands.go` (rewrite)
- Modify: `cmd/dependabot-bouncer/main.go` (remove close command, github-token flag)

**Step 1: Rewrite `commands.go`**

Replace the entire file with:

```go
package main

import (
	"fmt"
	"log"
	"strings"

	"github.com/promiseofcake/dependabot-bouncer/internal/scm"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// parseRepo splits an "owner/repo" string into its parts.
func parseRepo(arg string) (owner, repo string, err error) {
	parts := strings.Split(arg, "/")
	if len(parts) != 2 {
		return "", "", fmt.Errorf("invalid repository format: %s (expected owner/repo)", arg)
	}
	return parts[0], parts[1], nil
}

// buildDenyLists merges global and repo-specific deny lists from config.
func buildDenyLists(repoKey string) (deniedPackages, deniedOrgs []string) {
	deniedPackages = getStringSlice("global.denied_packages")
	deniedOrgs = getStringSlice("global.denied_orgs")

	deniedPackages = append(deniedPackages, getStringSlice("repositories."+repoKey+".denied_packages")...)
	deniedOrgs = append(deniedOrgs, getStringSlice("repositories."+repoKey+".denied_orgs")...)

	return removeDuplicates(deniedPackages), removeDuplicates(deniedOrgs)
}

var (
	approveCmd = &cobra.Command{
		Use:   "approve owner/repo",
		Short: "Approve dependency update pull requests",
		Long:  `Approve passing dependency update pull requests from Dependabot.`,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			owner, repo, err := parseRepo(args[0])
			if err != nil {
				return err
			}
			return runApprove(owner, repo)
		},
	}

	recreateCmd = &cobra.Command{
		Use:   "recreate owner/repo",
		Short: "Recreate dependency update pull requests",
		Long:  `Recreate all dependency update pull requests from Dependabot (including failing ones).`,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			owner, repo, err := parseRepo(args[0])
			if err != nil {
				return err
			}
			return runRecreate(owner, repo)
		},
	}

	checkCmd = &cobra.Command{
		Use:   "check [owner/repo...]",
		Short: "Check for open Dependabot PRs across repositories",
		Long: `Check for open Dependabot pull requests across multiple repositories.

If no repositories are specified as arguments, checks all repositories
configured in the 'repositories' section of your config file.

You can specify multiple repositories: check owner1/repo1 owner2/repo2`,
		RunE: runCheck,
	}
)

func runApprove(owner, repo string) error {
	prs, err := listFilteredPRs(owner, repo, true)
	if err != nil {
		return err
	}
	if len(prs) == 0 {
		fmt.Println("No dependency updates to process")
		return nil
	}

	fmt.Printf("Processing %d pull requests...\n", len(prs))

	for _, pr := range prs {
		if err := scm.ApprovePR(owner, repo, pr.Number); err != nil {
			log.Printf("Warning: failed to approve PR #%d: %v\n", pr.Number, err)
			continue
		}
		log.Printf("Approved PR #%d: %s (package: %s)\n", pr.Number, pr.Title, pr.PackageName)

		if pr.MergeStateStatus == "BEHIND" {
			if err := scm.RebasePR(owner, repo, pr.Number); err != nil {
				log.Printf("Warning: failed to rebase PR #%d: %v\n", pr.Number, err)
			} else {
				log.Printf("Requested rebase on PR #%d (behind main): %s\n", pr.Number, pr.Title)
			}
		}

		if err := scm.AutoMergePR(owner, repo, pr.Number); err != nil {
			log.Printf("Warning: failed to enable auto-merge on PR #%d: %v\n", pr.Number, err)
		} else {
			log.Printf("Enabled auto-merge on PR #%d: %s\n", pr.Number, pr.Title)
		}
	}

	return nil
}

func runRecreate(owner, repo string) error {
	prs, err := listFilteredPRs(owner, repo, false)
	if err != nil {
		return err
	}
	if len(prs) == 0 {
		fmt.Println("No dependency updates to process")
		return nil
	}

	fmt.Printf("Processing %d pull requests...\n", len(prs))

	for _, pr := range prs {
		if err := scm.RecreatePR(owner, repo, pr.Number); err != nil {
			log.Printf("Warning: failed to recreate PR #%d: %v\n", pr.Number, err)
		} else {
			log.Printf("Recreated PR #%d: %s (package: %s)\n", pr.Number, pr.Title, pr.PackageName)
		}
	}

	return nil
}

func runCheck(cmd *cobra.Command, args []string) error {
	var repos []string

	if len(args) > 0 {
		repos = args
	} else {
		repoMap := viper.GetStringMap("repositories")
		for repo := range repoMap {
			repos = append(repos, repo)
		}
		if len(repos) == 0 {
			repos = viper.GetStringSlice("check.repositories")
		}
	}

	if len(repos) == 0 {
		return fmt.Errorf("no repositories specified. Use command-line arguments or configure repositories in config file")
	}

	fmt.Println("Open Dependabot PRs:")
	fmt.Println("-------------------------")

	for _, repoPath := range repos {
		owner, repo, pErr := parseRepo(repoPath)
		if pErr != nil {
			fmt.Printf("  Invalid: %v\n\n", pErr)
			continue
		}

		fmt.Printf("%s/%s\n", owner, repo)

		repoKey := fmt.Sprintf("%s/%s", owner, repo)
		deniedPackages, deniedOrgs := buildDenyLists(repoKey)

		q := scm.DependencyUpdateQuery{
			Owner:          owner,
			Repo:           repo,
			DeniedPackages: deniedPackages,
			DeniedOrgs:     deniedOrgs,
		}

		prs, err := scm.ListDependabotPRs(q, false)
		if err != nil {
			fmt.Printf("   Error: %v\n\n", err)
			continue
		}

		if len(prs) == 0 {
			fmt.Println("   (no open Dependabot PRs)")
		} else {
			for _, pr := range prs {
				fmt.Printf("   #%d: %s\n", pr.Number, pr.Title)
				fmt.Printf("   %s\n", pr.URL)
				if pr.Skipped {
					fmt.Printf("   Status: SKIPPED (%s)\n", pr.SkipReason)
				} else if pr.CIStatus != "" {
					fmt.Printf("   Status: %s\n", pr.CIStatus)
				}
				fmt.Println()
			}
		}
		fmt.Println()
	}

	return nil
}

// listFilteredPRs builds a query from config and returns filtered Dependabot PRs.
func listFilteredPRs(owner, repo string, skipFailing bool) ([]scm.PRInfo, error) {
	repoKey := fmt.Sprintf("%s/%s", owner, repo)
	deniedPackages, deniedOrgs := buildDenyLists(repoKey)
	ignoredPRs := getIntSlice("repositories." + repoKey + ".ignored_prs")

	if cmdPackages := viper.GetStringSlice("deny-packages"); len(cmdPackages) > 0 {
		deniedPackages = removeDuplicates(append(deniedPackages, cmdPackages...))
	}
	if cmdOrgs := viper.GetStringSlice("deny-orgs"); len(cmdOrgs) > 0 {
		deniedOrgs = removeDuplicates(append(deniedOrgs, cmdOrgs...))
	}

	if len(deniedPackages) > 0 {
		log.Printf("Denying packages: %v\n", deniedPackages)
	}
	if len(deniedOrgs) > 0 {
		log.Printf("Denying organizations: %v\n", deniedOrgs)
	}
	if len(ignoredPRs) > 0 {
		log.Printf("Ignoring PRs: %v\n", ignoredPRs)
	}

	q := scm.DependencyUpdateQuery{
		Owner:          owner,
		Repo:           repo,
		IgnoredPRs:     ignoredPRs,
		DeniedPackages: deniedPackages,
		DeniedOrgs:     deniedOrgs,
	}

	return scm.ListDependabotPRs(q, skipFailing)
}

// Helper functions

func getStringSlice(key string) []string {
	if viper.IsSet(key) {
		return viper.GetStringSlice(key)
	}
	return []string{}
}

func getIntSlice(key string) []int {
	if viper.IsSet(key) {
		return viper.GetIntSlice(key)
	}
	return []int{}
}

func removeDuplicates(slice []string) []string {
	seen := make(map[string]bool)
	var result []string

	for _, item := range slice {
		normalized := strings.ToLower(strings.TrimSpace(item))
		if normalized != "" && !seen[normalized] {
			seen[normalized] = true
			result = append(result, item)
		}
	}

	return result
}
```

**Step 2: Update `main.go` — remove close command and github-token flag**

Remove `closeCmd` from `rootCmd.AddCommand` and remove the `github-token` flag and its viper binding. The updated `init` function:

```go
func init() {
	cobra.OnInitialize(initConfig)

	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is $HOME/.dependabot-bouncer/config.yaml)")
	rootCmd.PersistentFlags().StringSlice("deny-packages", []string{}, "Packages to deny")
	rootCmd.PersistentFlags().StringSlice("deny-orgs", []string{}, "Organizations to deny")

	viper.BindPFlag("deny-packages", rootCmd.PersistentFlags().Lookup("deny-packages"))
	viper.BindPFlag("deny-orgs", rootCmd.PersistentFlags().Lookup("deny-orgs"))

	rootCmd.AddCommand(approveCmd, recreateCmd, checkCmd)
}
```

Also remove from `initConfig`:
```go
// Also check for USER_GITHUB_TOKEN specifically
viper.BindEnv("github-token", "USER_GITHUB_TOKEN")
```

**Step 3: Build**

Run: `go build ./...`
Expected: Builds successfully.

**Step 4: Commit**

```bash
git add cmd/dependabot-bouncer/commands.go cmd/dependabot-bouncer/main.go
git commit -m "Rewrite commands to use gh CLI, remove close command and token management"
```

---

### Task 4: Update tests — delete EnableAutoMerge test, add ListDependabotPRs JSON parsing test

**Files:**
- Modify: `internal/scm/github_test.go`

**Step 1: Delete `TestEnableAutoMerge`**

Delete the entire `TestEnableAutoMerge` function (lines 523–628) and remove unused imports: `"context"`, `"encoding/json"`, `"io"`, `"net/http"`, `"net/http/httptest"`.

The remaining imports should be just `"strings"` and `"testing"`.

**Step 2: Add test for `ciStatus`**

Add a new test:

```go
func TestCIStatus(t *testing.T) {
	type check struct {
		Status     string `json:"status"`
		Conclusion string `json:"conclusion"`
	}

	tests := []struct {
		name   string
		checks []check
		want   string
	}{
		{
			name:   "no checks",
			checks: nil,
			want:   "pending",
		},
		{
			name: "all success",
			checks: []check{
				{Status: "COMPLETED", Conclusion: "SUCCESS"},
				{Status: "COMPLETED", Conclusion: "SUCCESS"},
			},
			want: "success",
		},
		{
			name: "one failure",
			checks: []check{
				{Status: "COMPLETED", Conclusion: "SUCCESS"},
				{Status: "COMPLETED", Conclusion: "FAILURE"},
			},
			want: "failure",
		},
		{
			name: "still running",
			checks: []check{
				{Status: "COMPLETED", Conclusion: "SUCCESS"},
				{Status: "IN_PROGRESS", Conclusion: ""},
			},
			want: "pending",
		},
		{
			name: "skipped counts as success",
			checks: []check{
				{Status: "COMPLETED", Conclusion: "SUCCESS"},
				{Status: "COMPLETED", Conclusion: "SKIPPED"},
			},
			want: "success",
		},
		{
			name: "neutral counts as success",
			checks: []check{
				{Status: "COMPLETED", Conclusion: "NEUTRAL"},
			},
			want: "success",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Convert to the anonymous struct type that ciStatus expects
			var checks []struct {
				Status     string `json:"status"`
				Conclusion string `json:"conclusion"`
			}
			for _, c := range tt.checks {
				checks = append(checks, struct {
					Status     string `json:"status"`
					Conclusion string `json:"conclusion"`
				}{Status: c.Status, Conclusion: c.Conclusion})
			}
			got := ciStatus(checks)
			if got != tt.want {
				t.Errorf("ciStatus() = %q, want %q", got, tt.want)
			}
		})
	}
}
```

**Step 3: Run all tests**

Run: `go test -race -v ./...`
Expected: All tests pass.

**Step 4: Commit**

```bash
git add internal/scm/github_test.go
git commit -m "Replace EnableAutoMerge test with ciStatus test"
```

---

### Task 5: Remove go-github dependency and tidy

**Files:**
- Modify: `go.mod`
- Modify: `go.sum`

**Step 1: Remove go-github from go.mod**

Run: `go mod tidy`
Expected: `go-github` and `go-querystring` are removed from `go.mod` and `go.sum`.

**Step 2: Verify go-github is gone**

Run: `grep go-github go.mod`
Expected: No output (dependency is gone).

**Step 3: Build and test**

Run: `go build ./... && go test -race -v ./...`
Expected: All pass.

**Step 4: Commit**

```bash
git add go.mod go.sum
git commit -m "Remove go-github dependency"
```

---

### Task 6: Verify everything end-to-end

**Step 1: Run full test suite with race detector**

Run: `go test -race -v ./...`
Expected: All tests pass.

**Step 2: Build the binary**

Run: `go build -o /dev/null ./cmd/dependabot-bouncer/`
Expected: Clean build.

**Step 3: Run go vet**

Run: `go vet ./...`
Expected: Clean.

**Step 4: Verify binary works (dry run)**

Run: `go run ./cmd/dependabot-bouncer/ check promiseofcake/circleci-trigger-action`
Expected: Lists open Dependabot PRs with their CI status and merge state.

**Step 5: Check line count**

Run: `wc -l internal/scm/github.go cmd/dependabot-bouncer/commands.go internal/scm/deps.go`
Expected: Significantly fewer lines than the 1140 total before.

**Step 6: Commit (if any fixes needed)**

If anything needed fixing, commit with descriptive message.
