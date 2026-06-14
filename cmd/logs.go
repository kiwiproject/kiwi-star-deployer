package cmd

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"text/tabwriter"

	"github.com/kiwiproject/kiwi-star-deployer/internal/config"
	"github.com/kiwiproject/kiwi-star-deployer/internal/state"
	"github.com/spf13/cobra"
)

var logsCmd = &cobra.Command{
	Use:   "logs",
	Short: "Commands for inspecting release run logs",
}

var logsListCmd = &cobra.Command{
	Use:   "list",
	Short: "List previous release runs",
	RunE:  runLogsList,
}

func runLogsList(_ *cobra.Command, _ []string) error {
	cfg, err := config.Load(configPath)
	if err != nil {
		return err
	}
	return listLogs(os.Stdout, cfg.Settings.LogsDir())
}

func listLogs(w io.Writer, logsDir string) error {
	entries, err := os.ReadDir(logsDir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			fmt.Fprintln(w, "No release runs found.")
			return nil
		}
		return fmt.Errorf("reading logs directory: %w", err)
	}

	// Collect only subdirectories; skip stray files.
	var dirs []os.DirEntry
	for _, e := range entries {
		if e.IsDir() {
			dirs = append(dirs, e)
		}
	}

	if len(dirs) == 0 {
		fmt.Fprintln(w, "No release runs found.")
		return nil
	}

	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "Run\tCompleted\tStatus")

	for i := len(dirs) - 1; i >= 0; i-- {
		entry := dirs[i]
		stateFile := filepath.Join(logsDir, entry.Name(), "state.json")
		s, err := state.Load(stateFile)
		if err != nil {
			fmt.Fprintf(tw, "%s\t-\tunreadable: %v\n", entry.Name(), err)
			continue
		}
		status := "ok"
		if s.Failed != nil {
			status = fmt.Sprintf("failed: %s (%s)", s.Failed.Library, s.Failed.Step)
		}
		fmt.Fprintf(tw, "%s\t%d\t%s\n", s.RunID, len(s.Completed), status)
	}
	_ = tw.Flush()
	return nil
}

func init() {
	rootCmd.AddCommand(logsCmd)
	logsCmd.AddCommand(logsListCmd)
}
