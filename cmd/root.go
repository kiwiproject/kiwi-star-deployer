package cmd

import (
	"os"

	"github.com/spf13/cobra"
)

var version = "dev"

var configPath string

var rootCmd = &cobra.Command{
	Use:          "kiwi-star-deployer",
	Short:        "Automates kiwiproject library releases in dependency order",
	SilenceUsage: true,
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func init() {
	rootCmd.Version = version
	rootCmd.PersistentFlags().StringVar(&configPath, "config", "kiwi-star-deployer.toml", "path to config file")
}
