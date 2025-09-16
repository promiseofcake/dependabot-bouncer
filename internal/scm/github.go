package scm

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"regexp"
	"strings"

	"github.com/google/go-github/v72/github"
)

const (
	dependabotUserID int64 = 49699333
)

type githubClient struct {
	client *github.Client
}

func NewGithubClient(client *http.Client, token string) *githubClient {
	return &githubClient{
		client: github.NewClient(client).WithAuthToken(token),
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
		// "Bump package from x to y" or "Bump package to y"
		{regexp.MustCompile(`(?i)^bump\s+([^\s]+)\s+(?:from|to)`), 1},
		// "Update package from x to y" or "Update package to y"
		{regexp.MustCompile(`(?i)^update\s+([^\s]+)\s+(?:from|to)`), 1},
		// "chore(deps): bump package from x to y"
		{regexp.MustCompile(`(?i)^chore.*bump\s+([^\s]+)\s+(?:from|to)`), 1},
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
			// Handle GitHub-style packages like github.com/datadog/datadog-go
			parts := strings.Split(packageName, "/")
			for i, part := range parts {
				// Look for organization name (usually after domain)
				if i > 0 && !strings.Contains(part, ".") {
					orgName = part
					break
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
		if strings.EqualFold(packageName, denied) {
			return true
		}
		// Also check if the denied string is contained in the package name
		if strings.Contains(strings.ToLower(packageName), strings.ToLower(denied)) {
			return true
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
