package cmd

import (
	"os"

	"github.com/spf13/cobra"
)

var version = "dev"

var configPath string

const configEnvVar = "KIWI_STAR_DEPLOYER_CONFIG"

var rootCmd = &cobra.Command{
	Use:          "kiwi-star-deployer",
	Short:        "Automates kiwiproject library releases in dependency order",
	SilenceUsage: true,
	PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
		if configPath != "" {
			return nil
		}
		if v := os.Getenv(configEnvVar); v != "" {
			configPath = v
			return nil
		}
		configPath = "kiwi-star-deployer.toml"
		return nil
	},
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func init() {
	rootCmd.Version = version
	rootCmd.PersistentFlags().StringVar(&configPath, "config", "", "path to config file (overrides $"+configEnvVar+")")
}
