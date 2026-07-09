package workspace_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kiwiproject/kiwi-star-deployer/internal/config"
	"github.com/kiwiproject/kiwi-star-deployer/internal/runner"
	"github.com/kiwiproject/kiwi-star-deployer/internal/workspace"
)

// --- EnsureCloned ---

func TestEnsureCloned_clonesWhenMissing(t *testing.T) {
	dir := t.TempDir()
	fr := &runner.FakeRunner{}
	fr.AddResponse(&runner.Result{}, nil)
	w := workspace.New(dir, fr)

	err := w.EnsureCloned("kiwi", config.Library{Repo: "kiwiproject/kiwi"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fr.CallCount() != 1 {
		t.Fatalf("expected 1 runner call, got %d", fr.CallCount())
	}
	call := fr.Calls[0]
	if call.Command != "gh" {
		t.Errorf("command: got %q, want gh", call.Command)
	}
	wantArgs := []string{"repo", "clone", "kiwiproject/kiwi", filepath.Join(dir, "kiwi")}
	if strings.Join(call.Args, " ") != strings.Join(wantArgs, " ") {
		t.Errorf("args: got %v, want %v", call.Args, wantArgs)
	}
}

func TestEnsureCloned_skipsWhenAlreadyPresent(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "kiwi"), 0o755); err != nil {
		t.Fatal(err)
	}
	fr := &runner.FakeRunner{}
	w := workspace.New(dir, fr)

	err := w.EnsureCloned("kiwi", config.Library{Repo: "kiwiproject/kiwi"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fr.CallCount() != 0 {
		t.Errorf("expected no runner calls, got %d", fr.CallCount())
	}
}

func TestEnsureCloned_returnsErrorOnCloneFailure(t *testing.T) {
	dir := t.TempDir()
	fr := &runner.FakeRunner{}
	fr.AddResponse(nil, errors.New("gh: authentication required"))
	w := workspace.New(dir, fr)

	err := w.EnsureCloned("kiwi", config.Library{Repo: "kiwiproject/kiwi"})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "cloning") {
		t.Errorf("expected 'cloning' in error, got: %v", err)
	}
}

// --- ReadFile ---

func TestReadFile_fetchesThenShowsRemoteRef(t *testing.T) {
	dir := t.TempDir()
	fr := &runner.FakeRunner{}
	fr.AddResponse(&runner.Result{}, nil)                                   // git fetch origin
	fr.AddResponse(&runner.Result{Stdout: "<version>1.2.3</version>"}, nil) // git show origin/main:pom.xml
	w := workspace.New(dir, fr)

	content, err := w.ReadFile("kiwi", "pom.xml")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if content != "<version>1.2.3</version>" {
		t.Errorf("content: got %q", content)
	}
	if fr.CallCount() != 2 {
		t.Fatalf("expected 2 runner calls, got %d: %v", fr.CallCount(), fr.Calls)
	}
	fetchCall := fr.Calls[0]
	if fetchCall.Command != "git" || strings.Join(fetchCall.Args, " ") != "fetch origin" {
		t.Errorf("expected git fetch origin, got %v", fetchCall)
	}
	showCall := fr.Calls[1]
	if showCall.Command != "git" || strings.Join(showCall.Args, " ") != "show origin/main:pom.xml" {
		t.Errorf("expected git show origin/main:pom.xml, got %v", showCall)
	}
	if showCall.WorkingDir != filepath.Join(dir, "kiwi") {
		t.Errorf("working dir: got %q, want %q", showCall.WorkingDir, filepath.Join(dir, "kiwi"))
	}
}

func TestReadFile_fetchFails(t *testing.T) {
	dir := t.TempDir()
	fr := &runner.FakeRunner{}
	fr.AddResponse(nil, errors.New("network unreachable"))
	w := workspace.New(dir, fr)

	_, err := w.ReadFile("kiwi", "pom.xml")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "git fetch") {
		t.Errorf("expected 'git fetch' in error, got: %v", err)
	}
}

func TestReadFile_showFails(t *testing.T) {
	dir := t.TempDir()
	fr := &runner.FakeRunner{}
	fr.AddResponse(&runner.Result{}, nil)
	fr.AddResponse(nil, errors.New("fatal: path 'pom.xml' does not exist in 'origin/main'"))
	w := workspace.New(dir, fr)

	_, err := w.ReadFile("kiwi", "pom.xml")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "origin/main") {
		t.Errorf("expected 'origin/main' in error, got: %v", err)
	}
}

// --- Prepare ---

// prepareRunner sets up a FakeRunner for the four Prepare commands in order:
// symbolic-ref, status --porcelain, fetch, reset --hard.
func prepareRunner(branch, status string, fetchErr, resetErr error) *runner.FakeRunner {
	fr := &runner.FakeRunner{}
	fr.AddResponse(&runner.Result{Stdout: branch + "\n"}, nil)
	fr.AddResponse(&runner.Result{Stdout: status}, nil)
	fr.AddResponse(&runner.Result{}, fetchErr)
	if fetchErr == nil {
		fr.AddResponse(&runner.Result{}, resetErr)
	}
	return fr
}

func TestPrepare_happyPath_mainBranch(t *testing.T) {
	dir := t.TempDir()
	fr := prepareRunner("main", "", nil, nil)
	w := workspace.New(dir, fr)

	if err := w.Prepare("kiwi"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fr.CallCount() != 4 {
		t.Errorf("expected 4 runner calls, got %d", fr.CallCount())
	}
}

func TestPrepare_releaseBranchIsRejected(t *testing.T) {
	dir := t.TempDir()
	fr := &runner.FakeRunner{}
	fr.AddResponse(&runner.Result{Stdout: "release/1.2\n"}, nil)
	w := workspace.New(dir, fr)

	err := w.Prepare("kiwi")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "release/1.2") {
		t.Errorf("expected branch name in error, got: %v", err)
	}
}

func TestPrepare_detachedHead(t *testing.T) {
	dir := t.TempDir()
	fr := &runner.FakeRunner{}
	fr.AddResponse(nil, errors.New("exit status 128"))
	w := workspace.New(dir, fr)

	err := w.Prepare("kiwi")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "detached HEAD") {
		t.Errorf("expected 'detached HEAD' in error, got: %v", err)
	}
}

func TestPrepare_unacceptableBranch(t *testing.T) {
	dir := t.TempDir()
	fr := &runner.FakeRunner{}
	fr.AddResponse(&runner.Result{Stdout: "feature/my-experiment\n"}, nil)
	w := workspace.New(dir, fr)

	err := w.Prepare("kiwi")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "feature/my-experiment") {
		t.Errorf("expected branch name in error, got: %v", err)
	}
}

func TestPrepare_uncommittedChanges(t *testing.T) {
	dir := t.TempDir()
	fr := &runner.FakeRunner{}
	fr.AddResponse(&runner.Result{Stdout: "main\n"}, nil)
	fr.AddResponse(&runner.Result{Stdout: " M internal/config/config.go\n"}, nil)
	w := workspace.New(dir, fr)

	err := w.Prepare("kiwi")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "uncommitted changes") {
		t.Errorf("expected 'uncommitted changes' in error, got: %v", err)
	}
}

func TestPrepare_fetchFails(t *testing.T) {
	dir := t.TempDir()
	fr := prepareRunner("main", "", errors.New("network unreachable"), nil)
	w := workspace.New(dir, fr)

	err := w.Prepare("kiwi")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "git fetch") {
		t.Errorf("expected 'git fetch' in error, got: %v", err)
	}
}

func TestPrepare_resetFails(t *testing.T) {
	dir := t.TempDir()
	fr := prepareRunner("main", "", nil, errors.New("exit status 1"))
	w := workspace.New(dir, fr)

	err := w.Prepare("kiwi")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "reset --hard") {
		t.Errorf("expected 'reset --hard' in error, got: %v", err)
	}
}
