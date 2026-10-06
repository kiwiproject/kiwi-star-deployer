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
	// InheritStdin gives the command the real os.Stdin instead of the default
	// /dev/null, but only if stdin is a real controlling terminal at all —
	// it is a complete no-op (plain /dev/null, exactly as if this were
	// false) for CI, piped input, or anything else that isn't an actual tty,
	// rather than passing through unrelated piped/redirected content the
	// command was never meant to receive.
	//
	// If Timeout > 0 (the command therefore runs in its own process group —
	// see setNewProcessGroup — distinct from the terminal's foreground
	// group), inheriting stdin also temporarily hands that process group
	// foreground ownership of the controlling terminal for the duration of
	// the command, so an interactive read it performs (e.g. mvn's nested
	// gpg:sign needing a passphrase prompt) doesn't get stopped by SIGTTIN.
	// Without a timeout this handoff is skipped — the command already
	// shares this runner's own, already-foreground group — so it is safe
	// (though not meaningful) to set InheritStdin alone.
	//
	// Only one process can safely own the terminal's input at a time, but
	// this is intentionally best-effort rather than a hard guarantee: if
	// another InheritStdin command is already running on the same *OsRunner,
	// this one falls back to plain /dev/null stdin for itself rather than
	// blocking for the other's entire (multi-minute) duration, which would
	// defeat the very concurrency a release stage runs these commands for.
	// That fallback is not a new failure mode — it's exactly the plain
	// /dev/null behavior every command had before this option existed.
	InheritStdin bool
}

// Result holds captured output from a completed command.
// Stdout and Stderr are only populated when the corresponding Options fields were nil.
type Result struct {
	Stdout string
	Stderr string
}
