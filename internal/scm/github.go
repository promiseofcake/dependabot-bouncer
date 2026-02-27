package scm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/google/go-github/v72/github"
)

const (
	dependabotUserID int64 = 49699333
)

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
		// Handle wildcard patterns
		if strings.Contains(denied, "*") {
			// Convert wildcard pattern to simple matching
			pattern := strings.ToLower(denied)
			pkg := strings.ToLower(packageName)

			// Simple wildcard matching
			if pattern == "*alpha*" && strings.Contains(pkg, "alpha") {
				return true
			}
			if pattern == "*beta*" && strings.Contains(pkg, "beta") {
				return true
			}
			if pattern == "*rc*" && strings.Contains(pkg, "rc") {
				return true
			}
			if pattern == "*/v0" && strings.HasSuffix(pkg, "/v0") {
				return true
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

func (g *githubClient) GetDependencyUpdates(ctx context.Context, q DependencyUpdateQuery, skipFailing bool) ([]DependencyUpdateRequest, error) {
	var reqs []DependencyUpdateRequest

	excluded := make(map[int]bool)
	for _, p := range q.IgnoredPRs {
		excluded[p] = true
	}

	// need to iterate throught the list
	pulls, resp, err := g.client.PullRequests.List(ctx, q.Owner, q.Repo, &github.PullRequestListOptions{
		Base: "main",
		ListOptions: github.ListOptions{
			Page:    0,
			PerPage: 100,
		},
	})
	if err != nil {
		return nil, err
	}

	fmt.Println(resp)

	for _, p := range pulls {
		// exclude excluded PRs
		if _, ok := excluded[*p.Number]; ok {
			continue
		}

		if skipFailing {
			if p.GetUser().GetID() == dependabotUserID {
				title := p.GetTitle()
				packageName, orgName := extractPackageInfo(title)

				// Check if package or org is denied
				if isDenied(packageName, orgName, q.DeniedPackages, q.DeniedOrgs) {
					log.Printf("Skipping denied package: %s (org: %s) - PR #%d: %s\n", packageName, orgName, p.GetNumber(), title)
					continue
				}

				status, _, sErr := g.client.Repositories.GetCombinedStatus(ctx, q.Owner, q.Repo, p.GetHead().GetSHA(), &github.ListOptions{})
				if sErr != nil {
					return nil, sErr
				}

				if status.GetState() == "success" {
					reqs = append(reqs, DependencyUpdateRequest{
						Owner:             q.Owner,
						Repo:              q.Repo,
						PullRequestNumber: p.GetNumber(),
						NodeID:            p.GetNodeID(),
						Title:             title,
						PackageName:       packageName,
					})
				}
			}
		} else {
			if p.GetUser().GetID() == dependabotUserID {
				title := p.GetTitle()
				packageName, orgName := extractPackageInfo(title)

				// Check if package or org is denied
				if isDenied(packageName, orgName, q.DeniedPackages, q.DeniedOrgs) {
					log.Printf("Skipping denied package: %s (org: %s) - PR #%d: %s\n", packageName, orgName, p.GetNumber(), title)
					continue
				}

				reqs = append(reqs, DependencyUpdateRequest{
					Owner:             q.Owner,
					Repo:              q.Repo,
					PullRequestNumber: p.GetNumber(),
					NodeID:            p.GetNodeID(),
					Title:             title,
					PackageName:       packageName,
				})
			}
		}
	}

	return reqs, nil
}

func (g *githubClient) ApprovePullRequests(ctx context.Context, reqs []DependencyUpdateRequest) error {
	approveMessage := `@dependabot merge`
	approveEvent := `APPROVE`

	for _, r := range reqs {
		request := &github.PullRequestReviewRequest{
			Body:  &approveMessage,
			Event: &approveEvent,
		}

		review, _, err := g.client.PullRequests.CreateReview(ctx, r.Owner, r.Repo, r.PullRequestNumber, &github.PullRequestReviewRequest{
			Body:  &approveMessage,
			Event: &approveEvent,
		})
		if err != nil {
			panic(err)
		}
		log.Printf("Approved PR #%d: %s (package: %s)\n", r.PullRequestNumber, r.Title, r.PackageName)
		_ = review
		_ = request
	}

	return nil
}

const GraphQLURL = "https://api.github.com/graphql"

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

func (g *githubClient) RebasePullRequests(ctx context.Context, reqs []DependencyUpdateRequest) error {
	recreateMessage := `@dependabot rebase`
	recreateEvent := `COMMENT`

	for _, r := range reqs {
		request := &github.PullRequestReviewRequest{
			Body:  &recreateMessage,
			Event: &recreateEvent,
		}

		review, _, err := g.client.PullRequests.CreateReview(ctx, r.Owner, r.Repo, r.PullRequestNumber, request)
		if err != nil {
			panic(err)
		}
		log.Printf("Rebased PR #%d: %s (package: %s)\n", r.PullRequestNumber, r.Title, r.PackageName)
		_ = review
		_ = request
	}

	return nil
}

func (g *githubClient) RecreatePullRequests(ctx context.Context, reqs []DependencyUpdateRequest) error {
	recreateMessage := `@dependabot recreate`
	recreateEvent := `COMMENT`

	for _, r := range reqs {
		request := &github.PullRequestReviewRequest{
			Body:  &recreateMessage,
			Event: &recreateEvent,
		}

		review, _, err := g.client.PullRequests.CreateReview(ctx, r.Owner, r.Repo, r.PullRequestNumber, request)
		if err != nil {
			panic(err)
		}
		log.Printf("Recreated PR #%d: %s (package: %s)\n", r.PullRequestNumber, r.Title, r.PackageName)
		_ = review
		_ = request
	}

	return nil
}

// GetDependabotPRs returns all open Dependabot PRs for a repository
func (g *githubClient) GetDependabotPRs(ctx context.Context, owner, repo string) ([]PRInfo, error) {
	var prs []PRInfo

	// List all open PRs
	pulls, _, err := g.client.PullRequests.List(ctx, owner, repo, &github.PullRequestListOptions{
		State: "open",
		ListOptions: github.ListOptions{
			Page:    0,
			PerPage: 100,
		},
	})
	if err != nil {
		return nil, err
	}

	for _, p := range pulls {
		// Only include Dependabot PRs
		if p.GetUser().GetID() == dependabotUserID {
			pr := PRInfo{
				Number: p.GetNumber(),
				Title:  p.GetTitle(),
				URL:    p.GetHTMLURL(),
			}

			// Get CI status
			status, _, err := g.client.Repositories.GetCombinedStatus(ctx, owner, repo, p.GetHead().GetSHA(), &github.ListOptions{})
			if err == nil {
				pr.Status = status.GetState()
			}

			prs = append(prs, pr)
		}
	}

	return prs, nil
}

// GetDependabotPRsWithDenyList returns all open Dependabot PRs with skip status based on deny lists
func (g *githubClient) GetDependabotPRsWithDenyList(ctx context.Context, q DependencyUpdateQuery) ([]PRInfo, error) {
	var prs []PRInfo

	// List all open PRs
	pulls, _, err := g.client.PullRequests.List(ctx, q.Owner, q.Repo, &github.PullRequestListOptions{
		State: "open",
		ListOptions: github.ListOptions{
			Page:    0,
			PerPage: 100,
		},
	})
	if err != nil {
		return nil, err
	}

	for _, p := range pulls {
		// Only include Dependabot PRs
		if p.GetUser().GetID() == dependabotUserID {
			title := p.GetTitle()
			packageName, orgName := extractPackageInfo(title)

			pr := PRInfo{
				Number: p.GetNumber(),
				Title:  title,
				URL:    p.GetHTMLURL(),
			}

			// Check if package or org is denied
			if isDenied(packageName, orgName, q.DeniedPackages, q.DeniedOrgs) {
				pr.Skipped = true
				if orgName != "" {
					for _, denied := range q.DeniedOrgs {
						if strings.EqualFold(orgName, denied) {
							pr.SkipReason = fmt.Sprintf("org '%s' is denied", orgName)
							break
						}
					}
				}
				if pr.SkipReason == "" && packageName != "" {
					for _, denied := range q.DeniedPackages {
						if strings.EqualFold(packageName, denied) || strings.Contains(strings.ToLower(packageName), strings.ToLower(denied)) {
							pr.SkipReason = fmt.Sprintf("package '%s' is denied", denied)
							break
						}
					}
				}
			}

			// Get CI status
			status, _, err := g.client.Repositories.GetCombinedStatus(ctx, q.Owner, q.Repo, p.GetHead().GetSHA(), &github.ListOptions{})
			if err == nil {
				pr.Status = status.GetState()
			}

			prs = append(prs, pr)
		}
	}

	return prs, nil
}

// GetOldLabeledPRs returns PRs with a specific label that are older than the specified duration
func (g *githubClient) GetOldLabeledPRs(ctx context.Context, owner, repo, label string, maxAge time.Duration) ([]ClosePRInfo, error) {
	var prs []ClosePRInfo

	cutoff := time.Now().Add(-maxAge)

	opts := &github.PullRequestListOptions{
		State: "open",
		ListOptions: github.ListOptions{
			Page:    1,
			PerPage: 100,
		},
	}

	for {
		pulls, resp, err := g.client.PullRequests.List(ctx, owner, repo, opts)
		if err != nil {
			return nil, err
		}

		for _, p := range pulls {
			// Check if PR has the required label
			hasLabel := false
			for _, l := range p.Labels {
				if strings.EqualFold(l.GetName(), label) {
					hasLabel = true
					break
				}
			}

			if !hasLabel {
				continue
			}

			// Check if PR is older than the cutoff
			createdAt := p.GetCreatedAt().Time
			if createdAt.After(cutoff) {
				continue
			}

			age := time.Since(createdAt)
			ageStr := formatDuration(age)

			prs = append(prs, ClosePRInfo{
				Number:    p.GetNumber(),
				Title:     p.GetTitle(),
				URL:       p.GetHTMLURL(),
				CreatedAt: createdAt.Format("2006-01-02"),
				Age:       ageStr,
			})
		}

		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}

	return prs, nil
}

// ClosePullRequests closes the given pull requests with a comment
func (g *githubClient) ClosePullRequests(ctx context.Context, owner, repo string, prs []ClosePRInfo) error {
	state := "closed"
	comment := "Closed due to inactivity."

	for _, pr := range prs {
		// Add comment explaining why the PR is being closed
		_, _, err := g.client.Issues.CreateComment(ctx, owner, repo, pr.Number, &github.IssueComment{
			Body: &comment,
		})
		if err != nil {
			return fmt.Errorf("failed to comment on PR #%d: %w", pr.Number, err)
		}

		// Close the PR
		_, _, err = g.client.PullRequests.Edit(ctx, owner, repo, pr.Number, &github.PullRequest{
			State: &state,
		})
		if err != nil {
			return fmt.Errorf("failed to close PR #%d: %w", pr.Number, err)
		}
		log.Printf("Closed PR #%d: %s (age: %s)\n", pr.Number, pr.Title, pr.Age)
	}

	return nil
}

// formatDuration formats a duration in a human-readable way
func formatDuration(d time.Duration) string {
	days := int(d.Hours() / 24)
	if days >= 365 {
		years := days / 365
		if years == 1 {
			return "1 year"
		}
		return fmt.Sprintf("%d years", years)
	}
	if days >= 30 {
		months := days / 30
		if months == 1 {
			return "1 month"
		}
		return fmt.Sprintf("%d months", months)
	}
	if days >= 7 {
		weeks := days / 7
		if weeks == 1 {
			return "1 week"
		}
		return fmt.Sprintf("%d weeks", weeks)
	}
	if days >= 1 {
		if days == 1 {
			return "1 day"
		}
		return fmt.Sprintf("%d days", days)
	}
	hours := int(d.Hours())
	if hours >= 1 {
		if hours == 1 {
			return "1 hour"
		}
		return fmt.Sprintf("%d hours", hours)
	}
	return "less than 1 hour"
}
