package cmd

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

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

var logsPurgeCmd = &cobra.Command{
	Use:   "purge",
	Short: "Delete run log directories older than a specified age",
	RunE:  runLogsPurge,
}

var (
	olderThan string
	purgeYes  bool
)

func runLogsList(_ *cobra.Command, _ []string) error {
	cfg, err := config.Load(configPath)
	if err != nil {
		return err
	}
	return listLogs(os.Stdout, cfg.Settings.LogsDir())
}

func runLogsPurge(_ *cobra.Command, _ []string) error {
	cfg, err := config.Load(configPath)
	if err != nil {
		return err
	}
	age, err := parseAge(olderThan)
	if err != nil {
		return err
	}
	return purgeLogs(os.Stdout, os.Stdin, cfg.Settings.LogsDir(), age, purgeYes)
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

func purgeLogs(w io.Writer, r io.Reader, logsDir string, age time.Duration, yes bool) error {
	entries, err := os.ReadDir(logsDir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			fmt.Fprintln(w, "No release runs found.")
			return nil
		}
		return fmt.Errorf("reading logs directory: %w", err)
	}

	cutoff := time.Now().Add(-age)

	var toDelete []os.DirEntry
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		t, err := time.ParseInLocation("2006-01-02T15-04-05", e.Name(), time.Local)
		if err != nil {
			fmt.Fprintf(w, "skipping %s: not a run directory\n", e.Name())
			continue
		}
		if t.Before(cutoff) {
			toDelete = append(toDelete, e)
		}
	}

	if len(toDelete) == 0 {
		fmt.Fprintln(w, "No run directories older than the specified age.")
		return nil
	}

	noun := "directory"
	if len(toDelete) > 1 {
		noun = "directories"
	}

	fmt.Fprintf(w, "The following %d run %s will be deleted:\n\n", len(toDelete), noun)
	for _, e := range toDelete {
		fmt.Fprintf(w, "  %s\n", e.Name())
	}
	fmt.Fprintln(w)

	if !yes {
		fmt.Fprintf(w, "Delete %d %s? [y/N]: ", len(toDelete), noun)
		var line string
		_, _ = fmt.Fscanln(r, &line)
		if strings.TrimSpace(strings.ToLower(line)) != "y" {
			fmt.Fprintln(w, "Aborted.")
			return nil
		}
	}

	var failed []string
	for _, e := range toDelete {
		if err := os.RemoveAll(filepath.Join(logsDir, e.Name())); err != nil {
			failed = append(failed, fmt.Sprintf("%s: %v", e.Name(), err))
		}
	}
	if len(failed) > 0 {
		return fmt.Errorf("failed to delete some directories:\n  %s", strings.Join(failed, "\n  "))
	}

	fmt.Fprintf(w, "Deleted %d %s.\n", len(toDelete), noun)
	return nil
}

// parseAge parses a duration string supporting days (e.g. "30d") in addition
// to standard Go duration format (e.g. "24h").
func parseAge(s string) (time.Duration, error) {
	if strings.HasSuffix(s, "d") {
		n, err := strconv.Atoi(strings.TrimSuffix(s, "d"))
		if err != nil || n <= 0 {
			return 0, fmt.Errorf("invalid duration %q: days must be a positive integer", s)
		}
		return time.Duration(n) * 24 * time.Hour, nil
	}
	d, err := time.ParseDuration(s)
	if err != nil {
		return 0, fmt.Errorf("invalid duration %q: use Go duration (e.g. 24h) or days (e.g. 30d)", s)
	}
	if d <= 0 {
		return 0, fmt.Errorf("duration %q must be positive", s)
	}
	return d, nil
}

func init() {
	rootCmd.AddCommand(logsCmd)
	logsCmd.AddCommand(logsListCmd)
	logsCmd.AddCommand(logsPurgeCmd)
	logsPurgeCmd.Flags().StringVar(&olderThan, "older-than", "", "delete runs older than this age (e.g. 30d, 24h)")
	logsPurgeCmd.Flags().BoolVar(&purgeYes, "yes", false, "skip confirmation prompt")
	if err := logsPurgeCmd.MarkFlagRequired("older-than"); err != nil {
		panic(err)
	}
}
