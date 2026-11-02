package cmd

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"
)

// writeSessionLog creates a run directory under logsDir holding a session.log
// with the given text, and returns the path of the log.
func writeSessionLog(t *testing.T, logsDir, run, text string) string {
	t.Helper()
	path := filepath.Join(makeDir(t, logsDir, run), "session.log")
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestShowLog_printsTheLatestRunByDefault(t *testing.T) {
	logsDir := t.TempDir()
	writeSessionLog(t, logsDir, "2026-01-15T09-30-00", "older run\n")
	writeSessionLog(t, logsDir, "2026-01-16T10-00-00", "latest run\n")
	f := outputFile(t)

	if err := showLog(context.Background(), f, "never", logsDir, "", "", false, time.Millisecond); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got := contents(t, f); got != "latest run\n" {
		t.Errorf("got %q, want the latest run's log", got)
	}
}

func TestShowLog_namedRunByDirectoryOrRunID(t *testing.T) {
	logsDir := t.TempDir()
	writeSessionLog(t, logsDir, "2026-01-15T09-30-00", "first run\n")
	writeSessionLog(t, logsDir, "2026-01-16T10-00-00", "second run\n")
	newState(t, filepath.Join(logsDir, "2026-01-15T09-30-00", "state.json"), "2026-01-15T09:30:00Z")

	for _, run := range []string{"2026-01-15T09-30-00", "2026-01-15T09:30:00Z"} {
		t.Run(run, func(t *testing.T) {
			f := outputFile(t)

			if err := showLog(context.Background(), f, "never", logsDir, run, "", false, time.Millisecond); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if got := contents(t, f); got != "first run\n" {
				t.Errorf("got %q, want the first run's log", got)
			}
		})
	}
}

func TestShowLog_ignoresDirectoriesThatAreNotRuns(t *testing.T) {
	logsDir := t.TempDir()
	writeSessionLog(t, logsDir, "2026-01-15T09-30-00", "real run\n")
	makeDir(t, logsDir, "tmp")
	f := outputFile(t)

	if err := showLog(context.Background(), f, "never", logsDir, "", "", false, time.Millisecond); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got := contents(t, f); got != "real run\n" {
		t.Errorf("got %q, want the real run's log", got)
	}
}

func TestShowLog_aResumedRunIDShowsTheNewestAttempt(t *testing.T) {
	logsDir := t.TempDir()
	writeSessionLog(t, logsDir, "2026-01-15T09-30-00", "first attempt\n")
	writeSessionLog(t, logsDir, "2026-01-15T11-00-00", "resumed attempt\n")
	for _, dir := range []string{"2026-01-15T09-30-00", "2026-01-15T11-00-00"} {
		newState(t, filepath.Join(logsDir, dir, "state.json"), "2026-01-15T09:30:00Z")
	}
	f := outputFile(t)

	if err := showLog(context.Background(), f, "never", logsDir, "2026-01-15T09:30:00Z", "", false, time.Millisecond); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got := contents(t, f); got != "resumed attempt\n" {
		t.Errorf("got %q, want the newest attempt's log", got)
	}
}

func TestShowLog_colorsLikeTheConsoleOnlyWhenAsked(t *testing.T) {
	logsDir := t.TempDir()
	writeSessionLog(t, logsDir, "2026-01-15T09-30-00", "21:40:21  Stage 1: releasing kiwi\n")

	colored, plain := outputFile(t), outputFile(t)
	if err := showLog(context.Background(), colored, "always", logsDir, "", "", false, time.Millisecond); err != nil {
		t.Fatal(err)
	}
	if err := showLog(context.Background(), plain, "never", logsDir, "", "", false, time.Millisecond); err != nil {
		t.Fatal(err)
	}

	if got := contents(t, colored); !strings.Contains(got, "\x1b[1;36mStage 1: releasing kiwi") {
		t.Errorf("expected the stage header in bold cyan, got %q", got)
	}
	if got := contents(t, plain); got != "21:40:21  Stage 1: releasing kiwi\n" {
		t.Errorf("expected the log unchanged, got %q", got)
	}
}

func TestShowLog_errors(t *testing.T) {
	logsDir := t.TempDir()
	writeSessionLog(t, logsDir, "2026-01-15T09-30-00", "x\n")
	noLog := t.TempDir()
	makeDir(t, noLog, "2026-01-15T09-30-00")

	tests := []struct {
		name    string
		logsDir string
		run     string
		mode    string
		want    string
	}{
		{"no logs directory", filepath.Join(t.TempDir(), "logs"), "", "never", "no release runs found"},
		{"no runs", t.TempDir(), "", "never", "no release runs found"},
		{"unknown run", logsDir, "nope", "never", `run "nope" not found`},
		{"run without a session log", noLog, "", "never", "opening session.log"},
		{"invalid color mode", logsDir, "", "sometimes", "invalid --color"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := showLog(context.Background(), outputFile(t), tt.mode, tt.logsDir, tt.run, "", false, time.Millisecond)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("got %v, want an error containing %q", err, tt.want)
			}
		})
	}
}

func TestShowLog_followPrintsWhatIsAppendedUntilInterrupted(t *testing.T) {
	logsDir := t.TempDir()
	path := writeSessionLog(t, logsDir, "2026-01-15T09-30-00", "first\n")
	f := outputFile(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan error, 1)
	go func() { done <- showLog(ctx, f, "never", logsDir, "", "", true, time.Millisecond) }()

	waitFor(t, func() bool { return contents(t, f) == "first\n" })
	appendTo, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = appendTo.Close() }()
	if _, err := appendTo.WriteString("second\n"); err != nil {
		t.Fatal(err)
	}
	waitFor(t, func() bool { return contents(t, f) == "first\nsecond\n" })

	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("following should end without an error, got %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("showLog did not return after the context was canceled")
	}
}

// waitFor polls cond until it holds, failing the test if it never does.
func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatal("timed out waiting for the expected output")
		}
		time.Sleep(time.Millisecond)
	}
}

// runShowCommand runs runLogsShow against a config whose logs directory is
// logsDir, with os.Stdout replaced by a file, and returns what it printed.
func runShowCommand(t *testing.T, logsDir string, args ...string) (string, error) {
	t.Helper()
	cfgPath := filepath.Join(t.TempDir(), "config.toml")
	cfg := "[settings]\nworkspace = " + strconv.Quote(filepath.Join(filepath.Dir(logsDir), "workspace")) + "\n"
	if err := os.WriteFile(cfgPath, []byte(cfg), 0o600); err != nil {
		t.Fatal(err)
	}
	stdout := outputFile(t)
	oldConfigPath, oldStdout, oldColor := configPath, os.Stdout, colorMode
	t.Cleanup(func() { configPath, os.Stdout, colorMode = oldConfigPath, oldStdout, oldColor })
	configPath, os.Stdout, colorMode = cfgPath, stdout, "never"

	cmd := &cobra.Command{}
	cmd.SetContext(context.Background())
	err := runLogsShow(cmd, args)
	return contents(t, stdout), err
}

func TestRunLogsShow_printsTheNamedRunFromTheConfiguredLogsDir(t *testing.T) {
	logsDir := makeDir(t, t.TempDir(), "logs")
	writeSessionLog(t, logsDir, "2026-01-15T09-30-00", "first run\n")
	writeSessionLog(t, logsDir, "2026-01-16T10-00-00", "second run\n")

	got, err := runShowCommand(t, logsDir, "2026-01-15T09-30-00")

	if err != nil || got != "first run\n" {
		t.Errorf("got (%q, %v), want the first run's log", got, err)
	}
}

func TestRunLogsShow_configLoadError(t *testing.T) {
	oldConfigPath := configPath
	t.Cleanup(func() { configPath = oldConfigPath })
	configPath = filepath.Join(t.TempDir(), "missing.toml")

	if err := runLogsShow(&cobra.Command{}, nil); err == nil {
		t.Error("expected an error for a missing config")
	}
}

func TestShowLog_logsDirectoryThatCannotBeRead(t *testing.T) {
	notADir := writeSessionLog(t, t.TempDir(), "2026-01-15T09-30-00", "x\n")

	err := showLog(context.Background(), outputFile(t), "never", notADir, "", "", false, time.Millisecond)

	if err == nil || !strings.Contains(err.Error(), "reading logs directory") {
		t.Errorf("expected a reading error, got %v", err)
	}
}

func TestShowLog_libraryLogIsPrintedAndNeverColored(t *testing.T) {
	logsDir := t.TempDir()
	runDir := filepath.Dir(writeSessionLog(t, logsDir, "2026-01-15T09-30-00", "session\n"))
	// Text the release console would color, to show that it is left alone.
	writeFile(t, filepath.Join(runDir, "kiwi.log"), "done    kiwi\n")
	f := outputFile(t)

	if err := showLog(context.Background(), f, "always", logsDir, "", "kiwi", false, time.Millisecond); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got := contents(t, f); got != "done    kiwi\n" {
		t.Errorf("got %q, want kiwi.log exactly as written", got)
	}
}

func TestShowLog_libraryLogOfANamedRun(t *testing.T) {
	logsDir := t.TempDir()
	first := filepath.Dir(writeSessionLog(t, logsDir, "2026-01-15T09-30-00", "s\n"))
	second := filepath.Dir(writeSessionLog(t, logsDir, "2026-01-16T10-00-00", "s\n"))
	writeFile(t, filepath.Join(first, "kiwi-pom-update.log"), "first pom update\n")
	writeFile(t, filepath.Join(second, "kiwi-pom-update.log"), "second pom update\n")
	f := outputFile(t)

	if err := showLog(context.Background(), f, "never", logsDir, "2026-01-15T09-30-00", "kiwi-pom-update", false, time.Millisecond); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got := contents(t, f); got != "first pom update\n" {
		t.Errorf("got %q, want the named run's log", got)
	}
}

func TestShowLog_missingLibraryLogListsTheOnesThatExist(t *testing.T) {
	logsDir := t.TempDir()
	runDir := filepath.Dir(writeSessionLog(t, logsDir, "2026-01-15T09-30-00", "s\n"))
	writeFile(t, filepath.Join(runDir, "kiwi.log"), "x\n")
	writeFile(t, filepath.Join(runDir, "kiwi-bom.log"), "x\n")

	err := showLog(context.Background(), outputFile(t), "never", logsDir, "", "nope", false, time.Millisecond)

	if err == nil || !strings.Contains(err.Error(), "no log nope.log in 2026-01-15T09-30-00") ||
		!strings.Contains(err.Error(), "kiwi-bom") || strings.Contains(err.Error(), "session") {
		t.Errorf("expected an error listing the library logs, got %v", err)
	}
}

func TestShowLog_libraryNameMustNotLeaveTheRunDirectory(t *testing.T) {
	logsDir := t.TempDir()
	writeSessionLog(t, logsDir, "2026-01-15T09-30-00", "s\n")

	for _, library := range []string{"../x", "..", "."} {
		t.Run(library, func(t *testing.T) {
			err := showLog(context.Background(), outputFile(t), "never", logsDir, "", library, false, time.Millisecond)

			if err == nil || !strings.Contains(err.Error(), "invalid --library") {
				t.Errorf("expected an invalid library error, got %v", err)
			}
		})
	}
}

func TestShowLog_missingLibraryLogWhenTheRunHasNone(t *testing.T) {
	logsDir := t.TempDir()
	writeSessionLog(t, logsDir, "2026-01-15T09-30-00", "s\n")

	err := showLog(context.Background(), outputFile(t), "never", logsDir, "", "kiwi", false, time.Millisecond)

	if err == nil || !strings.Contains(err.Error(), "it has no library logs") {
		t.Errorf("expected a message that there are no library logs, got %v", err)
	}
}

func TestShowLog_invalidColorModeIsAnErrorWithALibraryToo(t *testing.T) {
	logsDir := t.TempDir()
	writeSessionLog(t, logsDir, "2026-01-15T09-30-00", "s\n")

	err := showLog(context.Background(), outputFile(t), "sometimes", logsDir, "", "kiwi", false, time.Millisecond)

	if err == nil || !strings.Contains(err.Error(), "invalid --color") {
		t.Errorf("expected an invalid mode error, got %v", err)
	}
}

func writeFile(t *testing.T, path, text string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}
