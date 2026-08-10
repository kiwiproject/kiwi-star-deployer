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

	"github.com/kiwiproject/kiwi-star-deployer/internal/config"
	"github.com/kiwiproject/kiwi-star-deployer/internal/plan"
	"github.com/kiwiproject/kiwi-star-deployer/internal/runner"
	"github.com/kiwiproject/kiwi-star-deployer/internal/state"
	"github.com/kiwiproject/kiwi-star-deployer/internal/version"
	"github.com/kiwiproject/kiwi-star-deployer/internal/workspace"
)

// CentralChecker verifies that a released artifact is available in Maven Central.
type CentralChecker interface {
	Wait(w io.Writer, groupID, artifactID, version string, maxWait, interval time.Duration) error
	// Available performs a single existence check without polling. found
	// reports presence (true) or definite absence (false); a non-nil error
	// means presence could not be determined.
	Available(groupID, artifactID, version string) (found bool, err error)
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
	CIChecker      CIChecker
	CIMaxWait      time.Duration
	CIPollInterval time.Duration
	// LogDir pins the log directory for this run. When set (resume case), Execute
	// reuses it so all logs for a logical run stay in one directory. When empty,
	// Execute creates a new timestamped directory under logBaseDir.
	LogDir string
	// MavenTimeout is the per-library timeout for the mvn release:prepare release:perform
	// command. Zero means no timeout (not recommended for production use).
	MavenTimeout time.Duration
	// ChangelogSummaries maps library name to inline summary text (--summary flag).
	ChangelogSummaries map[string]string
	// ChangelogSummaryFiles maps library name to summary file path (--summary-file flag).
	ChangelogSummaryFiles map[string]string
	// SkipUnchanged automatically skips libraries whose last two commits are
	// both maven-release-plugin commits, indicating nothing has changed since
	// the most recent release. A skipped library still has its Maven Central
	// availability verified, and if the GitHub release for its tag is missing
	// (a previous run died before the changelog step), the changelog step is
	// run for the already-released version before the skip is recorded.
	SkipUnchanged bool
}

type libraryResult struct {
	name        string
	version     string
	logFile     string
	output      string // captured stdout+stderr, printed to console on failure
	failedStep  string
	err         error
	autoSkipped bool // true if skipped because no changes since last release
}

// Execute runs all release stages in order. Libraries within a stage are
// released in parallel. If any library in a stage fails, execution halts
// before the next stage begins.
func Execute(w io.Writer, stages [][]plan.Entry, ws *workspace.Workspace, r runner.Runner, logBaseDir string, opts Options) error {
	logDir := opts.LogDir
	if logDir == "" {
		var err error
		logDir, err = createLogDir(logBaseDir)
		if err != nil {
			return fmt.Errorf("creating log directory: %w", err)
		}
	} else if err := os.MkdirAll(logDir, 0o755); err != nil {
		return fmt.Errorf("creating log directory: %w", err)
	}
	if err := opts.StateWriter.SetLogDir(logDir); err != nil {
		return fmt.Errorf("recording log directory in state: %w", err)
	}
	defer func() { _ = opts.StateWriter.Archive(logDir) }()
	sessionLog, err := openSessionLog(logDir, opts.LogDir != "")
	if err != nil {
		return fmt.Errorf("creating session log: %w", err)
	}
	defer func() { _ = sessionLog.Close() }()
	w = io.MultiWriter(w, sessionLog)
	if opts.LogDir != "" {
		fmt.Fprintf(w, "\n--- resumed at %s ---\n\n", time.Now().UTC().Format(time.RFC3339))
	}
	fmt.Fprintf(w, "Logs: %s\n\n", logDir)

	skipVersions, err := buildSkipVersions(w, opts, stages, ws, r)
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
		var results []libraryResult
		if len(toRelease) > 0 {
			results = releaseStage(toRelease, ws, r, logDir, opts)
			for _, res := range results {
				if res.err != nil {
					fmt.Fprintf(w, "  FAILED  %s\n", res.name)
					fmt.Fprintf(w, "  error:  %v\n", res.err)
					fmt.Fprintf(w, "  log:    %s\n", res.logFile)
					fmt.Fprintf(w, "%s\n", res.output)
					_ = opts.StateWriter.RecordFailed(res.name, res.failedStep, res.err.Error())
					failed = true
				} else if res.autoSkipped {
					fmt.Fprintf(w, "  skip    %s  (no changes since %s)\n", res.name, res.version)
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
			// Build effectiveStage for downstream POM updates. Auto-skipped
			// entries use the detected released version, not the planned one.
			effectiveStage := make([]plan.Entry, 0, len(toSkip)+len(toRelease))
			effectiveStage = append(effectiveStage, toSkip...)
			for idx, e := range toRelease {
				if results[idx].autoSkipped && results[idx].version != "" {
					e.VersionPlan.ReleaseVersion = results[idx].version
				}
				effectiveStage = append(effectiveStage, e)
			}
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
// the version is read from the latest tag reachable from origin/main. The
// remote-tracking ref is used rather than the workspace HEAD because Prepare
// never runs for a skipped library, so its HEAD can predate a manual release;
// origin/main is current because plan.Build fetches every library first.
// Each resolved version is then verified to exist on Maven Central: --skip
// asserts "this library's release already succeeded", and treating a
// never-released library as completed would silently feed its old version to
// every downstream POM. Completed entries are not re-verified — their runs
// already did. Skip entries not present in stages (excluded by --only) are
// ignored entirely: their versions are never consulted, so resolving and
// verifying them could only add spurious failures.
func buildSkipVersions(w io.Writer, opts Options, stages [][]plan.Entry, ws *workspace.Workspace, r runner.Runner) (map[string]string, error) {
	if len(opts.Completed) == 0 && len(opts.Skip) == 0 {
		return nil, nil
	}
	staged := make(map[string]bool)
	for _, stage := range stages {
		for _, e := range stage {
			staged[e.Name] = true
		}
	}
	skip := make(map[string]string, len(opts.Completed)+len(opts.Skip))
	for name, ver := range opts.Completed {
		skip[name] = ver
	}
	for _, name := range opts.Skip {
		if _, already := skip[name]; already || !staged[name] {
			continue
		}
		v, err := latestReachableTag(r, ws.RepoDir(name), "origin/main")
		if err != nil {
			return nil, fmt.Errorf("resolving version for --skip %q: %w", name, err)
		}
		if "v"+v == syntheticBaselineTag {
			return nil, fmt.Errorf("--skip %q: latest tag is the synthetic changelog baseline %s; the library has never been released", name, syntheticBaselineTag)
		}
		fmt.Fprintf(w, "Verifying --skip %s %s on Maven Central...\n", name, v)
		found, err := opts.Checker.Available(opts.GroupID, name, v)
		if err != nil {
			return nil, fmt.Errorf("--skip %q: could not verify version %s on Maven Central: %w (this may be transient; retry, or check %s manually)", name, v, err, name)
		}
		if !found {
			return nil, fmt.Errorf("--skip %q: version %s is not on Maven Central; --skip is only for libraries whose release already succeeded — to recover a failed release, use plain --resume instead", name, v)
		}
		skip[name] = v
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

	if err := ws.Prepare(entry.Name); err != nil {
		return fmt.Errorf("preparing workspace: %w", err)
	}

	repoDir := ws.RepoDir(entry.Name)

	for _, dep := range deps {
		if _, err := r.Run(runner.Options{
			Command:    "mvn",
			Args:       mavenVersionArgs(dep, groupID),
			WorkingDir: repoDir,
			Stdout:     out,
			Stderr:     out,
		}); err != nil {
			return fmt.Errorf("mvn versions for %s: %w", dep.Name, err)
		}
	}

	statusResult, err := r.Run(runner.Options{
		Command:    "git",
		Args:       []string{"status", "--porcelain"},
		WorkingDir: repoDir,
	})
	if err != nil {
		return fmt.Errorf("git status: %w", err)
	}
	if strings.TrimSpace(statusResult.Stdout) == "" {
		return nil // POM already up to date; nothing to commit
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

// mavenVersionArgs returns the mvn arguments for updating one dependency version
// in a downstream POM.
//
// parent-pom deps are declared as a literal version in <parent>, so
// versions:use-dep-version handles them. -DprocessParent=true is required: the
// mojo's processParent parameter defaults to false, without which the <parent>
// element is silently left unchanged. -DforceVersion=true skips the mojo's
// repository-metadata availability check, which can lag behind actual artifact
// publication right after a release; the artifact has already been verified
// directly against Maven Central before any downstream POM update runs.
// All other dep types (bom, library-bom, regular libraries) are declared via a
// property named exactly <artifactId>.version (e.g., kiwi-bom.version,
// kiwi.version), so versions:set-property is required. If a dep deviates from
// this convention and uses a literal version element, set-property will
// silently do nothing and the git status check in updatePOM will skip the
// commit, surfacing the mismatch without corrupting the POM.
func mavenVersionArgs(dep plan.Entry, groupID string) []string {
	if dep.Type == config.TypeParentPOM {
		return []string{
			"-B",
			"versions:use-dep-version",
			"-Dincludes=" + groupID + ":" + dep.Name,
			"-DdepVersion=" + dep.VersionPlan.ReleaseVersion,
			"-DprocessParent=true",
			"-DforceVersion=true",
			"-DgenerateBackupPoms=false",
		}
	}
	return []string{
		"-B",
		"versions:set-property",
		"-Dproperty=" + dep.Name + ".version",
		"-DnewVersion=" + dep.VersionPlan.ReleaseVersion,
		"-DgenerateBackupPoms=false",
	}
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

	if err := ws.Prepare(entry.Name); err != nil {
		return libraryResult{name: entry.Name, logFile: logFile, failedStep: state.StepMavenRelease, err: fmt.Errorf("preparing workspace: %w", err)}
	}

	if opts.SkipUnchanged {
		changed, lastVersion, err := hasChangesSinceLastRelease(ws, r, entry.Name)
		if err != nil {
			return libraryResult{name: entry.Name, logFile: logFile, failedStep: state.StepMavenRelease, err: fmt.Errorf("checking for changes: %w", err)}
		}
		if !changed {
			return verifySkippedRelease(entry, ws, r, logFile, opts, lastVersion)
		}
	}

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
	previousRev, err := ensurePreviousReleaseTag(r, repoDir, "HEAD")
	if err != nil {
		return libraryResult{name: entry.Name, logFile: logFile, output: buf.String(), failedStep: state.StepMavenRelease, err: err}
	}

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
		Timeout:    opts.MavenTimeout,
	}); err != nil {
		return libraryResult{name: entry.Name, logFile: logFile, output: buf.String(), failedStep: state.StepMavenRelease, err: fmt.Errorf("mvn release: %w", err)}
	}

	if err := opts.Checker.Wait(out, opts.GroupID, entry.Name, vp.ReleaseVersion, opts.MaxWait, opts.PollInterval); err != nil {
		return libraryResult{name: entry.Name, logFile: logFile, output: buf.String(), failedStep: state.StepCentralVerify, err: err}
	}

	nextMilestone := strings.TrimSuffix(vp.NextDevVersion, "-SNAPSHOT")
	if err := runChangelog(r, entry, repoDir, previousRev, vp.ReleaseVersion, nextMilestone, out, opts); err != nil {
		return libraryResult{name: entry.Name, logFile: logFile, output: buf.String(), failedStep: state.StepChangelog, err: err}
	}

	return libraryResult{name: entry.Name, version: vp.ReleaseVersion, logFile: logFile}
}

// syntheticBaselineTag is created at the root commit before a library's first
// release, following the kiwiproject-changelog documentation: the changelog
// tool always needs a previous revision to compute the commit range, and a
// synthetic annotated tag at the very first commit anchors the first
// release's changelog to the full history.
const syntheticBaselineTag = "v0.0.0"

// latestReachableTag returns the most recent tag reachable from ref, stripped
// of its v prefix.
func latestReachableTag(r runner.Runner, repoDir, ref string) (string, error) {
	result, err := r.Run(runner.Options{
		Command:    "git",
		Args:       []string{"describe", "--tags", "--abbrev=0", ref},
		WorkingDir: repoDir,
	})
	if err != nil {
		return "", err
	}
	return strings.TrimPrefix(strings.TrimSpace(result.Stdout), "v"), nil
}

// ensurePreviousReleaseTag returns the most recent release tag reachable from
// ref, stripped of its v prefix. When no tag is reachable — the first release
// of a library — it creates the synthetic annotated baseline tag at the
// repository's root commit and pushes it, then returns that tag's version.
func ensurePreviousReleaseTag(r runner.Runner, repoDir, ref string) (string, error) {
	v, describeErr := latestReachableTag(r, repoDir, ref)
	if describeErr == nil {
		return v, nil
	}
	rootResult, err := r.Run(runner.Options{
		Command:    "git",
		Args:       []string{"rev-list", "--max-parents=0", ref},
		WorkingDir: repoDir,
	})
	if err != nil {
		return "", fmt.Errorf("no release tag reachable from %s (%v) and finding root commit failed: %w", ref, describeErr, err)
	}
	lines := strings.Split(strings.TrimSpace(rootResult.Stdout), "\n")
	root := strings.TrimSpace(lines[len(lines)-1])
	if root == "" {
		return "", fmt.Errorf("no release tag reachable from %s and no root commit found", ref)
	}
	if _, err := r.Run(runner.Options{
		Command:    "git",
		Args:       []string{"tag", "-a", syntheticBaselineTag, root, "-m", "Initial synthetic tag for changelog baseline"},
		WorkingDir: repoDir,
	}); err != nil {
		return "", fmt.Errorf("creating synthetic baseline tag %s: %w", syntheticBaselineTag, err)
	}
	if _, err := r.Run(runner.Options{
		Command:    "git",
		Args:       []string{"push", "origin", syntheticBaselineTag},
		WorkingDir: repoDir,
	}); err != nil {
		return "", fmt.Errorf("pushing synthetic baseline tag %s: %w", syntheticBaselineTag, err)
	}
	return strings.TrimPrefix(syntheticBaselineTag, "v"), nil
}

// runChangelog invokes the changelog script for one released version.
func runChangelog(r runner.Runner, entry plan.Entry, repoDir, previousRev, releaseVersion, nextMilestone string, out io.Writer, opts Options) error {
	if _, err := r.Run(runner.Options{
		Command:    opts.ChangelogScript,
		Args:       changelogArgs(entry, previousRev, releaseVersion, nextMilestone, opts),
		WorkingDir: repoDir,
		Stdout:     out,
		Stderr:     out,
	}); err != nil {
		return fmt.Errorf("changelog: %w", err)
	}
	return nil
}

// changelogArgs builds the argument list for one changelog script invocation.
func changelogArgs(entry plan.Entry, previousRev, releaseVersion, nextMilestone string, opts Options) []string {
	args := []string{
		"--repository", entry.Repo,
		"--previous-rev", previousRev,
		"--revision", releaseVersion,
		"--output-type", "GITHUB",
		"--close-milestone",
		"--create-next-milestone", nextMilestone,
		"--add-v-prefix-to-revisions",
	}
	if text, ok := opts.ChangelogSummaries[entry.Name]; ok {
		args = append(args, "--summary", text)
	} else if path, ok := opts.ChangelogSummaryFiles[entry.Name]; ok {
		args = append(args, "--summary-file", path)
	}
	return args
}

// verifySkippedRelease finishes verification for a library that auto-skip
// determined has nothing new to release: lastVersion was already released by a
// previous run. That run may have died after mvn pushed the release but before
// Maven Central confirmed the artifact, or before the changelog step ran (the
// typical failure when Maven Central publication is slow), so this re-verifies
// Central availability and, if the GitHub release for the tag is missing, runs
// the changelog step to create it, close the milestone, and open the next one.
// Both checks are cheap no-ops for a release that fully completed.
func verifySkippedRelease(entry plan.Entry, ws *workspace.Workspace, r runner.Runner, logFile string, opts Options, lastVersion string) libraryResult {
	f, err := os.Create(logFile)
	if err != nil {
		return libraryResult{name: entry.Name, logFile: logFile, err: fmt.Errorf("creating log file: %w", err)}
	}
	defer func() { _ = f.Close() }()

	var buf bytes.Buffer
	out := io.MultiWriter(f, &buf)

	if err := opts.Checker.Wait(out, opts.GroupID, entry.Name, lastVersion, opts.MaxWait, opts.PollInterval); err != nil {
		// Unlike the post-release wait, where mvn just uploaded the artifact
		// and slowness is the only explanation, here the artifact may never
		// have been deployed: auto-skip fires on the pushed release commits
		// and tag, which release:prepare creates before release:perform
		// uploads anything.
		err = fmt.Errorf("%w (auto-skip found release tag v%s already pushed; if the run that pushed it died before release:perform uploaded the artifact, it was never deployed and waiting longer cannot help — deploy v%s manually from its tag, then run release --resume to finish the changelog and GitHub release)", err, lastVersion, lastVersion)
		return libraryResult{name: entry.Name, logFile: logFile, output: buf.String(), failedStep: state.StepCentralVerify, err: err}
	}

	exists, err := githubReleaseExists(r, entry.Repo, "v"+lastVersion)
	if err != nil {
		return libraryResult{name: entry.Name, logFile: logFile, output: buf.String(), failedStep: state.StepChangelog, err: err}
	}
	if exists {
		return libraryResult{name: entry.Name, version: lastVersion, logFile: logFile, autoSkipped: true}
	}

	fmt.Fprintf(out, "GitHub release v%s is missing; running changelog for the previously released version\n", lastVersion)

	repoDir := ws.RepoDir(entry.Name)
	previousRev, err := ensurePreviousReleaseTag(r, repoDir, "v"+lastVersion+"^")
	if err != nil {
		return libraryResult{name: entry.Name, logFile: logFile, output: buf.String(), failedStep: state.StepChangelog, err: fmt.Errorf("finding tag before v%s: %w", lastVersion, err)}
	}

	nextMilestone, err := version.NextPatch(lastVersion)
	if err != nil {
		return libraryResult{name: entry.Name, logFile: logFile, output: buf.String(), failedStep: state.StepChangelog, err: fmt.Errorf("computing next milestone: %w", err)}
	}

	if err := runChangelog(r, entry, repoDir, previousRev, lastVersion, nextMilestone, out, opts); err != nil {
		return libraryResult{name: entry.Name, logFile: logFile, output: buf.String(), failedStep: state.StepChangelog, err: err}
	}

	return libraryResult{name: entry.Name, version: lastVersion, logFile: logFile, autoSkipped: true}
}

// githubReleaseExists reports whether repo has a GitHub release for tag. It
// queries the release-by-tag endpoint via gh; a 404 means the release is
// missing, and any other failure (auth, network) is a real error the caller
// should halt on rather than guessing.
func githubReleaseExists(r runner.Runner, repo, tag string) (bool, error) {
	result, err := r.Run(runner.Options{
		Command: "gh",
		Args:    []string{"api", "repos/" + repo + "/releases/tags/" + tag},
	})
	if err == nil {
		return true, nil
	}
	if result != nil && strings.Contains(result.Stderr, "404") {
		return false, nil
	}
	return false, fmt.Errorf("checking GitHub release for %s: %w", tag, err)
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

func openSessionLog(logDir string, isResume bool) (*os.File, error) {
	path := filepath.Join(logDir, "session.log")
	if isResume {
		return os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	}
	return os.Create(path)
}

// hasChangesSinceLastRelease returns false (no changes) when the two most
// recent commits are both maven-release-plugin commits AND the "prepare
// release" commit carries a version tag, indicating nothing was committed
// since the last release. It also returns the version from that commit so
// the caller can record it. A git error, fewer than 2 commits, or a missing
// tag causes it to return true (has changes) so the library releases normally.
func hasChangesSinceLastRelease(ws *workspace.Workspace, r runner.Runner, name string) (changed bool, lastVersion string, err error) {
	result, err := r.Run(runner.Options{
		Command:    "git",
		Args:       []string{"log", "--oneline", "--no-decorate", "-2"},
		WorkingDir: ws.RepoDir(name),
	})
	if err != nil {
		return true, "", fmt.Errorf("git log: %w", err)
	}
	lines := strings.Split(strings.TrimSpace(result.Stdout), "\n")
	if len(lines) < 2 {
		return true, "", nil
	}
	msg0 := gitCommitMessage(lines[0])
	msg1 := gitCommitMessage(lines[1])
	if !strings.HasPrefix(msg0, "[maven-release-plugin]") ||
		!strings.HasPrefix(msg1, "[maven-release-plugin]") ||
		!strings.Contains(msg1, "prepare release") {
		return true, "", nil
	}
	// Confirm the release commit carries a tag that matches the version in the
	// commit message — guards against crafted messages, partial releases that
	// left no tag, and any mismatch between the two.
	releasedVersion := extractReleaseVersion(msg1)
	sha, _, _ := strings.Cut(lines[1], " ")
	tagResult, err := r.Run(runner.Options{
		Command:    "git",
		Args:       []string{"tag", "--points-at", sha},
		WorkingDir: ws.RepoDir(name),
	})
	if err != nil || !containsExactTag(tagResult.Stdout, "v"+releasedVersion) {
		return true, "", nil
	}
	return false, releasedVersion, nil
}

// containsExactTag reports whether output (from "git tag --points-at")
// contains a line that is exactly the given tag name.
func containsExactTag(output, tag string) bool {
	for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
		if strings.TrimSpace(line) == tag {
			return true
		}
	}
	return false
}

func gitCommitMessage(logLine string) string {
	_, msg, _ := strings.Cut(logLine, " ")
	return msg
}

func extractReleaseVersion(msg string) string {
	const prefix = "[maven-release-plugin] prepare release "
	return strings.TrimPrefix(strings.TrimPrefix(msg, prefix), "v")
}
