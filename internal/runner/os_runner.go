package runner

import (
	"bytes"
	"context"
	"os/exec"
	"time"
)

type OsRunner struct{}

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
	var cmd *exec.Cmd
	if opts.Timeout > 0 {
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

	err := cmd.Run()
	return &Result{
		Stdout: stdoutBuf.String(),
		Stderr: stderrBuf.String(),
	}, err
}
