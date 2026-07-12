package state_test

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kiwiproject/kiwi-star-deployer/internal/state"
)

func TestNew_createsFileWithRunID(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	w, err := state.New(path, "2025-11-15T14:30:00Z")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if w == nil {
		t.Fatal("expected non-nil writer")
	}

	s, err := state.Load(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if s.RunID != "2025-11-15T14:30:00Z" {
		t.Errorf("run_id: got %q, want 2025-11-15T14:30:00Z", s.RunID)
	}
	if len(s.Completed) != 0 {
		t.Errorf("completed: got %d, want 0", len(s.Completed))
	}
	if s.Failed != nil {
		t.Errorf("failed: got %v, want nil", s.Failed)
	}
}

func TestWriter_RecordCompleted(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	w, err := state.New(path, "run-1")
	if err != nil {
		t.Fatalf("new: %v", err)
	}

	if err := w.RecordCompleted("kiwi-parent", "3.0.0"); err != nil {
		t.Fatalf("record: %v", err)
	}
	if err := w.RecordCompleted("kiwi", "5.0.0"); err != nil {
		t.Fatalf("record: %v", err)
	}

	s, err := state.Load(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(s.Completed) != 2 {
		t.Fatalf("completed count: got %d, want 2", len(s.Completed))
	}
	if s.Completed[0].Library != "kiwi-parent" || s.Completed[0].Version != "3.0.0" {
		t.Errorf("first entry: got %+v", s.Completed[0])
	}
	if s.Completed[1].Library != "kiwi" || s.Completed[1].Version != "5.0.0" {
		t.Errorf("second entry: got %+v", s.Completed[1])
	}
	if s.Completed[0].CompletedAt.IsZero() {
		t.Error("completed_at should not be zero")
	}
}

func TestWriter_RecordFailed(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	w, err := state.New(path, "run-1")
	if err != nil {
		t.Fatalf("new: %v", err)
	}

	if err := w.RecordCompleted("kiwi-parent", "3.0.0"); err != nil {
		t.Fatalf("record completed: %v", err)
	}
	if err := w.RecordFailed("kiwi", state.StepMavenRelease, "exit status 1"); err != nil {
		t.Fatalf("record failed: %v", err)
	}

	s, err := state.Load(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(s.Completed) != 1 {
		t.Errorf("completed count: got %d, want 1", len(s.Completed))
	}
	if s.Failed == nil {
		t.Fatal("failed: got nil, want non-nil")
	}
	if s.Failed.Library != "kiwi" {
		t.Errorf("failed library: got %q, want kiwi", s.Failed.Library)
	}
	if s.Failed.Step != state.StepMavenRelease {
		t.Errorf("failed step: got %q, want %s", s.Failed.Step, state.StepMavenRelease)
	}
	if s.Failed.Error != "exit status 1" {
		t.Errorf("failed error: got %q, want exit status 1", s.Failed.Error)
	}
}

func TestWriter_RecordCompleted_clearsFailed(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	w, err := state.New(path, "run-1")
	if err != nil {
		t.Fatalf("new: %v", err)
	}

	_ = w.RecordFailed("kiwi", state.StepMavenRelease, "some error")
	_ = w.RecordCompleted("kiwi", "5.0.0")

	s, err := state.Load(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if s.Failed != nil {
		t.Errorf("failed should be cleared after RecordCompleted, got %+v", s.Failed)
	}
}

func TestWriter_RecordCompleted_concurrentSafe(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	w, err := state.New(path, "run-1")
	if err != nil {
		t.Fatalf("new: %v", err)
	}

	done := make(chan struct{})
	for i := 0; i < 10; i++ {
		go func(i int) {
			_ = w.RecordCompleted("lib", "1.0.0")
			done <- struct{}{}
		}(i)
	}
	for i := 0; i < 10; i++ {
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Fatal("timed out waiting for goroutines")
		}
	}
}

func TestWriter_Archive_writesStateToDestDir(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	w, err := state.New(path, "run-1")
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	_ = w.RecordCompleted("kiwi-parent", "3.1.0")

	destDir := t.TempDir()
	if err := w.Archive(destDir); err != nil {
		t.Fatalf("Archive: %v", err)
	}

	s, err := state.Load(filepath.Join(destDir, "state.json"))
	if err != nil {
		t.Fatalf("load archived state: %v", err)
	}
	if s.RunID != "run-1" {
		t.Errorf("run_id: got %q, want run-1", s.RunID)
	}
	if len(s.Completed) != 1 || s.Completed[0].Library != "kiwi-parent" {
		t.Errorf("completed: got %v, want kiwi-parent", s.Completed)
	}
}

func TestWriter_Archive_nilSafe(t *testing.T) {
	var w *state.Writer
	if err := w.Archive(t.TempDir()); err != nil {
		t.Errorf("nil Archive should not error: %v", err)
	}
}

func TestWriter_SetLogDir(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	w, err := state.New(path, "run-1")
	if err != nil {
		t.Fatalf("new: %v", err)
	}

	if err := w.SetLogDir("/logs/2025-11-15T14-30-00"); err != nil {
		t.Fatalf("SetLogDir: %v", err)
	}

	s, err := state.Load(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if s.LogDir != "/logs/2025-11-15T14-30-00" {
		t.Errorf("log_dir: got %q, want /logs/2025-11-15T14-30-00", s.LogDir)
	}
	if got := w.LogDir(); got != "/logs/2025-11-15T14-30-00" {
		t.Errorf("LogDir(): got %q, want /logs/2025-11-15T14-30-00", got)
	}
}

func TestWriter_LogDir_emptyBeforeSet(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	w, err := state.New(path, "run-1")
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	if got := w.LogDir(); got != "" {
		t.Errorf("LogDir(): got %q, want empty string before SetLogDir is called", got)
	}
}

func TestWriter_LogDir_nilSafe(t *testing.T) {
	var w *state.Writer
	if got := w.LogDir(); got != "" {
		t.Errorf("LogDir() on nil Writer: got %q, want empty string", got)
	}
}

func TestResume_preservesRunIDAndCompletedAndLogDir(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	orig, err := state.New(path, "2025-11-15T14:30:00Z")
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	_ = orig.SetLogDir("/logs/2025-11-15T14-30-00")
	_ = orig.RecordCompleted("kiwi-parent", "3.0.0")
	_ = orig.RecordFailed("kiwi", state.StepMavenRelease, "exit status 1")

	w, err := state.Resume(path, func() *state.State {
		s, _ := state.Load(path)
		return s
	}())
	if err != nil {
		t.Fatalf("resume: %v", err)
	}
	_ = w

	s, err := state.Load(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if s.RunID != "2025-11-15T14:30:00Z" {
		t.Errorf("run_id: got %q, want original", s.RunID)
	}
	if s.LogDir != "/logs/2025-11-15T14-30-00" {
		t.Errorf("log_dir: got %q, want original", s.LogDir)
	}
	if len(s.Completed) != 1 || s.Completed[0].Library != "kiwi-parent" {
		t.Errorf("completed: got %v, want kiwi-parent", s.Completed)
	}
	if s.Failed != nil {
		t.Errorf("failed: got %v, want nil (should be cleared on resume)", s.Failed)
	}
}

func TestResume_withNoLogDir(t *testing.T) {
	// A state file from before log_dir was introduced has an empty LogDir.
	// Resume should copy it as empty without error.
	path := filepath.Join(t.TempDir(), "state.json")
	orig, err := state.New(path, "old-run")
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	_ = orig.RecordCompleted("kiwi", "1.0.0")

	prev, err := state.Load(path)
	if err != nil {
		t.Fatalf("load prev: %v", err)
	}
	w, err := state.Resume(path, prev)
	if err != nil {
		t.Fatalf("resume: %v", err)
	}
	_ = w

	s, err := state.Load(path)
	if err != nil {
		t.Fatalf("load after resume: %v", err)
	}
	if s.LogDir != "" {
		t.Errorf("log_dir: got %q, want empty", s.LogDir)
	}
	if s.RunID != "old-run" {
		t.Errorf("run_id: got %q, want old-run", s.RunID)
	}
}

func TestWriter_nilSafe(t *testing.T) {
	var w *state.Writer
	if err := w.RecordCompleted("kiwi", "1.0.0"); err != nil {
		t.Errorf("nil RecordCompleted should not error: %v", err)
	}
	if err := w.RecordFailed("kiwi", state.StepMavenRelease, "err"); err != nil {
		t.Errorf("nil RecordFailed should not error: %v", err)
	}
	if err := w.SetLogDir("/logs/run"); err != nil {
		t.Errorf("nil SetLogDir should not error: %v", err)
	}
}

func TestLoad_fileNotFound(t *testing.T) {
	_, err := state.Load(filepath.Join(t.TempDir(), "nonexistent.json"))
	if err == nil {
		t.Error("expected error for missing file, got nil")
	}
}

func TestPrint_withCompletedAndFailed(t *testing.T) {
	s := &state.State{
		RunID: "2025-11-15T14:30:00Z",
		Completed: []state.CompletedEntry{
			{Library: "kiwi-parent", Version: "3.1.0", CompletedAt: time.Date(2025, 11, 15, 14, 31, 22, 0, time.UTC)},
			{Library: "kiwi-bom", Version: "2.4.0", CompletedAt: time.Date(2025, 11, 15, 14, 45, 10, 0, time.UTC)},
		},
		Failed: &state.FailedEntry{
			Library: "kiwi",
			Step:    state.StepMavenRelease,
			Error:   "exit status 1",
		},
	}

	var buf bytes.Buffer
	state.Print(&buf, s)
	out := buf.String()

	for _, want := range []string{
		"Run: 2025-11-15T14:30:00Z",
		"Completed:",
		"kiwi-parent", "3.1.0",
		"kiwi-bom", "2.4.0",
		"Failed:",
		"library:  kiwi",
		"step:     " + state.StepMavenRelease,
		"error:    exit status 1",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("expected %q in output:\n%s", want, out)
		}
	}
}

func TestPrint_withNoCompleted(t *testing.T) {
	s := &state.State{
		RunID:     "2025-11-15T14:30:00Z",
		Completed: []state.CompletedEntry{},
	}

	var buf bytes.Buffer
	state.Print(&buf, s)
	out := buf.String()

	if !strings.Contains(out, "(none)") {
		t.Errorf("expected '(none)' in output:\n%s", out)
	}
	if strings.Contains(out, "Failed:") {
		t.Errorf("unexpected 'Failed:' section in output:\n%s", out)
	}
}

func TestPrint_withNoFailed(t *testing.T) {
	s := &state.State{
		RunID: "2025-11-15T14:30:00Z",
		Completed: []state.CompletedEntry{
			{Library: "kiwi-parent", Version: "3.1.0", CompletedAt: time.Date(2025, 11, 15, 14, 31, 22, 0, time.UTC)},
		},
	}

	var buf bytes.Buffer
	state.Print(&buf, s)
	out := buf.String()

	if !strings.Contains(out, "kiwi-parent") {
		t.Errorf("expected kiwi-parent in output:\n%s", out)
	}
	if strings.Contains(out, "Failed:") {
		t.Errorf("unexpected 'Failed:' section when no failure:\n%s", out)
	}
}
