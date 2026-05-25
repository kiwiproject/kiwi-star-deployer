package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/kiwiproject/kiwi-star-deployer/internal/config"
	"github.com/kiwiproject/kiwi-star-deployer/internal/plan"
	"github.com/kiwiproject/kiwi-star-deployer/internal/preflight"
	"github.com/kiwiproject/kiwi-star-deployer/internal/release"
	"github.com/kiwiproject/kiwi-star-deployer/internal/runner"
	"github.com/kiwiproject/kiwi-star-deployer/internal/workspace"
	"github.com/spf13/cobra"
)

var releaseCmd = &cobra.Command{
	Use:   "release",
	Short: "Release kiwiproject libraries in dependency order",
	RunE:  runRelease,
}

func runRelease(_ *cobra.Command, _ []string) error {
	r := runner.NewOsRunner()

	results := preflight.RunAll(r)
	preflight.Print(os.Stdout, results)
	if !preflight.AllPassed(results) {
		return fmt.Errorf("preflight failed; fix the issues above before releasing")
	}

	cfg, err := config.Load(configPath)
	if err != nil {
		return err
	}

	ws := workspace.New(cfg.Settings.Workspace, r)

	stages, err := plan.Build(cfg, ws)
	if err != nil {
		return err
	}

	logBaseDir := filepath.Join(filepath.Dir(cfg.Settings.Workspace), "logs")

	return release.Execute(os.Stdout, stages, ws, r, logBaseDir)
}

func init() {
	rootCmd.AddCommand(releaseCmd)
}
