package state

import (
	"encoding/json"
	"os"
	"sync"
	"time"
)

const (
	StepMavenRelease  = "maven-release"
	StepCentralVerify = "maven-central-verify"
	StepChangelog     = "changelog"
	StepPOMUpdate     = "pom-update"
)

type State struct {
	RunID     string           `json:"run_id"`
	Completed []CompletedEntry `json:"completed"`
	Failed    *FailedEntry     `json:"failed,omitempty"`
}

type CompletedEntry struct {
	Library     string    `json:"library"`
	Version     string    `json:"version"`
	CompletedAt time.Time `json:"completed_at"`
}

type FailedEntry struct {
	Library string `json:"library"`
	Step    string `json:"step"`
	Error   string `json:"error"`
}

// Writer writes release state to a JSON file. A nil *Writer is valid and
// silently discards all writes.
type Writer struct {
	path  string
	mu    sync.Mutex
	state State
}

// New creates a Writer and initializes the state file at path with the given
// runID. Any existing file is overwritten.
func New(path, runID string) (*Writer, error) {
	w := &Writer{
		path: path,
		state: State{
			RunID:     runID,
			Completed: []CompletedEntry{},
		},
	}
	return w, w.flush()
}

// RecordCompleted appends a completed entry and flushes to disk. Safe to call
// from concurrent goroutines.
func (w *Writer) RecordCompleted(library, version string) error {
	if w == nil {
		return nil
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	w.state.Completed = append(w.state.Completed, CompletedEntry{
		Library:     library,
		Version:     version,
		CompletedAt: time.Now().UTC(),
	})
	w.state.Failed = nil
	return w.flush()
}

// RecordFailed sets the failed entry and flushes to disk.
func (w *Writer) RecordFailed(library, step, errMsg string) error {
	if w == nil {
		return nil
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	w.state.Failed = &FailedEntry{
		Library: library,
		Step:    step,
		Error:   errMsg,
	}
	return w.flush()
}

// Load reads and parses the state file at path.
func Load(path string) (*State, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var s State
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, err
	}
	return &s, nil
}

func (w *Writer) flush() error {
	data, err := json.MarshalIndent(w.state, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(w.path, data, 0o644)
}
