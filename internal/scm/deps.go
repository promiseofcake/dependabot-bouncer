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
