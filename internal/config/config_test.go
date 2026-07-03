package config_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kiwiproject/kiwi-star-deployer/internal/config"
)

func TestLoad_parsesValidConfig(t *testing.T) {
	cfg, err := config.Load("testdata/valid.toml")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.Settings.Workspace != "/tmp/test-workspace" {
		t.Errorf("workspace: got %q, want /tmp/test-workspace", cfg.Settings.Workspace)
	}
	if cfg.Settings.GroupID != "org.example" {
		t.Errorf("group_id: got %q, want org.example", cfg.Settings.GroupID)
	}
	if cfg.Settings.ChangelogScript != "gkc" {
		t.Errorf("changelog_script: got %q, want gkc", cfg.Settings.ChangelogScript)
	}
	if cfg.Settings.MavenCentralMaxWait != config.Duration(30*time.Minute) {
		t.Errorf("max wait: got %v, want 30m", time.Duration(cfg.Settings.MavenCentralMaxWait))
	}
	if cfg.Settings.MavenCentralPollInterval != config.Duration(15*time.Second) {
		t.Errorf("poll interval: got %v, want 15s", time.Duration(cfg.Settings.MavenCentralPollInterval))
	}
	if len(cfg.Libraries) != 4 {
		t.Errorf("libraries: got %d, want 4", len(cfg.Libraries))
	}

	parent, ok := cfg.Libraries["kiwi-parent"]
	if !ok {
		t.Fatal("missing library kiwi-parent")
	}
	if parent.Repo != "kiwiproject/kiwi-parent" {
		t.Errorf("kiwi-parent repo: got %q", parent.Repo)
	}
	if parent.Type != config.TypeParentPOM {
		t.Errorf("kiwi-parent type: got %q, want %q", parent.Type, config.TypeParentPOM)
	}

	bom, ok := cfg.Libraries["kiwi-bom"]
	if !ok {
		t.Fatal("missing library kiwi-bom")
	}
	if len(bom.DependsOn) != 1 || bom.DependsOn[0] != "kiwi-parent" {
		t.Errorf("kiwi-bom depends_on: got %v, want [kiwi-parent]", bom.DependsOn)
	}

	if version, ok := cfg.Release.Overrides["kiwi"]; !ok || version != "3.0.0" {
		t.Errorf("release override for kiwi: got %q, want 3.0.0", version)
	}
}

func TestLoad_appliesDefaultWorkspace(t *testing.T) {
	cfg, err := config.Load("testdata/minimal.toml")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(home, ".kiwi-star-deployer/workspace")
	if cfg.Settings.Workspace != want {
		t.Errorf("workspace: got %q, want %q", cfg.Settings.Workspace, want)
	}
}

func TestLoad_appliesDefaultTimeouts(t *testing.T) {
	cfg, err := config.Load("testdata/minimal.toml")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Settings.MavenCentralMaxWait != config.Duration(60*time.Minute) {
		t.Errorf("max wait: got %v, want 1h", time.Duration(cfg.Settings.MavenCentralMaxWait))
	}
	if cfg.Settings.MavenCentralPollInterval != config.Duration(30*time.Second) {
		t.Errorf("poll interval: got %v, want 30s", time.Duration(cfg.Settings.MavenCentralPollInterval))
	}
}

func TestLoad_appliesDefaultStatePath(t *testing.T) {
	cfg, err := config.Load("testdata/minimal.toml")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(home, ".kiwi-star-deployer/state.json")
	if cfg.Settings.StatePath != want {
		t.Errorf("state_path: got %q, want %q", cfg.Settings.StatePath, want)
	}
}

func TestLoad_appliesDefaultGroupID(t *testing.T) {
	cfg, err := config.Load("testdata/minimal.toml")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Settings.GroupID != "org.kiwiproject" {
		t.Errorf("group_id: got %q, want org.kiwiproject", cfg.Settings.GroupID)
	}
}

func TestLoad_appliesDefaultChangelogScript(t *testing.T) {
	cfg, err := config.Load("testdata/minimal.toml")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Settings.ChangelogScript != ".generate-kiwi-changelog" {
		t.Errorf("changelog_script: got %q, want .generate-kiwi-changelog", cfg.Settings.ChangelogScript)
	}
}

func TestLoad_expandsTildeInChangelogScript(t *testing.T) {
	path := writeTempTOML(t, `
[library.kiwi]
repo = "kiwiproject/kiwi"

[settings]
changelog_script = "~/scripts/.generate-kiwi-changelog"
`)
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(home, "scripts/.generate-kiwi-changelog")
	if cfg.Settings.ChangelogScript != want {
		t.Errorf("changelog_script: got %q, want %q", cfg.Settings.ChangelogScript, want)
	}
}

func TestLoad_defaultsCIVerifyToTrue(t *testing.T) {
	cfg, err := config.Load("testdata/minimal.toml")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Settings.CIVerify == nil {
		t.Fatal("CIVerify: got nil, want non-nil")
	}
	if !*cfg.Settings.CIVerify {
		t.Error("CIVerify: got false, want true (default)")
	}
}

func TestLoad_ciVerifyExplicitFalse(t *testing.T) {
	cfg, err := config.Load(writeTempTOML(t, `
[library.kiwi]
repo = "kiwiproject/kiwi"

[settings]
ci_verify = false
`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Settings.CIVerify == nil {
		t.Fatal("CIVerify: got nil, want non-nil")
	}
	if *cfg.Settings.CIVerify {
		t.Error("CIVerify: got true, want false (explicit)")
	}
}

func TestLoad_fileNotFound(t *testing.T) {
	_, err := config.Load("testdata/nonexistent.toml")
	if err == nil {
		t.Fatal("expected error for missing file, got nil")
	}
}

func TestLoad_validationErrors(t *testing.T) {
	tests := []struct {
		name    string
		content string
		wantErr string
	}{
		{
			name: "unknown dependency",
			content: `
[library.kiwi]
repo = "kiwiproject/kiwi"
depends_on = ["nonexistent"]
`,
			wantErr: `unknown dependency "nonexistent"`,
		},
		{
			name: "self dependency",
			content: `
[library.kiwi]
repo = "kiwiproject/kiwi"
depends_on = ["kiwi"]
`,
			wantErr: "depends on itself",
		},
		{
			name: "multiple library-boms",
			content: `
[library.kiwi-libraries-bom]
repo = "kiwiproject/kiwi-libraries-bom"
type = "library-bom"

[library.other-bom]
repo = "kiwiproject/other-bom"
type = "library-bom"
`,
			wantErr: `at most one library may have type "library-bom"`,
		},
		{
			name: "invalid override version",
			content: `
[library.kiwi]
repo = "kiwiproject/kiwi"

[release.overrides]
kiwi = "not-a-version"
`,
			wantErr: "not a valid version",
		},
		{
			name: "override version with leading zero",
			content: `
[library.kiwi]
repo = "kiwiproject/kiwi"

[release.overrides]
kiwi = "01.2.3"
`,
			wantErr: "not a valid version",
		},
		{
			name: "override for unknown library",
			content: `
[library.kiwi]
repo = "kiwiproject/kiwi"

[release.overrides]
nonexistent = "1.0.0"
`,
			wantErr: `release override for unknown library "nonexistent"`,
		},
		{
			name: "missing repo",
			content: `
[library.kiwi]
depends_on = []
`,
			wantErr: "repo is required",
		},
		{
			name: "invalid type",
			content: `
[library.kiwi]
repo = "kiwiproject/kiwi"
type = "unknown-type"
`,
			wantErr: `invalid type "unknown-type"`,
		},
		{
			name: "ci_max_wait negative when ci_verify true",
			content: `
[library.kiwi]
repo = "kiwiproject/kiwi"

[settings]
ci_verify = true
ci_max_wait = "-1s"
`,
			wantErr: "ci_max_wait must be positive",
		},
		{
			name: "ci_poll_interval negative when ci_verify true",
			content: `
[library.kiwi]
repo = "kiwiproject/kiwi"

[settings]
ci_verify = true
ci_poll_interval = "-1s"
`,
			wantErr: "ci_poll_interval must be positive",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := writeTempTOML(t, tt.content)
			_, err := config.Load(path)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("expected error containing %q\ngot: %v", tt.wantErr, err)
			}
		})
	}
}

func writeTempTOML(t *testing.T, content string) string {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "*.toml")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString(content); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	return f.Name()
}
