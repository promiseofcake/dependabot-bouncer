package scm

type DependencyUpdateQuery struct {
	Owner      string
	Repo       string
	IgnoredPRs []int
}

type DependencyUpdateRequest struct {
	Owner             string
	Repo              string
	PullRequestNumber int
}
