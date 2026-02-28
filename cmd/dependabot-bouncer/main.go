package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

const banner = `
    ___  ___  ___  ___  _  _  ___   _   ___  ___ _____
   |   \| __|| _ \| __|| \| ||   \ /_\ | _ )|   \_   _|
   | |) | _| |  _/| _| | .  || |) |/ _ \| _ \| | | | |
   |___/|___||_|  |___||_|\_||___//_/ \_\___/|___/ |_|
    ___  ___  _   _ _  _  ___ ___ ___
   | _ )/ _ \| | | | \| |/ __| __| _ \
   | _ \ (_) | |_| | .  | (__| _||   /
   |___/\___/ \___/|_|\_|\___|___|_|_\

          ___
         /   \
        | O O |
        |  _  |  "You're not on the list."
        | |_| |
    ____|_____|____
   |  ___________  |
   | |    [DB]    | |
   | |___________| |
   |___ _______ ___|
      / \     / \
     /   \   /   \
    =======+=======
   __|__       __|__
  |     |     |     |
  |     |     |     |
  |_____|     |_____|
`

var (
	cfgFile string
	rootCmd = &cobra.Command{
		Use:   "dependabot-bouncer",
		Short: "Manage GitHub dependency updates",
		Long: `A tool to manage GitHub dependency updates from Dependabot.

Supports both approve and recreate modes with flexible deny lists for
packages and organizations. Configuration can be provided via YAML file
or command-line flags.`,
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			fmt.Print(banner)
		},
	}
)

func init() {
	cobra.OnInitialize(initConfig)

	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is $HOME/.dependabot-bouncer/config.yaml)")
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

		// Search for config in home directory
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
