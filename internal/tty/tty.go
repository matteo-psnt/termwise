package tty

import (
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	"golang.org/x/term"
)

// IsTerminal reports whether f is connected to a terminal.
func IsTerminal(f *os.File) bool {
	return term.IsTerminal(int(f.Fd()))
}

// QueryCursorRow returns the cursor's current 0-indexed row by sending DSR
// (ESC[6n) to out and parsing the ESC[<row>;<col>R reply from in. Briefly
// puts in into raw mode; times out at ~200ms.
func QueryCursorRow(in *os.File, out io.Writer) (int, error) {
	fd := int(in.Fd())
	if !term.IsTerminal(fd) {
		return 0, errors.New("input is not a terminal")
	}
	oldState, err := term.MakeRaw(fd)
	if err != nil {
		return 0, fmt.Errorf("raw mode: %w", err)
	}
	defer func() { _ = term.Restore(fd, oldState) }()

	if _, err := out.Write([]byte("\x1b[6n")); err != nil {
		return 0, fmt.Errorf("writing DSR: %w", err)
	}

	deadline := time.Now().Add(200 * time.Millisecond)
	_ = in.SetReadDeadline(deadline)
	defer func() { _ = in.SetReadDeadline(time.Time{}) }()

	// Reply: ESC[<row>;<col>R. Read byte-by-byte until 'R'.
	var buf [32]byte
	var got []byte
	for len(got) < len(buf) {
		var b [1]byte
		n, err := in.Read(b[:])
		if err != nil {
			return 0, fmt.Errorf("reading DSR reply: %w", err)
		}
		if n == 0 {
			continue
		}
		got = append(got, b[0])
		if b[0] == 'R' {
			break
		}
	}

	var row, col int
	esc := -1
	for i, c := range got {
		if c == 0x1b {
			esc = i
			break
		}
	}
	if esc < 0 || esc+1 >= len(got) || got[esc+1] != '[' {
		return 0, fmt.Errorf("malformed DSR reply: %q", got)
	}
	if _, err := fmt.Sscanf(string(got[esc:]), "\x1b[%d;%dR", &row, &col); err != nil {
		return 0, fmt.Errorf("parsing DSR reply %q: %w", got, err)
	}
	if row < 1 {
		return 0, fmt.Errorf("DSR row out of range: %d", row)
	}
	return row - 1, nil
}
