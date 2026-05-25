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

func defaultOpts() release.Options {
	return release.Options{
		GroupID:      "org.kiwiproject",
		MaxWait:      time.Minute,
		PollInterval: time.Second,
		Checker:      &fakeCentral{},
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

func TestExecute_singleLibrarySuccess(t *testing.T) {
	dir := t.TempDir()
	ws := workspace.New(dir, &runner.FakeRunner{})

	fr := &runner.FakeRunner{}
	fr.AddResponse(&runner.Result{}, nil) // single mvn invocation

	stages := makeStages(
		plan.Entry{Name: "kiwi", Stage: 1, VersionPlan: mustPlan("kiwi", "2.5.1-SNAPSHOT", "")},
	)

	var buf bytes.Buffer
	err := release.Execute(&buf, stages, ws, fr, t.TempDir(), defaultOpts())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fr.CallCount() != 1 {
		t.Errorf("expected 1 runner call, got %d", fr.CallCount())
	}
	out := buf.String()
	if !strings.Contains(out, "done") {
		t.Errorf("expected 'done' in output:\n%s", out)
	}
	if !strings.Contains(out, "kiwi") {
		t.Errorf("expected library name in output:\n%s", out)
	}
}

func TestExecute_multipleStages(t *testing.T) {
	dir := t.TempDir()
	ws := workspace.New(dir, &runner.FakeRunner{})

	fr := &runner.FakeRunner{}
	for range 2 { // 2 libraries × 1 mvn call each
		fr.AddResponse(&runner.Result{}, nil)
	}

	stages := makeStages(
		plan.Entry{Name: "kiwi-parent", Stage: 1, VersionPlan: mustPlan("kiwi-parent", "3.0.0-SNAPSHOT", "")},
		plan.Entry{Name: "kiwi", Stage: 2, VersionPlan: mustPlan("kiwi", "2.5.1-SNAPSHOT", "")},
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
	for range 2 { // 2 libraries × 1 mvn call each
		fr.AddResponse(&runner.Result{}, nil)
	}

	stages := makeStages(
		plan.Entry{Name: "kiwi", Stage: 1, VersionPlan: mustPlan("kiwi", "2.5.1-SNAPSHOT", "")},
		plan.Entry{Name: "kiwi-test", Stage: 1, VersionPlan: mustPlan("kiwi-test", "3.0.0-SNAPSHOT", "")},
	)

	var buf bytes.Buffer
	err := release.Execute(&buf, stages, ws, fr, t.TempDir(), defaultOpts())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fr.CallCount() != 2 {
		t.Errorf("expected 2 runner calls, got %d", fr.CallCount())
	}
}

func TestExecute_mvnFailureHalts(t *testing.T) {
	dir := t.TempDir()
	ws := workspace.New(dir, &runner.FakeRunner{})

	fr := &runner.FakeRunner{}
	fr.AddResponse(nil, errors.New("exit status 1"))

	stages := makeStages(
		plan.Entry{Name: "kiwi", Stage: 1, VersionPlan: mustPlan("kiwi", "2.5.1-SNAPSHOT", "")},
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
	fr.AddResponse(nil, errors.New("exit status 1")) // stage 1 fails
	// stage 2 should never run

	stages := makeStages(
		plan.Entry{Name: "kiwi-parent", Stage: 1, VersionPlan: mustPlan("kiwi-parent", "3.0.0-SNAPSHOT", "")},
		plan.Entry{Name: "kiwi", Stage: 2, VersionPlan: mustPlan("kiwi", "2.5.1-SNAPSHOT", "")},
	)

	var buf bytes.Buffer
	err := release.Execute(&buf, stages, ws, fr, t.TempDir(), defaultOpts())
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if fr.CallCount() != 1 {
		t.Errorf("expected 1 runner call (stage 2 should not run), got %d", fr.CallCount())
	}
}

func TestExecute_centralCheckFailureHalts(t *testing.T) {
	dir := t.TempDir()
	ws := workspace.New(dir, &runner.FakeRunner{})

	fr := &runner.FakeRunner{}
	fr.AddResponse(&runner.Result{}, nil) // mvn succeeds

	opts := release.Options{
		GroupID:      "org.kiwiproject",
		MaxWait:      time.Minute,
		PollInterval: time.Second,
		Checker:      &fakeCentral{err: errors.New("HTTP 404")},
	}

	stages := makeStages(
		plan.Entry{Name: "kiwi", Stage: 1, VersionPlan: mustPlan("kiwi", "2.5.1-SNAPSHOT", "")},
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

func TestExecute_verifyMvnArgs(t *testing.T) {
	dir := t.TempDir()
	ws := workspace.New(dir, &runner.FakeRunner{})

	fr := &runner.FakeRunner{}
	fr.AddResponse(&runner.Result{}, nil)

	stages := makeStages(
		plan.Entry{Name: "kiwi", Stage: 1, VersionPlan: mustPlan("kiwi", "2.5.1-SNAPSHOT", "")},
	)

	release.Execute(&bytes.Buffer{}, stages, ws, fr, t.TempDir(), defaultOpts()) //nolint:errcheck

	call := fr.Calls[0]
	if call.Command != "mvn" {
		t.Errorf("expected mvn, got: %s", call.Command)
	}
	args := strings.Join(call.Args, " ")
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
