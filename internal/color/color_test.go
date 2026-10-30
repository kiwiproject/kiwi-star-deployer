package color_test

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/kiwiproject/kiwi-star-deployer/internal/color"
)

const (
	reset    = "\x1b[0m"
	dim      = "\x1b[2m"
	red      = "\x1b[31m"
	green    = "\x1b[32m"
	yellow   = "\x1b[33m"
	boldCyan = "\x1b[1;36m"
)

func render(t *testing.T, input string) string {
	t.Helper()
	var out bytes.Buffer
	w := color.NewWriter(&out)
	if _, err := w.Write([]byte(input)); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if err := w.Flush(); err != nil {
		t.Fatalf("Flush: %v", err)
	}
	return out.String()
}

func TestWriter_ColorsLinesByTheirStart(t *testing.T) {
	tests := []struct {
		name string
		line string
		want string
	}{
		{"stage header", "Stage 2: releasing kiwi-bom", boldCyan + "Stage 2: releasing kiwi-bom" + reset},
		{"done", "  done    kiwi  (log: /x/kiwi.log)", green + "  done    kiwi  (log: /x/kiwi.log)" + reset},
		{"pom update done", "  done   POM update kiwi", green + "  done   POM update kiwi" + reset},
		{"ci passed", "  CI passed kiwi", green + "  CI passed kiwi" + reset},
		{"released", "Released 9 libraries.", green + "Released 9 libraries." + reset},
		{"preflight pass", "[PASS]  gpg signing", green + "[PASS]  gpg signing" + reset},
		{"preflight fail", "[FAIL]  gh auth  not logged in", red + "[FAIL]  gh auth  not logged in" + reset},
		{"failed", "  FAILED  kiwi", red + "  FAILED  kiwi" + reset},
		{"failed pom update", "  FAILED POM update kiwi", red + "  FAILED POM update kiwi" + reset},
		{"error", "  error:  exit status 1", red + "  error:  exit status 1" + reset},
		{"skip", "  skip    kiwi  (version: 1.0.0)", yellow + "  skip    kiwi  (version: 1.0.0)" + reset},
		{"retry", "changelog failed (boom); retrying in 30s (attempt 2 of 3)", yellow + "changelog failed (boom); retrying in 30s (attempt 2 of 3)" + reset},
		{"poll failure", "    CI: poll failed (boom); will retry (failure 1 of 3 allowed in a row)", yellow + "    CI: poll failed (boom); will retry (failure 1 of 3 allowed in a row)" + reset},
		{"polling noise", "    CI: run 123 status: in_progress", dim + "    CI: run 123 status: in_progress" + reset},
		{"date line", "--- 2026-10-23 ---", dim + "--- 2026-10-23 ---" + reset},
		{"could not at the start", "could not read pom.xml: boom", yellow + "could not read pom.xml: boom" + reset},
		{"ci could not", "    CI: could not check workflows for x/y (boom); continuing to wait", yellow + "    CI: could not check workflows for x/y (boom); continuing to wait" + reset},
		{"retry words in the middle of a name", "Logs: /x/could not-retrying in-exist", "Logs: /x/could not-retrying in-exist"},
		{"plain", "Logs: /x/logs", "Logs: /x/logs"},
		{"not a done line", "doneness is not a result", "doneness is not a result"},
		{"not a skip line", "skipper", "skipper"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := render(t, tt.line+"\n"); got != tt.want+"\n" {
				t.Errorf("got %q, want %q", got, tt.want+"\n")
			}
		})
	}
}

func TestWriter_FailureBeatsRetry(t *testing.T) {
	got := render(t, "  FAILED  kiwi (could not sign)\n")
	if want := red + "  FAILED  kiwi (could not sign)" + reset + "\n"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestWriter_DimsTheStampAndColorsTheRest(t *testing.T) {
	got := render(t, "21:40:21  Stage 1: releasing kiwi-parent\n")

	want := dim + "21:40:21" + reset + "  " + boldCyan + "Stage 1: releasing kiwi-parent" + reset + "\n"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestWriter_DimsTheStampOnAnUncoloredLine(t *testing.T) {
	got := render(t, "21:40:21  Logs: /x\n")

	want := dim + "21:40:21" + reset + "  Logs: /x\n"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestWriter_LeavesBlankLinesAlone(t *testing.T) {
	if got := render(t, "\n\n"); got != "\n\n" {
		t.Errorf("got %q, want two bare newlines", got)
	}
}

func TestWriter_HoldsALineBackUntilItIsComplete(t *testing.T) {
	var out bytes.Buffer
	w := color.NewWriter(&out)

	_, _ = w.Write([]byte("  do"))
	_, _ = w.Write([]byte("ne    kiwi"))
	if out.Len() != 0 {
		t.Fatalf("wrote %q before the line was complete", out.String())
	}
	_, _ = w.Write([]byte("\nnext line\n"))

	want := green + "  done    kiwi" + reset + "\nnext line\n"
	if out.String() != want {
		t.Errorf("got %q, want %q", out.String(), want)
	}
}

func TestWriter_FlushWritesAnUnfinishedLine(t *testing.T) {
	var out bytes.Buffer
	w := color.NewWriter(&out)

	_, _ = w.Write([]byte("Released 1 library."))
	if out.Len() != 0 {
		t.Fatalf("wrote %q before Flush", out.String())
	}
	if err := w.Flush(); err != nil {
		t.Fatalf("Flush: %v", err)
	}
	if want := green + "Released 1 library." + reset; out.String() != want {
		t.Errorf("got %q, want %q", out.String(), want)
	}

	if err := w.Flush(); err != nil || out.String() != green+"Released 1 library."+reset {
		t.Errorf("a second Flush should write nothing, got %q (err %v)", out.String(), err)
	}
}

func TestWriter_ReportsLengthOfInput(t *testing.T) {
	var out bytes.Buffer
	w := color.NewWriter(&out)

	input := "  done    kiwi\n"
	n, err := w.Write([]byte(input))
	if err != nil || n != len(input) {
		t.Errorf("Write = (%d, %v), want (%d, nil)", n, err, len(input))
	}
}

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, errors.New("broken pipe") }

func TestWriter_ReturnsUnderlyingError(t *testing.T) {
	w := color.NewWriter(failingWriter{})

	if _, err := w.Write([]byte("x\n")); err == nil {
		t.Error("expected an error from Write")
	}

	w = color.NewWriter(failingWriter{})
	_, _ = w.Write([]byte("no newline"))
	if err := w.Flush(); err == nil {
		t.Error("expected an error from Flush")
	}
}

func TestWriter_FailedWriteKeepsWhatWasHeldAndLetsTheCallerRetry(t *testing.T) {
	fw := &flakyWriter{fails: true}
	w := color.NewWriter(fw)

	_, _ = w.Write([]byte("  do"))
	if _, err := w.Write([]byte("ne    kiwi\n")); err == nil {
		t.Fatal("expected the failed write to be reported")
	}
	fw.fails = false
	if _, err := w.Write([]byte("ne    kiwi\n")); err != nil {
		t.Fatalf("retry: %v", err)
	}

	if want := green + "  done    kiwi" + reset + "\n"; fw.out.String() != want {
		t.Errorf("got %q, want %q", fw.out.String(), want)
	}
}

func TestWriter_FailedFlushKeepsTheLine(t *testing.T) {
	fw := &flakyWriter{fails: true}
	w := color.NewWriter(fw)

	_, _ = w.Write([]byte("Released 1 library."))
	if err := w.Flush(); err == nil {
		t.Fatal("expected the failed flush to be reported")
	}
	fw.fails = false
	if err := w.Flush(); err != nil {
		t.Fatalf("second flush: %v", err)
	}

	if want := green + "Released 1 library." + reset; fw.out.String() != want {
		t.Errorf("got %q, want %q", fw.out.String(), want)
	}
}

type flakyWriter struct {
	out   bytes.Buffer
	fails bool
}

func (f *flakyWriter) Write(p []byte) (int, error) {
	if f.fails {
		return 0, errors.New("broken pipe")
	}
	return f.out.Write(p)
}

func TestWriter_ConcurrentWritesKeepLinesWhole(t *testing.T) {
	var out bytes.Buffer
	w := color.NewWriter(&out)

	done := make(chan struct{})
	for i := range 4 {
		go func() {
			for j := range 50 {
				fmt.Fprintf(w, "  done    lib-%d-%d\n", i, j)
			}
			done <- struct{}{}
		}()
	}
	for range 4 {
		<-done
	}

	lines := bytes.Split(bytes.TrimSuffix(out.Bytes(), []byte("\n")), []byte("\n"))
	if len(lines) != 200 {
		t.Fatalf("got %d lines, want 200", len(lines))
	}
	for _, line := range lines {
		if !bytes.HasPrefix(line, []byte(green+"  done    lib-")) || !bytes.HasSuffix(line, []byte(reset)) {
			t.Fatalf("mixed up line: %q", line)
		}
	}
}

func TestParseMode(t *testing.T) {
	for _, s := range []string{"auto", "always", "never"} {
		m, err := color.ParseMode(s)
		if err != nil || string(m) != s {
			t.Errorf("ParseMode(%q) = (%q, %v)", s, m, err)
		}
	}
	if _, err := color.ParseMode("sometimes"); err == nil {
		t.Error("expected an error for an unknown mode")
	}
}

func env(vars map[string]string) func(string) string {
	return func(key string) string { return vars[key] }
}

func TestEnabled(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses /dev/null as a stand-in for a terminal")
	}
	terminal, err := os.Open(os.DevNull) // a character device, like a terminal
	if err != nil {
		t.Fatalf("opening %s: %v", os.DevNull, err)
	}
	t.Cleanup(func() { _ = terminal.Close() })

	file, err := os.Create(filepath.Join(t.TempDir(), "out.txt"))
	if err != nil {
		t.Fatalf("creating file: %v", err)
	}
	t.Cleanup(func() { _ = file.Close() })

	tests := []struct {
		name string
		mode color.Mode
		f    *os.File
		env  map[string]string
		want bool
	}{
		{"auto on a terminal", color.Auto, terminal, nil, true},
		{"auto on a file", color.Auto, file, nil, false},
		{"auto honors NO_COLOR", color.Auto, terminal, map[string]string{"NO_COLOR": "1"}, false},
		{"auto ignores an empty NO_COLOR", color.Auto, terminal, map[string]string{"NO_COLOR": ""}, true},
		{"auto on a dumb terminal", color.Auto, terminal, map[string]string{"TERM": "dumb"}, false},
		{"always on a file", color.Always, file, nil, true},
		{"always overrides NO_COLOR", color.Always, terminal, map[string]string{"NO_COLOR": "1"}, true},
		{"never on a terminal", color.Never, terminal, nil, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := color.Enabled(tt.mode, tt.f, env(tt.env)); got != tt.want {
				t.Errorf("Enabled = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestEnabled_AutoOnAClosedFile(t *testing.T) {
	f, err := os.Create(filepath.Join(t.TempDir(), "out.txt"))
	if err != nil {
		t.Fatalf("creating file: %v", err)
	}
	_ = f.Close()

	if color.Enabled(color.Auto, f, env(nil)) {
		t.Error("a file that cannot be examined should not get color")
	}
}
