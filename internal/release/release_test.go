package release_test

import (
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/kiwiproject/kiwi-star-deployer/internal/plan"
	"github.com/kiwiproject/kiwi-star-deployer/internal/release"
	"github.com/kiwiproject/kiwi-star-deployer/internal/runner"
	"github.com/kiwiproject/kiwi-star-deployer/internal/version"
	"github.com/kiwiproject/kiwi-star-deployer/internal/workspace"
)

type fakeCentral struct{ err error }

func (f *fakeCentral) Wait(_ io.Writer, _, _, _ string, _, _ time.Duration) error {
	return f.err
}

type fakeCIChecker struct{ err error }

func (f *fakeCIChecker) Wait(_ io.Writer, _, _ string, _, _ time.Duration) error {
	return f.err
}

func defaultOpts() release.Options {
	return release.Options{
		GroupID:         "org.kiwiproject",
		MaxWait:         time.Minute,
		PollInterval:    time.Second,
		Checker:         &fakeCentral{},
		ChangelogScript: ".generate-kiwi-changelog",
	}
}

func mustPlan(name, pomVersion, override string) version.Plan {
	vp, err := version.Compute(name, pomVersion, override)
	if err != nil {
		panic(err)
	}
	return vp
}

func makeStages(entries ...plan.Entry) [][]plan.Entry {
	stages := make([][]plan.Entry, 0)
	for _, e := range entries {
		for len(stages) < e.Stage {
			stages = append(stages, nil)
		}
		stages[e.Stage-1] = append(stages[e.Stage-1], e)
	}
	return stages
}

// addLibraryResponses adds the three runner responses for one successful library release:
// git describe, mvn, and changelog.
func addLibraryResponses(fr *runner.FakeRunner, previousTag string) {
	fr.AddResponse(&runner.Result{Stdout: previousTag}, nil) // git describe
	fr.AddResponse(&runner.Result{}, nil)                   // mvn
	fr.AddResponse(&runner.Result{}, nil)                   // changelog
}

func TestExecute_singleLibrarySuccess(t *testing.T) {
	dir := t.TempDir()
	ws := workspace.New(dir, &runner.FakeRunner{})

	fr := &runner.FakeRunner{}
	addLibraryResponses(fr, "v2.5.0")

	stages := makeStages(
		plan.Entry{Name: "kiwi", Repo: "kiwiproject/kiwi", Stage: 1, VersionPlan: mustPlan("kiwi", "2.5.1-SNAPSHOT", "")},
	)

	var buf bytes.Buffer
	err := release.Execute(&buf, stages, ws, fr, t.TempDir(), defaultOpts())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fr.CallCount() != 3 {
		t.Errorf("expected 3 runner calls, got %d", fr.CallCount())
	}
	out := buf.String()
	if !strings.Contains(out, "done") {
		t.Errorf("expected 'done' in output:\n%s", out)
	}
	if !strings.Contains(out, "kiwi") {
		t.Errorf("expected library name in output:\n%s", out)
	}
}

func TestExecute_completionMessageSingular(t *testing.T) {
	dir := t.TempDir()
	ws := workspace.New(dir, &runner.FakeRunner{})

	fr := &runner.FakeRunner{}
	addLibraryResponses(fr, "v2.5.0")

	stages := makeStages(
		plan.Entry{Name: "kiwi", Repo: "kiwiproject/kiwi", Stage: 1, VersionPlan: mustPlan("kiwi", "2.5.1-SNAPSHOT", "")},
	)

	var buf bytes.Buffer
	if err := release.Execute(&buf, stages, ws, fr, t.TempDir(), defaultOpts()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(buf.String(), "Released 1 library.") {
		t.Errorf("expected singular completion message, got:\n%s", buf.String())
	}
}

func TestExecute_completionMessagePlural(t *testing.T) {
	dir := t.TempDir()
	ws := workspace.New(dir, &runner.FakeRunner{})

	fr := &runner.FakeRunner{}
	addLibraryResponses(fr, "v2.9.0")
	addLibraryResponses(fr, "v2.5.0")

	stages := makeStages(
		plan.Entry{Name: "kiwi-parent", Repo: "kiwiproject/kiwi-parent", Stage: 1, VersionPlan: mustPlan("kiwi-parent", "3.0.0-SNAPSHOT", "")},
		plan.Entry{Name: "kiwi", Repo: "kiwiproject/kiwi", Stage: 2, VersionPlan: mustPlan("kiwi", "2.5.1-SNAPSHOT", "")},
	)

	var buf bytes.Buffer
	if err := release.Execute(&buf, stages, ws, fr, t.TempDir(), defaultOpts()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(buf.String(), "Released 2 libraries.") {
		t.Errorf("expected plural completion message, got:\n%s", buf.String())
	}
}

func TestExecute_multipleStages(t *testing.T) {
	dir := t.TempDir()
	ws := workspace.New(dir, &runner.FakeRunner{})

	fr := &runner.FakeRunner{}
	addLibraryResponses(fr, "v2.9.0")
	addLibraryResponses(fr, "v2.5.0")

	stages := makeStages(
		plan.Entry{Name: "kiwi-parent", Repo: "kiwiproject/kiwi-parent", Stage: 1, VersionPlan: mustPlan("kiwi-parent", "3.0.0-SNAPSHOT", "")},
		plan.Entry{Name: "kiwi", Repo: "kiwiproject/kiwi", Stage: 2, VersionPlan: mustPlan("kiwi", "2.5.1-SNAPSHOT", "")},
	)

	var buf bytes.Buffer
	err := release.Execute(&buf, stages, ws, fr, t.TempDir(), defaultOpts())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "Stage 1:") {
		t.Errorf("expected Stage 1 in output:\n%s", out)
	}
	if !strings.Contains(out, "Stage 2:") {
		t.Errorf("expected Stage 2 in output:\n%s", out)
	}
}

func TestExecute_parallelWithinStage(t *testing.T) {
	dir := t.TempDir()
	ws := workspace.New(dir, &runner.FakeRunner{})

	fr := &runner.FakeRunner{}
	addLibraryResponses(fr, "v2.5.0")
	addLibraryResponses(fr, "v2.9.0")

	stages := makeStages(
		plan.Entry{Name: "kiwi", Repo: "kiwiproject/kiwi", Stage: 1, VersionPlan: mustPlan("kiwi", "2.5.1-SNAPSHOT", "")},
		plan.Entry{Name: "kiwi-test", Repo: "kiwiproject/kiwi-test", Stage: 1, VersionPlan: mustPlan("kiwi-test", "3.0.0-SNAPSHOT", "")},
	)

	var buf bytes.Buffer
	err := release.Execute(&buf, stages, ws, fr, t.TempDir(), defaultOpts())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fr.CallCount() != 6 {
		t.Errorf("expected 6 runner calls, got %d", fr.CallCount())
	}
}

func TestExecute_mvnFailureHalts(t *testing.T) {
	dir := t.TempDir()
	ws := workspace.New(dir, &runner.FakeRunner{})

	fr := &runner.FakeRunner{}
	fr.AddResponse(&runner.Result{Stdout: "v2.5.0"}, nil) // git describe succeeds
	fr.AddResponse(nil, errors.New("exit status 1"))      // mvn fails

	stages := makeStages(
		plan.Entry{Name: "kiwi", Repo: "kiwiproject/kiwi", Stage: 1, VersionPlan: mustPlan("kiwi", "2.5.1-SNAPSHOT", "")},
	)

	var buf bytes.Buffer
	err := release.Execute(&buf, stages, ws, fr, t.TempDir(), defaultOpts())
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	out := buf.String()
	if !strings.Contains(out, "FAILED") {
		t.Errorf("expected FAILED in output:\n%s", out)
	}
}

func TestExecute_stageFailureHaltsBeforeNextStage(t *testing.T) {
	dir := t.TempDir()
	ws := workspace.New(dir, &runner.FakeRunner{})

	fr := &runner.FakeRunner{}
	fr.AddResponse(&runner.Result{Stdout: "v2.9.0"}, nil) // git describe succeeds
	fr.AddResponse(nil, errors.New("exit status 1"))      // stage 1 mvn fails
	// stage 2 should never run

	stages := makeStages(
		plan.Entry{Name: "kiwi-parent", Repo: "kiwiproject/kiwi-parent", Stage: 1, VersionPlan: mustPlan("kiwi-parent", "3.0.0-SNAPSHOT", "")},
		plan.Entry{Name: "kiwi", Repo: "kiwiproject/kiwi", Stage: 2, VersionPlan: mustPlan("kiwi", "2.5.1-SNAPSHOT", "")},
	)

	var buf bytes.Buffer
	err := release.Execute(&buf, stages, ws, fr, t.TempDir(), defaultOpts())
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if fr.CallCount() != 2 {
		t.Errorf("expected 2 runner calls (stage 2 should not run), got %d", fr.CallCount())
	}
}

func TestExecute_centralCheckFailureHalts(t *testing.T) {
	dir := t.TempDir()
	ws := workspace.New(dir, &runner.FakeRunner{})

	fr := &runner.FakeRunner{}
	fr.AddResponse(&runner.Result{Stdout: "v2.5.0"}, nil) // git describe
	fr.AddResponse(&runner.Result{}, nil)                 // mvn succeeds

	opts := release.Options{
		GroupID:         "org.kiwiproject",
		MaxWait:         time.Minute,
		PollInterval:    time.Second,
		Checker:         &fakeCentral{err: errors.New("HTTP 404")},
		ChangelogScript: ".generate-kiwi-changelog",
	}

	stages := makeStages(
		plan.Entry{Name: "kiwi", Repo: "kiwiproject/kiwi", Stage: 1, VersionPlan: mustPlan("kiwi", "2.5.1-SNAPSHOT", "")},
	)

	var buf bytes.Buffer
	err := release.Execute(&buf, stages, ws, fr, t.TempDir(), opts)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	out := buf.String()
	if !strings.Contains(out, "FAILED") {
		t.Errorf("expected FAILED in output:\n%s", out)
	}
}

func TestExecute_changelogFailureHalts(t *testing.T) {
	dir := t.TempDir()
	ws := workspace.New(dir, &runner.FakeRunner{})

	fr := &runner.FakeRunner{}
	fr.AddResponse(&runner.Result{Stdout: "v2.5.0"}, nil) // git describe
	fr.AddResponse(&runner.Result{}, nil)                 // mvn succeeds
	fr.AddResponse(nil, errors.New("exit status 1"))      // changelog fails

	stages := makeStages(
		plan.Entry{Name: "kiwi", Repo: "kiwiproject/kiwi", Stage: 1, VersionPlan: mustPlan("kiwi", "2.5.1-SNAPSHOT", "")},
	)

	var buf bytes.Buffer
	err := release.Execute(&buf, stages, ws, fr, t.TempDir(), defaultOpts())
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	out := buf.String()
	if !strings.Contains(out, "FAILED") {
		t.Errorf("expected FAILED in output:\n%s", out)
	}
}

func TestExecute_updatesDownstreamPOMs(t *testing.T) {
	dir := t.TempDir()
	ws := workspace.New(dir, &runner.FakeRunner{})

	fr := &runner.FakeRunner{}
	addLibraryResponses(fr, "v2.9.0") // kiwi-parent release (git describe, mvn, changelog)
	fr.AddResponse(&runner.Result{}, nil) // mvn versions:use-dep-version (kiwi POM update)
	fr.AddResponse(&runner.Result{}, nil) // git add
	fr.AddResponse(&runner.Result{}, nil) // git commit
	fr.AddResponse(&runner.Result{}, nil) // git push
	addLibraryResponses(fr, "v2.5.0") // kiwi release

	stages := makeStages(
		plan.Entry{Name: "kiwi-parent", Repo: "kiwiproject/kiwi-parent", Stage: 1, VersionPlan: mustPlan("kiwi-parent", "3.0.0-SNAPSHOT", "")},
		plan.Entry{Name: "kiwi", Repo: "kiwiproject/kiwi", Stage: 2, DependsOn: []string{"kiwi-parent"}, VersionPlan: mustPlan("kiwi", "2.5.1-SNAPSHOT", "")},
	)

	var buf bytes.Buffer
	err := release.Execute(&buf, stages, ws, fr, t.TempDir(), defaultOpts())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fr.CallCount() != 10 {
		t.Errorf("expected 10 runner calls, got %d", fr.CallCount())
	}
	out := buf.String()
	if !strings.Contains(out, "POM update") {
		t.Errorf("expected POM update in output:\n%s", out)
	}
}

func TestExecute_updatesLibraryBOMPOMs(t *testing.T) {
	dir := t.TempDir()
	ws := workspace.New(dir, &runner.FakeRunner{})

	fr := &runner.FakeRunner{}
	addLibraryResponses(fr, "v2.9.0")     // kiwi-parent release
	fr.AddResponse(&runner.Result{}, nil) // mvn versions:use-dep-version
	fr.AddResponse(&runner.Result{}, nil) // git add
	fr.AddResponse(&runner.Result{}, nil) // git commit
	fr.AddResponse(&runner.Result{}, nil) // git push
	addLibraryResponses(fr, "v1.0.0") // kiwi-libraries-bom release

	stages := makeStages(
		plan.Entry{Name: "kiwi-parent", Repo: "kiwiproject/kiwi-parent", Stage: 1, VersionPlan: mustPlan("kiwi-parent", "3.0.0-SNAPSHOT", "")},
		plan.Entry{Name: "kiwi-libraries-bom", Repo: "kiwiproject/kiwi-libraries-bom", Stage: 2, Type: "library-bom", DependsOn: []string{"kiwi-parent"}, VersionPlan: mustPlan("kiwi-libraries-bom", "2.0.0-SNAPSHOT", "")},
	)

	var buf bytes.Buffer
	err := release.Execute(&buf, stages, ws, fr, t.TempDir(), defaultOpts())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fr.CallCount() != 10 {
		t.Errorf("expected 10 runner calls (with POM update), got %d", fr.CallCount())
	}
	out := buf.String()
	if !strings.Contains(out, "POM update") {
		t.Errorf("expected POM update in output:\n%s", out)
	}
}

func TestExecute_pomUpdateFailureHalts(t *testing.T) {
	dir := t.TempDir()
	ws := workspace.New(dir, &runner.FakeRunner{})

	fr := &runner.FakeRunner{}
	addLibraryResponses(fr, "v2.9.0")                    // kiwi-parent release succeeds
	fr.AddResponse(nil, errors.New("exit status 1"))     // mvn versions fails

	stages := makeStages(
		plan.Entry{Name: "kiwi-parent", Repo: "kiwiproject/kiwi-parent", Stage: 1, VersionPlan: mustPlan("kiwi-parent", "3.0.0-SNAPSHOT", "")},
		plan.Entry{Name: "kiwi", Repo: "kiwiproject/kiwi", Stage: 2, DependsOn: []string{"kiwi-parent"}, VersionPlan: mustPlan("kiwi", "2.5.1-SNAPSHOT", "")},
	)

	var buf bytes.Buffer
	err := release.Execute(&buf, stages, ws, fr, t.TempDir(), defaultOpts())
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if fr.CallCount() != 4 {
		t.Errorf("expected 4 runner calls, got %d", fr.CallCount())
	}
}

func TestExecute_verifyPOMUpdateArgs(t *testing.T) {
	dir := t.TempDir()
	ws := workspace.New(dir, &runner.FakeRunner{})

	fr := &runner.FakeRunner{}
	addLibraryResponses(fr, "v2.9.0") // kiwi-parent release
	fr.AddResponse(&runner.Result{}, nil) // mvn versions
	fr.AddResponse(&runner.Result{}, nil) // git add
	fr.AddResponse(&runner.Result{}, nil) // git commit
	fr.AddResponse(&runner.Result{}, nil) // git push
	addLibraryResponses(fr, "v2.5.0") // kiwi release

	stages := makeStages(
		plan.Entry{Name: "kiwi-parent", Repo: "kiwiproject/kiwi-parent", Stage: 1, VersionPlan: mustPlan("kiwi-parent", "3.0.0-SNAPSHOT", "")},
		plan.Entry{Name: "kiwi", Repo: "kiwiproject/kiwi", Stage: 2, DependsOn: []string{"kiwi-parent"}, VersionPlan: mustPlan("kiwi", "2.5.1-SNAPSHOT", "")},
	)

	release.Execute(&bytes.Buffer{}, stages, ws, fr, t.TempDir(), defaultOpts()) //nolint:errcheck

	// Calls: [0]=git describe, [1]=mvn release, [2]=changelog, [3]=mvn versions, [4]=git add, [5]=git commit, [6]=git push, ...
	mvnVersionsCall := fr.Calls[3]
	if mvnVersionsCall.Command != "mvn" {
		t.Errorf("expected mvn, got %s", mvnVersionsCall.Command)
	}
	versionArgs := strings.Join(mvnVersionsCall.Args, " ")
	for _, want := range []string{
		"versions:use-dep-version",
		"-Dincludes=org.kiwiproject:kiwi-parent",
		"-DdepVersion=3.0.0",
		"-DgenerateBackupPoms=false",
	} {
		if !strings.Contains(versionArgs, want) {
			t.Errorf("expected %q in mvn versions args: %s", want, versionArgs)
		}
	}

	commitCall := fr.Calls[5]
	if commitCall.Command != "git" {
		t.Errorf("expected git, got %s", commitCall.Command)
	}
	commitMsg := strings.Join(commitCall.Args, " ")
	if !strings.Contains(commitMsg, "chore: update dependency versions") {
		t.Errorf("expected summary line in commit message: %s", commitMsg)
	}
	if !strings.Contains(commitMsg, "- kiwi-parent 3.0.0") {
		t.Errorf("expected bullet point in commit message: %s", commitMsg)
	}
}

func TestExecute_verifyMvnArgs(t *testing.T) {
	dir := t.TempDir()
	ws := workspace.New(dir, &runner.FakeRunner{})

	fr := &runner.FakeRunner{}
	addLibraryResponses(fr, "v2.5.0")

	stages := makeStages(
		plan.Entry{Name: "kiwi", Repo: "kiwiproject/kiwi", Stage: 1, VersionPlan: mustPlan("kiwi", "2.5.1-SNAPSHOT", "")},
	)

	release.Execute(&bytes.Buffer{}, stages, ws, fr, t.TempDir(), defaultOpts()) //nolint:errcheck

	mvnCall := fr.Calls[1] // Calls[0] is git describe
	if mvnCall.Command != "mvn" {
		t.Errorf("expected mvn, got: %s", mvnCall.Command)
	}
	args := strings.Join(mvnCall.Args, " ")
	for _, want := range []string{
		"clean",
		"release:clean",
		"release:prepare",
		"release:perform",
		"-Darguments=-DskipTests",
		"-DreleaseVersion=2.5.1",
		"-DdevelopmentVersion=2.5.2-SNAPSHOT",
		"-e",
	} {
		if !strings.Contains(args, want) {
			t.Errorf("expected %q in mvn args: %s", want, args)
		}
	}
}

func TestExecute_completedLibraryIsSkipped(t *testing.T) {
	dir := t.TempDir()
	ws := workspace.New(dir, &runner.FakeRunner{})

	fr := &runner.FakeRunner{}
	// kiwi-parent is in Completed — no runner calls for it
	fr.AddResponse(&runner.Result{}, nil) // mvn versions:use-dep-version (POM update for kiwi)
	fr.AddResponse(&runner.Result{}, nil) // git add
	fr.AddResponse(&runner.Result{}, nil) // git commit
	fr.AddResponse(&runner.Result{}, nil) // git push
	addLibraryResponses(fr, "v2.5.0")    // kiwi: git describe, mvn, changelog

	stages := makeStages(
		plan.Entry{Name: "kiwi-parent", Repo: "kiwiproject/kiwi-parent", Stage: 1, VersionPlan: mustPlan("kiwi-parent", "3.0.0-SNAPSHOT", "")},
		plan.Entry{Name: "kiwi", Repo: "kiwiproject/kiwi", Stage: 2, DependsOn: []string{"kiwi-parent"}, VersionPlan: mustPlan("kiwi", "2.5.1-SNAPSHOT", "")},
	)

	opts := defaultOpts()
	opts.Completed = map[string]string{"kiwi-parent": "3.0.0"}

	var buf bytes.Buffer
	err := release.Execute(&buf, stages, ws, fr, t.TempDir(), opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "skip") {
		t.Errorf("expected 'skip' in output:\n%s", out)
	}
	if !strings.Contains(out, "kiwi-parent") {
		t.Errorf("expected kiwi-parent in output:\n%s", out)
	}
	if !strings.Contains(out, "3.0.0") {
		t.Errorf("expected version 3.0.0 in output:\n%s", out)
	}
	// 4 (POM update) + 3 (kiwi release) = 7 calls; no calls for kiwi-parent
	if fr.CallCount() != 7 {
		t.Errorf("expected 7 runner calls, got %d", fr.CallCount())
	}
}

func TestExecute_skipResolvesVersionFromGitTag(t *testing.T) {
	dir := t.TempDir()
	ws := workspace.New(dir, &runner.FakeRunner{})

	fr := &runner.FakeRunner{}
	fr.AddResponse(&runner.Result{Stdout: "v3.0.0\n"}, nil) // git describe for kiwi-parent (--skip)
	// kiwi-parent is skipped — no release calls for it
	addLibraryResponses(fr, "v2.5.0") // kiwi: git describe, mvn, changelog

	stages := makeStages(
		plan.Entry{Name: "kiwi-parent", Repo: "kiwiproject/kiwi-parent", Stage: 1, VersionPlan: mustPlan("kiwi-parent", "3.0.0-SNAPSHOT", "")},
		plan.Entry{Name: "kiwi", Repo: "kiwiproject/kiwi", Stage: 2, VersionPlan: mustPlan("kiwi", "2.5.1-SNAPSHOT", "")},
	)

	opts := defaultOpts()
	opts.Skip = []string{"kiwi-parent"}

	var buf bytes.Buffer
	err := release.Execute(&buf, stages, ws, fr, t.TempDir(), opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "skip") {
		t.Errorf("expected 'skip' in output:\n%s", out)
	}
	if !strings.Contains(out, "3.0.0") {
		t.Errorf("expected version 3.0.0 in output:\n%s", out)
	}
	// 1 (git describe for skip) + 3 (kiwi release) = 4 calls
	if fr.CallCount() != 4 {
		t.Errorf("expected 4 runner calls, got %d", fr.CallCount())
	}
}

func TestExecute_skippedVersionUsedInPOMUpdate(t *testing.T) {
	dir := t.TempDir()
	ws := workspace.New(dir, &runner.FakeRunner{})

	fr := &runner.FakeRunner{}
	// kiwi-parent completed as 2.9.0, but plan would compute 3.0.0 from 3.0.0-SNAPSHOT
	fr.AddResponse(&runner.Result{}, nil) // mvn versions:use-dep-version (should use 2.9.0)
	fr.AddResponse(&runner.Result{}, nil) // git add
	fr.AddResponse(&runner.Result{}, nil) // git commit
	fr.AddResponse(&runner.Result{}, nil) // git push
	addLibraryResponses(fr, "v2.5.0")    // kiwi: git describe, mvn, changelog

	stages := makeStages(
		plan.Entry{Name: "kiwi-parent", Repo: "kiwiproject/kiwi-parent", Stage: 1, VersionPlan: mustPlan("kiwi-parent", "3.0.0-SNAPSHOT", "")},
		plan.Entry{Name: "kiwi", Repo: "kiwiproject/kiwi", Stage: 2, DependsOn: []string{"kiwi-parent"}, VersionPlan: mustPlan("kiwi", "2.5.1-SNAPSHOT", "")},
	)

	opts := defaultOpts()
	opts.Completed = map[string]string{"kiwi-parent": "2.9.0"}

	release.Execute(&bytes.Buffer{}, stages, ws, fr, t.TempDir(), opts) //nolint:errcheck

	mvnVersionsCall := fr.Calls[0]
	versionArgs := strings.Join(mvnVersionsCall.Args, " ")
	if !strings.Contains(versionArgs, "-DdepVersion=2.9.0") {
		t.Errorf("expected -DdepVersion=2.9.0 in POM update args: %s", versionArgs)
	}
	if strings.Contains(versionArgs, "3.0.0") {
		t.Errorf("unexpected 3.0.0 in POM update args: %s", versionArgs)
	}
}

func TestExecute_ciVerificationPassesAfterPOMUpdate(t *testing.T) {
	dir := t.TempDir()
	ws := workspace.New(dir, &runner.FakeRunner{})

	fr := &runner.FakeRunner{}
	addLibraryResponses(fr, "v2.9.0")     // kiwi-parent: git describe, mvn, changelog
	fr.AddResponse(&runner.Result{}, nil) // mvn versions (POM update for kiwi)
	fr.AddResponse(&runner.Result{}, nil) // git add
	fr.AddResponse(&runner.Result{}, nil) // git commit
	fr.AddResponse(&runner.Result{}, nil) // git push
	// fakeCIChecker makes no runner calls; git rev-parse HEAD is called by release.go
	fr.AddResponse(&runner.Result{Stdout: "abc123\n"}, nil) // git rev-parse HEAD
	addLibraryResponses(fr, "v2.5.0")                      // kiwi: git describe, mvn, changelog

	stages := makeStages(
		plan.Entry{Name: "kiwi-parent", Repo: "kiwiproject/kiwi-parent", Stage: 1, VersionPlan: mustPlan("kiwi-parent", "3.0.0-SNAPSHOT", "")},
		plan.Entry{Name: "kiwi", Repo: "kiwiproject/kiwi", Stage: 2, DependsOn: []string{"kiwi-parent"}, VersionPlan: mustPlan("kiwi", "2.5.1-SNAPSHOT", "")},
	)

	opts := defaultOpts()
	opts.CIChecker = &fakeCIChecker{}
	opts.CIMaxWait = time.Minute
	opts.CIPollInterval = time.Millisecond

	var buf bytes.Buffer
	err := release.Execute(&buf, stages, ws, fr, t.TempDir(), opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fr.CallCount() != 11 {
		t.Errorf("expected 11 runner calls, got %d", fr.CallCount())
	}
	out := buf.String()
	if !strings.Contains(out, "CI verify") {
		t.Errorf("expected 'CI verify' in output:\n%s", out)
	}
	if !strings.Contains(out, "CI passed") {
		t.Errorf("expected 'CI passed' in output:\n%s", out)
	}
}

func TestExecute_ciVerificationFailureHalts(t *testing.T) {
	dir := t.TempDir()
	ws := workspace.New(dir, &runner.FakeRunner{})

	fr := &runner.FakeRunner{}
	addLibraryResponses(fr, "v2.9.0")     // kiwi-parent: git describe, mvn, changelog
	fr.AddResponse(&runner.Result{}, nil) // mvn versions (POM update for kiwi)
	fr.AddResponse(&runner.Result{}, nil) // git add
	fr.AddResponse(&runner.Result{}, nil) // git commit
	fr.AddResponse(&runner.Result{}, nil) // git push
	fr.AddResponse(&runner.Result{Stdout: "abc123\n"}, nil) // git rev-parse HEAD
	// kiwi release should NOT run after CI failure

	stages := makeStages(
		plan.Entry{Name: "kiwi-parent", Repo: "kiwiproject/kiwi-parent", Stage: 1, VersionPlan: mustPlan("kiwi-parent", "3.0.0-SNAPSHOT", "")},
		plan.Entry{Name: "kiwi", Repo: "kiwiproject/kiwi", Stage: 2, DependsOn: []string{"kiwi-parent"}, VersionPlan: mustPlan("kiwi", "2.5.1-SNAPSHOT", "")},
	)

	opts := defaultOpts()
	opts.CIChecker = &fakeCIChecker{err: errors.New("CI failed: conclusion: failure")}
	opts.CIMaxWait = time.Minute
	opts.CIPollInterval = time.Millisecond

	var buf bytes.Buffer
	err := release.Execute(&buf, stages, ws, fr, t.TempDir(), opts)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "CI verification") {
		t.Errorf("expected 'CI verification' in error: %v", err)
	}
	// 3 (kiwi-parent) + 4 (POM update) + 1 (git rev-parse HEAD) = 8 calls; kiwi release never starts
	if fr.CallCount() != 8 {
		t.Errorf("expected 8 runner calls, got %d", fr.CallCount())
	}
}

func TestExecute_verifyChangelogArgs(t *testing.T) {
	dir := t.TempDir()
	ws := workspace.New(dir, &runner.FakeRunner{})

	fr := &runner.FakeRunner{}
	addLibraryResponses(fr, "v2.5.0")

	stages := makeStages(
		plan.Entry{Name: "kiwi", Repo: "kiwiproject/kiwi", Stage: 1, VersionPlan: mustPlan("kiwi", "2.5.1-SNAPSHOT", "")},
	)

	release.Execute(&bytes.Buffer{}, stages, ws, fr, t.TempDir(), defaultOpts()) //nolint:errcheck

	changelogCall := fr.Calls[2] // Calls[0]=git describe, Calls[1]=mvn
	if changelogCall.Command != ".generate-kiwi-changelog" {
		t.Errorf("expected .generate-kiwi-changelog, got: %s", changelogCall.Command)
	}
	args := strings.Join(changelogCall.Args, " ")
	for _, want := range []string{
		"--repository kiwiproject/kiwi",
		"--previous-rev 2.5.0",
		"--revision 2.5.1",
		"--output-type GITHUB",
		"--close-milestone",
		"--create-next-milestone 2.5.2",
		"--add-v-prefix-to-revisions",
	} {
		if !strings.Contains(args, want) {
			t.Errorf("expected %q in changelog args: %s", want, args)
		}
	}
}
