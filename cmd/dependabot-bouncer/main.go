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

	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default search: $XDG_CONFIG_HOME/dependabot-bouncer/config.yaml, or $HOME/.config/dependabot-bouncer/config.yaml if XDG_CONFIG_HOME is unset, then $HOME/.dependabot-bouncer/config.yaml)")
	rootCmd.PersistentFlags().StringSlice("deny-packages", []string{}, "Packages to deny")
	rootCmd.PersistentFlags().StringSlice("deny-orgs", []string{}, "Organizations to deny")

	viper.BindPFlag("deny-packages", rootCmd.PersistentFlags().Lookup("deny-packages"))
	viper.BindPFlag("deny-orgs", rootCmd.PersistentFlags().Lookup("deny-orgs"))

	rootCmd.AddCommand(approveCmd, recreateCmd, checkCmd)
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

		// XDG config directory ($XDG_CONFIG_HOME or ~/.config)
		xdgConfigHome := os.Getenv("XDG_CONFIG_HOME")
		if xdgConfigHome == "" {
			xdgConfigHome = filepath.Join(home, ".config")
		}

		// Search XDG path first, then legacy home directory path
		viper.AddConfigPath(filepath.Join(xdgConfigHome, "dependabot-bouncer"))
		viper.AddConfigPath(filepath.Join(home, ".dependabot-bouncer"))
		viper.SetConfigType("yaml")
		viper.SetConfigName("config")
	}

	// Bind environment variables
	viper.SetEnvPrefix("DEPENDABOT_BOUNCER")
	viper.AutomaticEnv()

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
