package state

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"text/tabwriter"
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

// Print writes a human-readable summary of s to w.
func Print(w io.Writer, s *State) {
	fmt.Fprintf(w, "Run: %s\n\nCompleted:\n", s.RunID)
	if len(s.Completed) == 0 {
		fmt.Fprintf(w, "  (none)\n")
	} else {
		tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
		for _, e := range s.Completed {
			fmt.Fprintf(tw, "  %s\t%s\t%s\n", e.Library, e.Version, e.CompletedAt.Format(time.RFC3339))
		}
		_ = tw.Flush()
	}
	if s.Failed != nil {
		fmt.Fprintf(w, "\nFailed:\n")
		fmt.Fprintf(w, "  library:  %s\n", s.Failed.Library)
		fmt.Fprintf(w, "  step:     %s\n", s.Failed.Step)
		fmt.Fprintf(w, "  error:    %s\n", s.Failed.Error)
	}
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
	// Write to a temp file in the same directory then rename atomically so a
	// concurrent status read always sees a complete JSON file, never a partial one.
	tmp, err := os.CreateTemp(filepath.Dir(w.path), "state-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
		return err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return err
	}
	return os.Rename(tmpName, w.path)
}
