package cmd

import (
	"fmt"
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

	fmt.Fprintf(os.Stderr, "Checking %d %s against GitHub milestones...\n", len(cfg.Libraries), libNoun(len(cfg.Libraries)))
	results := checkversions.RunAll(runner.NewOsRunner(), cfg.Libraries)
	checkversions.Print(os.Stdout, results)
	if !checkversions.AllPassed(results) {
		return fmt.Errorf("version check failed")
	}
	return nil
}

func init() {
	rootCmd.AddCommand(checkVersionsCmd)
}
