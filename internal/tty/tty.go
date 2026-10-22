// Package tty holds the small terminal helpers shared by the CLI and the TUIs:
// detecting a terminal, reading piped stdin, prompting for a secret without
// echo, and asking the terminal where the cursor is.
package tty

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"golang.org/x/term"
)

// IsTerminal reports whether f is connected to a terminal.
func IsTerminal(f *os.File) bool {
	return term.IsTerminal(int(f.Fd()))
}

// ReadPiped reads all of f when it is not a terminal, with trailing newlines
// trimmed, and reports true. A terminal is never read, so an interactive
// stdin does not block waiting for EOF; it yields ("", false, nil).
func ReadPiped(f *os.File) (string, bool, error) {
	if IsTerminal(f) {
		return "", false, nil
	}
	data, err := io.ReadAll(f)
	if err != nil {
		return "", true, err
	}
	return strings.TrimRight(string(data), "\n"), true, nil
}

// ReadSecret reads one secret, such as an API key, from stdin. A piped stdin
// is read whole, so scripts can supply it; on a terminal, prompt is written to
// stderr (keeping stdout clean) and the input is read without echo. Surrounding
// whitespace is trimmed, and an empty result is an error.
func ReadSecret(prompt string) (string, error) {
	var raw []byte
	var err error
	if IsTerminal(os.Stdin) {
		fmt.Fprint(os.Stderr, prompt)
		raw, err = term.ReadPassword(int(os.Stdin.Fd()))
		// ReadPassword swallows the Enter, so end the prompt line ourselves.
		fmt.Fprintln(os.Stderr)
	} else {
		raw, err = io.ReadAll(os.Stdin)
	}
	if err != nil {
		return "", fmt.Errorf("reading key: %w", err)
	}
	key := strings.TrimSpace(string(raw))
	if key == "" {
		return "", errors.New("no key given")
	}
	return key, nil
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

	got, err := readDSRReply(in, 200*time.Millisecond)
	if err != nil {
		return 0, err
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

// readDSRReply reads a cursor position reply (ESC[<row>;<col>R) from in,
// byte by byte until 'R', giving up after timeout. It waits with poll(2) rather
// than a read deadline: deadlines need a pollable file, and a terminal never is
// on macOS (kqueue does not support ttys), so SetReadDeadline fails there and a
// terminal that never answers would block startup forever. Polling also means
// no read is left pending to swallow the user's first keystroke later.
func readDSRReply(in *os.File, timeout time.Duration) ([]byte, error) {
	fd := int(in.Fd())
	deadline := time.Now().Add(timeout)
	// Far more than any real reply needs.
	const maxReply = 32
	var got []byte
	for len(got) < maxReply {
		if err := waitReadable(fd, time.Until(deadline)); err != nil {
			return nil, fmt.Errorf("reading DSR reply: %w", err)
		}
		var b [1]byte
		n, err := in.Read(b[:])
		if err != nil {
			return nil, fmt.Errorf("reading DSR reply: %w", err)
		}
		if n == 0 {
			continue
		}
		got = append(got, b[0])
		if b[0] == 'R' {
			break
		}
	}
	return got, nil
}
