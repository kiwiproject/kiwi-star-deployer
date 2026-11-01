package cmd

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kiwiproject/kiwi-star-deployer/internal/runner"
)

func outputFile(t *testing.T) *os.File {
	t.Helper()
	f, err := os.Create(filepath.Join(t.TempDir(), "out.txt"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = f.Close() })
	return f
}

func contents(t *testing.T, f *os.File) string {
	t.Helper()
	data, err := os.ReadFile(f.Name())
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestConsoleOutput_invalidMode(t *testing.T) {
	if _, _, err := consoleOutput(outputFile(t), "sometimes"); err == nil {
		t.Fatal("expected an error for an unknown --color value")
	}
}

func TestConsoleOutput_neverLeavesTextPlain(t *testing.T) {
	f := outputFile(t)
	out, flush, err := consoleOutput(f, "never")
	if err != nil {
		t.Fatal(err)
	}
	fmt.Fprintln(out, "  done    kiwi")
	flush()

	if got := contents(t, f); got != "  done    kiwi\n" {
		t.Errorf("got %q, want plain text", got)
	}
}

func TestConsoleOutput_autoOnAFileLeavesTextPlain(t *testing.T) {
	f := outputFile(t)
	out, flush, err := consoleOutput(f, "auto")
	if err != nil {
		t.Fatal(err)
	}
	fmt.Fprintln(out, "  done    kiwi")
	flush()

	if got := contents(t, f); got != "  done    kiwi\n" {
		t.Errorf("got %q, want plain text because a file is not a terminal", got)
	}
}

func TestConsoleOutput_alwaysColorsAndFlushWritesTheLastLine(t *testing.T) {
	f := outputFile(t)
	out, flush, err := consoleOutput(f, "always")
	if err != nil {
		t.Fatal(err)
	}
	fmt.Fprintln(out, "  done    kiwi")
	fmt.Fprint(out, "Released 1 library.")
	flush()

	got := contents(t, f)
	if !strings.Contains(got, "\x1b[32m  done    kiwi\x1b[0m\n") {
		t.Errorf("expected the done line in green, got %q", got)
	}
	if !strings.HasSuffix(got, "\x1b[32mReleased 1 library.\x1b[0m") {
		t.Errorf("expected flush to write the last line in green, got %q", got)
	}
}

func TestColorFlagIsOnlyOnTheCommandsThatColor(t *testing.T) {
	for _, c := range rootCmd.Commands() {
		want := c.Name() == "release" || c.Name() == "preflight" || c.Name() == "check-versions"
		if got := c.Flags().Lookup("color") != nil; got != want {
			t.Errorf("%s: has --color = %v, want %v", c.Name(), got, want)
		}
	}
}

// failingRunner makes every command fail, so every check reports [FAIL].
type failingRunner struct{}

func (failingRunner) Run(runner.Options) (*runner.Result, error) {
	return nil, errors.New("exit status 1")
}

func TestReportPreflight_colorsOnlyWhenAsked(t *testing.T) {
	tests := []struct {
		mode      string
		wantColor bool
	}{
		{"always", true},
		{"never", false},
	}
	for _, tt := range tests {
		t.Run(tt.mode, func(t *testing.T) {
			f := outputFile(t)

			err := reportPreflight(f, tt.mode, failingRunner{}, "no-such-changelog-script")

			if err == nil || !strings.Contains(err.Error(), "preflight failed") {
				t.Errorf("expected preflight to fail, got %v", err)
			}
			got := contents(t, f)
			if !strings.Contains(got, "[FAIL]") {
				t.Errorf("expected failed checks in:\n%s", got)
			}
			if colored := strings.Contains(got, "\x1b[31m[FAIL]"); colored != tt.wantColor {
				t.Errorf("colored = %v, want %v in %q", colored, tt.wantColor, got)
			}
		})
	}
}

func TestReportPreflight_invalidColorMode(t *testing.T) {
	err := reportPreflight(outputFile(t), "sometimes", failingRunner{}, "x")
	if err == nil || !strings.Contains(err.Error(), "invalid --color") {
		t.Errorf("expected an invalid mode error, got %v", err)
	}
}

func TestReportVersions_colorsOnlyWhenAsked(t *testing.T) {
	tests := []struct {
		mode      string
		wantColor bool
	}{
		{"always", true},
		{"never", false},
	}
	for _, tt := range tests {
		t.Run(tt.mode, func(t *testing.T) {
			f := outputFile(t)
			var progress bytes.Buffer

			err := reportVersions(f, &progress, tt.mode, failingRunner{}, changesLibs())

			if err == nil || !strings.Contains(err.Error(), "version check failed") {
				t.Errorf("expected the version check to fail, got %v", err)
			}
			if !strings.Contains(progress.String(), "Checking 1 library against GitHub milestones") {
				t.Errorf("unexpected progress output: %q", progress.String())
			}
			got := contents(t, f)
			if colored := strings.Contains(got, "\x1b[31m[FAIL]"); colored != tt.wantColor {
				t.Errorf("colored = %v, want %v in %q", colored, tt.wantColor, got)
			}
		})
	}
}

func TestReportVersions_invalidColorMode(t *testing.T) {
	err := reportVersions(outputFile(t), io.Discard, "sometimes", failingRunner{}, changesLibs())
	if err == nil || !strings.Contains(err.Error(), "invalid --color") {
		t.Errorf("expected an invalid mode error, got %v", err)
	}
}
