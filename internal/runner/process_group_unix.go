//go:build unix

package runner

import (
	"os/exec"
	"syscall"
)

// setNewProcessGroup makes cmd the leader of a new process group, distinct
// from this tool's own, so the group can be signaled independently. It must
// only be used when the command has a timeout: an interactive Ctrl+C sends
// SIGINT to the foreground process group, and a command left in that group
// receives it directly, which is the desired behavior for commands with no
// timeout of their own.
func setNewProcessGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

// killProcessGroup sends SIGKILL to every process in cmd's process group.
// mvn release:perform forks a second Maven build; killing only the direct
// child (the default behavior of exec.CommandContext on timeout) leaves that
// fork running, free to keep deploying to Maven Central after the tool has
// already recorded the step as failed.
func killProcessGroup(cmd *exec.Cmd) error {
	// A negative PID tells kill(2) to signal the whole process group (whose
	// ID equals the leader's PID) rather than just that one process.
	return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
}
