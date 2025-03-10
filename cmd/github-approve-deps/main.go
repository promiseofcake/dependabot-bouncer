package main

import (
	"context"
	"net/http"
	"os"

	"github.com/promiseofcake/github-deps/internal/scm"
)

func main() {
	ctx := context.Background()

	if len(os.Args) != 3 {
		panic("usage: github-approve-deps <owner> <repo>")
	}

	c := scm.NewGithubClient(http.DefaultClient, os.Getenv("USER_GITHUB_TOKEN"))
	u := scm.DependencyUpdateQuery{
		Owner: os.Args[1],
		Repo:  os.Args[2],
	}

	fn := c.ApprovePullRequests
	skipFailing := true

	// fn := c.RecreatePullRequests
	// skipFailing := false

	updates, err := c.GetDependencyUpdates(ctx, u, skipFailing)
	if err != nil {
		panic(err)
	}

	err = fn(ctx, updates)
	if err != nil {
		panic(err)
	}
}
