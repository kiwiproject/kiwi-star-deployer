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

const ghVersionOutput = "gh version 2.62.0 (2024-11-14)\nhttps://github.com/cli/cli/releases/tag/v2.62.0\n"

func TestRunAll_ghAuthPass(t *testing.T) {
	fr := &runner.FakeRunner{}
	fr.AddResponse(&runner.Result{Stdout: ghVersionOutput}, nil) // gh --version
	fr.AddResponse(&runner.Result{}, nil)                        // gh auth status

	results := preflight.RunAll(fr, ".generate-kiwi-changelog")

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
	fr.AddResponse(&runner.Result{Stdout: ghVersionOutput}, nil) // gh --version
	fr.AddResponse(nil, errors.New("exit status 1"))             // gh auth status

	results := preflight.RunAll(fr, ".generate-kiwi-changelog")

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

func findResult(t *testing.T, results []preflight.Result, name string) preflight.Result {
	t.Helper()
	for _, r := range results {
		if r.Name == name {
			return r
		}
	}
	t.Fatalf("no %q result found", name)
	return preflight.Result{}
}

func TestRunAll_ghVersionPass(t *testing.T) {
	fr := &runner.FakeRunner{}
	fr.AddResponse(&runner.Result{Stdout: ghVersionOutput}, nil) // gh --version
	fr.AddResponse(&runner.Result{}, nil)                        // gh auth status

	res := findResult(t, preflight.RunAll(fr, ".generate-kiwi-changelog"), "gh version")
	if !res.OK {
		t.Errorf("expected gh version OK=true, got false: %s", res.Message)
	}
}

func TestRunAll_ghVersionTooOld(t *testing.T) {
	fr := &runner.FakeRunner{}
	fr.AddResponse(&runner.Result{Stdout: "gh version 2.39.2 (2023-11-01)\n"}, nil) // gh --version
	fr.AddResponse(&runner.Result{}, nil)                                           // gh auth status

	res := findResult(t, preflight.RunAll(fr, ".generate-kiwi-changelog"), "gh version")
	if res.OK {
		t.Error("expected gh version OK=false for 2.39.2, got true")
	}
	if !strings.Contains(res.Message, "2.40.0") {
		t.Errorf("expected required version in message, got: %q", res.Message)
	}
}

func TestRunAll_ghVersionExactMinimum(t *testing.T) {
	fr := &runner.FakeRunner{}
	fr.AddResponse(&runner.Result{Stdout: "gh version 2.40.0 (2023-11-14)\n"}, nil) // gh --version
	fr.AddResponse(&runner.Result{}, nil)                                           // gh auth status

	res := findResult(t, preflight.RunAll(fr, ".generate-kiwi-changelog"), "gh version")
	if !res.OK {
		t.Errorf("expected gh version 2.40.0 to pass, got: %s", res.Message)
	}
}

func TestRunAll_ghVersionUnparseable(t *testing.T) {
	fr := &runner.FakeRunner{}
	fr.AddResponse(&runner.Result{Stdout: "something unexpected\n"}, nil) // gh --version
	fr.AddResponse(&runner.Result{}, nil)                                 // gh auth status

	res := findResult(t, preflight.RunAll(fr, ".generate-kiwi-changelog"), "gh version")
	if res.OK {
		t.Error("expected gh version OK=false for unparseable output, got true")
	}
}

func TestRunAll_ghVersionCommandError(t *testing.T) {
	fr := &runner.FakeRunner{}
	fr.AddResponse(nil, errors.New("exec: gh: not found")) // gh --version
	fr.AddResponse(&runner.Result{}, nil)                  // gh auth status

	res := findResult(t, preflight.RunAll(fr, ".generate-kiwi-changelog"), "gh version")
	if res.OK {
		t.Error("expected gh version OK=false when gh --version fails, got true")
	}
}

func TestPrint_includesUnverifiableNote(t *testing.T) {
	var buf bytes.Buffer
	preflight.Print(&buf, []preflight.Result{{Name: "git", OK: true}})
	out := buf.String()
	if !strings.Contains(out, "cannot be verified") {
		t.Errorf("expected unverifiable-prerequisites note in output:\n%s", out)
	}
	if !strings.Contains(out, "GPG") {
		t.Errorf("expected GPG mention in note:\n%s", out)
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
