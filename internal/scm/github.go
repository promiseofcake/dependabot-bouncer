package scm

import (
	"context"
	"log"
	"net/http"

	"github.com/google/go-github/v56/github"
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

func (g *githubClient) GetDependencyUpdates(ctx context.Context, q DependencyUpdateQuery) ([]DependencyUpdateRequest, error) {

	var reqs []DependencyUpdateRequest

	excluded := make(map[int]bool)
	for _, p := range q.IgnoredPRs {
		excluded[p] = true
	}

	pulls, _, err := g.client.PullRequests.List(ctx, q.Owner, q.Repo, &github.PullRequestListOptions{})
	if err != nil {
		return nil, err
	}

	for _, p := range pulls {
		// exclude excluded PRs
		if _, ok := excluded[*p.Number]; ok {
			continue
		}

		if *p.User.ID == dependabotUserID {
			reqs = append(reqs, DependencyUpdateRequest{
				Owner:             q.Owner,
				Repo:              q.Repo,
				PullRequestNumber: *p.Number,
			})
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
