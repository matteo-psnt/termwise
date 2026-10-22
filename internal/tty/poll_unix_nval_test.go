//go:build unix

package tty

import (
	"os"
	"testing"
	"time"
)

// poll reports POLLNVAL for a descriptor it can't watch (as macOS does for
// /dev/tty). That must be an error, not "readable": the read that follows
// would block forever on a terminal that never answers.
func TestWaitReadableRejectsUnpollableFD(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	fd := int(r.Fd())
	_ = r.Close()
	_ = w.Close()

	start := time.Now()
	if err := waitReadable(fd, 2*time.Second); err == nil {
		t.Fatal("waitReadable on a closed descriptor reported it readable")
	}
	if time.Since(start) > time.Second {
		t.Error("waitReadable waited out its timeout instead of failing at once")
	}
}
