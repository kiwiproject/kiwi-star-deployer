package runner_test

import (
	"strings"
	"testing"
	"time"

	"github.com/kiwiproject/kiwi-star-deployer/internal/runner"
)

func TestOsRunner_capturesStdout(t *testing.T) {
	r := runner.NewOsRunner()
	result, err := r.Run(runner.Options{
		Command: "echo",
		Args:    []string{"hello"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result.Stdout, "hello") {
		t.Errorf("expected stdout to contain 'hello', got %q", result.Stdout)
	}
}

func TestOsRunner_returnsErrorOnNonZeroExit(t *testing.T) {
	r := runner.NewOsRunner()
	_, err := r.Run(runner.Options{Command: "false"})
	if err == nil {
		t.Fatal("expected error for non-zero exit, got nil")
	}
}

func TestFakeRunner_recordsCalls(t *testing.T) {
	f := &runner.FakeRunner{}
	f.AddResponse(&runner.Result{Stdout: "ok"}, nil)

	result, err := f.Run(runner.Options{Command: "git", Args: []string{"fetch"}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Stdout != "ok" {
		t.Errorf("expected stdout 'ok', got %q", result.Stdout)
	}
	if f.CallCount() != 1 {
		t.Errorf("expected 1 call, got %d", f.CallCount())
	}
	if f.Calls[0].Command != "git" {
		t.Errorf("expected command 'git', got %q", f.Calls[0].Command)
	}
}

func TestOsRunner_timeoutCancelsCommand(t *testing.T) {
	r := runner.NewOsRunner()
	_, err := r.Run(runner.Options{
		Command: "sleep",
		Args:    []string{"10"},
		Timeout: 50 * time.Millisecond,
	})
	if err == nil {
		t.Fatal("expected error from timeout, got nil")
	}
}

func TestFakeRunner_panicOnUnexpectedCall(t *testing.T) {
	f := &runner.FakeRunner{}
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic on unconfigured call, got none")
		}
	}()
	_, _ = f.Run(runner.Options{Command: "git", Args: []string{"fetch"}})
}
