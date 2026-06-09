package runner

import (
	"bytes"
	"context"
	"os/exec"
)

type OsRunner struct{}

func NewOsRunner() *OsRunner {
	return &OsRunner{}
}

func (r *OsRunner) Run(opts Options) (*Result, error) {
	var cmd *exec.Cmd
	if opts.Timeout > 0 {
		ctx, cancel := context.WithTimeout(context.Background(), opts.Timeout)
		defer cancel()
		cmd = exec.CommandContext(ctx, opts.Command, opts.Args...)
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
