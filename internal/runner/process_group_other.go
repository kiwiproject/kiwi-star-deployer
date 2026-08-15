//go:build !unix

package runner

import "os/exec"

// setNewProcessGroup and killProcessGroup have no portable equivalent outside
// Unix; this tool is only built and run on Unix hosts, so these are no-ops
// that fall back to exec.CommandContext's default kill-the-direct-child
// behavior rather than leaving the build broken on other platforms.
func setNewProcessGroup(_ *exec.Cmd) {}

func killProcessGroup(cmd *exec.Cmd) error {
	return cmd.Process.Kill()
}
