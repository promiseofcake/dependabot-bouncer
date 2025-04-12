package scm

import (
	"context"
	"fmt"
	"log"
	"net/http"

	"github.com/google/go-github/v71/github"
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
				status, _, sErr := g.client.Repositories.GetCombinedStatus(ctx, q.Owner, q.Repo, p.GetHead().GetSHA(), &github.ListOptions{})
				if sErr != nil {
					return nil, sErr
				}

				if status.GetState() == "success" {
					reqs = append(reqs, DependencyUpdateRequest{
						Owner:             q.Owner,
						Repo:              q.Repo,
						PullRequestNumber: p.GetNumber(),
					})
				}
			}
		} else {
			if p.GetUser().GetID() == dependabotUserID {
				reqs = append(reqs, DependencyUpdateRequest{
					Owner:             q.Owner,
					Repo:              q.Repo,
					PullRequestNumber: p.GetNumber(),
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
		log.Printf("%d: request: %s, result: %s\n", r.PullRequestNumber, request, review)
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
		log.Printf("%d: request: %s, result: %s\n", r.PullRequestNumber, request, review)
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
		log.Printf("%d: request: %s, result: %s\n", r.PullRequestNumber, request, review)
	}

	return nil
}
