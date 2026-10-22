package cmd

import (
	"fmt"
	"io"
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

	return reportChanges(os.Stdout, os.Stderr, runner.NewOsRunner(), selectLibraries(cfg.Libraries, changesOnly), changesVerbose)
}

// reportChanges looks up what changed in libs since their last releases and
// prints the table to out, with the progress line going to progress. It
// returns an error when any library could not be checked.
func reportChanges(out, progress io.Writer, r runner.Runner, libs map[string]config.Library, verbose bool) error {
	fmt.Fprintf(progress, "Checking %d %s for changes since their last release...\n", len(libs), libNoun(len(libs)))
	results := changes.RunAll(r, libs)
	changes.Print(out, results, verbose)
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
