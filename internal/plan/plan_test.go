package plan_test

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kiwiproject/kiwi-star-deployer/internal/config"
	"github.com/kiwiproject/kiwi-star-deployer/internal/plan"
	"github.com/kiwiproject/kiwi-star-deployer/internal/runner"
	"github.com/kiwiproject/kiwi-star-deployer/internal/version"
	"github.com/kiwiproject/kiwi-star-deployer/internal/workspace"
)

// createRepo writes a minimal pom.xml into wsDir/<name>/ so EnsureCloned skips cloning.
func createRepo(t *testing.T, wsDir, name, pomVersion string) {
	t.Helper()
	repoDir := filepath.Join(wsDir, name)
	if err := os.MkdirAll(repoDir, 0o755); err != nil {
		t.Fatal(err)
	}
	content := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<project xmlns="http://maven.apache.org/POM/4.0.0">
    <modelVersion>4.0.0</modelVersion>
    <artifactId>%s</artifactId>
    <version>%s</version>
</project>`, name, pomVersion)
	if err := os.WriteFile(filepath.Join(repoDir, "pom.xml"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func makeWorkspace(t *testing.T) (string, *workspace.Workspace) {
	t.Helper()
	dir := t.TempDir()
	// FakeRunner with no responses: EnsureCloned should never call the runner
	// because all repos are pre-created.
	return dir, workspace.New(dir, &runner.FakeRunner{})
}

func TestBuild_singleLibrary(t *testing.T) {
	dir, ws := makeWorkspace(t)
	createRepo(t, dir, "kiwi", "2.5.1-SNAPSHOT")

	cfg := &config.Config{
		Settings:  config.Settings{Workspace: dir},
		Libraries: map[string]config.Library{
			"kiwi": {Repo: "kiwiproject/kiwi"},
		},
	}

	stages, err := plan.Build(cfg, ws)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(stages) != 1 {
		t.Fatalf("stage count: got %d, want 1", len(stages))
	}
	if len(stages[0]) != 1 {
		t.Fatalf("stage 1 library count: got %d, want 1", len(stages[0]))
	}
	entry := stages[0][0]
	if entry.Name != "kiwi" {
		t.Errorf("name: got %q, want kiwi", entry.Name)
	}
	if entry.Stage != 1 {
		t.Errorf("stage: got %d, want 1", entry.Stage)
	}
	if entry.VersionPlan.ReleaseVersion != "2.5.1" {
		t.Errorf("release: got %q, want 2.5.1", entry.VersionPlan.ReleaseVersion)
	}
	if entry.VersionPlan.NextDevVersion != "2.5.2-SNAPSHOT" {
		t.Errorf("next dev: got %q, want 2.5.2-SNAPSHOT", entry.VersionPlan.NextDevVersion)
	}
}

func TestBuild_respectsStageOrder(t *testing.T) {
	dir, ws := makeWorkspace(t)
	createRepo(t, dir, "kiwi-parent", "3.0.0-SNAPSHOT")
	createRepo(t, dir, "kiwi-bom", "2.0.0-SNAPSHOT")
	createRepo(t, dir, "kiwi", "4.0.0-SNAPSHOT")

	cfg := &config.Config{
		Settings: config.Settings{Workspace: dir},
		Libraries: map[string]config.Library{
			"kiwi-parent": {Repo: "kiwiproject/kiwi-parent", Type: "parent-pom"},
			"kiwi-bom":    {Repo: "kiwiproject/kiwi-bom", Type: "bom", DependsOn: []string{"kiwi-parent"}},
			"kiwi":        {Repo: "kiwiproject/kiwi", DependsOn: []string{"kiwi-parent", "kiwi-bom"}},
		},
	}

	stages, err := plan.Build(cfg, ws)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(stages) != 3 {
		t.Fatalf("stage count: got %d, want 3", len(stages))
	}
	if stages[0][0].Name != "kiwi-parent" {
		t.Errorf("stage 1: got %q, want kiwi-parent", stages[0][0].Name)
	}
	if stages[1][0].Name != "kiwi-bom" {
		t.Errorf("stage 2: got %q, want kiwi-bom", stages[1][0].Name)
	}
	if stages[2][0].Name != "kiwi" {
		t.Errorf("stage 3: got %q, want kiwi", stages[2][0].Name)
	}
}

func TestBuild_appliesOverride(t *testing.T) {
	dir, ws := makeWorkspace(t)
	createRepo(t, dir, "kiwi", "2.5.1-SNAPSHOT")

	cfg := &config.Config{
		Settings:  config.Settings{Workspace: dir},
		Libraries: map[string]config.Library{
			"kiwi": {Repo: "kiwiproject/kiwi"},
		},
		Release: config.ReleaseConfig{
			Overrides: map[string]string{"kiwi": "3.0.0"},
		},
	}

	stages, err := plan.Build(cfg, ws)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	entry := stages[0][0]
	if entry.VersionPlan.ReleaseVersion != "3.0.0" {
		t.Errorf("release: got %q, want 3.0.0", entry.VersionPlan.ReleaseVersion)
	}
	if !entry.VersionPlan.OverrideApplied {
		t.Error("OverrideApplied: got false, want true")
	}
}

func TestPrint_basicOutput(t *testing.T) {
	stages := [][]plan.Entry{
		{{Name: "kiwi-parent", Stage: 1, VersionPlan: mustPlan("kiwi-parent", "3.0.0-SNAPSHOT", "")}},
		{
			{Name: "kiwi-bom", Stage: 2, VersionPlan: mustPlan("kiwi-bom", "2.0.0-SNAPSHOT", "")},
			{Name: "kiwi", Stage: 2, VersionPlan: mustPlan("kiwi", "4.0.0-SNAPSHOT", "3.0.0")},
		},
	}

	var buf bytes.Buffer
	plan.Print(&buf, stages)
	out := buf.String()

	if !strings.Contains(out, "Stage 1:") {
		t.Errorf("expected 'Stage 1:' in output:\n%s", out)
	}
	if !strings.Contains(out, "Stage 2:") {
		t.Errorf("expected 'Stage 2:' in output:\n%s", out)
	}
	if !strings.Contains(out, "[OVERRIDE]") {
		t.Errorf("expected '[OVERRIDE]' in output:\n%s", out)
	}
	if !strings.Contains(out, "3.0.0-SNAPSHOT -> 3.0.0") {
		t.Errorf("expected version arrow in output:\n%s", out)
	}
	if !strings.Contains(out, "(next: 3.0.1-SNAPSHOT)") {
		t.Errorf("expected next dev version in output:\n%s", out)
	}
	// Second library in stage 2 should not repeat "Stage 2:"
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 3 {
		t.Fatalf("line count: got %d, want 3\n%s", len(lines), out)
	}
	if strings.Contains(lines[2], "Stage") {
		t.Errorf("second library in stage should not have stage label: %q", lines[2])
	}
}

func TestPrint_emptyPlan(t *testing.T) {
	var buf bytes.Buffer
	plan.Print(&buf, nil)
	if buf.Len() != 0 {
		t.Errorf("expected empty output for nil stages, got: %q", buf.String())
	}
}

// mustPlan is a test helper that calls version.Compute and panics on error.
func mustPlan(name, pomVersion, override string) version.Plan {
	vp, err := version.Compute(name, pomVersion, override)
	if err != nil {
		panic(err)
	}
	return vp
}