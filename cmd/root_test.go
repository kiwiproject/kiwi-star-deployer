package cmd

import (
	"os"
	"testing"
)

func TestConfigPathResolution_flagWins(t *testing.T) {
	t.Setenv(configEnvVar, "/from/env.toml")
	configPath = "/from/flag.toml"

	rootCmd.PersistentPreRunE(rootCmd, nil)

	if configPath != "/from/flag.toml" {
		t.Errorf("flag should win: got %q, want /from/flag.toml", configPath)
	}
}

func TestConfigPathResolution_envVarUsedWhenNoFlag(t *testing.T) {
	t.Setenv(configEnvVar, "/from/env.toml")
	configPath = ""

	rootCmd.PersistentPreRunE(rootCmd, nil)

	if configPath != "/from/env.toml" {
		t.Errorf("env var should be used: got %q, want /from/env.toml", configPath)
	}
}

func TestConfigPathResolution_defaultWhenNeitherSet(t *testing.T) {
	os.Unsetenv(configEnvVar)
	configPath = ""

	rootCmd.PersistentPreRunE(rootCmd, nil)

	if configPath != "kiwi-star-deployer.toml" {
		t.Errorf("default should be used: got %q, want kiwi-star-deployer.toml", configPath)
	}
}
