package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os/exec"
	"strings"
	"time"

	"github.com/promiseofcake/dependabot-bouncer/internal/scm"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	approveCmd = &cobra.Command{
		Use:   "approve owner/repo",
		Short: "Approve dependency update pull requests",
		Long:  `Approve passing dependency update pull requests from Dependabot.`,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			parts := strings.Split(args[0], "/")
			if len(parts) != 2 {
				return fmt.Errorf("invalid repository format: %s (expected owner/repo)", args[0])
			}
			return runDependencyUpdate(parts[0], parts[1], false)
		},
	}

	recreateCmd = &cobra.Command{
		Use:   "recreate owner/repo",
		Short: "Recreate dependency update pull requests",
		Long:  `Recreate all dependency update pull requests from Dependabot (including failing ones).`,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			parts := strings.Split(args[0], "/")
			if len(parts) != 2 {
				return fmt.Errorf("invalid repository format: %s (expected owner/repo)", args[0])
			}
			return runDependencyUpdate(parts[0], parts[1], true)
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

	closeCmd = &cobra.Command{
		Use:   "close owner/repo",
		Short: "Close old PRs with the dependencies label",
		Long: `Close pull requests that have the 'dependencies' label and are older than
the specified maximum age.

Duration format uses Go's standard time.Duration parsing:
  - Hours: 720h (30 days), 4320h (6 months), 8760h (1 year)
  - Minutes: 43200m (30 days)
  - Combined: 720h30m

Examples:
  dependabot-bouncer close owner/repo --older-than 720h
  dependabot-bouncer close owner/repo --older-than 4320h --label dependencies
  dependabot-bouncer close owner/repo --older-than 2160h --dry-run`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			parts := strings.Split(args[0], "/")
			if len(parts) != 2 {
				return fmt.Errorf("invalid repository format: %s (expected owner/repo)", args[0])
			}
			return runClose(parts[0], parts[1])
		},
	}
)

func init() {
	closeCmd.Flags().Duration("older-than", 0, "Close PRs older than this duration (e.g., 720h for 30 days)")
	closeCmd.Flags().String("label", "dependencies", "Label to filter PRs by")
	closeCmd.Flags().Bool("dry-run", false, "Show PRs that would be closed without closing them")
	viper.BindPFlag("older-than", closeCmd.Flags().Lookup("older-than"))
	viper.BindPFlag("label", closeCmd.Flags().Lookup("label"))
	viper.BindPFlag("dry-run", closeCmd.Flags().Lookup("dry-run"))
}

func runCheck(cmd *cobra.Command, args []string) error {
	ctx := context.Background()

	// Get GitHub token
	token, err := resolveGitHubToken()
	if err != nil {
		return err
	}

	// Get list of repositories to check
	var repos []string

	if len(args) > 0 {
		// Use command-line arguments
		repos = args
	} else {
		// Get all configured repositories from the main config
		repoMap := viper.GetStringMap("repositories")
		for repo := range repoMap {
			repos = append(repos, repo)
		}

		// If no repositories in main config, check for legacy check.repositories
		if len(repos) == 0 {
			repos = viper.GetStringSlice("check.repositories")
		}
	}

	if len(repos) == 0 {
		return fmt.Errorf("no repositories specified. Use command-line arguments or configure repositories in config file")
	}

	// Create GitHub client
	c := scm.NewGithubClient(http.DefaultClient, token)

	fmt.Println("📦 Open Dependabot PRs:")
	fmt.Println("-------------------------")

	for _, repoPath := range repos {
		parts := strings.Split(repoPath, "/")
		if len(parts) != 2 {
			fmt.Printf("⚠️  Invalid repository format: %s (expected owner/repo)\n\n", repoPath)
			continue
		}

		owner, repo := parts[0], parts[1]
		fmt.Printf("🔍 %s/%s\n", owner, repo)

		// Build query with deny lists
		repoKey := fmt.Sprintf("%s/%s", owner, repo)

		// Get deny lists - merge global and repo-specific
		deniedPackages := getStringSlice("global.denied_packages")
		deniedOrgs := getStringSlice("global.denied_orgs")

		// Add repo-specific denies
		deniedPackages = append(deniedPackages, getStringSlice("repositories."+repoKey+".denied_packages")...)
		deniedOrgs = append(deniedOrgs, getStringSlice("repositories."+repoKey+".denied_orgs")...)

		// Remove duplicates
		deniedPackages = removeDuplicates(deniedPackages)
		deniedOrgs = removeDuplicates(deniedOrgs)

		q := scm.DependencyUpdateQuery{
			Owner:          owner,
			Repo:           repo,
			DeniedPackages: deniedPackages,
			DeniedOrgs:     deniedOrgs,
		}

		// Get open Dependabot PRs with deny list info
		prs, err := c.GetDependabotPRsWithDenyList(ctx, q)
		if err != nil {
			fmt.Printf("   ❌ Error: %v\n\n", err)
			continue
		}

		if len(prs) == 0 {
			fmt.Println("   (no open Dependabot PRs)")
		} else {
			for _, pr := range prs {
				if pr.Skipped {
					fmt.Printf("   #%d: %s\n", pr.Number, pr.Title)
					fmt.Printf("   %s\n", pr.URL)
					fmt.Printf("   Status: 🚫 SKIPPED (%s)\n", pr.SkipReason)
				} else {
					fmt.Printf("   #%d: %s\n", pr.Number, pr.Title)
					fmt.Printf("   %s\n", pr.URL)
					if pr.Status != "" {
						statusIcon := "⏳"
						if pr.Status == "success" {
							statusIcon = "✅"
						} else if pr.Status == "failure" {
							statusIcon = "❌"
						}
						fmt.Printf("   Status: %s %s\n", statusIcon, pr.Status)
					}
				}
				fmt.Println()
			}
		}
		fmt.Println()
	}

	return nil
}

func runDependencyUpdate(owner, repo string, recreate bool) error {
	ctx := context.Background()

	// Get GitHub token
	token, err := resolveGitHubToken()
	if err != nil {
		return err
	}

	// Build the repository key for looking up repo-specific config
	repoKey := fmt.Sprintf("%s/%s", owner, repo)

	// Get deny lists - merge global and repo-specific
	deniedPackages := getStringSlice("global.denied_packages")
	deniedOrgs := getStringSlice("global.denied_orgs")
	ignoredPRs := getIntSlice("repositories." + repoKey + ".ignored_prs")

	// Add repo-specific denies
	deniedPackages = append(deniedPackages, getStringSlice("repositories."+repoKey+".denied_packages")...)
	deniedOrgs = append(deniedOrgs, getStringSlice("repositories."+repoKey+".denied_orgs")...)

	// Add command-line overrides (these take precedence)
	if cmdPackages := viper.GetStringSlice("deny-packages"); len(cmdPackages) > 0 {
		deniedPackages = append(deniedPackages, cmdPackages...)
	}
	if cmdOrgs := viper.GetStringSlice("deny-orgs"); len(cmdOrgs) > 0 {
		deniedOrgs = append(deniedOrgs, cmdOrgs...)
	}

	// Remove duplicates
	deniedPackages = removeDuplicates(deniedPackages)
	deniedOrgs = removeDuplicates(deniedOrgs)

	// Log what we're doing
	if len(deniedPackages) > 0 {
		log.Printf("Denying packages: %v\n", deniedPackages)
	}
	if len(deniedOrgs) > 0 {
		log.Printf("Denying organizations: %v\n", deniedOrgs)
	}
	if len(ignoredPRs) > 0 {
		log.Printf("Ignoring PRs: %v\n", ignoredPRs)
	}

	// Create GitHub client
	c := scm.NewGithubClient(http.DefaultClient, token)
	q := scm.DependencyUpdateQuery{
		Owner:          owner,
		Repo:           repo,
		IgnoredPRs:     ignoredPRs,
		DeniedPackages: deniedPackages,
		DeniedOrgs:     deniedOrgs,
	}

	// Determine skip failing behavior
	skipFailing := !recreate // Approve mode skips failing, recreate mode doesn't

	// Get dependency updates
	updates, err := c.GetDependencyUpdates(ctx, q, skipFailing)
	if err != nil {
		return fmt.Errorf("failed to get dependency updates: %w", err)
	}

	if len(updates) == 0 {
		fmt.Println("No dependency updates to process")
		return nil
	}

	// Execute the appropriate action
	fmt.Printf("Processing %d pull requests...\n", len(updates))

	if recreate {
		err = c.RecreatePullRequests(ctx, updates)
	} else {
		err = c.ApprovePullRequests(ctx, updates)
	}

	if err != nil {
		return fmt.Errorf("failed to process pull requests: %w", err)
	}

	return nil
}

// resolveGitHubToken returns the GitHub token from config/env, falling back to `gh auth token`.
func resolveGitHubToken() (string, error) {
	token := viper.GetString("github-token")
	if token == "" {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if out, err := exec.CommandContext(ctx, "gh", "auth", "token").Output(); err != nil {
			log.Printf("gh auth token fallback failed: %v", err)
		} else {
			token = strings.TrimSpace(string(out))
		}
	}
	if token == "" {
		return "", fmt.Errorf("GitHub token not found. Either run 'gh auth login' or set --github-token flag / USER_GITHUB_TOKEN environment variable")
	}
	return token, nil
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
	result := []string{}

	for _, item := range slice {
		normalized := strings.ToLower(strings.TrimSpace(item))
		if normalized != "" && !seen[normalized] {
			seen[normalized] = true
			result = append(result, item)
		}
	}

	return result
}

func runClose(owner, repo string) error {
	ctx := context.Background()

	// Get GitHub token
	token, err := resolveGitHubToken()
	if err != nil {
		return err
	}

	// Get command options
	olderThan := viper.GetDuration("older-than")
	if olderThan == 0 {
		return fmt.Errorf("--older-than flag is required (e.g., 720h for 30 days, 4320h for 6 months)")
	}

	label := viper.GetString("label")
	dryRun := viper.GetBool("dry-run")

	// Create GitHub client
	c := scm.NewGithubClient(http.DefaultClient, token)

	// Get old PRs matching criteria
	prs, err := c.GetOldLabeledPRs(ctx, owner, repo, label, olderThan)
	if err != nil {
		return fmt.Errorf("failed to get old PRs: %w", err)
	}

	if len(prs) == 0 {
		fmt.Printf("No PRs found with label '%s' older than %s\n", label, olderThan)
		return nil
	}

	// Display PRs to be closed
	fmt.Printf("Found %d PR(s) with label '%s' older than %s:\n\n", len(prs), label, olderThan)
	for _, pr := range prs {
		fmt.Printf("  #%d: %s\n", pr.Number, pr.Title)
		fmt.Printf("       Created: %s (age: %s)\n", pr.CreatedAt, pr.Age)
		fmt.Printf("       %s\n\n", pr.URL)
	}

	if dryRun {
		fmt.Println("Dry run mode - no PRs were closed")
		return nil
	}

	// Close the PRs
	fmt.Printf("Closing %d PR(s)...\n", len(prs))
	if err := c.ClosePullRequests(ctx, owner, repo, prs); err != nil {
		return fmt.Errorf("failed to close PRs: %w", err)
	}

	fmt.Printf("Successfully closed %d PR(s)\n", len(prs))
	return nil
}
