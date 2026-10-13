package gpg_test

import (
	"errors"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/kiwiproject/kiwi-star-deployer/internal/gpg"
	"github.com/kiwiproject/kiwi-star-deployer/internal/runner"
)

func TestWarm_runsInteractiveStyleSign(t *testing.T) {
	fr := &runner.FakeRunner{}
	fr.AddResponse(&runner.Result{}, nil)

	if err := gpg.Warm(fr); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if fr.CallCount() != 1 {
		t.Fatalf("expected 1 call, got %d", fr.CallCount())
	}
	call := fr.Calls[0]
	if call.Command != "gpg" {
		t.Errorf("expected command gpg, got %q", call.Command)
	}
	want := []string{"--armor", "--detach-sign", "--output", os.DevNull}
	if strings.Join(call.Args, " ") != strings.Join(want, " ") {
		t.Errorf("expected args %v, got %v", want, call.Args)
	}
	for _, a := range call.Args {
		if a == "--batch" || a == "--pinentry-mode" {
			t.Errorf("args must not forbid pinentry, got %v", call.Args)
		}
	}
	if call.Timeout <= 0 || call.Timeout > 10*time.Minute {
		t.Errorf("expected a bounded timeout, got %v", call.Timeout)
	}
}

func TestWarm_failureIncludesStderrAndHint(t *testing.T) {
	fr := &runner.FakeRunner{}
	fr.AddResponse(&runner.Result{Stderr: "gpg: signing failed: No pinentry\ngpg: signing failed: No pinentry\n"}, errors.New("exit status 2"))

	err := gpg.Warm(fr)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	msg := err.Error()
	for _, want := range []string{"exit status 2", "No pinentry", "default-cache-ttl"} {
		if !strings.Contains(msg, want) {
			t.Errorf("expected error to contain %q, got: %s", want, msg)
		}
	}
	if strings.Contains(msg, "\n") {
		t.Errorf("expected a single-line error, got: %q", msg)
	}
}

func TestWarm_gpgNotInstalled(t *testing.T) {
	fr := &runner.FakeRunner{}
	fr.AddResponse(nil, &exec.Error{Name: "gpg", Err: exec.ErrNotFound})

	err := gpg.Warm(fr)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if err.Error() != "gpg not found on PATH" {
		t.Errorf("expected a plain not-found message, got: %v", err)
	}
}
