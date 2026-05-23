package preflight_test

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/kiwiproject/kiwi-star-deployer/internal/preflight"
	"github.com/kiwiproject/kiwi-star-deployer/internal/runner"
)

func TestAllPassed_trueWhenAllOK(t *testing.T) {
	results := []preflight.Result{
		{Name: "git", OK: true},
		{Name: "gh", OK: true},
		{Name: "gh auth", OK: true},
	}
	if !preflight.AllPassed(results) {
		t.Error("expected AllPassed=true, got false")
	}
}

func TestAllPassed_falseWhenAnyFails(t *testing.T) {
	results := []preflight.Result{
		{Name: "git", OK: true},
		{Name: "mvn", OK: false, Message: "mvn not found on PATH"},
	}
	if preflight.AllPassed(results) {
		t.Error("expected AllPassed=false, got true")
	}
}

func TestAllPassed_emptySlice(t *testing.T) {
	if !preflight.AllPassed(nil) {
		t.Error("expected AllPassed=true for empty results")
	}
}

func TestRunAll_ghAuthPass(t *testing.T) {
	fr := &runner.FakeRunner{}
	fr.AddResponse(&runner.Result{}, nil)

	results := preflight.RunAll(fr)

	var authResult *preflight.Result
	for i := range results {
		if results[i].Name == "gh auth" {
			authResult = &results[i]
			break
		}
	}
	if authResult == nil {
		t.Fatal("no 'gh auth' result found")
	}
	if !authResult.OK {
		t.Errorf("expected gh auth OK=true, got false: %s", authResult.Message)
	}
}

func TestRunAll_ghAuthFail(t *testing.T) {
	fr := &runner.FakeRunner{}
	fr.AddResponse(nil, errors.New("exit status 1"))

	results := preflight.RunAll(fr)

	var authResult *preflight.Result
	for i := range results {
		if results[i].Name == "gh auth" {
			authResult = &results[i]
			break
		}
	}
	if authResult == nil {
		t.Fatal("no 'gh auth' result found")
	}
	if authResult.OK {
		t.Error("expected gh auth OK=false, got true")
	}
	if !strings.Contains(authResult.Message, "gh auth login") {
		t.Errorf("expected fix hint in message, got: %q", authResult.Message)
	}
}

func TestPrint_showsPassAndFail(t *testing.T) {
	results := []preflight.Result{
		{Name: "git", OK: true},
		{Name: "mvn", OK: false, Message: "mvn not found on PATH"},
	}
	var buf bytes.Buffer
	preflight.Print(&buf, results)
	out := buf.String()

	if !strings.Contains(out, "[PASS]") {
		t.Errorf("expected [PASS] in output:\n%s", out)
	}
	if !strings.Contains(out, "[FAIL]") {
		t.Errorf("expected [FAIL] in output:\n%s", out)
	}
	if !strings.Contains(out, "mvn not found on PATH") {
		t.Errorf("expected failure message in output:\n%s", out)
	}
}
