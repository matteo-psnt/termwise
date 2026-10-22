//go:build unix

package tools

import (
	"os/exec"
	"syscall"
)

// killProcessGroup makes cancelling cmd kill everything it started. bash
// forks children for pipelines and command lists, and killing bash alone
// leaves them running and holding its stdout open, so Esc would not stop
// the command and the tool call would not return until they finished.
//
// The command gets a session of its own (Setsid), not just a process group.
// A group alone would stay attached to termwise's terminal as a background
// group, and anything reading the terminal (an ssh, sudo or git credential
// prompt) would be stopped by SIGTTIN and hang silently. In a new session
// there is no controlling terminal, so such a read fails at once and the
// command reports it.
func killProcessGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	cmd.Cancel = func() error {
		// A negative pid signals the whole group; a new session's leader
		// heads a group whose id is its own pid.
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
}
