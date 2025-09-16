package scm

type DependencyUpdateQuery struct {
	Owner          string
	Repo           string
	IgnoredPRs     []int
	DeniedPackages []string // List of package names to exclude
	DeniedOrgs     []string // List of organization names to exclude (e.g., "datadog")
}

type DependencyUpdateRequest struct {
	Owner             string
	Repo              string
	PullRequestNumber int
	Title             string // PR title for logging
	PackageName       string // Extracted package name
}
