package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kiwiproject/kiwi-star-deployer/internal/state"
)

func TestListLogs_noLogsDir(t *testing.T) {
	base := t.TempDir()
	out := runListLogs(t, filepath.Join(base, "logs"))
	if !strings.Contains(out, "No release runs found") {
		t.Errorf("expected 'No release runs found', got:\n%s", out)
	}
}

func TestListLogs_emptyLogsDir(t *testing.T) {
	logsDir := filepath.Join(t.TempDir(), "logs")
	if err := os.MkdirAll(logsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	out := runListLogs(t, logsDir)
	if !strings.Contains(out, "No release runs found") {
		t.Errorf("expected 'No release runs found', got:\n%s", out)
	}
}

func TestListLogs_successfulRun(t *testing.T) {
	logsDir := filepath.Join(t.TempDir(), "logs")
	runDir := filepath.Join(logsDir, "2026-01-15T09-30-00")
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		t.Fatal(err)
	}

	w, err := state.New(filepath.Join(runDir, "state.json"), "2026-01-15T09:30:00Z")
	if err != nil {
		t.Fatalf("new state: %v", err)
	}
	if err := w.RecordCompleted("kiwi-parent", "3.1.0"); err != nil {
		t.Fatal(err)
	}
	if err := w.RecordCompleted("kiwi", "5.0.0"); err != nil {
		t.Fatal(err)
	}

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
	logsDir := filepath.Join(t.TempDir(), "logs")
	runDir := filepath.Join(logsDir, "2026-01-15T10-00-00")
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		t.Fatal(err)
	}

	w, err := state.New(filepath.Join(runDir, "state.json"), "2026-01-15T10:00:00Z")
	if err != nil {
		t.Fatalf("new state: %v", err)
	}
	if err := w.RecordCompleted("kiwi-parent", "3.1.0"); err != nil {
		t.Fatal(err)
	}
	if err := w.RecordFailed("kiwi", state.StepMavenRelease, "exit status 1"); err != nil {
		t.Fatal(err)
	}

	out := runListLogs(t, logsDir)
	if !strings.Contains(out, "failed: kiwi (maven-release)") {
		t.Errorf("expected failure status, got:\n%s", out)
	}
}

func TestListLogs_newestFirst(t *testing.T) {
	logsDir := filepath.Join(t.TempDir(), "logs")

	for _, tc := range []struct{ dir, runID string }{
		{"2026-01-15T09-00-00", "2026-01-15T09:00:00Z"},
		{"2026-01-15T10-00-00", "2026-01-15T10:00:00Z"},
	} {
		dir := filepath.Join(logsDir, tc.dir)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		w, err := state.New(filepath.Join(dir, "state.json"), tc.runID)
		if err != nil {
			t.Fatalf("new state: %v", err)
		}
		if err := w.RecordCompleted("kiwi", "1.0.0"); err != nil {
			t.Fatal(err)
		}
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
	logsDir := filepath.Join(t.TempDir(), "logs")
	runDir := filepath.Join(logsDir, "2026-01-15T09-30-00")
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// No state.json written — Load will fail.

	out := runListLogs(t, logsDir)
	if !strings.Contains(out, "unreadable:") {
		t.Errorf("expected 'unreadable:' with error detail for missing state.json, got:\n%s", out)
	}
	if !strings.Contains(out, "2026-01-15T09-30-00") {
		t.Errorf("expected directory name in output, got:\n%s", out)
	}
}

func runListLogs(t *testing.T, logsDir string) string {
	t.Helper()
	var buf bytes.Buffer
	if err := listLogs(&buf, logsDir); err != nil {
		t.Fatalf("listLogs: %v", err)
	}
	return buf.String()
}
