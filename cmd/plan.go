package cmd

import (
	"fmt"
	"io"
	"os"

	"github.com/kiwiproject/kiwi-star-deployer/internal/config"
	"github.com/kiwiproject/kiwi-star-deployer/internal/plan"
	"github.com/kiwiproject/kiwi-star-deployer/internal/runner"
	"github.com/kiwiproject/kiwi-star-deployer/internal/workspace"
	"github.com/spf13/cobra"
)

var planCmd = &cobra.Command{
	Use:   "plan",
	Short: "Print release stages, ordering, and resolved versions",
	RunE:  runPlan,
}

func runPlan(_ *cobra.Command, _ []string) error {
	cfg, err := config.Load(configPath)
	if err != nil {
		return err
	}

	ws := workspace.New(cfg.Settings.Workspace, runner.NewOsRunner())

	fmt.Fprintf(os.Stderr, "Reading POMs from origin/main (%d %s)...\n", len(cfg.Libraries), libNoun(len(cfg.Libraries)))
	stages, err := plan.Build(cfg, ws)
	if err != nil {
		return err
	}

	plan.Print(os.Stdout, stages)
	printMilestoneNote(os.Stdout)
	return nil
}

// printMilestoneNote reminds the user that plan derives everything from POMs
// and does not reconcile against GitHub milestones, which release gates on.
func printMilestoneNote(w io.Writer) {
	fmt.Fprintln(w, "\nNote: plan reads versions from POMs only and does not check GitHub milestones.")
	fmt.Fprintln(w, "Run check-versions to verify each library has a matching open milestone;")
	fmt.Fprintln(w, "release runs that check automatically before starting.")
}

func init() {
	rootCmd.AddCommand(planCmd)
}
