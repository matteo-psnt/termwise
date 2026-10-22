//go:build unix

package tty

import (
	"errors"
	"io"
	"os"
	"time"

	"golang.org/x/sys/unix"
)

// errNotPollable means poll can't watch the descriptor. macOS reports
// /dev/tty that way (POLLNVAL at once), and treating that as readable would
// leave the following read blocked forever on a terminal that never answers.
var errNotPollable = errors.New("terminal does not support poll")

// waitReadable blocks until fd has data to read or timeout passes, in which
// case it returns os.ErrDeadlineExceeded.
func waitReadable(fd int, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		remaining := time.Until(deadline)
		if remaining <= 0 {
			return os.ErrDeadlineExceeded
		}
		fds := []unix.PollFd{{Fd: int32(fd), Events: unix.POLLIN}}
		n, err := unix.Poll(fds, int(remaining.Milliseconds())+1)
		if errors.Is(err, unix.EINTR) {
			continue
		}
		if err != nil {
			return err
		}
		if n == 0 {
			continue
		}
		switch rev := fds[0].Revents; {
		case rev&unix.POLLIN != 0:
			return nil
		case rev&unix.POLLNVAL != 0:
			return errNotPollable
		default: // POLLERR or POLLHUP with nothing to read
			return io.EOF
		}
	}
}
