//go:build unix

package runner_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/kiwiproject/kiwi-star-deployer/internal/runner"
)

// TestOsRunner_timeoutKillsProcessGroup is the actual regression test for
// #80: a timed-out command that has already forked a background child (the
// mvn release:perform scenario, which forks a second Maven build) must not
// leave that fork running. The direct child backgrounds a subshell that
// sleeps briefly and then touches a marker file; if only the direct child is
// killed on timeout (exec.CommandContext's default behavior), the
// backgrounded fork survives and creates the marker after Run has already
// returned. If the whole process group is killed, it never does.
func TestOsRunner_timeoutKillsProcessGroup(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "grandchild-ran")

	r := runner.NewOsRunner()
	_, err := r.Run(runner.Options{
		Command: "bash",
		Args:    []string{"-c", "(sleep 0.3; touch " + marker + ") & sleep 10"},
		Timeout: 100 * time.Millisecond,
	})
	if err == nil {
		t.Fatal("expected error from timeout, got nil")
	}

	// Give the backgrounded subshell time to run if it survived the kill.
	time.Sleep(600 * time.Millisecond)

	if _, statErr := os.Stat(marker); statErr == nil {
		t.Fatal("backgrounded grandchild survived the timeout and ran; process group was not killed")
	}
}
