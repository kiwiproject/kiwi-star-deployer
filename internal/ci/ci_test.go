package ci_test

import (
	"bytes"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/kiwiproject/kiwi-star-deployer/internal/ci"
	"github.com/kiwiproject/kiwi-star-deployer/internal/runner"
)

const (
	runListOne    = `[{"databaseId":1,"status":"queued"}]`
	runListTwo    = `[{"databaseId":1,"status":"queued"},{"databaseId":2,"status":"queued"}]`
	runListEmpty  = `[]`
	runInProgress = `{"status":"in_progress","conclusion":""}`
	workflowsOne  = "1"
	workflowsNone = "0"
	runSuccess    = `{"status":"completed","conclusion":"success"}`
	runFailure    = `{"status":"completed","conclusion":"failure"}`
	runCancelled  = `{"status":"completed","conclusion":"cancelled"}`
)

func newChecker(fr *runner.FakeRunner) *ci.GHChecker {
	return &ci.GHChecker{Runner: fr}
}

func TestGHChecker_runFoundImmediatelyAndPasses(t *testing.T) {
	fr := &runner.FakeRunner{}
	fr.AddResponse(&runner.Result{Stdout: runListOne}, nil) // gh run list
	fr.AddResponse(&runner.Result{Stdout: runSuccess}, nil) // gh run view

	var buf bytes.Buffer
	err := newChecker(fr).Wait(&buf, "kiwiproject/kiwi", "abc123def456", time.Minute, time.Millisecond)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fr.CallCount() != 2 {
		t.Errorf("expected 2 calls, got %d", fr.CallCount())
	}
}

func TestGHChecker_runAppearsAfterRetry(t *testing.T) {
	fr := &runner.FakeRunner{}
	fr.AddResponse(&runner.Result{Stdout: runListEmpty}, nil)   // gh run list (empty)
	fr.AddResponse(&runner.Result{Stdout: workflowsOne}, nil)   // gh api workflows (has CI)
	fr.AddResponse(&runner.Result{Stdout: runListOne}, nil)     // gh run list (found)
	fr.AddResponse(&runner.Result{Stdout: runSuccess}, nil)     // gh run view

	var buf bytes.Buffer
	err := newChecker(fr).Wait(&buf, "kiwiproject/kiwi", "abc123def456", time.Minute, time.Millisecond)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(buf.String(), "waiting for run to appear") {
		t.Errorf("expected waiting message in output:\n%s", buf.String())
	}
	if fr.CallCount() != 4 {
		t.Errorf("expected 4 calls, got %d", fr.CallCount())
	}
}

func TestGHChecker_noActiveWorkflowsSkipsVerification(t *testing.T) {
	fr := &runner.FakeRunner{}
	fr.AddResponse(&runner.Result{Stdout: runListEmpty}, nil)  // gh run list (empty)
	fr.AddResponse(&runner.Result{Stdout: workflowsNone}, nil) // gh api workflows: none

	var buf bytes.Buffer
	err := newChecker(fr).Wait(&buf, "kiwiproject/kiwi", "abc123def456", time.Minute, time.Millisecond)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fr.CallCount() != 2 {
		t.Errorf("expected 2 calls, got %d", fr.CallCount())
	}
	if !strings.Contains(buf.String(), "no active workflows") {
		t.Errorf("expected no-active-workflows message in output:\n%s", buf.String())
	}
}

func TestGHChecker_workflowCheckRunsOnlyOnce(t *testing.T) {
	fr := &runner.FakeRunner{}
	fr.AddResponse(&runner.Result{Stdout: runListEmpty}, nil) // gh run list (empty, cycle 1)
	fr.AddResponse(&runner.Result{Stdout: workflowsOne}, nil) // gh api workflows (once)
	fr.AddResponse(&runner.Result{Stdout: runListEmpty}, nil) // gh run list (empty, cycle 2)
	fr.AddResponse(&runner.Result{Stdout: runListOne}, nil)   // gh run list (found)
	fr.AddResponse(&runner.Result{Stdout: runSuccess}, nil)   // gh run view

	err := newChecker(fr).Wait(&bytes.Buffer{}, "kiwiproject/kiwi", "abc123def456", time.Minute, time.Millisecond)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	workflowCalls := 0
	for _, call := range fr.Calls {
		if strings.Contains(strings.Join(call.Args, " "), "actions/workflows") {
			workflowCalls++
		}
	}
	if workflowCalls != 1 {
		t.Errorf("expected exactly 1 workflow check call, got %d", workflowCalls)
	}
}

func TestGHChecker_workflowCheckErrorContinuesWaiting(t *testing.T) {
	fr := &runner.FakeRunner{}
	fr.AddResponse(&runner.Result{Stdout: runListEmpty}, nil) // gh run list (empty)
	fr.AddResponse(nil, errors.New("exit status 1"))          // gh api workflows fails
	fr.AddResponse(&runner.Result{Stdout: runListOne}, nil)   // gh run list (found)
	fr.AddResponse(&runner.Result{Stdout: runSuccess}, nil)   // gh run view

	var buf bytes.Buffer
	err := newChecker(fr).Wait(&buf, "kiwiproject/kiwi", "abc123def456", time.Minute, time.Millisecond)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(buf.String(), "could not check workflows") {
		t.Errorf("expected workflow check warning in output:\n%s", buf.String())
	}
}

// The merged loop calls listRuns on every cycle, so two cycles means two
// listRuns calls total: one per cycle.
func TestGHChecker_runInProgressThenCompletes(t *testing.T) {
	fr := &runner.FakeRunner{}
	fr.AddResponse(&runner.Result{Stdout: runListOne}, nil)   // gh run list  (cycle 1)
	fr.AddResponse(&runner.Result{Stdout: runInProgress}, nil) // gh run view  (cycle 1)
	fr.AddResponse(&runner.Result{Stdout: runListOne}, nil)   // gh run list  (cycle 2)
	fr.AddResponse(&runner.Result{Stdout: runSuccess}, nil)   // gh run view  (cycle 2)

	var buf bytes.Buffer
	err := newChecker(fr).Wait(&buf, "kiwiproject/kiwi", "abc123def456", time.Minute, time.Millisecond)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fr.CallCount() != 4 {
		t.Errorf("expected 4 calls, got %d", fr.CallCount())
	}
}

func TestGHChecker_multipleRuns(t *testing.T) {
	fr := &runner.FakeRunner{}
	// Both runs complete on the first cycle — no sleep needed.
	fr.AddResponse(&runner.Result{Stdout: runListTwo}, nil) // gh run list: [1, 2]
	// Map iteration order is not guaranteed, so add two success responses
	// for whichever order the two viewRun calls arrive in.
	fr.AddResponse(&runner.Result{Stdout: runSuccess}, nil) // gh run view (one of 1 or 2)
	fr.AddResponse(&runner.Result{Stdout: runSuccess}, nil) // gh run view (the other)

	err := newChecker(fr).Wait(&bytes.Buffer{}, "kiwiproject/kiwi", "abc123def456", time.Minute, time.Millisecond)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fr.CallCount() != 3 {
		t.Errorf("expected 3 calls (1 listRuns + 2 viewRun), got %d", fr.CallCount())
	}
}

func TestGHChecker_runFails(t *testing.T) {
	fr := &runner.FakeRunner{}
	fr.AddResponse(&runner.Result{Stdout: runListOne}, nil) // gh run list
	fr.AddResponse(&runner.Result{Stdout: runFailure}, nil) // gh run view

	var buf bytes.Buffer
	err := newChecker(fr).Wait(&buf, "kiwiproject/kiwi", "abc123def456", time.Minute, time.Millisecond)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "failure") {
		t.Errorf("expected 'failure' in error: %v", err)
	}
	if !strings.Contains(err.Error(), "kiwiproject/kiwi") {
		t.Errorf("expected repo name in error: %v", err)
	}
}

func TestGHChecker_runCancelled(t *testing.T) {
	fr := &runner.FakeRunner{}
	fr.AddResponse(&runner.Result{Stdout: runListOne}, nil)   // gh run list
	fr.AddResponse(&runner.Result{Stdout: runCancelled}, nil) // gh run view

	err := newChecker(fr).Wait(&bytes.Buffer{}, "kiwiproject/kiwi", "abc123def456", time.Minute, time.Millisecond)
	if err == nil {
		t.Fatal("expected error for cancelled run, got nil")
	}
	if !strings.Contains(err.Error(), "cancelled") {
		t.Errorf("expected 'cancelled' in error: %v", err)
	}
}

func TestGHChecker_discoveryTimesOut(t *testing.T) {
	fr := &runner.FakeRunner{}
	fr.AddResponse(&runner.Result{Stdout: runListEmpty}, nil) // first empty poll
	fr.AddResponse(&runner.Result{Stdout: workflowsOne}, nil) // workflows exist; keep waiting
	for i := 0; i < 10; i++ {
		fr.AddResponse(&runner.Result{Stdout: runListEmpty}, nil)
	}

	err := newChecker(fr).Wait(&bytes.Buffer{}, "kiwiproject/kiwi", "abc123def456", 50*time.Millisecond, 10*time.Millisecond)
	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}
	if !strings.Contains(err.Error(), "timed out") {
		t.Errorf("expected 'timed out' in error: %v", err)
	}
	if !strings.Contains(err.Error(), "appear") {
		t.Errorf("expected discovery timeout message (not completion timeout): %v", err)
	}
}

func TestGHChecker_completionTimesOut(t *testing.T) {
	fr := &runner.FakeRunner{}
	// Pre-load enough listRuns+viewRun pairs for several poll cycles before timeout.
	for i := 0; i < 20; i++ {
		fr.AddResponse(&runner.Result{Stdout: runListOne}, nil)    // gh run list
		fr.AddResponse(&runner.Result{Stdout: runInProgress}, nil) // gh run view
	}

	err := newChecker(fr).Wait(&bytes.Buffer{}, "kiwiproject/kiwi", "abc123def456", 50*time.Millisecond, 10*time.Millisecond)
	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}
	if !strings.Contains(err.Error(), "timed out") {
		t.Errorf("expected 'timed out' in error: %v", err)
	}
	// Must be the completion timeout message, not the discovery one.
	if strings.Contains(err.Error(), "appear") {
		t.Errorf("expected completion timeout message, got discovery message: %v", err)
	}
}

func TestGHChecker_listRunsError(t *testing.T) {
	fr := &runner.FakeRunner{}
	fr.AddResponse(nil, errors.New("exit status 1"))

	err := newChecker(fr).Wait(&bytes.Buffer{}, "kiwiproject/kiwi", "abc123def456", time.Minute, time.Millisecond)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "gh run list") {
		t.Errorf("expected 'gh run list' in error: %v", err)
	}
}

func TestGHChecker_viewRunError(t *testing.T) {
	fr := &runner.FakeRunner{}
	fr.AddResponse(&runner.Result{Stdout: runListOne}, nil)
	fr.AddResponse(nil, errors.New("exit status 1"))

	err := newChecker(fr).Wait(&bytes.Buffer{}, "kiwiproject/kiwi", "abc123def456", time.Minute, time.Millisecond)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "gh run view") {
		t.Errorf("expected 'gh run view' in error: %v", err)
	}
}

func TestGHChecker_shortSHAInMessages(t *testing.T) {
	fr := &runner.FakeRunner{}
	fr.AddResponse(&runner.Result{Stdout: runListEmpty}, nil) // first empty poll
	fr.AddResponse(&runner.Result{Stdout: workflowsOne}, nil) // workflows exist; keep waiting
	for i := 0; i < 10; i++ {
		fr.AddResponse(&runner.Result{Stdout: runListEmpty}, nil)
	}

	var buf bytes.Buffer
	err := newChecker(fr).Wait(&buf, "kiwiproject/kiwi", "abcdef1234567890", 50*time.Millisecond, 10*time.Millisecond)
	if err == nil {
		t.Fatal("expected error")
	}
	if strings.Contains(err.Error(), "abcdef1234567890") {
		t.Errorf("expected short SHA in error, got full SHA: %v", err)
	}
	if !strings.Contains(err.Error(), "abcdef12") {
		t.Errorf("expected short SHA 'abcdef12' in error: %v", err)
	}
}

func TestGHChecker_verifiesRunListArgs(t *testing.T) {
	fr := &runner.FakeRunner{}
	fr.AddResponse(&runner.Result{Stdout: runListOne}, nil) // gh run list
	fr.AddResponse(&runner.Result{Stdout: runSuccess}, nil) // gh run view

	_ = newChecker(fr).Wait(&bytes.Buffer{}, "kiwiproject/kiwi", "abc123def456", time.Minute, time.Millisecond)

	listCall := fr.Calls[0]
	if listCall.Command != "gh" {
		t.Fatalf("expected gh, got %s", listCall.Command)
	}
	args := strings.Join(listCall.Args, " ")
	for _, want := range []string{
		"run list",
		"--repo kiwiproject/kiwi",
		"--commit abc123def456",
		"--event push",
		"--json databaseId,status",
	} {
		if !strings.Contains(args, want) {
			t.Errorf("expected %q in gh run list args: %s", want, args)
		}
	}

	viewCall := fr.Calls[1]
	viewArgs := strings.Join(viewCall.Args, " ")
	for _, want := range []string{
		"run view",
		"--repo kiwiproject/kiwi",
		"--json status,conclusion",
	} {
		if !strings.Contains(viewArgs, want) {
			t.Errorf("expected %q in gh run view args: %s", want, viewArgs)
		}
	}
}
