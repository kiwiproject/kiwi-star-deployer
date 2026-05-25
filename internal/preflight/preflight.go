package preflight

import (
	"fmt"
	"io"
	"os/exec"
	"text/tabwriter"

	"github.com/kiwiproject/kiwi-star-deployer/internal/runner"
)

// Result holds the outcome of a single preflight check.
type Result struct {
	Name    string
	OK      bool
	Message string // non-empty on failure, explains what to do
}

// RunAll runs all preflight checks and returns their results in order.
func RunAll(r runner.Runner, changelogScript string) []Result {
	return []Result{
		checkInPath("git"),
		checkInPath("gh"),
		checkGHAuth(r),
		checkInPath("mvn"),
		checkInPath(changelogScript),
	}
}

// AllPassed returns true if every result passed.
func AllPassed(results []Result) bool {
	for _, r := range results {
		if !r.OK {
			return false
		}
	}
	return true
}

// Print writes a formatted pass/fail table to w.
func Print(w io.Writer, results []Result) {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	for _, r := range results {
		status := "[PASS]"
		msg := ""
		if !r.OK {
			status = "[FAIL]"
			msg = r.Message
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\n", status, r.Name, msg)
	}
	_ = tw.Flush()
}

func checkInPath(name string) Result {
	if _, err := exec.LookPath(name); err != nil {
		return Result{
			Name:    name,
			OK:      false,
			Message: fmt.Sprintf("%s not found on PATH", name),
		}
	}
	return Result{Name: name, OK: true}
}

func checkGHAuth(r runner.Runner) Result {
	if _, err := r.Run(runner.Options{
		Command: "gh",
		Args:    []string{"auth", "status"},
	}); err != nil {
		return Result{
			Name:    "gh auth",
			OK:      false,
			Message: "gh is not authenticated; run 'gh auth login'",
		}
	}
	return Result{Name: "gh auth", OK: true}
}
