//go:build unix

package tty

import (
	"os"
	"syscall"
	"testing"
	"time"
)

// blockingPipe returns a pipe whose read end, like a terminal on macOS, is
// not pollable by the Go runtime, so SetReadDeadline does not work on it.
func blockingPipe(t *testing.T) (*os.File, *os.File) {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	fd, err := syscall.Dup(int(r.Fd())) // Fd leaves r blocking
	if err != nil {
		t.Fatal(err)
	}
	_ = r.Close()
	in := os.NewFile(uintptr(fd), "pipe")
	if in.SetReadDeadline(time.Now()) == nil {
		t.Skip("pipe unexpectedly supports deadlines")
	}
	t.Cleanup(func() { _ = in.Close(); _ = w.Close() })
	return in, w
}

func TestReadDSRReplyTimesOutWithoutDeadlineSupport(t *testing.T) {
	in, _ := blockingPipe(t)
	done := make(chan error, 1)
	go func() {
		_, err := readDSRReply(in, 100*time.Millisecond)
		done <- err
	}()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("expected a timeout error")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("readDSRReply blocked on a silent terminal")
	}
}

func TestReadDSRReplyReadsReply(t *testing.T) {
	in, w := blockingPipe(t)
	if _, err := w.WriteString("\x1b[12;5R"); err != nil {
		t.Fatal(err)
	}
	got, err := readDSRReply(in, time.Second)
	if err != nil || string(got) != "\x1b[12;5R" {
		t.Fatalf("got (%q, %v)", got, err)
	}
}
