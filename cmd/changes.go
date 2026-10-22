package cmd

import (
	"fmt"
	"os"

	"github.com/kiwiproject/kiwi-star-deployer/internal/changes"
	"github.com/kiwiproject/kiwi-star-deployer/internal/config"
	"github.com/kiwiproject/kiwi-star-deployer/internal/runner"
	"github.com/spf13/cobra"
)

var (
	changesOnly    []string
	changesVerbose bool
)

var changesCmd = &cobra.Command{
	Use:   "changes",
	Short: "Show what changed in each library since its last release",
	RunE:  runChanges,
}

func runChanges(_ *cobra.Command, _ []string) error {
	cfg, err := config.Load(configPath)
	if err != nil {
		return err
	}
	for _, name := range changesOnly {
		if _, ok := cfg.Libraries[name]; !ok {
			return fmt.Errorf("--only: unknown library %q", name)
		}
	}

	libs := selectLibraries(cfg.Libraries, changesOnly)
	fmt.Fprintf(os.Stderr, "Checking %d %s for changes since their last release...\n", len(libs), libNoun(len(libs)))
	results := changes.RunAll(runner.NewOsRunner(), libs)
	changes.Print(os.Stdout, results, changesVerbose)
	if changes.AnyFailed(results) {
		return fmt.Errorf("could not check every library")
	}
	return nil
}

func init() {
	changesCmd.Flags().StringSliceVar(&changesOnly, "only", nil, "check only these libraries (comma-separated)")
	changesCmd.Flags().BoolVarP(&changesVerbose, "verbose", "v", false, "list the commits behind each library's counts")
	rootCmd.AddCommand(changesCmd)
}
