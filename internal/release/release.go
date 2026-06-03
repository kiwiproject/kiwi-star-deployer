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
	"github.com/kiwiproject/kiwi-star-deployer/internal/state"
	"github.com/kiwiproject/kiwi-star-deployer/internal/workspace"
)

// CentralChecker verifies that a released artifact is available in Maven Central.
type CentralChecker interface {
	Wait(w io.Writer, groupID, artifactID, version string, maxWait, interval time.Duration) error
}

// CIChecker verifies that GitHub Actions CI passes after a downstream POM update push.
type CIChecker interface {
	Wait(w io.Writer, repo, commitSHA string, maxWait, interval time.Duration) error
}

// Options configures the release executor.
type Options struct {
	GroupID         string
	MaxWait         time.Duration
	PollInterval    time.Duration
	Checker         CentralChecker
	ChangelogScript string
	StateWriter     *state.Writer
	// Completed maps library name → already-released version for --resume runs.
	// These libraries are skipped and recorded as completed without re-releasing.
	Completed map[string]string
	// Skip lists additional library names to treat as already released.
	// Versions are resolved from the latest git tag in the workspace.
	Skip []string
	// CIChecker verifies GitHub Actions CI passes after each downstream POM update push.
	// If nil, CI verification is skipped.
	CIChecker     CIChecker
	CIMaxWait     time.Duration
	CIPollInterval time.Duration
}

type libraryResult struct {
	name       string
	version    string
	logFile    string
	output     string // captured stdout+stderr, printed to console on failure
	failedStep string
	err        error
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

	skipVersions, err := buildSkipVersions(opts, ws, r)
	if err != nil {
		return err
	}

	var totalReleased int
	for i, stage := range stages {
		names := make([]string, len(stage))
		for j, e := range stage {
			names[j] = e.Name
		}
		fmt.Fprintf(w, "Stage %d: releasing %s\n", i+1, strings.Join(names, ", "))

		var toSkip, toRelease []plan.Entry
		for _, e := range stage {
			if v, ok := skipVersions[e.Name]; ok {
				e.VersionPlan.ReleaseVersion = v
				toSkip = append(toSkip, e)
			} else {
				toRelease = append(toRelease, e)
			}
		}

		for _, e := range toSkip {
			fmt.Fprintf(w, "  skip    %s  (version: %s)\n", e.Name, e.VersionPlan.ReleaseVersion)
			_ = opts.StateWriter.RecordCompleted(e.Name, e.VersionPlan.ReleaseVersion)
		}

		var failed bool
		if len(toRelease) > 0 {
			results := releaseStage(toRelease, ws, r, logDir, opts)
			for _, res := range results {
				if res.err != nil {
					fmt.Fprintf(w, "  FAILED  %s\n", res.name)
					fmt.Fprintf(w, "  log:    %s\n", res.logFile)
					fmt.Fprintf(w, "%s\n", res.output)
					_ = opts.StateWriter.RecordFailed(res.name, res.failedStep, res.err.Error())
					failed = true
				} else {
					fmt.Fprintf(w, "  done    %s  (log: %s)\n", res.name, res.logFile)
					totalReleased++
				}
			}
		}

		if failed {
			return fmt.Errorf("stage %d failed; halting", i+1)
		}

		if i < len(stages)-1 {
			effectiveStage := append(toSkip, toRelease...)
			if err := updateDownstreamPOMs(w, effectiveStage, stages[i+1:], ws, r, logDir, opts); err != nil {
				return err
			}
		}
	}

	fmt.Fprintf(w, "\nReleased %d %s.\n", totalReleased, pluralize("library", "libraries", totalReleased))
	return nil
}

// buildSkipVersions merges opts.Completed and opts.Skip into a single map of
// library name → released version. For Skip entries not already in Completed,
// the version is read from the latest git tag in the workspace.
func buildSkipVersions(opts Options, ws *workspace.Workspace, r runner.Runner) (map[string]string, error) {
	if len(opts.Completed) == 0 && len(opts.Skip) == 0 {
		return nil, nil
	}
	skip := make(map[string]string, len(opts.Completed)+len(opts.Skip))
	for name, ver := range opts.Completed {
		skip[name] = ver
	}
	for _, name := range opts.Skip {
		if _, already := skip[name]; already {
			continue
		}
		result, err := r.Run(runner.Options{
			Command:    "git",
			Args:       []string{"describe", "--tags", "--abbrev=0"},
			WorkingDir: ws.RepoDir(name),
		})
		if err != nil {
			return nil, fmt.Errorf("resolving version for --skip %q: %w", name, err)
		}
		skip[name] = strings.TrimPrefix(strings.TrimSpace(result.Stdout), "v")
	}
	return skip, nil
}

func updateDownstreamPOMs(w io.Writer, releasedStage []plan.Entry, futureStages [][]plan.Entry, ws *workspace.Workspace, r runner.Runner, logDir string, opts Options) error {
	released := make(map[string]plan.Entry)
	for _, e := range releasedStage {
		released[e.Name] = e
	}

	for _, stage := range futureStages {
		for _, entry := range stage {
			var deps []plan.Entry
			for _, depName := range entry.DependsOn {
				if re, ok := released[depName]; ok {
					deps = append(deps, re)
				}
			}
			if len(deps) == 0 {
				continue
			}

			logFile := filepath.Join(logDir, entry.Name+"-pom-update.log")
			depSummary := make([]string, len(deps))
			for i, d := range deps {
				depSummary[i] = d.Name + " " + d.VersionPlan.ReleaseVersion
			}
			fmt.Fprintf(w, "  POM update %s: %s\n", entry.Name, strings.Join(depSummary, ", "))

			if err := updatePOM(entry, deps, ws, r, logFile, opts.GroupID); err != nil {
				fmt.Fprintf(w, "  FAILED POM update %s\n", entry.Name)
				fmt.Fprintf(w, "  log:   %s\n", logFile)
				_ = opts.StateWriter.RecordFailed(entry.Name, state.StepPOMUpdate, err.Error())
				return fmt.Errorf("POM update for %s: %w", entry.Name, err)
			}
			fmt.Fprintf(w, "  done   POM update %s  (log: %s)\n", entry.Name, logFile)

			if opts.CIChecker != nil {
				shaResult, err := r.Run(runner.Options{
					Command:    "git",
					Args:       []string{"rev-parse", "HEAD"},
					WorkingDir: ws.RepoDir(entry.Name),
				})
				if err != nil {
					return fmt.Errorf("git rev-parse HEAD after POM update for %s: %w", entry.Name, err)
				}
				commitSHA := strings.TrimSpace(shaResult.Stdout)
				fmt.Fprintf(w, "  CI verify %s...\n", entry.Name)
				if err := opts.CIChecker.Wait(w, entry.Repo, commitSHA, opts.CIMaxWait, opts.CIPollInterval); err != nil {
					_ = opts.StateWriter.RecordFailed(entry.Name, state.StepCIVerify, err.Error())
					return fmt.Errorf("CI verification for %s: %w", entry.Name, err)
				}
				fmt.Fprintf(w, "  CI passed %s\n", entry.Name)
			}
		}
	}
	return nil
}

func updatePOM(entry plan.Entry, deps []plan.Entry, ws *workspace.Workspace, r runner.Runner, logFile string, groupID string) error {
	f, err := os.OpenFile(logFile, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("opening log file: %w", err)
	}
	defer func() { _ = f.Close() }()

	var buf bytes.Buffer
	out := io.MultiWriter(f, &buf)
	repoDir := ws.RepoDir(entry.Name)

	for _, dep := range deps {
		if _, err := r.Run(runner.Options{
			Command: "mvn",
			Args: []string{
				"-B",
				"versions:use-dep-version",
				"-Dincludes=" + groupID + ":" + dep.Name,
				"-DdepVersion=" + dep.VersionPlan.ReleaseVersion,
				"-DgenerateBackupPoms=false",
			},
			WorkingDir: repoDir,
			Stdout:     out,
			Stderr:     out,
		}); err != nil {
			return fmt.Errorf("mvn versions:use-dep-version for %s: %w", dep.Name, err)
		}
	}

	if _, err := r.Run(runner.Options{
		Command:    "git",
		Args:       []string{"add", "pom.xml"},
		WorkingDir: repoDir,
		Stdout:     out,
		Stderr:     out,
	}); err != nil {
		return fmt.Errorf("git add: %w", err)
	}

	if _, err := r.Run(runner.Options{
		Command:    "git",
		Args:       []string{"commit", "-m", buildPOMUpdateCommitMessage(deps)},
		WorkingDir: repoDir,
		Stdout:     out,
		Stderr:     out,
	}); err != nil {
		return fmt.Errorf("git commit: %w", err)
	}

	if _, err := r.Run(runner.Options{
		Command:    "git",
		Args:       []string{"push"},
		WorkingDir: repoDir,
		Stdout:     out,
		Stderr:     out,
	}); err != nil {
		return fmt.Errorf("git push: %w", err)
	}

	return nil
}

func buildPOMUpdateCommitMessage(deps []plan.Entry) string {
	var sb strings.Builder
	sb.WriteString("chore: update dependency versions\n\n")
	for _, dep := range deps {
		fmt.Fprintf(&sb, "- %s %s\n", dep.Name, dep.VersionPlan.ReleaseVersion)
	}
	return sb.String()
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
			res := releaseLibrary(e, ws, r, logDir, opts)
			if res.err == nil {
				_ = opts.StateWriter.RecordCompleted(res.name, res.version)
			}
			results[idx] = res
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

	// Capture the previous release tag before mvn creates the new one.
	tagResult, err := r.Run(runner.Options{
		Command:    "git",
		Args:       []string{"describe", "--tags", "--abbrev=0"},
		WorkingDir: repoDir,
	})
	if err != nil {
		return libraryResult{name: entry.Name, logFile: logFile, output: buf.String(), failedStep: state.StepMavenRelease, err: fmt.Errorf("git describe: %w", err)}
	}
	previousRev := strings.TrimPrefix(strings.TrimSpace(tagResult.Stdout), "v")

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
		return libraryResult{name: entry.Name, logFile: logFile, output: buf.String(), failedStep: state.StepMavenRelease, err: fmt.Errorf("mvn release: %w", err)}
	}

	if err := opts.Checker.Wait(out, opts.GroupID, entry.Name, vp.ReleaseVersion, opts.MaxWait, opts.PollInterval); err != nil {
		return libraryResult{name: entry.Name, logFile: logFile, output: buf.String(), failedStep: state.StepCentralVerify, err: err}
	}

	nextMilestone := strings.TrimSuffix(vp.NextDevVersion, "-SNAPSHOT")
	if _, err := r.Run(runner.Options{
		Command: opts.ChangelogScript,
		Args: []string{
			"--repository", entry.Repo,
			"--previous-rev", previousRev,
			"--revision", vp.ReleaseVersion,
			"--output-type", "GITHUB",
			"--close-milestone",
			"--create-next-milestone", nextMilestone,
			"--add-v-prefix-to-revisions",
		},
		WorkingDir: repoDir,
		Stdout:     out,
		Stderr:     out,
	}); err != nil {
		return libraryResult{name: entry.Name, logFile: logFile, output: buf.String(), failedStep: state.StepChangelog, err: fmt.Errorf("changelog: %w", err)}
	}

	return libraryResult{name: entry.Name, version: vp.ReleaseVersion, logFile: logFile}
}

func pluralize(singular, plural string, n int) string {
	if n == 1 {
		return singular
	}
	return plural
}

func createLogDir(baseDir string) (string, error) {
	dir := filepath.Join(baseDir, time.Now().Format("2006-01-02T15-04-05"))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	return dir, nil
}
