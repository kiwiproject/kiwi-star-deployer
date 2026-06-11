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

// createRepo writes a minimal pom.xml (no <properties>) into wsDir/<name>/.
func createRepo(t *testing.T, wsDir, name, pomVersion string) {
	t.Helper()
	createRepoWithProperties(t, wsDir, name, pomVersion, nil)
}

// createRepoWithProperties writes a pom.xml with an optional <properties> block.
// props maps property name to value; nil means no <properties> element.
func createRepoWithProperties(t *testing.T, wsDir, name, pomVersion string, props map[string]string) {
	t.Helper()
	repoDir := filepath.Join(wsDir, name)
	if err := os.MkdirAll(repoDir, 0o755); err != nil {
		t.Fatal(err)
	}
	var propBlock string
	if len(props) > 0 {
		var sb strings.Builder
		sb.WriteString("    <properties>\n")
		for k, v := range props {
			fmt.Fprintf(&sb, "        <%s>%s</%s>\n", k, v, k)
		}
		sb.WriteString("    </properties>\n")
		propBlock = sb.String()
	}
	content := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<project xmlns="http://maven.apache.org/POM/4.0.0">
    <modelVersion>4.0.0</modelVersion>
    <artifactId>%s</artifactId>
    <version>%s</version>
%s</project>`, name, pomVersion, propBlock)
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
	if entry.Repo != "kiwiproject/kiwi" {
		t.Errorf("repo: got %q, want kiwiproject/kiwi", entry.Repo)
	}
	if entry.Type != "" {
		t.Errorf("type: got %q, want empty", entry.Type)
	}
	if len(entry.DependsOn) != 0 {
		t.Errorf("depends_on: got %v, want empty", entry.DependsOn)
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
	createRepoWithProperties(t, dir, "kiwi", "4.0.0-SNAPSHOT", map[string]string{"kiwi-bom.version": "2.0.0"})

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

func TestBuild_populatesTypeAndDependsOn(t *testing.T) {
	dir, ws := makeWorkspace(t)
	createRepo(t, dir, "kiwi-parent", "3.0.0-SNAPSHOT")
	createRepo(t, dir, "kiwi-bom", "2.0.0-SNAPSHOT")

	cfg := &config.Config{
		Settings: config.Settings{Workspace: dir},
		Libraries: map[string]config.Library{
			"kiwi-parent": {Repo: "kiwiproject/kiwi-parent", Type: "parent-pom"},
			"kiwi-bom":    {Repo: "kiwiproject/kiwi-bom", Type: "bom", DependsOn: []string{"kiwi-parent"}},
		},
	}

	stages, err := plan.Build(cfg, ws)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	parent := stages[0][0]
	if parent.Type != "parent-pom" {
		t.Errorf("kiwi-parent type: got %q, want parent-pom", parent.Type)
	}
	if len(parent.DependsOn) != 0 {
		t.Errorf("kiwi-parent depends_on: got %v, want empty", parent.DependsOn)
	}

	bom := stages[1][0]
	if bom.Type != "bom" {
		t.Errorf("kiwi-bom type: got %q, want bom", bom.Type)
	}
	if len(bom.DependsOn) != 1 || bom.DependsOn[0] != "kiwi-parent" {
		t.Errorf("kiwi-bom depends_on: got %v, want [kiwi-parent]", bom.DependsOn)
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

func TestBuild_validationPassesWhenPropertiesPresent(t *testing.T) {
	dir, ws := makeWorkspace(t)
	createRepo(t, dir, "kiwi-parent", "3.0.0-SNAPSHOT")
	createRepo(t, dir, "kiwi-bom", "2.0.0-SNAPSHOT") // depends only on kiwi-parent (parent-pom); no property needed
	createRepoWithProperties(t, dir, "kiwi", "4.0.0-SNAPSHOT", map[string]string{
		"kiwi-bom.version": "2.0.0",
	})

	cfg := &config.Config{
		Settings: config.Settings{Workspace: dir},
		Libraries: map[string]config.Library{
			"kiwi-parent": {Repo: "kiwiproject/kiwi-parent", Type: "parent-pom"},
			"kiwi-bom":    {Repo: "kiwiproject/kiwi-bom", Type: "bom", DependsOn: []string{"kiwi-parent"}},
			"kiwi":        {Repo: "kiwiproject/kiwi", DependsOn: []string{"kiwi-parent", "kiwi-bom"}},
		},
	}

	_, err := plan.Build(cfg, ws)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestBuild_validationFailsWhenPropertyMissing(t *testing.T) {
	dir, ws := makeWorkspace(t)
	createRepo(t, dir, "kiwi-bom", "2.0.0-SNAPSHOT")
	// kiwi is missing kiwi-bom.version property
	createRepo(t, dir, "kiwi", "4.0.0-SNAPSHOT")

	cfg := &config.Config{
		Settings: config.Settings{Workspace: dir},
		Libraries: map[string]config.Library{
			"kiwi-bom": {Repo: "kiwiproject/kiwi-bom", Type: "bom"},
			"kiwi":     {Repo: "kiwiproject/kiwi", DependsOn: []string{"kiwi-bom"}},
		},
	}

	_, err := plan.Build(cfg, ws)
	if err == nil {
		t.Fatal("expected validation error, got nil")
	}
	if !strings.Contains(err.Error(), "kiwi-bom.version") {
		t.Errorf("expected missing property name in error: %v", err)
	}
	if !strings.Contains(err.Error(), "kiwi") {
		t.Errorf("expected library name in error: %v", err)
	}
}

func TestBuild_validationCollectsAllErrors(t *testing.T) {
	dir, ws := makeWorkspace(t)
	// Both kiwi and kiwi-test are missing their kiwi-bom.version property
	createRepo(t, dir, "kiwi-bom", "2.0.0-SNAPSHOT")
	createRepo(t, dir, "kiwi", "4.0.0-SNAPSHOT")
	createRepo(t, dir, "kiwi-test", "3.0.0-SNAPSHOT")

	cfg := &config.Config{
		Settings: config.Settings{Workspace: dir},
		Libraries: map[string]config.Library{
			"kiwi-bom":  {Repo: "kiwiproject/kiwi-bom", Type: "bom"},
			"kiwi":      {Repo: "kiwiproject/kiwi", DependsOn: []string{"kiwi-bom"}},
			"kiwi-test": {Repo: "kiwiproject/kiwi-test", DependsOn: []string{"kiwi-bom"}},
		},
	}

	_, err := plan.Build(cfg, ws)
	if err == nil {
		t.Fatal("expected validation error, got nil")
	}
	msg := err.Error()
	// Check entry name at start of each error line to distinguish "kiwi" from "kiwi-test"
	if !strings.Contains(msg, "kiwi: missing property") {
		t.Errorf("expected kiwi error line in message: %v", err)
	}
	if !strings.Contains(msg, "kiwi-test: missing property") {
		t.Errorf("expected kiwi-test error line in message: %v", err)
	}
}

func TestBuild_validationSkipsParentPOMDeps(t *testing.T) {
	dir, ws := makeWorkspace(t)
	createRepo(t, dir, "kiwi-parent", "3.0.0-SNAPSHOT")
	// kiwi has no properties — kiwi-parent is parent-pom type so no property needed
	createRepo(t, dir, "kiwi", "4.0.0-SNAPSHOT")

	cfg := &config.Config{
		Settings: config.Settings{Workspace: dir},
		Libraries: map[string]config.Library{
			"kiwi-parent": {Repo: "kiwiproject/kiwi-parent", Type: "parent-pom"},
			"kiwi":        {Repo: "kiwiproject/kiwi", DependsOn: []string{"kiwi-parent"}},
		},
	}

	_, err := plan.Build(cfg, ws)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
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