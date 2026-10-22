package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
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
