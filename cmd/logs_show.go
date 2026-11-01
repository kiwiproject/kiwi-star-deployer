package cmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"time"

	"github.com/kiwiproject/kiwi-star-deployer/internal/config"
	"github.com/kiwiproject/kiwi-star-deployer/internal/state"
	"github.com/spf13/cobra"
)

// followInterval is how often logs show --follow checks for new output.
const followInterval = time.Second

var logsShowCmd = &cobra.Command{
	Use:   "show [run]",
	Short: "Print a run's session log, optionally following it as it grows",
	Long: `Print the session log of a release run, colored like the release console.
The run is named by its directory under the logs directory or by the run ID
that "logs list" prints, and defaults to the latest run. With --follow the
command keeps printing new output until it is interrupted with Ctrl-C.`,
	Args: cobra.MaximumNArgs(1),
	RunE: runLogsShow,
}

var showFollow bool

func runLogsShow(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load(configPath)
	if err != nil {
		return err
	}
	run := ""
	if len(args) == 1 {
		run = args[0]
	}

	ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt)
	defer stop()
	return showLog(ctx, os.Stdout, colorMode, cfg.Settings.LogsDir(), run, showFollow, followInterval)
}

// showLog prints the session log of run (the latest run when empty) to f, in
// color when mode and the environment call for it. When follow is set it keeps
// checking for new output every interval until ctx is done.
func showLog(ctx context.Context, f *os.File, mode, logsDir, run string, follow bool, interval time.Duration) error {
	out, flush, err := consoleOutput(f, mode)
	if err != nil {
		return err
	}
	defer flush()

	dir, err := resolveRun(logsDir, run)
	if err != nil {
		return err
	}
	log, err := os.Open(filepath.Join(dir, "session.log"))
	if err != nil {
		return fmt.Errorf("opening the session log: %w", err)
	}
	defer func() { _ = log.Close() }()

	for {
		// The file offset stays where the last copy stopped, so each pass
		// prints only what was appended since.
		if _, err := io.Copy(out, log); err != nil {
			return fmt.Errorf("reading the session log: %w", err)
		}
		if !follow {
			return nil
		}
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(interval):
		}
	}
}

// resolveRun returns the directory of the named run: the latest run when run
// is empty, otherwise the directory with that name or the newest one whose
// state file has that run ID (a resumed run keeps the ID of the run it
// resumes). Directories whose names are not run directory names are ignored.
func resolveRun(logsDir, run string) (string, error) {
	entries, err := os.ReadDir(logsDir)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf("reading logs directory: %w", err)
	}
	var dirs []string
	for _, e := range entries {
		if _, err := time.ParseInLocation(runDirLayout, e.Name(), time.Local); err == nil && e.IsDir() {
			dirs = append(dirs, e.Name())
		}
	}
	if len(dirs) == 0 {
		return "", errors.New("no release runs found")
	}

	if run == "" {
		return filepath.Join(logsDir, dirs[len(dirs)-1]), nil
	}
	for i := len(dirs) - 1; i >= 0; i-- {
		name := dirs[i]
		if name == run {
			return filepath.Join(logsDir, name), nil
		}
		if s, err := state.Load(filepath.Join(logsDir, name, "state.json")); err == nil && s.RunID == run {
			return filepath.Join(logsDir, name), nil
		}
	}
	return "", fmt.Errorf("run %q not found; \"logs list\" shows the runs", run)
}

func init() {
	logsCmd.AddCommand(logsShowCmd)
	logsShowCmd.Flags().BoolVarP(&showFollow, "follow", "f", false, "keep printing new output until interrupted")
	addColorFlag(logsShowCmd)
}
