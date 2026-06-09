package runner

import (
	"io"
	"time"
)

// Runner executes external commands (gh, git, mvn, kiwiproject-changelog).
// All callers must go through this interface rather than exec.Command directly.
type Runner interface {
	Run(opts Options) (*Result, error)
}

// Options configures a single external command invocation.
type Options struct {
	Command    string
	Args       []string
	WorkingDir string
	// If non-nil, stdout is written here (streamed); otherwise captured in Result.Stdout.
	Stdout io.Writer
	// If non-nil, stderr is written here (streamed); otherwise captured in Result.Stderr.
	Stderr io.Writer
	// Timeout, if positive, cancels the command after the given duration.
	// Zero means no timeout.
	Timeout time.Duration
}

// Result holds captured output from a completed command.
// Stdout and Stderr are only populated when the corresponding Options fields were nil.
type Result struct {
	Stdout string
	Stderr string
}
