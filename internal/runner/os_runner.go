package runner

import (
	"bytes"
	"os/exec"
)

type OsRunner struct{}

func NewOsRunner() *OsRunner {
	return &OsRunner{}
}

func (r *OsRunner) Run(opts Options) (*Result, error) {
	cmd := exec.Command(opts.Command, opts.Args...)
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
