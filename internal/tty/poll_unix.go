//go:build unix

package tty

import (
	"errors"
	"os"
	"time"

	"golang.org/x/sys/unix"
)

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
		if n > 0 {
			return nil
		}
	}
}
