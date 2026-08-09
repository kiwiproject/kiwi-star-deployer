package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kiwiproject/kiwi-star-deployer/internal/config"
	"github.com/kiwiproject/kiwi-star-deployer/internal/plan"
	ver "github.com/kiwiproject/kiwi-star-deployer/internal/version"
)

var testLibs = map[string]config.Library{
	"kiwi":        {Repo: "kiwiproject/kiwi"},
	"kiwi-parent": {Repo: "kiwiproject/kiwi-parent"},
}

func TestValidateReleaseFlags(t *testing.T) {
	tests := []struct {
		name       string
		resume     bool
		noAutoSkip bool
		skipLibs   []string
		wantErr    string
	}{
		{name: "no flags"},
		{name: "resume alone", resume: true},
		{name: "no-auto-skip alone", noAutoSkip: true},
		{name: "resume with skip", resume: true, skipLibs: []string{"kiwi"}},
		{
			name:     "skip without resume",
			skipLibs: []string{"kiwi"},
			wantErr:  "--skip requires --resume",
		},
		{
			name:       "resume with no-auto-skip",
			resume:     true,
			noAutoSkip: true,
			wantErr:    "--no-auto-skip cannot be combined with --resume",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateReleaseFlags(tt.resume, tt.noAutoSkip, tt.skipLibs)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("expected %q in error, got: %v", tt.wantErr, err)
			}
		})
	}
}

func TestParseSummaryFlags(t *testing.T) {
	tests := []struct {
		name    string
		flags   []string
		wantErr string
		wantMap map[string]string
	}{
		{
			name:    "nil input returns nil",
			flags:   nil,
			wantMap: nil,
		},
		{
			name:    "empty input returns nil",
			flags:   []string{},
			wantMap: nil,
		},
		{
			name:    "single entry",
			flags:   []string{"kiwi=Breaking changes."},
			wantMap: map[string]string{"kiwi": "Breaking changes."},
		},
		{
			name:  "multiple entries",
			flags: []string{"kiwi=Summary A", "kiwi-parent=Summary B"},
			wantMap: map[string]string{
				"kiwi":        "Summary A",
				"kiwi-parent": "Summary B",
			},
		},
		{
			name:    "value contains equals sign — splits on first only",
			flags:   []string{"kiwi=see foo=bar"},
			wantMap: map[string]string{"kiwi": "see foo=bar"},
		},
		{
			name:    "no equals sign",
			flags:   []string{"kiwi"},
			wantErr: "expected libname=text",
		},
		{
			name:    "empty library name",
			flags:   []string{"=some text"},
			wantErr: "missing library name",
		},
		{
			name:    "unknown library",
			flags:   []string{"nonexistent=text"},
			wantErr: "unknown library",
		},
		{
			name:    "empty summary text",
			flags:   []string{"kiwi="},
			wantErr: "empty summary text",
		},
		{
			name:    "duplicate library",
			flags:   []string{"kiwi=first", "kiwi=second"},
			wantErr: "specified more than once",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseSummaryFlags(tt.flags, testLibs)
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tt.wantErr)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Errorf("expected error containing %q, got: %v", tt.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(got) != len(tt.wantMap) {
				t.Fatalf("map length: got %d, want %d", len(got), len(tt.wantMap))
			}
			for k, v := range tt.wantMap {
				if got[k] != v {
					t.Errorf("key %q: got %q, want %q", k, got[k], v)
				}
			}
		})
	}
}

func TestParseSummaryFileFlags(t *testing.T) {
	dir := t.TempDir()
	existingFile := filepath.Join(dir, "summary.txt")
	if err := os.WriteFile(existingFile, []byte("content"), 0o644); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name    string
		flags   []string
		wantErr string
		wantMap map[string]string
	}{
		{
			name:    "nil input returns nil",
			flags:   nil,
			wantMap: nil,
		},
		{
			name:    "empty input returns nil",
			flags:   []string{},
			wantMap: nil,
		},
		{
			name:    "single entry with existing file",
			flags:   []string{"kiwi=" + existingFile},
			wantMap: map[string]string{"kiwi": existingFile},
		},
		{
			name:    "no equals sign",
			flags:   []string{"kiwi"},
			wantErr: "expected libname=/path",
		},
		{
			name:    "empty library name",
			flags:   []string{"=" + existingFile},
			wantErr: "missing library name",
		},
		{
			name:    "unknown library",
			flags:   []string{"nonexistent=" + existingFile},
			wantErr: "unknown library",
		},
		{
			name:    "empty path",
			flags:   []string{"kiwi="},
			wantErr: "empty path",
		},
		{
			name:    "duplicate library",
			flags:   []string{"kiwi=" + existingFile, "kiwi=" + existingFile},
			wantErr: "specified more than once",
		},
		{
			name:    "file does not exist",
			flags:   []string{"kiwi=/nonexistent/path/summary.txt"},
			wantErr: "no such file",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseSummaryFileFlags(tt.flags, testLibs)
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tt.wantErr)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Errorf("expected error containing %q, got: %v", tt.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(got) != len(tt.wantMap) {
				t.Fatalf("map length: got %d, want %d", len(got), len(tt.wantMap))
			}
			for k, v := range tt.wantMap {
				if got[k] != v {
					t.Errorf("key %q: got %q, want %q", k, got[k], v)
				}
			}
		})
	}
}

func mustPlanEntry(name string) plan.Entry {
	vp, err := ver.Compute(name, "1.0.0-SNAPSHOT")
	if err != nil {
		panic(err)
	}
	return plan.Entry{Name: name, Stage: 1, VersionPlan: vp}
}

func TestFilterStages_keepsMatchingEntries(t *testing.T) {
	stages := [][]plan.Entry{
		{mustPlanEntry("kiwi-parent")},
		{mustPlanEntry("kiwi"), mustPlanEntry("kiwi-test")},
	}

	got := filterStages(stages, []string{"kiwi-parent", "kiwi"})

	if len(got) != 2 {
		t.Fatalf("expected 2 stages, got %d", len(got))
	}
	if len(got[0]) != 1 || got[0][0].Name != "kiwi-parent" {
		t.Errorf("stage 1: got %v, want [kiwi-parent]", names(got[0]))
	}
	if len(got[1]) != 1 || got[1][0].Name != "kiwi" {
		t.Errorf("stage 2: got %v, want [kiwi]", names(got[1]))
	}
}

func TestFilterStages_dropsEmptyStages(t *testing.T) {
	stages := [][]plan.Entry{
		{mustPlanEntry("kiwi-parent")},
		{mustPlanEntry("kiwi")},
	}

	got := filterStages(stages, []string{"kiwi"})

	if len(got) != 1 {
		t.Fatalf("expected 1 stage (empty stage dropped), got %d", len(got))
	}
	if got[0][0].Name != "kiwi" {
		t.Errorf("expected kiwi, got %s", got[0][0].Name)
	}
}

func TestFilterStages_emptyOnlyReturnsEmpty(t *testing.T) {
	stages := [][]plan.Entry{
		{mustPlanEntry("kiwi-parent")},
		{mustPlanEntry("kiwi")},
	}

	got := filterStages(stages, []string{"nonexistent"})

	if len(got) != 0 {
		t.Errorf("expected 0 stages, got %d", len(got))
	}
}

func TestFilterStages_allMatchedPreservesAll(t *testing.T) {
	stages := [][]plan.Entry{
		{mustPlanEntry("kiwi-parent")},
		{mustPlanEntry("kiwi"), mustPlanEntry("kiwi-test")},
	}

	got := filterStages(stages, []string{"kiwi-parent", "kiwi", "kiwi-test"})

	if len(got) != 2 {
		t.Fatalf("expected 2 stages, got %d", len(got))
	}
	if len(got[1]) != 2 {
		t.Errorf("stage 2: expected 2 entries, got %d", len(got[1]))
	}
}

func TestAutoPurgeLogs_disabledWhenRetentionIsZero(t *testing.T) {
	logsDir := t.TempDir()
	old := makeDir(t, logsDir, "2020-01-01T00-00-00")

	var buf bytes.Buffer
	if err := autoPurgeLogs(&buf, logsDir, "", 0); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, err := os.Stat(old); err != nil {
		t.Errorf("expected directory to be kept when retention is disabled, got error: %v", err)
	}
	if buf.Len() != 0 {
		t.Errorf("expected no output when retention is disabled, got: %q", buf.String())
	}
}

func TestAutoPurgeLogs_deletesOldDirectoriesWithoutPrompting(t *testing.T) {
	logsDir := t.TempDir()
	old := makeDir(t, logsDir, "2020-01-01T00-00-00")
	recent := makeDir(t, logsDir, "2099-01-01T00-00-00")

	var buf bytes.Buffer
	if err := autoPurgeLogs(&buf, logsDir, "", 30); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, err := os.Stat(old); !os.IsNotExist(err) {
		t.Errorf("expected old directory to be deleted")
	}
	if _, err := os.Stat(recent); err != nil {
		t.Errorf("expected recent directory to be kept, got error: %v", err)
	}
	if !strings.Contains(buf.String(), "Deleted 1 directory") {
		t.Errorf("expected deletion confirmation in output, got:\n%s", buf.String())
	}
}

func TestAutoPurgeLogs_excludesOwnRunDirectoryEvenIfOld(t *testing.T) {
	logsDir := t.TempDir()
	// Simulates a resumed run: its directory is named for the ORIGINAL start
	// time, which can look older than the retention window by the time the
	// resumed run finishes successfully.
	ownDir := makeDir(t, logsDir, "2020-01-01T00-00-00")
	other := makeDir(t, logsDir, "2020-06-01T00-00-00")

	var buf bytes.Buffer
	if err := autoPurgeLogs(&buf, logsDir, "2020-01-01T00-00-00", 30); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, err := os.Stat(ownDir); err != nil {
		t.Errorf("expected own run directory to be kept despite its age, got error: %v", err)
	}
	if _, err := os.Stat(other); !os.IsNotExist(err) {
		t.Errorf("expected other old directory to still be deleted")
	}
}

func names(entries []plan.Entry) []string {
	out := make([]string, len(entries))
	for i, e := range entries {
		out[i] = e.Name
	}
	return out
}
