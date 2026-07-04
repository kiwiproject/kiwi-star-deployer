package cmd

import (
	"testing"
)

func TestResolveConfigPath_flagWins(t *testing.T) {
	t.Setenv(configEnvVar, "/from/env.toml")
	result := resolveConfigPath(true, "/from/flag.toml")
	if result != "/from/flag.toml" {
		t.Errorf("flag should win: got %q, want /from/flag.toml", result)
	}
}

func TestResolveConfigPath_envVarUsedWhenNoFlag(t *testing.T) {
	t.Setenv(configEnvVar, "/from/env.toml")
	result := resolveConfigPath(false, "kiwi-star-deployer.toml")
	if result != "/from/env.toml" {
		t.Errorf("env var should be used: got %q, want /from/env.toml", result)
	}
}

func TestResolveConfigPath_defaultWhenNeitherSet(t *testing.T) {
	t.Setenv(configEnvVar, "")
	result := resolveConfigPath(false, "kiwi-star-deployer.toml")
	if result != "kiwi-star-deployer.toml" {
		t.Errorf("default should be used: got %q, want kiwi-star-deployer.toml", result)
	}
}
