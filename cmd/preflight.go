package cmd

import (
	"fmt"
	"os"

	"github.com/kiwiproject/kiwi-star-deployer/internal/config"
	"github.com/kiwiproject/kiwi-star-deployer/internal/preflight"
	"github.com/kiwiproject/kiwi-star-deployer/internal/runner"
	"github.com/spf13/cobra"
)

var preflightCmd = &cobra.Command{
	Use:   "preflight",
	Short: "Verify that all required tools are installed and configured",
	RunE:  runPreflight,
}

func runPreflight(_ *cobra.Command, _ []string) error {
	changelogScript := ".generate-kiwi-changelog"
	if cfg, err := config.Load(configPath); err == nil {
		changelogScript = cfg.Settings.ChangelogScript
	}

	results := preflight.RunAll(runner.NewOsRunner(), changelogScript)
	preflight.Print(os.Stdout, results)
	if !preflight.AllPassed(results) {
		return fmt.Errorf("preflight failed")
	}
	return nil
}

func init() {
	rootCmd.AddCommand(preflightCmd)
}
