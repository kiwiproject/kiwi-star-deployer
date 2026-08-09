package cmd

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/kiwiproject/kiwi-star-deployer/internal/checkversions"
	"github.com/kiwiproject/kiwi-star-deployer/internal/ci"
	"github.com/kiwiproject/kiwi-star-deployer/internal/config"
	"github.com/kiwiproject/kiwi-star-deployer/internal/mavencentral"
	"github.com/kiwiproject/kiwi-star-deployer/internal/plan"
	"github.com/kiwiproject/kiwi-star-deployer/internal/preflight"
	"github.com/kiwiproject/kiwi-star-deployer/internal/release"
	"github.com/kiwiproject/kiwi-star-deployer/internal/runner"
	"github.com/kiwiproject/kiwi-star-deployer/internal/state"
	"github.com/kiwiproject/kiwi-star-deployer/internal/workspace"
	"github.com/spf13/cobra"
)

var releaseCmd = &cobra.Command{
	Use:   "release",
	Short: "Release kiwiproject libraries in dependency order",
	RunE:  runRelease,
}

var (
	resume           bool
	skipLibs         []string
	dryRun           bool
	onlyLibs         []string
	summaryFlags     []string
	summaryFileFlags []string
	noAutoSkip       bool
)

func runRelease(_ *cobra.Command, _ []string) error {
	cfg, err := config.Load(configPath)
	if err != nil {
		return err
	}

	if err := validateReleaseFlags(resume, noAutoSkip, skipLibs); err != nil {
		return err
	}
	for _, name := range skipLibs {
		if _, ok := cfg.Libraries[name]; !ok {
			return fmt.Errorf("--skip: unknown library %q", name)
		}
	}
	for _, name := range onlyLibs {
		if _, ok := cfg.Libraries[name]; !ok {
			return fmt.Errorf("--only: unknown library %q", name)
		}
	}

	summaries, err := parseSummaryFlags(summaryFlags, cfg.Libraries)
	if err != nil {
		return err
	}
	summaryFiles, err := parseSummaryFileFlags(summaryFileFlags, cfg.Libraries)
	if err != nil {
		return err
	}
	for name := range summaries {
		if _, ok := summaryFiles[name]; ok {
			return fmt.Errorf("library %q: --summary and --summary-file are mutually exclusive", name)
		}
	}

	r := runner.NewOsRunner()

	if !dryRun {
		results := preflight.RunAll(r, cfg.Settings.ChangelogScript)
		preflight.Print(os.Stdout, results)
		if !preflight.AllPassed(results) {
			return fmt.Errorf("preflight failed; fix the issues above before releasing")
		}

		// The milestone gate is skipped on --resume: a run that failed at
		// maven-central-verify or changelog leaves the POM bumped to the next
		// SNAPSHOT while the matching milestone is only created by the
		// changelog step — the very step resume's recovery re-runs. Gating a
		// resume on it would block the recovery that restores the invariant.
		if !resume {
			fmt.Fprintf(os.Stderr, "Checking %d %s against GitHub milestones...\n", len(cfg.Libraries), libNoun(len(cfg.Libraries)))
			cvResults := checkversions.RunAll(r, cfg.Libraries)
			checkversions.Print(os.Stdout, cvResults)
			if !checkversions.AllPassed(cvResults) {
				return fmt.Errorf("version check failed; fix the mismatches above before releasing")
			}
		}
	}

	ws := workspace.New(cfg.Settings.Workspace, r)

	fmt.Fprintf(os.Stderr, "Reading POMs from origin/main (%d %s)...\n", len(cfg.Libraries), libNoun(len(cfg.Libraries)))
	stages, err := plan.Build(cfg, ws)
	if err != nil {
		return err
	}

	if len(onlyLibs) > 0 {
		stages = filterStages(stages, onlyLibs)
	}

	if dryRun {
		plan.Print(os.Stdout, stages)
		printMilestoneNote(os.Stdout)
		return nil
	}

	logBaseDir := cfg.Settings.LogsDir()

	var completedVersions map[string]string
	var prevState *state.State
	if resume {
		s, err := state.Load(cfg.Settings.StatePath)
		if err != nil {
			return fmt.Errorf("loading state file for --resume: %w", err)
		}
		prevState = s
		completedVersions = make(map[string]string, len(s.Completed))
		for _, e := range s.Completed {
			if _, ok := cfg.Libraries[e.Library]; !ok {
				return fmt.Errorf("state file references unknown library %q; config may have changed since last run", e.Library)
			}
			completedVersions[e.Library] = e.Version
		}
		for i, stage := range stages {
			for j, entry := range stage {
				if v, ok := completedVersions[entry.Name]; ok {
					stages[i][j].VersionPlan.ReleaseVersion = v
				}
			}
		}
	}

	var sw *state.Writer
	var swErr error
	if prevState != nil {
		sw, swErr = state.Resume(cfg.Settings.StatePath, prevState)
	} else {
		sw, swErr = state.New(cfg.Settings.StatePath, time.Now().UTC().Format(time.RFC3339))
	}
	if swErr != nil {
		return fmt.Errorf("initializing state file: %w", swErr)
	}

	var resumeLogDir string
	if prevState != nil {
		resumeLogDir = prevState.LogDir
	}

	opts := release.Options{
		GroupID:               cfg.Settings.GroupID,
		LogDir:                resumeLogDir,
		MavenTimeout:          time.Duration(cfg.Settings.MavenReleaseTimeout),
		MaxWait:               time.Duration(cfg.Settings.MavenCentralMaxWait),
		PollInterval:          time.Duration(cfg.Settings.MavenCentralPollInterval),
		Checker:               mavencentral.New(),
		ChangelogScript:       cfg.Settings.ChangelogScript,
		StateWriter:           sw,
		Completed:             completedVersions,
		Skip:                  skipLibs,
		ChangelogSummaries:    summaries,
		ChangelogSummaryFiles: summaryFiles,
		SkipUnchanged:         !noAutoSkip,
		CIChecker:             &ci.GHChecker{Runner: r},
		CIMaxWait:             time.Duration(cfg.Settings.CIMaxWait),
		CIPollInterval:        time.Duration(cfg.Settings.CIPollInterval),
	}

	if err := release.Execute(os.Stdout, stages, ws, r, logBaseDir, opts); err != nil {
		return err
	}

	excludeDir := filepath.Base(sw.LogDir())
	if err := autoPurgeLogs(os.Stdout, logBaseDir, excludeDir, cfg.Settings.LogRetentionDays); err != nil {
		fmt.Fprintf(os.Stderr, "warning: auto-purging old logs: %v\n", err)
	}
	return nil
}

// validateReleaseFlags rejects flag combinations that undermine each other.
// --no-auto-skip with --resume is forbidden because the recovery paths for a
// library that failed at maven-central-verify or changelog are only reachable
// through auto-skip detection: with it disabled, a resumed run would perform a
// full new Maven release at the next version (the POM is already bumped) on
// top of the unrecorded previous one, orphaning its changelog and milestone.
func validateReleaseFlags(resume, noAutoSkip bool, skipLibs []string) error {
	if len(skipLibs) > 0 && !resume {
		return fmt.Errorf("--skip requires --resume")
	}
	if resume && noAutoSkip {
		return fmt.Errorf("--no-auto-skip cannot be combined with --resume: resume recovery relies on auto-skip to verify already-released libraries (resume without it, then force any still-unchanged libraries with --no-auto-skip --only <libs>)")
	}
	return nil
}

// autoPurgeLogs deletes run log directories older than retentionDays without
// prompting, except excludeDir, which is always kept regardless of age — this
// run's own directory, which can look "old" by name if the run was resumed
// from a much earlier start time. It is a no-op if retentionDays is zero (the
// default, meaning auto-purge is disabled). Only called after a successful
// release, so it can never interrupt or delay the release itself.
func autoPurgeLogs(w io.Writer, logsDir, excludeDir string, retentionDays int) error {
	if retentionDays <= 0 {
		return nil
	}
	age, err := parseAge(fmt.Sprintf("%dd", retentionDays))
	if err != nil {
		return err
	}
	return purgeLogs(w, nil, logsDir, age, true, excludeDir)
}

func filterStages(stages [][]plan.Entry, only []string) [][]plan.Entry {
	onlySet := make(map[string]struct{}, len(only))
	for _, name := range only {
		onlySet[name] = struct{}{}
	}
	var result [][]plan.Entry
	for _, stage := range stages {
		var filtered []plan.Entry
		for _, e := range stage {
			if _, ok := onlySet[e.Name]; ok {
				filtered = append(filtered, e)
			}
		}
		if len(filtered) > 0 {
			result = append(result, filtered)
		}
	}
	return result
}

func init() {
	rootCmd.AddCommand(releaseCmd)
	releaseCmd.Flags().BoolVar(&resume, "resume", false, "resume from a previous failed run")
	releaseCmd.Flags().StringArrayVar(&skipLibs, "skip", nil, "treat library as already released (requires --resume)")
	releaseCmd.Flags().BoolVar(&dryRun, "dry-run", false, "show the release plan without executing any release steps")
	releaseCmd.Flags().StringSliceVar(&onlyLibs, "only", nil, "release only these libraries (comma-separated)")
	releaseCmd.Flags().StringArrayVar(&summaryFlags, "summary", nil, "prepend summary text to changelog for a library (libname=text, repeatable)")
	releaseCmd.Flags().StringArrayVar(&summaryFileFlags, "summary-file", nil, "prepend summary file to changelog for a library (libname=/path, repeatable)")
	releaseCmd.Flags().BoolVar(&noAutoSkip, "no-auto-skip", false, "release all libraries even if unchanged since last release (cannot be combined with --resume)")
}

func parseSummaryFlags(flags []string, libs map[string]config.Library) (map[string]string, error) {
	if len(flags) == 0 {
		return nil, nil
	}
	result := make(map[string]string, len(flags))
	for _, f := range flags {
		idx := strings.IndexByte(f, '=')
		if idx < 0 {
			return nil, fmt.Errorf("--summary: expected libname=text, got %q", f)
		}
		name, text := f[:idx], f[idx+1:]
		if name == "" {
			return nil, fmt.Errorf("--summary: missing library name in %q", f)
		}
		if _, ok := libs[name]; !ok {
			return nil, fmt.Errorf("--summary: unknown library %q", name)
		}
		if text == "" {
			return nil, fmt.Errorf("--summary: empty summary text for library %q", name)
		}
		if _, exists := result[name]; exists {
			return nil, fmt.Errorf("--summary: library %q specified more than once", name)
		}
		result[name] = text
	}
	return result, nil
}

func parseSummaryFileFlags(flags []string, libs map[string]config.Library) (map[string]string, error) {
	if len(flags) == 0 {
		return nil, nil
	}
	result := make(map[string]string, len(flags))
	for _, f := range flags {
		idx := strings.IndexByte(f, '=')
		if idx < 0 {
			return nil, fmt.Errorf("--summary-file: expected libname=/path, got %q", f)
		}
		name, path := f[:idx], f[idx+1:]
		if name == "" {
			return nil, fmt.Errorf("--summary-file: missing library name in %q", f)
		}
		if _, ok := libs[name]; !ok {
			return nil, fmt.Errorf("--summary-file: unknown library %q", name)
		}
		if path == "" {
			return nil, fmt.Errorf("--summary-file: empty path for library %q", name)
		}
		if _, exists := result[name]; exists {
			return nil, fmt.Errorf("--summary-file: library %q specified more than once", name)
		}
		if _, err := os.Stat(path); err != nil {
			return nil, fmt.Errorf("--summary-file: file for library %q: %w", name, err)
		}
		result[name] = path
	}
	return result, nil
}
