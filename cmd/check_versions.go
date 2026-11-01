package cmd

import (
	"fmt"
	"io"
	"os"

	"github.com/kiwiproject/kiwi-star-deployer/internal/checkversions"
	"github.com/kiwiproject/kiwi-star-deployer/internal/config"
	"github.com/kiwiproject/kiwi-star-deployer/internal/runner"
	"github.com/spf13/cobra"
)

var checkVersionsCmd = &cobra.Command{
	Use:   "check-versions",
	Short: "Validate pom.xml versions against open GitHub milestones",
	RunE:  runCheckVersions,
}

func runCheckVersions(_ *cobra.Command, _ []string) error {
	cfg, err := config.Load(configPath)
	if err != nil {
		return err
	}

	return reportVersions(os.Stdout, os.Stderr, colorMode, runner.NewOsRunner(), cfg.Libraries)
}

// reportVersions checks the libraries' versions against their milestones and
// prints the results to f, in color when mode and the environment call for
// it, with the progress line going to progress. It returns an error when any
// check failed.
func reportVersions(f *os.File, progress io.Writer, mode string, r runner.Runner, libs map[string]config.Library) error {
	out, flush, err := consoleOutput(f, mode)
	if err != nil {
		return err
	}
	defer flush()

	fmt.Fprintf(progress, "Checking %d %s against GitHub milestones...\n", len(libs), libNoun(len(libs)))
	results := checkversions.RunAll(r, libs)
	checkversions.Print(out, results)
	if !checkversions.AllPassed(results) {
		return fmt.Errorf("version check failed")
	}
	return nil
}

func init() {
	rootCmd.AddCommand(checkVersionsCmd)
	addColorFlag(checkVersionsCmd)
}
