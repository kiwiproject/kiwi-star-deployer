//go:build unix

package runner_test

import (
	"os"
	"path/filepath"
	"sync"
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

// withStdin temporarily replaces os.Stdin with f for the duration of the
// test, restoring the original on cleanup.
func withStdin(t *testing.T, f *os.File) {
	t.Helper()
	orig := os.Stdin
	os.Stdin = f
	t.Cleanup(func() { os.Stdin = orig })
}

// TestOsRunner_inheritStdinWithoutRealTTYFallsBackToDevNull proves
// InheritStdin is a complete no-op — plain /dev/null, not the piped content
// — when stdin isn't a real controlling terminal at all, matching the
// Options.InheritStdin doc comment's promise.
func TestOsRunner_inheritStdinWithoutRealTTYFallsBackToDevNull(t *testing.T) {
	stdinR, stdinW, err := os.Pipe()
	if err != nil {
		t.Fatalf("creating pipe: %v", err)
	}
	withStdin(t, stdinR)

	go func() {
		_, _ = stdinW.WriteString("hello-from-stdin")
		_ = stdinW.Close()
	}()

	r := runner.NewOsRunner()
	result, err := r.Run(runner.Options{
		Command:      "cat",
		Timeout:      5 * time.Second,
		InheritStdin: true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Stdout != "" {
		t.Errorf("expected empty stdout (cat reading /dev/null, not the piped content), got %q", result.Stdout)
	}
}

// TestOsRunner_timeoutWithoutInheritStdinDoesNotInheritStdin proves Timeout
// alone no longer implies stdin inheritance — the two are fully decoupled,
// so a future caller passing a timeout for an unrelated bounded command
// doesn't silently inherit stdin (and its SIGTTIN hazard) too.
func TestOsRunner_timeoutWithoutInheritStdinDoesNotInheritStdin(t *testing.T) {
	stdinR, stdinW, err := os.Pipe()
	if err != nil {
		t.Fatalf("creating pipe: %v", err)
	}
	withStdin(t, stdinR)

	go func() {
		_, _ = stdinW.WriteString("hello-from-stdin")
		_ = stdinW.Close()
	}()

	r := runner.NewOsRunner()
	result, err := r.Run(runner.Options{
		Command: "cat",
		Timeout: 5 * time.Second,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Stdout != "" {
		t.Errorf("expected empty stdout (cat reading /dev/null), got %q", result.Stdout)
	}
}

// TestOsRunner_inheritStdinDoesNotBlockConcurrentCalls proves two concurrent
// InheritStdin commands on the same runner do NOT block one another: this is
// deliberately best-effort (see Options.InheritStdin) rather than a hard
// serialization, specifically so a release stage's concurrent Maven release
// commands keep overlapping their (multi-minute) runtime instead of one
// waiting out the other's entire duration — which also previously caused a
// waiting command's own context timeout to be burned away before its
// command ever ran. If they were still fully serialized, two 0.2s sleeps
// would take ~0.4s total; run concurrently, ~0.2s — the threshold sits well
// between the two to avoid scheduling-jitter flakiness in either direction.
func TestOsRunner_inheritStdinDoesNotBlockConcurrentCalls(t *testing.T) {
	r := runner.NewOsRunner()
	const n = 2
	start := time.Now()

	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = r.Run(runner.Options{
				Command:      "sleep",
				Args:         []string{"0.2"},
				InheritStdin: true,
			})
		}()
	}
	wg.Wait()

	if elapsed := time.Since(start); elapsed > 300*time.Millisecond {
		t.Errorf("expected non-blocking concurrent execution (~0.2s for two overlapping 0.2s sleeps), took %v", elapsed)
	}
}

// TestOsRunner_concurrentInheritStdinWithoutRealTTYBothFallBack proves that
// when stdin isn't a real tty, *both* sides of a concurrent InheritStdin
// race fall back correctly and independently — not just the one that loses
// TryLock, but also the one that wins it, since winning TryLock only grants
// a chance at real stdin, not a guarantee (isRealTTY still gates it). Both
// should complete quickly with empty output, neither blocking the other.
func TestOsRunner_concurrentInheritStdinWithoutRealTTYBothFallBack(t *testing.T) {
	stdinR, stdinW, err := os.Pipe()
	if err != nil {
		t.Fatalf("creating pipe: %v", err)
	}
	withStdin(t, stdinR)
	defer func() { _ = stdinW.Close() }()

	r := runner.NewOsRunner()
	results := make([]string, 2)

	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			result, _ := r.Run(runner.Options{
				Command:      "cat",
				Timeout:      5 * time.Second,
				InheritStdin: true,
			})
			results[idx] = result.Stdout
		}(i)
	}
	wg.Wait()

	for i, out := range results {
		if out != "" {
			t.Errorf("expected empty (/dev/null) output from call %d, got %q", i, out)
		}
	}
}

// The positive path — a genuinely claimed real controlling terminal, where
// InheritStdin actually inherits it and the foreground handoff (and its
// later restore) actually run — is not covered by an automated test here.
// A freshly-opened pty (e.g. via github.com/creack/pty) is not itself a
// claimed controlling terminal, and attempting to claim one from within a
// test process (via Setsid+Setctty) was tried directly and found to still
// fail TIOCGPGRP with ENOTTY on Darwin — this would need a properly
// established session, which is disproportionately fragile and
// platform-specific to build reliably in an automated test. This path is
// instead verified by deliberately running the real #85 reproduction
// protocol (cold gpg-agent cache, no warm-up) against a real release before
// closing that issue.
