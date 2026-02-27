package main

import (
	"fmt"
	"log"
	"strings"

	"github.com/promiseofcake/dependabot-bouncer/internal/scm"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// parseRepo splits an "owner/repo" string into its parts.
func parseRepo(arg string) (owner, repo string, err error) {
	parts := strings.Split(arg, "/")
	if len(parts) != 2 {
		return "", "", fmt.Errorf("invalid repository format: %s (expected owner/repo)", arg)
	}
	return parts[0], parts[1], nil
}

// buildDenyLists merges global and repo-specific deny lists from config.
func buildDenyLists(repoKey string) (deniedPackages, deniedOrgs []string) {
	deniedPackages = getStringSlice("global.denied_packages")
	deniedOrgs = getStringSlice("global.denied_orgs")

	deniedPackages = append(deniedPackages, getStringSlice("repositories."+repoKey+".denied_packages")...)
	deniedOrgs = append(deniedOrgs, getStringSlice("repositories."+repoKey+".denied_orgs")...)

	return removeDuplicates(deniedPackages), removeDuplicates(deniedOrgs)
}

var (
	approveCmd = &cobra.Command{
		Use:   "approve owner/repo",
		Short: "Approve dependency update pull requests",
		Long:  `Approve passing dependency update pull requests from Dependabot.`,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			owner, repo, err := parseRepo(args[0])
			if err != nil {
				return err
			}
			return runApprove(owner, repo)
		},
	}

	recreateCmd = &cobra.Command{
		Use:   "recreate owner/repo",
		Short: "Recreate dependency update pull requests",
		Long:  `Recreate all dependency update pull requests from Dependabot (including failing ones).`,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			owner, repo, err := parseRepo(args[0])
			if err != nil {
				return err
			}
			return runRecreate(owner, repo)
		},
	}

	checkCmd = &cobra.Command{
		Use:   "check [owner/repo...]",
		Short: "Check for open Dependabot PRs across repositories",
		Long: `Check for open Dependabot pull requests across multiple repositories.

If no repositories are specified as arguments, checks all repositories
configured in the 'repositories' section of your config file.

You can specify multiple repositories: check owner1/repo1 owner2/repo2`,
		RunE: runCheck,
	}
)

func runApprove(owner, repo string) error {
	prs, err := listFilteredPRs(owner, repo, true)
	if err != nil {
		return err
	}
	if len(prs) == 0 {
		fmt.Println("No dependency updates to process")
		return nil
	}

	fmt.Printf("Processing %d pull requests...\n", len(prs))

	for _, pr := range prs {
		switch pr.MergeStateStatus {
		case "DIRTY":
			// Conflicts — recreate the PR so Dependabot resolves them.
			if err := scm.RecreatePR(owner, repo, pr.Number); err != nil {
				log.Printf("Warning: failed to recreate PR #%d: %v\n", pr.Number, err)
				continue
			}
			log.Printf("Recreated PR #%d (conflicts): %s\n", pr.Number, pr.Title)

		case "BEHIND":
			// Behind main — request a rebase.
			if err := scm.RebasePR(owner, repo, pr.Number); err != nil {
				log.Printf("Warning: failed to rebase PR #%d: %v\n", pr.Number, err)
			} else {
				log.Printf("Requested rebase on PR #%d (behind main): %s\n", pr.Number, pr.Title)
			}
		}

		if pr.ReviewDecision == "APPROVED" {
			log.Printf("Already approved PR #%d: %s\n", pr.Number, pr.Title)
		} else {
			if err := scm.ApprovePR(owner, repo, pr.Number); err != nil {
				log.Printf("Warning: failed to approve PR #%d: %v\n", pr.Number, err)
				continue
			}
			log.Printf("Approved PR #%d: %s (package: %s)\n", pr.Number, pr.Title, pr.PackageName)
		}

		if err := scm.AutoMergePR(owner, repo, pr.Number); err != nil {
			log.Printf("Warning: failed to enable auto-merge on PR #%d: %v\n", pr.Number, err)
		} else {
			log.Printf("Enabled auto-merge on PR #%d: %s\n", pr.Number, pr.Title)
		}
	}

	return nil
}

func runRecreate(owner, repo string) error {
	prs, err := listFilteredPRs(owner, repo, false)
	if err != nil {
		return err
	}
	if len(prs) == 0 {
		fmt.Println("No dependency updates to process")
		return nil
	}

	fmt.Printf("Processing %d pull requests...\n", len(prs))

	for _, pr := range prs {
		if err := scm.RecreatePR(owner, repo, pr.Number); err != nil {
			log.Printf("Warning: failed to recreate PR #%d: %v\n", pr.Number, err)
		} else {
			log.Printf("Recreated PR #%d: %s (package: %s)\n", pr.Number, pr.Title, pr.PackageName)
		}
	}

	return nil
}

func runCheck(cmd *cobra.Command, args []string) error {
	var repos []string

	if len(args) > 0 {
		repos = args
	} else {
		repoMap := viper.GetStringMap("repositories")
		for repo := range repoMap {
			repos = append(repos, repo)
		}
		if len(repos) == 0 {
			repos = viper.GetStringSlice("check.repositories")
		}
	}

	if len(repos) == 0 {
		return fmt.Errorf("no repositories specified. Use command-line arguments or configure repositories in config file")
	}

	fmt.Println("Open Dependabot PRs:")
	fmt.Println("-------------------------")

	for _, repoPath := range repos {
		owner, repo, pErr := parseRepo(repoPath)
		if pErr != nil {
			fmt.Printf("  Invalid: %v\n\n", pErr)
			continue
		}

		fmt.Printf("%s/%s\n", owner, repo)

		repoKey := fmt.Sprintf("%s/%s", owner, repo)
		deniedPackages, deniedOrgs := buildDenyLists(repoKey)

		q := scm.DependencyUpdateQuery{
			Owner:          owner,
			Repo:           repo,
			DeniedPackages: deniedPackages,
			DeniedOrgs:     deniedOrgs,
		}

		prs, err := scm.ListDependabotPRs(q, false)
		if err != nil {
			fmt.Printf("   Error: %v\n\n", err)
			continue
		}

		if len(prs) == 0 {
			fmt.Println("   (no open Dependabot PRs)")
		} else {
			for _, pr := range prs {
				fmt.Printf("   #%d: %s\n", pr.Number, pr.Title)
				fmt.Printf("   %s\n", pr.URL)
				if pr.Skipped {
					fmt.Printf("   Status: SKIPPED (%s)\n", pr.SkipReason)
				} else {
					fmt.Printf("   CI: %s | Merge: %s\n", pr.CIStatus, pr.MergeStateStatus)
				}
				fmt.Println()
			}
		}
		fmt.Println()
	}

	return nil
}

// listFilteredPRs builds a query from config and returns filtered Dependabot PRs.
func listFilteredPRs(owner, repo string, skipFailing bool) ([]scm.PRInfo, error) {
	repoKey := fmt.Sprintf("%s/%s", owner, repo)
	deniedPackages, deniedOrgs := buildDenyLists(repoKey)
	ignoredPRs := getIntSlice("repositories." + repoKey + ".ignored_prs")

	if cmdPackages := viper.GetStringSlice("deny-packages"); len(cmdPackages) > 0 {
		deniedPackages = removeDuplicates(append(deniedPackages, cmdPackages...))
	}
	if cmdOrgs := viper.GetStringSlice("deny-orgs"); len(cmdOrgs) > 0 {
		deniedOrgs = removeDuplicates(append(deniedOrgs, cmdOrgs...))
	}

	if len(deniedPackages) > 0 {
		log.Printf("Denying packages: %v\n", deniedPackages)
	}
	if len(deniedOrgs) > 0 {
		log.Printf("Denying organizations: %v\n", deniedOrgs)
	}
	if len(ignoredPRs) > 0 {
		log.Printf("Ignoring PRs: %v\n", ignoredPRs)
	}

	q := scm.DependencyUpdateQuery{
		Owner:          owner,
		Repo:           repo,
		IgnoredPRs:     ignoredPRs,
		DeniedPackages: deniedPackages,
		DeniedOrgs:     deniedOrgs,
	}

	return scm.ListDependabotPRs(q, skipFailing)
}

// Helper functions

func getStringSlice(key string) []string {
	if viper.IsSet(key) {
		return viper.GetStringSlice(key)
	}
	return []string{}
}

func getIntSlice(key string) []int {
	if viper.IsSet(key) {
		return viper.GetIntSlice(key)
	}
	return []int{}
}

func removeDuplicates(slice []string) []string {
	seen := make(map[string]bool)
	var result []string

	for _, item := range slice {
		normalized := strings.ToLower(strings.TrimSpace(item))
		if normalized != "" && !seen[normalized] {
			seen[normalized] = true
			result = append(result, item)
		}
	}

	return result
}
