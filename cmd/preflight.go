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

	return reportPreflight(os.Stdout, colorMode, runner.NewOsRunner(), changelogScript)
}

// reportPreflight runs the preflight checks and prints the results to f, in
// color when mode and the environment call for it. It returns an error when
// any check failed.
func reportPreflight(f *os.File, mode string, r runner.Runner, changelogScript string) error {
	out, flush, err := consoleOutput(f, mode)
	if err != nil {
		return err
	}
	defer flush()

	results := preflight.RunAll(r, changelogScript)
	preflight.Print(out, results)
	if !preflight.AllPassed(results) {
		return fmt.Errorf("preflight failed")
	}
	return nil
}

func init() {
	rootCmd.AddCommand(preflightCmd)
	addColorFlag(preflightCmd)
}
