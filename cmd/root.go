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
		configPath = resolveConfigPath(
			cmd.Root().PersistentFlags().Changed("config"),
			configPath,
		)
		return nil
	},
}

// resolveConfigPath returns the effective config path using the priority:
// --config flag > KIWI_STAR_DEPLOYER_CONFIG env var > defaultPath.
func resolveConfigPath(flagChanged bool, defaultPath string) string {
	if flagChanged {
		return defaultPath
	}
	if v := os.Getenv(configEnvVar); v != "" {
		return v
	}
	return defaultPath
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

// libNoun returns "library" or "libraries" for count n.
func libNoun(n int) string {
	if n == 1 {
		return "library"
	}
	return "libraries"
}

func init() {
	cobra.EnableTraverseRunHooks = true
	rootCmd.Version = version
	rootCmd.PersistentFlags().StringVar(&configPath, "config", "kiwi-star-deployer.toml", "path to config file (overrides $"+configEnvVar+")")
}
