package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/kiwiproject/kiwi-star-deployer/internal/config"
	"github.com/kiwiproject/kiwi-star-deployer/internal/mavencentral"
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
	cfg, err := config.Load(configPath)
	if err != nil {
		return err
	}

	r := runner.NewOsRunner()

	results := preflight.RunAll(r, cfg.Settings.ChangelogScript)
	preflight.Print(os.Stdout, results)
	if !preflight.AllPassed(results) {
		return fmt.Errorf("preflight failed; fix the issues above before releasing")
	}

	ws := workspace.New(cfg.Settings.Workspace, r)

	stages, err := plan.Build(cfg, ws)
	if err != nil {
		return err
	}

	logBaseDir := filepath.Join(filepath.Dir(cfg.Settings.Workspace), "logs")

	opts := release.Options{
		GroupID:         cfg.Settings.GroupID,
		MaxWait:         time.Duration(cfg.Settings.MavenCentralMaxWait),
		PollInterval:    time.Duration(cfg.Settings.MavenCentralPollInterval),
		Checker:         mavencentral.New(),
		ChangelogScript: cfg.Settings.ChangelogScript,
	}

	return release.Execute(os.Stdout, stages, ws, r, logBaseDir, opts)
}

func init() {
	rootCmd.AddCommand(releaseCmd)
}
