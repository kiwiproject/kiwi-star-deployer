package cmd

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kiwiproject/kiwi-star-deployer/internal/config"
	"github.com/kiwiproject/kiwi-star-deployer/internal/runner"
)

func TestRunChanges_unknownOnlyLibrary(t *testing.T) {
	cfgPath := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(cfgPath, []byte("[library.kiwi]\nrepo = \"kiwiproject/kiwi\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	oldConfigPath, oldOnly := configPath, changesOnly
	t.Cleanup(func() { configPath, changesOnly = oldConfigPath, oldOnly })
	configPath, changesOnly = cfgPath, []string{"nonexistent"}

	err := runChanges(nil, nil)

	if err == nil || !strings.Contains(err.Error(), `--only: unknown library "nonexistent"`) {
		t.Errorf("expected an unknown library error, got %v", err)
	}
}

func changesLibs() map[string]config.Library {
	return map[string]config.Library{"kiwi": {Repo: "kiwiproject/kiwi"}}
}

func TestReportChanges_printsTable(t *testing.T) {
	fr := &runner.FakeRunner{}
	fr.AddResponse(&runner.Result{Stdout: `{"tag_name":"v1.0.0"}`}, nil)
	fr.AddResponse(&runner.Result{Stdout: `{"ahead_by":2,"commits":[` +
		`{"author":{"login":"someone"},"commit":{"message":"[maven-release-plugin] prepare for next development iteration"}},` +
		`{"author":{"login":"someone"},"commit":{"message":"Add a thing"}}]}`}, nil)
	var out, progress bytes.Buffer

	err := reportChanges(&out, &progress, fr, changesLibs(), true)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(progress.String(), "Checking 1 library for changes") {
		t.Errorf("unexpected progress output: %q", progress.String())
	}
	// One real commit remains once the release plugin's commit is excluded, and
	// it counts as an other change.
	if row := strings.Fields(strings.Split(out.String(), "\n")[1]); strings.Join(row, " ") != "kiwi v1.0.0 1 0 0 1" {
		t.Errorf("unexpected table row %q in:\n%s", strings.Join(row, " "), out.String())
	}
	if !strings.Contains(out.String(), "Add a thing (someone)") {
		t.Errorf("verbose output should list the commit:\n%s", out.String())
	}
	if strings.Contains(out.String(), "maven-release-plugin") {
		t.Errorf("the release plugin's commit should not be listed:\n%s", out.String())
	}
}

func TestReportChanges_failedLookupReturnsError(t *testing.T) {
	fr := &runner.FakeRunner{}
	fr.AddResponse(nil, errors.New("exit status 1"))
	var out, progress bytes.Buffer

	err := reportChanges(&out, &progress, fr, changesLibs(), false)

	if err == nil || !strings.Contains(err.Error(), "could not check every library") {
		t.Errorf("expected an error when a lookup fails, got %v", err)
	}
	if !strings.Contains(out.String(), "could not find the latest release") {
		t.Errorf("the failure should still be in the table:\n%s", out.String())
	}
}

func TestRunChanges_configLoadError(t *testing.T) {
	oldConfigPath := configPath
	t.Cleanup(func() { configPath = oldConfigPath })
	configPath = filepath.Join(t.TempDir(), "missing.toml")

	if err := runChanges(nil, nil); err == nil {
		t.Error("expected an error for a config file that does not exist")
	}
}
