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

// PRInfo contains information about a pull request
type PRInfo struct {
	Number      int
	Title       string
	URL         string
	Status      string // CI status: "success", "failure", "pending", or ""
	Skipped     bool   // Whether PR would be skipped due to deny lists
	SkipReason  string // Reason for skipping (denied package/org name)
}

// ClosePRInfo contains information about a PR to be closed
type ClosePRInfo struct {
	Number    int
	Title     string
	URL       string
	CreatedAt string
	Age       string
}
