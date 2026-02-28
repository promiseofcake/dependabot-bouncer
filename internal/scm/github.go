package scm

import (
	"encoding/json"
	"fmt"
	"log"
	"os/exec"
	"regexp"
	"strings"
)

// statusCheck represents a single entry in statusCheckRollup.
// gh returns two types: CheckRun (has status/conclusion) and StatusContext (has state).
type statusCheck struct {
	TypeName   string `json:"__typename"`
	Status     string `json:"status"`     // CheckRun: COMPLETED, IN_PROGRESS, etc.
	Conclusion string `json:"conclusion"` // CheckRun: SUCCESS, FAILURE, etc.
	State      string `json:"state"`      // StatusContext: SUCCESS, FAILURE, PENDING, ERROR, EXPECTED
}

// ghPR represents a pull request as returned by `gh pr list --json`.
type ghPR struct {
	Number           int    `json:"number"`
	Title            string `json:"title"`
	URL              string `json:"url"`
	MergeStateStatus string `json:"mergeStateStatus"`
	ReviewDecision   string `json:"reviewDecision"`
	Author           struct {
		Login string `json:"login"`
	} `json:"author"`
	StatusCheckRollup []statusCheck `json:"statusCheckRollup"`
}

// ListDependabotPRs lists open Dependabot PRs for the given repository,
// applying the filters described in the query. When skipFailing is true,
// only PRs whose CI status is "success" are returned.
func ListDependabotPRs(q DependencyUpdateQuery, skipFailing bool) ([]PRInfo, error) {
	cmd := exec.Command("gh", "pr", "list",
		"--repo", q.Owner+"/"+q.Repo,
		"--base", "main",
		"--json", "number,title,url,author,mergeStateStatus,reviewDecision,statusCheckRollup",
		"--limit", "100",
	)

	out, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return nil, fmt.Errorf("gh pr list failed: %s", strings.TrimSpace(string(exitErr.Stderr)))
		}
		return nil, fmt.Errorf("gh pr list failed: %w", err)
	}

	var ghPRs []ghPR
	if err := json.Unmarshal(out, &ghPRs); err != nil {
		return nil, fmt.Errorf("failed to parse gh output: %w", err)
	}

	excluded := make(map[int]bool, len(q.IgnoredPRs))
	for _, n := range q.IgnoredPRs {
		excluded[n] = true
	}

	var prs []PRInfo
	for _, p := range ghPRs {
		if excluded[p.Number] {
			continue
		}

		if p.Author.Login != "app/dependabot" {
			continue
		}

		packageName, orgName := extractPackageInfo(p.Title)

		if isDenied(packageName, orgName, q.DeniedPackages, q.DeniedOrgs) {
			log.Printf("Skipping denied package: %s (org: %s) - PR #%d: %s\n", packageName, orgName, p.Number, p.Title)
			continue
		}

		status := ciStatus(p.StatusCheckRollup)

		if skipFailing && status != "success" {
			continue
		}

		prs = append(prs, PRInfo{
			Number:           p.Number,
			Title:            p.Title,
			URL:              p.URL,
			MergeStateStatus: p.MergeStateStatus,
			ReviewDecision:   p.ReviewDecision,
			CIStatus:         status,
			PackageName:      packageName,
		})
	}

	return prs, nil
}

// ciStatus determines the overall CI status from a statusCheckRollup.
//
// The rollup contains two types: CheckRun (status/conclusion) and
// StatusContext (state). Returns "pending" if there are no checks or any
// check is still running, "failure" if any check failed, "success" otherwise.
func ciStatus(checks []statusCheck) string {
	if len(checks) == 0 {
		return "pending"
	}
	for _, c := range checks {
		if c.TypeName == "StatusContext" {
			switch c.State {
			case "SUCCESS":
				// ok
			case "PENDING", "EXPECTED":
				return "pending"
			default:
				return "failure"
			}
		} else {
			// CheckRun
			if c.Status != "COMPLETED" {
				return "pending"
			}
			switch c.Conclusion {
			case "SUCCESS", "SKIPPED", "NEUTRAL":
				// ok
			default:
				return "failure"
			}
		}
	}
	return "success"
}

// ApprovePR approves a pull request.
func ApprovePR(owner, repo string, number int) error {
	return ghCommand("approve PR", "gh", "pr", "review", "--approve",
		"--repo", owner+"/"+repo, fmt.Sprintf("%d", number))
}

// AutoMergePR enables auto-merge (squash) on a pull request.
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

// extractPackageInfo extracts package name and organization from a Dependabot PR title
// Examples:
// "Bump github.com/datadog/datadog-go from 1.0.0 to 2.0.0" -> "github.com/datadog/datadog-go", "datadog"
// "Bump @datadog/browser-rum from 4.0.0 to 5.0.0" -> "@datadog/browser-rum", "datadog"
// "Update rails to 7.0.0" -> "rails", ""
func extractPackageInfo(title string) (packageName string, orgName string) {
	// Common patterns for Dependabot PR titles
	patterns := []struct {
		regex    *regexp.Regexp
		pkgIndex int
	}{
		// "⬆️ (deps): Bump package from x to y" or "⬆️ (deps): bump package from x to y"
		{regexp.MustCompile(`(?i)⬆️\s+\(deps\):\s+[Bb]ump\s+([^\s]+)\s+(?:from|to)`), 1},
		// "⬆️ (deps): Bump the aws-sdk-go-v2 group with N updates"
		{regexp.MustCompile(`(?i)⬆️\s+\(deps\):\s+[Bb]ump\s+the\s+([^\s]+)\s+group`), 1},
		// "Bump package from x to y" or "Bump package to y"
		{regexp.MustCompile(`(?i)^[Bb]ump\s+([^\s]+)\s+(?:from|to)`), 1},
		// "Update package from x to y" or "Update package to y"
		{regexp.MustCompile(`(?i)^[Uu]pdate\s+([^\s]+)\s+(?:from|to)`), 1},
		// "chore(deps): bump package from x to y"
		{regexp.MustCompile(`(?i)^chore.*[Bb]ump\s+([^\s]+)\s+(?:from|to)`), 1},
	}

	for _, p := range patterns {
		if matches := p.regex.FindStringSubmatch(title); len(matches) > p.pkgIndex {
			packageName = matches[p.pkgIndex]
			break
		}
	}

	if packageName == "" {
		// Fallback: try to extract any package-like string
		if parts := strings.Fields(title); len(parts) > 1 {
			for _, part := range parts[1:] {
				if strings.Contains(part, "/") || strings.Contains(part, "@") {
					packageName = part
					break
				}
			}
		}
	}

	// Extract organization from package name
	if packageName != "" {
		// Handle scoped npm packages like @datadog/browser-rum
		if strings.HasPrefix(packageName, "@") && strings.Contains(packageName, "/") {
			parts := strings.Split(packageName, "/")
			orgName = strings.TrimPrefix(parts[0], "@")
		} else if strings.Contains(packageName, "/") {
			// Special case for golang.org/x and google.golang.org packages - they don't have an org
			if strings.HasPrefix(packageName, "golang.org/x/") || strings.HasPrefix(packageName, "google.golang.org/") {
				orgName = ""
			} else if strings.HasPrefix(packageName, "gopkg.in/") {
				// gopkg.in packages can have orgs like gopkg.in/DataDog/dd-trace-go.v1
				// Extract the org from the second part if it exists
				parts := strings.Split(packageName, "/")
				if len(parts) > 2 {
					// gopkg.in/DataDog/dd-trace-go.v1 -> DataDog
					orgName = strings.ToLower(parts[1])
				} else {
					orgName = ""
				}
			} else {
				// Handle GitHub-style packages like github.com/datadog/datadog-go
				parts := strings.Split(packageName, "/")
				// For github.com/owner/repo or github.com/owner/repo/v2
				// We want the owner (second part)
				if len(parts) >= 3 && strings.HasPrefix(packageName, "github.com/") {
					orgName = parts[1]
				} else {
					// Fallback for other patterns
					for i, part := range parts {
						// Skip domain parts and version indicators
						if i > 0 && !strings.Contains(part, ".") && !strings.HasPrefix(part, "v") {
							orgName = part
							break
						}
					}
				}
			}
		}
	}

	return packageName, orgName
}

// isDenied checks if a package or organization is in the deny list
func isDenied(packageName, orgName string, deniedPackages, deniedOrgs []string) bool {
	// Check if package is denied
	for _, denied := range deniedPackages {
		// Handle wildcard patterns (leading and/or trailing * only)
		if strings.Contains(denied, "*") {
			pattern := strings.ToLower(denied)
			pkg := strings.ToLower(packageName)

			hasPrefix := strings.HasPrefix(pattern, "*")
			hasSuffix := strings.HasSuffix(pattern, "*")
			inner := strings.Trim(pattern, "*")

			switch {
			case hasPrefix && hasSuffix:
				// *foo* → contains
				if strings.Contains(pkg, inner) {
					return true
				}
			case hasPrefix:
				// *foo → suffix match
				if strings.HasSuffix(pkg, inner) {
					return true
				}
			case hasSuffix:
				// foo* → prefix match
				if strings.HasPrefix(pkg, inner) {
					return true
				}
			}
			continue
		}

		// Exact match (case insensitive)
		if strings.EqualFold(packageName, denied) {
			return true
		}

		// Check if it's a partial match (for versioned denials like github.com/gin-gonic/gin@v1)
		// But don't match if the denied package is a substring of a different package
		// e.g., don't match aws-sdk-go-v2 when aws-sdk-go is denied
		if strings.Contains(denied, "@") {
			// Version-specific denial
			if strings.Contains(strings.ToLower(packageName), strings.ToLower(denied)) {
				return true
			}
		} else {
			// For non-versioned denials, check for exact package name match
			// This prevents aws-sdk-go from matching aws-sdk-go-v2
			deniedLower := strings.ToLower(denied)
			pkgLower := strings.ToLower(packageName)

			// Check if they're the same package (not just a substring)
			if pkgLower == deniedLower {
				return true
			}

			// Also check with common version suffixes removed for comparison
			// This allows "github.com/gin-gonic/gin@v1.7.0" to match "github.com/gin-gonic/gin@v1"
			if idx := strings.Index(pkgLower, "@"); idx > 0 {
				pkgBase := pkgLower[:idx]
				if pkgBase == deniedLower {
					return true
				}
			}
		}
	}

	// Check if organization is denied
	for _, denied := range deniedOrgs {
		if strings.EqualFold(orgName, denied) {
			return true
		}
	}

	return false
}
