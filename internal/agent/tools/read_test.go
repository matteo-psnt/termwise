package tools

import (
	"context"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestReadRejectsFIFO(t *testing.T) {
	fifo := filepath.Join(t.TempDir(), "fifo")
	if err := syscall.Mkfifo(fifo, 0o600); err != nil {
		t.Skip("mkfifo unsupported:", err)
	}
	done := make(chan string, 1)
	go func() {
		out, _ := read(context.Background(), map[string]any{"path": fifo})
		done <- out
	}()
	select {
	case out := <-done:
		if !strings.Contains(out, "not a regular file") {
			t.Fatalf("got %q", out)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("read blocked on a FIFO")
	}
}
