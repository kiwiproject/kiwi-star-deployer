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
	TypeParentPOM     = "parent-pom"
	TypeBOM           = "bom"
	TypeBOMAggregator = "bom-aggregator"
)

type Config struct {
	Settings  Settings           `toml:"settings"`
	Libraries map[string]Library `toml:"library"`
	Release   ReleaseConfig      `toml:"release"`
}

type Settings struct {
	Workspace                string   `toml:"workspace"`
	GroupID                  string   `toml:"group_id"`
	ChangelogScript          string   `toml:"changelog_script"`
	MavenCentralMaxWait      Duration `toml:"maven_central_max_wait"`
	MavenCentralPollInterval Duration `toml:"maven_central_poll_interval"`
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
	if cfg.Settings.GroupID == "" {
		cfg.Settings.GroupID = "org.kiwiproject"
	}
	if cfg.Settings.ChangelogScript == "" {
		cfg.Settings.ChangelogScript = ".generate-kiwi-changelog"
	}
	if cfg.Settings.MavenCentralMaxWait == 0 {
		cfg.Settings.MavenCentralMaxWait = Duration(60 * time.Minute)
	}
	if cfg.Settings.MavenCentralPollInterval == 0 {
		cfg.Settings.MavenCentralPollInterval = Duration(30 * time.Second)
	}
}

func expandPaths(cfg *Config) error {
	expanded, err := expandHome(cfg.Settings.Workspace)
	if err != nil {
		return fmt.Errorf("expanding workspace path %q: %w", cfg.Settings.Workspace, err)
	}
	cfg.Settings.Workspace = expanded
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
