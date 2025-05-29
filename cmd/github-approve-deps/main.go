package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"

	"github.com/promiseofcake/github-deps/internal/scm"
)

func main() {
	ctx := context.Background()

	owner := flag.String("owner", "", "GitHub organization or user (required)")
	repo := flag.String("repo", "", "GitHub repository name (required)")
	recreate := flag.Bool("recreate", false, "Whether to recreate PRs instead of approving")
	flag.Parse()

	if *owner == "" || *repo == "" {
		fmt.Fprintln(os.Stderr, "error: --owner and --repo are required")
		flag.Usage()
		os.Exit(1)
	}

	token := os.Getenv("USER_GITHUB_TOKEN")
	if token == "" {
		fmt.Fprintln(os.Stderr, "error: USER_GITHUB_TOKEN must be set in environment")
		os.Exit(1)
	}

	c := scm.NewGithubClient(http.DefaultClient, token)
	u := scm.DependencyUpdateQuery{
		Owner: *owner,
		Repo:  *repo,
	}

	var fn func(context.Context, []scm.DependencyUpdateRequest) error
	var skipFailing bool
	if *recreate {
		fn = c.RecreatePullRequests
		skipFailing = false // Never skip failing for recreate
	} else {
		fn = c.ApprovePullRequests
		skipFailing = true // Always skip failing for approve
	}

	updates, err := c.GetDependencyUpdates(ctx, u, skipFailing)
	if err != nil {
		panic(err)
	}

	err = fn(ctx, updates)
	if err != nil {
		panic(err)
	}
}
