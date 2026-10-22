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
func killProcessGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		// A negative pid signals the whole group, which Setpgid made the
		// same as bash's pid.
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
}
