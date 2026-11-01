package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
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
