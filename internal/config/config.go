package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
)

const (
	TypeParentPOM  = "parent-pom"
	TypeBOM        = "bom"
	TypeLibraryBOM = "library-bom"
)

type Config struct {
	Settings  Settings           `toml:"settings"`
	Libraries map[string]Library `toml:"library"`
	Release   ReleaseConfig      `toml:"release"`
}

type Settings struct {
	Workspace                string   `toml:"workspace"`
	StatePath                string   `toml:"state_path"`
	GroupID                  string   `toml:"group_id"`
	ChangelogScript          string   `toml:"changelog_script"`
	MavenReleaseTimeout      Duration `toml:"maven_release_timeout"`
	MavenCentralMaxWait      Duration `toml:"maven_central_max_wait"`
	MavenCentralPollInterval Duration `toml:"maven_central_poll_interval"`
	CIVerify                 *bool    `toml:"ci_verify"`
	CIMaxWait                Duration `toml:"ci_max_wait"`
	CIPollInterval           Duration `toml:"ci_poll_interval"`
}

// LogsDir returns the base directory where release run logs are stored.
func (s Settings) LogsDir() string {
	return filepath.Join(filepath.Dir(s.Workspace), "logs")
}

type Library struct {
	Repo      string   `toml:"repo"`
	Type      string   `toml:"type"`
	DependsOn []string `toml:"depends_on"`
}

type ReleaseConfig struct {
	Overrides map[string]string `toml:"overrides"`
}

// Duration is a time.Duration that deserializes from a TOML string (e.g. "30s", "1h").
type Duration time.Duration

func (d *Duration) UnmarshalText(text []byte) error {
	dur, err := time.ParseDuration(string(text))
	if err != nil {
		return fmt.Errorf("invalid duration %q: %w", string(text), err)
	}
	*d = Duration(dur)
	return nil
}

func Load(path string) (*Config, error) {
	expanded, err := expandHome(path)
	if err != nil {
		return nil, fmt.Errorf("expanding config path %q: %w", path, err)
	}
	path = expanded

	var cfg Config
	if _, err := toml.DecodeFile(path, &cfg); err != nil {
		return nil, fmt.Errorf("loading config %s: %w", path, err)
	}
	applyDefaults(&cfg)
	if err := expandPaths(&cfg); err != nil {
		return nil, err
	}
	if err := validate(&cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func applyDefaults(cfg *Config) {
	if cfg.Settings.Workspace == "" {
		cfg.Settings.Workspace = "~/.kiwi-star-deployer/workspace"
	}
	if cfg.Settings.StatePath == "" {
		cfg.Settings.StatePath = "~/.kiwi-star-deployer/state.json"
	}
	if cfg.Settings.GroupID == "" {
		cfg.Settings.GroupID = "org.kiwiproject"
	}
	if cfg.Settings.ChangelogScript == "" {
		cfg.Settings.ChangelogScript = ".generate-kiwi-changelog"
	}
	if cfg.Settings.MavenReleaseTimeout == 0 {
		cfg.Settings.MavenReleaseTimeout = Duration(60 * time.Minute)
	}
	if cfg.Settings.MavenCentralMaxWait == 0 {
		cfg.Settings.MavenCentralMaxWait = Duration(60 * time.Minute)
	}
	if cfg.Settings.MavenCentralPollInterval == 0 {
		cfg.Settings.MavenCentralPollInterval = Duration(30 * time.Second)
	}
	if cfg.Settings.CIVerify == nil {
		t := true
		cfg.Settings.CIVerify = &t
	}
	if cfg.Settings.CIMaxWait == 0 {
		cfg.Settings.CIMaxWait = Duration(30 * time.Minute)
	}
	if cfg.Settings.CIPollInterval == 0 {
		cfg.Settings.CIPollInterval = Duration(30 * time.Second)
	}
}

func expandPaths(cfg *Config) error {
	expanded, err := expandHome(cfg.Settings.Workspace)
	if err != nil {
		return fmt.Errorf("expanding workspace path %q: %w", cfg.Settings.Workspace, err)
	}
	cfg.Settings.Workspace = expanded

	expanded, err = expandHome(cfg.Settings.StatePath)
	if err != nil {
		return fmt.Errorf("expanding state_path %q: %w", cfg.Settings.StatePath, err)
	}
	cfg.Settings.StatePath = expanded

	expanded, err = expandHome(cfg.Settings.ChangelogScript)
	if err != nil {
		return fmt.Errorf("expanding changelog_script %q: %w", cfg.Settings.ChangelogScript, err)
	}
	cfg.Settings.ChangelogScript = expanded
	return nil
}

func expandHome(path string) (string, error) {
	if !strings.HasPrefix(path, "~") {
		return path, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, path[1:]), nil
}
