//go:build !unix

package tools

import "os/exec"

// killProcessGroup is a no-op where process groups are unavailable; the
// WaitDelay set by bash still bounds how long a cancelled command can block.
func killProcessGroup(*exec.Cmd) {}
