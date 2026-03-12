package tty

import (
	"os"

	"golang.org/x/term"
)

// IsTerminal reports whether f is connected to a terminal.
func IsTerminal(f *os.File) bool {
	return term.IsTerminal(int(f.Fd()))
}
