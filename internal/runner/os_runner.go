package runner

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"sync"
	"time"
)

type OsRunner struct {
	// stdinMu guards which single InheritStdin command, if more than one is
	// running concurrently on this runner, gets real terminal access — see
	// Run's use of TryLock below for why this is advisory (best-effort),
	// not a hard serialization.
	stdinMu sync.Mutex
}

func NewOsRunner() *OsRunner {
	return &OsRunner{}
}

// killGracePeriod bounds how long Run waits, after a timed-out command's
// process group has been sent SIGKILL, for its stdout/stderr pipes to close
// before giving up on them. SIGKILL cannot be caught or ignored, so this only
// guards against a descendant that escaped the process group and is still
// holding a pipe open.
const killGracePeriod = 5 * time.Second

func (r *OsRunner) Run(opts Options) (*Result, error) {
	// Resolved up front, before the context timeout below is created, and
	// never blocks: only one process can safely own the terminal's input at
	// a time, but a release stage runs several libraries' Maven release
	// commands concurrently specifically to overlap their (multi-minute)
	// runtime, and blocking one behind another for that whole duration would
	// both defeat that overlap entirely and (worse) burn away the blocked
	// command's own context timeout while it was merely waiting for the
	// lock, before its command ever ran. So this is deliberately best-effort
	// rather than a hard serialization: whichever InheritStdin command gets
	// here first keeps real stdin and the foreground handoff below; any
	// other running concurrently with it on this runner just falls back to
	// the plain /dev/null behavior for itself, same as before this fix
	// existed — not a silent SIGTTIN hang (that requires actually reading a
	// real terminal, which a /dev/null stdin never attempts), just the
	// original, already-understood, fast "No pinentry" failure from
	// kiwiproject/kiwi-star-deployer#85 if it also hits a cold gpg-agent
	// cache at the same moment — narrower than, and no worse than, the bug
	// this fix exists to address.
	inheritStdin := opts.InheritStdin
	if inheritStdin {
		if r.stdinMu.TryLock() {
			defer r.stdinMu.Unlock()
		} else {
			inheritStdin = false
		}
	}

	var cmd *exec.Cmd
	// hasOwnProcessGroup tracks whether setNewProcessGroup ran, which is the
	// only situation InheritStdin's foreground handoff is relevant to: a
	// command sharing this process's own group is already in the terminal's
	// foreground group, with nothing to hand off.
	hasOwnProcessGroup := opts.Timeout > 0
	if hasOwnProcessGroup {
		ctx, cancel := context.WithTimeout(context.Background(), opts.Timeout)
		defer cancel()
		cmd = exec.CommandContext(ctx, opts.Command, opts.Args...)
		setNewProcessGroup(cmd)
		cmd.Cancel = func() error { return killProcessGroup(cmd) }
		cmd.WaitDelay = killGracePeriod
	} else {
		cmd = exec.Command(opts.Command, opts.Args...)
	}
	if opts.WorkingDir != "" {
		cmd.Dir = opts.WorkingDir
	}

	var stdoutBuf, stderrBuf bytes.Buffer
	result := func() *Result { return &Result{Stdout: stdoutBuf.String(), Stderr: stderrBuf.String()} }

	if opts.Stdout != nil {
		cmd.Stdout = opts.Stdout
	} else {
		cmd.Stdout = &stdoutBuf
	}

	if opts.Stderr != nil {
		cmd.Stderr = opts.Stderr
	} else {
		cmd.Stderr = &stderrBuf
	}

	if !inheritStdin {
		err := cmd.Run()
		return result(), err
	}

	// Options.InheritStdin promises a no-op when stdin isn't a real
	// controlling terminal at all (CI, piped input) — fall back to the
	// plain default rather than passing through unrelated piped/redirected
	// content the command was never meant to receive.
	fd := int(os.Stdin.Fd())
	origPgid, isTTY := isRealTTY(fd)
	if !isTTY {
		err := cmd.Run()
		return result(), err
	}

	cmd.Stdin = os.Stdin

	var restore func()
	if hasOwnProcessGroup {
		restore = prepareForeground(cmd, fd, origPgid)
	}

	if err := cmd.Start(); err != nil {
		if restore != nil {
			// The foreground handoff happens inside the child itself,
			// before it execs — it can already have taken effect even if
			// the child then fails before (or during) exec, e.g. a bad
			// working directory or a permissions error.
			restore()
		}
		return result(), err
	}
	err := cmd.Wait()
	if restore != nil {
		restore()
	}

	return result(), err
}
