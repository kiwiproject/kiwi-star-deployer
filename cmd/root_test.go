package cmd

import (
	"os"
	"testing"
)

func TestConfigPathResolution_flagWins(t *testing.T) {
	t.Setenv(configEnvVar, "/from/env.toml")
	configPath = "/from/flag.toml"

	if err := rootCmd.PersistentPreRunE(rootCmd, nil); err != nil {
		t.Fatal(err)
	}

	if configPath != "/from/flag.toml" {
		t.Errorf("flag should win: got %q, want /from/flag.toml", configPath)
	}
}

func TestConfigPathResolution_envVarUsedWhenNoFlag(t *testing.T) {
	t.Setenv(configEnvVar, "/from/env.toml")
	configPath = ""

	if err := rootCmd.PersistentPreRunE(rootCmd, nil); err != nil {
		t.Fatal(err)
	}

	if configPath != "/from/env.toml" {
		t.Errorf("env var should be used: got %q, want /from/env.toml", configPath)
	}
}

func TestConfigPathResolution_defaultWhenNeitherSet(t *testing.T) {
	if orig, exists := os.LookupEnv(configEnvVar); exists {
		if err := os.Unsetenv(configEnvVar); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { os.Setenv(configEnvVar, orig) }) //nolint:errcheck
	}
	configPath = ""

	if err := rootCmd.PersistentPreRunE(rootCmd, nil); err != nil {
		t.Fatal(err)
	}

	if configPath != "kiwi-star-deployer.toml" {
		t.Errorf("default should be used: got %q, want kiwi-star-deployer.toml", configPath)
	}
}
