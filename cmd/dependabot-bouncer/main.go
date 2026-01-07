package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	cfgFile string
	rootCmd = &cobra.Command{
		Use:   "dependabot-bouncer",
		Short: "Manage GitHub dependency updates",
		Long: `A tool to manage GitHub dependency updates from Dependabot.

Supports both approve and recreate modes with flexible deny lists for
packages and organizations. Configuration can be provided via YAML file
or command-line flags.`,
	}
)

func init() {
	cobra.OnInitialize(initConfig)

	// Global flags
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is $HOME/.dependabot-bouncer/config.yaml)")
	rootCmd.PersistentFlags().String("github-token", "", "GitHub token (defaults to USER_GITHUB_TOKEN env var)")
	rootCmd.PersistentFlags().StringSlice("deny-packages", []string{}, "Packages to deny")
	rootCmd.PersistentFlags().StringSlice("deny-orgs", []string{}, "Organizations to deny")

	// Bind flags to viper
	viper.BindPFlag("github-token", rootCmd.PersistentFlags().Lookup("github-token"))
	viper.BindPFlag("deny-packages", rootCmd.PersistentFlags().Lookup("deny-packages"))
	viper.BindPFlag("deny-orgs", rootCmd.PersistentFlags().Lookup("deny-orgs"))

	// Add subcommands
	rootCmd.AddCommand(approveCmd, recreateCmd, checkCmd, closeCmd)

	// Close command flags
	closeCmd.Flags().Duration("older-than", 0, "Close PRs older than this duration (e.g., 720h for 30 days)")
	closeCmd.Flags().String("label", "dependencies", "Label to filter PRs by")
	closeCmd.Flags().Bool("dry-run", false, "Show PRs that would be closed without closing them")
	viper.BindPFlag("older-than", closeCmd.Flags().Lookup("older-than"))
	viper.BindPFlag("label", closeCmd.Flags().Lookup("label"))
	viper.BindPFlag("dry-run", closeCmd.Flags().Lookup("dry-run"))
}

func initConfig() {
	if cfgFile != "" {
		// Use config file from the flag
		viper.SetConfigFile(cfgFile)
	} else {
		// Find home directory
		home, err := os.UserHomeDir()
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}

		// Search for config in home directory
		viper.AddConfigPath(filepath.Join(home, ".dependabot-bouncer"))
		viper.SetConfigType("yaml")
		viper.SetConfigName("config")
	}

	// Bind environment variables
	viper.SetEnvPrefix("DEPENDABOT_BOUNCER")
	viper.AutomaticEnv()

	// Also check for USER_GITHUB_TOKEN specifically
	viper.BindEnv("github-token", "USER_GITHUB_TOKEN")

	// Read config file if it exists
	if err := viper.ReadInConfig(); err == nil {
		fmt.Println("Using config file:", viper.ConfigFileUsed())
	}
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
