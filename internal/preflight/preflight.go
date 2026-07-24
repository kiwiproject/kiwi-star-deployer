package preflight

import (
	"fmt"
	"io"
	"os/exec"
	"strconv"
	"strings"
	"text/tabwriter"

	"github.com/kiwiproject/kiwi-star-deployer/internal/runner"
)

// minGHVersion is the minimum required gh CLI version. CI verification runs
// gh run list --commit, which was added in gh 2.40.0.
var minGHVersion = [3]int{2, 40, 0}

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
		checkGHVersion(r),
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

// Print writes a formatted pass/fail table to w, followed by a note about
// prerequisites that cannot be verified in advance.
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
	fmt.Fprintln(w, "\nNote: GPG signing and Maven Central publishing credentials cannot be verified")
	fmt.Fprintln(w, "in advance; mvn release:perform is the first step that exercises them. Ensure")
	fmt.Fprintln(w, "gpg-agent and ~/.m2/settings.xml are configured before the first release.")
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

func checkGHVersion(r runner.Runner) Result {
	result, err := r.Run(runner.Options{
		Command: "gh",
		Args:    []string{"--version"},
	})
	if err != nil {
		return Result{Name: "gh version", OK: false, Message: "could not run gh --version"}
	}
	v, err := parseGHVersion(result.Stdout)
	if err != nil {
		return Result{Name: "gh version", OK: false, Message: err.Error()}
	}
	if lessThan(v, minGHVersion) {
		return Result{
			Name: "gh version",
			OK:   false,
			Message: fmt.Sprintf("gh %d.%d.%d found, but %d.%d.%d or later is required (for gh run list --commit)",
				v[0], v[1], v[2], minGHVersion[0], minGHVersion[1], minGHVersion[2]),
		}
	}
	return Result{Name: "gh version", OK: true}
}

// parseGHVersion extracts the semantic version from gh --version output,
// whose first line looks like "gh version 2.62.0 (2024-11-14)". Pre-release
// or build suffixes after a hyphen are ignored.
func parseGHVersion(output string) ([3]int, error) {
	line, _, _ := strings.Cut(strings.TrimSpace(output), "\n")
	fields := strings.Fields(line)
	if len(fields) < 3 || fields[0] != "gh" || fields[1] != "version" {
		return [3]int{}, fmt.Errorf("unrecognized gh --version output: %q", line)
	}
	verStr, _, _ := strings.Cut(fields[2], "-")
	segs := strings.Split(verStr, ".")
	if len(segs) != 3 {
		return [3]int{}, fmt.Errorf("unrecognized gh version %q", fields[2])
	}
	var v [3]int
	for i, s := range segs {
		n, err := strconv.Atoi(s)
		if err != nil {
			return [3]int{}, fmt.Errorf("unrecognized gh version %q", fields[2])
		}
		v[i] = n
	}
	return v, nil
}

func lessThan(a, b [3]int) bool {
	for i := range a {
		if a[i] != b[i] {
			return a[i] < b[i]
		}
	}
	return false
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
