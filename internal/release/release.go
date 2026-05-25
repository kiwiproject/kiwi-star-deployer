package release

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/kiwiproject/kiwi-star-deployer/internal/plan"
	"github.com/kiwiproject/kiwi-star-deployer/internal/runner"
	"github.com/kiwiproject/kiwi-star-deployer/internal/workspace"
)

// CentralChecker verifies that a released artifact is available in Maven Central.
type CentralChecker interface {
	Wait(w io.Writer, groupID, artifactID, version string, maxWait, interval time.Duration) error
}

// Options configures the release executor.
type Options struct {
	GroupID      string
	MaxWait      time.Duration
	PollInterval time.Duration
	Checker      CentralChecker
}

type libraryResult struct {
	name    string
	logFile string
	output  string // captured stdout+stderr, printed to console on failure
	err     error
}

// Execute runs all release stages in order. Libraries within a stage are
// released in parallel. If any library in a stage fails, execution halts
// before the next stage begins.
func Execute(w io.Writer, stages [][]plan.Entry, ws *workspace.Workspace, r runner.Runner, logBaseDir string, opts Options) error {
	logDir, err := createLogDir(logBaseDir)
	if err != nil {
		return fmt.Errorf("creating log directory: %w", err)
	}
	fmt.Fprintf(w, "Logs: %s\n\n", logDir)

	for i, stage := range stages {
		names := make([]string, len(stage))
		for j, e := range stage {
			names[j] = e.Name
		}
		fmt.Fprintf(w, "Stage %d: releasing %s\n", i+1, strings.Join(names, ", "))

		results := releaseStage(stage, ws, r, logDir, opts)

		var failed bool
		for _, res := range results {
			if res.err != nil {
				fmt.Fprintf(w, "  FAILED  %s\n", res.name)
				fmt.Fprintf(w, "  log:    %s\n", res.logFile)
				fmt.Fprintf(w, "%s\n", res.output)
				failed = true
			} else {
				fmt.Fprintf(w, "  done    %s  (log: %s)\n", res.name, res.logFile)
			}
		}

		if failed {
			return fmt.Errorf("stage %d failed; halting", i+1)
		}
	}

	return nil
}

// releaseStage releases all libraries in a stage concurrently and waits for
// all to finish before returning. Results are returned in the same order as
// the input entries.
func releaseStage(entries []plan.Entry, ws *workspace.Workspace, r runner.Runner, logDir string, opts Options) []libraryResult {
	results := make([]libraryResult, len(entries))
	var wg sync.WaitGroup

	for i, entry := range entries {
		wg.Add(1)
		go func(idx int, e plan.Entry) {
			defer wg.Done()
			results[idx] = releaseLibrary(e, ws, r, logDir, opts)
		}(i, entry)
	}

	wg.Wait()
	return results
}

func releaseLibrary(entry plan.Entry, ws *workspace.Workspace, r runner.Runner, logDir string, opts Options) libraryResult {
	logFile := filepath.Join(logDir, entry.Name+".log")

	f, err := os.Create(logFile)
	if err != nil {
		return libraryResult{name: entry.Name, logFile: logFile, err: fmt.Errorf("creating log file: %w", err)}
	}
	defer func() { _ = f.Close() }()

	var buf bytes.Buffer
	out := io.MultiWriter(f, &buf)

	repoDir := ws.RepoDir(entry.Name)
	vp := entry.VersionPlan

	if _, err := r.Run(runner.Options{
		Command: "mvn",
		Args: []string{
			"-B",
			"clean",
			"release:clean",
			"release:prepare",
			"release:perform",
			"-Darguments=-DskipTests",
			"-DreleaseVersion=" + vp.ReleaseVersion,
			"-DdevelopmentVersion=" + vp.NextDevVersion,
			"-e",
		},
		WorkingDir: repoDir,
		Stdout:     out,
		Stderr:     out,
	}); err != nil {
		return libraryResult{name: entry.Name, logFile: logFile, output: buf.String(), err: fmt.Errorf("mvn release: %w", err)}
	}

	if err := opts.Checker.Wait(out, opts.GroupID, entry.Name, vp.ReleaseVersion, opts.MaxWait, opts.PollInterval); err != nil {
		return libraryResult{name: entry.Name, logFile: logFile, output: buf.String(), err: err}
	}

	return libraryResult{name: entry.Name, logFile: logFile}
}

func createLogDir(baseDir string) (string, error) {
	dir := filepath.Join(baseDir, time.Now().Format("2006-01-02T15-04-05"))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	return dir, nil
}
