//go:build unix

package runner

import (
	"os/exec"
	"os/signal"
	"syscall"
	"unsafe"
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

// isRealTTY reports whether fd is a real controlling terminal at all — as
// opposed to a pipe, /dev/null, or other non-terminal stdin (CI, piped
// input) — returning its current foreground process group if so. Callers
// use this to decide whether inheriting stdin (and the foreground handoff
// below) applies at all: without a real tty there is no SIGTTIN hazard to
// guard against, and inheriting a piped/redirected non-terminal stdin into
// the child would be an unrelated, unintended behavior change.
//
// A non-error result alone is not sufficient: TIOCGPGRP against a plain
// pipe succeeds on at least Darwin, returning pgid 0 rather than failing —
// confirmed by direct testing, not merely platform docs. 0 is never a valid
// process group ID for a real foreground group, so it is treated the same
// as an error here.
func isRealTTY(fd int) (origPgid int, ok bool) {
	pgid, err := tcgetpgrp(fd)
	if err != nil || pgid <= 0 {
		return 0, false
	}
	return pgid, true
}

// prepareForeground arranges for cmd's own process group (created by
// setNewProcessGroup, which must already have run — cmd.SysProcAttr must be
// non-nil) to be the terminal's foreground group for as long as cmd needs to
// read real interactive input — e.g. a passphrase prompt during mvn's nested
// gpg:sign. Without this, cmd's process group is a *background* group
// relative to the terminal purely because it is its own separate group
// (setNewProcessGroup's doing, for an unrelated reason: so a timeout kill
// can reach cmd's whole process tree, not just the direct child) — and the
// kernel stops (SIGTTIN) any attempt a background group makes to actually
// read from the terminal, turning what would otherwise be a fast, clear
// failure into a silent hang until the caller's timeout kills it. Confirmed
// by direct reproduction: see kiwiproject/kiwi-star-deployer#85.
//
// This uses the standard library's Foreground/Ctty SysProcAttr fields rather
// than a separate ioctl call after cmd.Start(): the handoff then happens
// inside the child itself, as part of the fork/exec sequence, before it ever
// execs into the real program — there is no window between the process
// existing and the handoff actually taking effect for it to race against.
//
// Must be called before cmd.Start(), and only once isRealTTY has confirmed
// fd is a real tty and produced origPgid. Returns a restore func that must
// be called once cmd finishes, regardless of outcome, to hand the terminal
// back to whatever group owned it before — otherwise this tool's own later
// terminal interaction (e.g. Ctrl+C) stops working correctly for the rest of
// the run.
func prepareForeground(cmd *exec.Cmd, fd, origPgid int) func() {
	cmd.SysProcAttr.Foreground = true
	cmd.SysProcAttr.Ctty = fd
	return func() {
		// Restoring from a group the terminal no longer considers
		// foreground raises SIGTTOU on the caller — ignored defensively
		// for just this one call, the same way a shell ignores it around
		// its own tcsetpgrp calls, then reset immediately afterward so
		// SIGTTOU's normal handling isn't disabled for the rest of this
		// tool's own process for the remainder of the run.
		signal.Ignore(syscall.SIGTTOU)
		_ = tcsetpgrp(fd, origPgid)
		signal.Reset(syscall.SIGTTOU)
	}
}

func tcgetpgrp(fd int) (int, error) {
	var pgid int32
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, uintptr(fd), uintptr(syscall.TIOCGPGRP), uintptr(unsafe.Pointer(&pgid)))
	if errno != 0 {
		return 0, errno
	}
	return int(pgid), nil
}

func tcsetpgrp(fd, pgid int) error {
	p := int32(pgid)
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, uintptr(fd), uintptr(syscall.TIOCSPGRP), uintptr(unsafe.Pointer(&p)))
	if errno != 0 {
		return errno
	}
	return nil
}
