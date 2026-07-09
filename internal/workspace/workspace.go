package workspace

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/kiwiproject/kiwi-star-deployer/internal/config"
	"github.com/kiwiproject/kiwi-star-deployer/internal/runner"
)

// Workspace manages the local directory where library repos are cloned and
// release operations are performed.
//
// Two ways to get repo content are available and serve different purposes:
// ReadFile is read-only and never touches the local working copy — use it for
// previewing state (e.g. plan). Prepare mutates the local working copy to
// match the remote — use it immediately before performing real release work
// that reads or writes the checked-out files.
type Workspace struct {
	dir    string
	runner runner.Runner
}

// New returns a Workspace rooted at dir, using r for all external commands.
func New(dir string, r runner.Runner) *Workspace {
	return &Workspace{dir: dir, runner: r}
}

// Dir returns the workspace root directory.
func (w *Workspace) Dir() string {
	return w.dir
}

// RepoDir returns the local path for the named library.
func (w *Workspace) RepoDir(name string) string {
	return filepath.Join(w.dir, name)
}

// EnsureCloned clones lib's repo into the workspace if not already present.
// The workspace directory is created if it does not exist.
func (w *Workspace) EnsureCloned(name string, lib config.Library) error {
	dest := w.RepoDir(name)
	if _, err := os.Stat(dest); err == nil {
		return nil
	}
	if err := os.MkdirAll(w.dir, 0o755); err != nil {
		return fmt.Errorf("creating workspace %s: %w", w.dir, err)
	}
	if _, err := w.runner.Run(runner.Options{
		Command: "gh",
		Args:    []string{"repo", "clone", lib.Repo, dest},
	}); err != nil {
		return fmt.Errorf("cloning %s: %w", lib.Repo, err)
	}
	return nil
}

// ReadFile returns the contents of relPath as committed on origin/main for
// name, without checking out, resetting, or otherwise modifying the local
// working copy. It fetches first so the read reflects the latest remote
// state regardless of what is currently checked out locally.
func (w *Workspace) ReadFile(name, relPath string) (string, error) {
	dir := w.RepoDir(name)
	if err := w.fetch(dir, name); err != nil {
		return "", err
	}
	result, err := w.runner.Run(runner.Options{
		Command:    "git",
		Args:       []string{"show", "origin/main:" + relPath},
		WorkingDir: dir,
	})
	if err != nil {
		return "", fmt.Errorf("%s: reading %s from origin/main: %w", name, relPath, err)
	}
	return result.Stdout, nil
}

// Prepare verifies that name's working copy is on main, has no uncommitted
// changes, and is not in a detached HEAD state. It then fetches from origin
// and resets to the latest remote commit. Returns an error without attempting
// any repair if any check fails.
//
// This tool has no branch-switching logic of its own and always operates
// against main; a repo checked out on any other branch is rejected rather
// than tolerated.
func (w *Workspace) Prepare(name string) error {
	dir := w.RepoDir(name)

	branch, err := w.currentBranch(dir)
	if err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}
	if branch != "main" {
		return fmt.Errorf("%s: on branch %q; must be main", name, branch)
	}
	if err := w.verifyClean(dir, name); err != nil {
		return err
	}
	if err := w.fetch(dir, name); err != nil {
		return err
	}
	return w.resetHard(dir, name, branch)
}

func (w *Workspace) currentBranch(dir string) (string, error) {
	result, err := w.runner.Run(runner.Options{
		Command:    "git",
		Args:       []string{"symbolic-ref", "--short", "HEAD"},
		WorkingDir: dir,
	})
	if err != nil {
		return "", fmt.Errorf("could not determine branch (detached HEAD?): %w", err)
	}
	return strings.TrimSpace(result.Stdout), nil
}

func (w *Workspace) verifyClean(dir, name string) error {
	result, err := w.runner.Run(runner.Options{
		Command:    "git",
		Args:       []string{"status", "--porcelain"},
		WorkingDir: dir,
	})
	if err != nil {
		return fmt.Errorf("%s: checking git status: %w", name, err)
	}
	if strings.TrimSpace(result.Stdout) != "" {
		return fmt.Errorf("%s: working copy has uncommitted changes", name)
	}
	return nil
}

func (w *Workspace) fetch(dir, name string) error {
	if _, err := w.runner.Run(runner.Options{
		Command:    "git",
		Args:       []string{"fetch", "origin"},
		WorkingDir: dir,
	}); err != nil {
		return fmt.Errorf("%s: git fetch: %w", name, err)
	}
	return nil
}

func (w *Workspace) resetHard(dir, name, branch string) error {
	if _, err := w.runner.Run(runner.Options{
		Command:    "git",
		Args:       []string{"reset", "--hard", "origin/" + branch},
		WorkingDir: dir,
	}); err != nil {
		return fmt.Errorf("%s: git reset --hard origin/%s: %w", name, branch, err)
	}
	return nil
}
