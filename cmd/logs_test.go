package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kiwiproject/kiwi-star-deployer/internal/state"
)

// ---- logs list ----

func TestListLogs_noLogsDir(t *testing.T) {
	out := runListLogs(t, filepath.Join(t.TempDir(), "logs"))
	if !strings.Contains(out, "No release runs found") {
		t.Errorf("expected 'No release runs found', got:\n%s", out)
	}
}

func TestListLogs_emptyLogsDir(t *testing.T) {
	logsDir := makeDir(t, t.TempDir(), "logs")
	out := runListLogs(t, logsDir)
	if !strings.Contains(out, "No release runs found") {
		t.Errorf("expected 'No release runs found', got:\n%s", out)
	}
}

func TestListLogs_onlyFilesInLogsDir(t *testing.T) {
	logsDir := makeDir(t, t.TempDir(), "logs")
	if err := os.WriteFile(filepath.Join(logsDir, "README"), []byte("hi"), 0o644); err != nil {
		t.Fatal(err)
	}
	out := runListLogs(t, logsDir)
	if !strings.Contains(out, "No release runs found") {
		t.Errorf("expected 'No release runs found' when dir has only files, got:\n%s", out)
	}
	if strings.Contains(out, "Run") && strings.Contains(out, "Completed") {
		t.Errorf("header should not appear when there are no run directories, got:\n%s", out)
	}
}

func TestListLogs_successfulRun(t *testing.T) {
	logsDir := t.TempDir()
	runDir := makeDir(t, logsDir, "2026-01-15T09-30-00")
	w := newState(t, filepath.Join(runDir, "state.json"), "2026-01-15T09:30:00Z")
	mustRecord(t, w.RecordCompleted("kiwi-parent", "3.1.0"))
	mustRecord(t, w.RecordCompleted("kiwi", "5.0.0"))

	out := runListLogs(t, logsDir)
	if !strings.Contains(out, "2026-01-15T09:30:00Z") {
		t.Errorf("expected run ID in output, got:\n%s", out)
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) < 2 {
		t.Fatalf("expected at least 2 lines (header + data), got:\n%s", out)
	}
	fields := strings.Fields(lines[1])
	if len(fields) < 2 || fields[1] != "2" {
		t.Errorf("expected completed count 2 in data row, got fields: %v", fields)
	}
	if !strings.Contains(out, "ok") {
		t.Errorf("expected 'ok' status, got:\n%s", out)
	}
}

func TestListLogs_failedRun(t *testing.T) {
	logsDir := t.TempDir()
	runDir := makeDir(t, logsDir, "2026-01-15T10-00-00")
	w := newState(t, filepath.Join(runDir, "state.json"), "2026-01-15T10:00:00Z")
	mustRecord(t, w.RecordCompleted("kiwi-parent", "3.1.0"))
	mustRecord(t, w.RecordFailed("kiwi", state.StepMavenRelease, "exit status 1"))

	out := runListLogs(t, logsDir)
	if !strings.Contains(out, "failed: kiwi (maven-release)") {
		t.Errorf("expected failure status, got:\n%s", out)
	}
}

func TestListLogs_newestFirst(t *testing.T) {
	logsDir := t.TempDir()
	for _, tc := range []struct{ dir, runID string }{
		{"2026-01-15T09-00-00", "2026-01-15T09:00:00Z"},
		{"2026-01-15T10-00-00", "2026-01-15T10:00:00Z"},
	} {
		dir := makeDir(t, logsDir, tc.dir)
		w := newState(t, filepath.Join(dir, "state.json"), tc.runID)
		mustRecord(t, w.RecordCompleted("kiwi", "1.0.0"))
	}

	out := runListLogs(t, logsDir)
	idx09 := strings.Index(out, "09:00:00")
	idx10 := strings.Index(out, "10:00:00")
	if idx09 < 0 || idx10 < 0 {
		t.Fatalf("expected both run IDs in output, got:\n%s", out)
	}
	if idx10 > idx09 {
		t.Errorf("expected newest run (10:00) before older run (09:00), got:\n%s", out)
	}
}

func TestListLogs_unreadableStateFile(t *testing.T) {
	logsDir := t.TempDir()
	makeDir(t, logsDir, "2026-01-15T09-30-00") // no state.json

	out := runListLogs(t, logsDir)
	if !strings.Contains(out, "unreadable:") {
		t.Errorf("expected 'unreadable:' with error detail, got:\n%s", out)
	}
	if !strings.Contains(out, "2026-01-15T09-30-00") {
		t.Errorf("expected directory name in output, got:\n%s", out)
	}
}

// ---- logs purge ----

func TestPurgeLogs_noLogsDir(t *testing.T) {
	out := runPurgeLogs(t, filepath.Join(t.TempDir(), "logs"), 30*24*time.Hour, true, "")
	if !strings.Contains(out, "No release runs found") {
		t.Errorf("expected 'No release runs found', got:\n%s", out)
	}
}

func TestPurgeLogs_nothingOldEnough(t *testing.T) {
	logsDir := t.TempDir()
	// Directory named with a future timestamp — will never be older than 30d.
	makeDir(t, logsDir, "2099-01-01T00-00-00")

	out := runPurgeLogs(t, logsDir, 30*24*time.Hour, true, "")
	if !strings.Contains(out, "No run directories older than") {
		t.Errorf("expected 'No run directories older than', got:\n%s", out)
	}
}

func TestPurgeLogs_deletesOldDirectories(t *testing.T) {
	logsDir := t.TempDir()
	old := makeDir(t, logsDir, "2020-01-01T00-00-00")
	recent := makeDir(t, logsDir, "2099-01-01T00-00-00")

	out := runPurgeLogs(t, logsDir, 30*24*time.Hour, true, "")

	if _, err := os.Stat(old); !os.IsNotExist(err) {
		t.Errorf("expected old directory to be deleted")
	}
	if _, err := os.Stat(recent); err != nil {
		t.Errorf("expected recent directory to be kept, got error: %v", err)
	}
	if !strings.Contains(out, "Deleted 1 directory") {
		t.Errorf("expected deletion confirmation in output, got:\n%s", out)
	}
}

func TestPurgeLogs_promptConfirmed(t *testing.T) {
	logsDir := t.TempDir()
	dir := makeDir(t, logsDir, "2020-01-01T00-00-00")

	runPurgeLogs(t, logsDir, 30*24*time.Hour, false, "y\n")

	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Errorf("expected directory to be deleted after confirmation")
	}
}

func TestPurgeLogs_promptAborted(t *testing.T) {
	logsDir := t.TempDir()
	dir := makeDir(t, logsDir, "2020-01-01T00-00-00")

	out := runPurgeLogs(t, logsDir, 30*24*time.Hour, false, "n\n")

	if _, err := os.Stat(dir); err != nil {
		t.Errorf("expected directory to be kept after abort, got error: %v", err)
	}
	if !strings.Contains(out, "Aborted") {
		t.Errorf("expected 'Aborted' in output, got:\n%s", out)
	}
}

func TestPurgeLogs_skipsNonRunDirectories(t *testing.T) {
	logsDir := t.TempDir()
	makeDir(t, logsDir, "not-a-timestamp")

	out := runPurgeLogs(t, logsDir, 30*24*time.Hour, true, "")
	if !strings.Contains(out, "skipping not-a-timestamp") {
		t.Errorf("expected skip message, got:\n%s", out)
	}
}

// ---- parseAge ----

func TestParseAge(t *testing.T) {
	tests := []struct {
		input   string
		want    time.Duration
		wantErr bool
	}{
		{"30d", 30 * 24 * time.Hour, false},
		{"1d", 24 * time.Hour, false},
		{"24h", 24 * time.Hour, false},
		{"0d", 0, true},
		{"-1d", 0, true},
		{"0h", 0, true},
		{"-24h", 0, true},
		{"bad", 0, true},
		{"", 0, true},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := parseAge(tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("parseAge(%q) error = %v, wantErr = %v", tt.input, err, tt.wantErr)
			}
			if !tt.wantErr && got != tt.want {
				t.Errorf("parseAge(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

// ---- helpers ----

func runListLogs(t *testing.T, logsDir string) string {
	t.Helper()
	var buf bytes.Buffer
	if err := listLogs(&buf, logsDir); err != nil {
		t.Fatalf("listLogs: %v", err)
	}
	return buf.String()
}

func runPurgeLogs(t *testing.T, logsDir string, age time.Duration, yes bool, input string) string {
	t.Helper()
	var buf bytes.Buffer
	r := strings.NewReader(input)
	if err := purgeLogs(&buf, r, logsDir, age, yes); err != nil {
		t.Fatalf("purgeLogs: %v", err)
	}
	return buf.String()
}

func makeDir(t *testing.T, parent, name string) string {
	t.Helper()
	dir := filepath.Join(parent, name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	return dir
}

func newState(t *testing.T, path, runID string) *state.Writer {
	t.Helper()
	w, err := state.New(path, runID)
	if err != nil {
		t.Fatalf("state.New: %v", err)
	}
	return w
}

func mustRecord(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}
