package tools

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"testing"
	"time"
	"unicode/utf8"
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

func writeTemp(t *testing.T, content string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "f.txt")
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestReadTrailingNewlineIsNotALine(t *testing.T) {
	p := writeTemp(t, strings.Repeat("x\n", maxReadLines))
	out, _ := read(context.Background(), map[string]any{"path": p})
	if strings.Contains(out, "truncated") {
		t.Fatalf("a %d-line file was reported truncated: %q", maxReadLines, out[len(out)-80:])
	}
	out, _ = read(context.Background(), map[string]any{"path": p, "offset": float64(maxReadLines)})
	if !strings.Contains(out, fmt.Sprintf("File has %d lines", maxReadLines)) {
		t.Fatalf("got %q", out)
	}
}

func TestReadEmptyFile(t *testing.T) {
	out, isErr := read(context.Background(), map[string]any{"path": writeTemp(t, "")})
	if isErr || out != "[File is empty]" {
		t.Fatalf("got (%q, %v)", out, isErr)
	}
}

func TestReadCapsLongLines(t *testing.T) {
	long := strings.Repeat("é", maxReadLineBytes) // twice the cap in bytes
	out, _ := read(context.Background(), map[string]any{"path": writeTemp(t, "short\n"+long+"\n")})
	first, second, _ := strings.Cut(out, "\n")
	if first != "short" {
		t.Fatalf("short line changed: %q", first)
	}
	if !strings.Contains(second, "[line truncated") || len(second) > maxReadLineBytes+64 || !utf8.ValidString(second) {
		t.Fatalf("long line not capped cleanly: %d bytes", len(second))
	}
}

func TestReadTruncationNamesNextOffset(t *testing.T) {
	p := writeTemp(t, "a\nb\nc\nd\n")
	out, _ := read(context.Background(), map[string]any{"path": p, "offset": float64(1), "limit": float64(2)})
	if want := "b\nc\n[truncated — showing lines 2–3; the file has more. Use offset=3 to read more.]"; out != want {
		t.Fatalf("got %q, want %q", out, want)
	}
	next, _ := read(context.Background(), map[string]any{"path": p, "offset": float64(3)})
	if next != "d" {
		t.Fatalf("continuing at the named offset gave %q, want %q", next, "d")
	}
}

// A huge log must cost only the lines asked for. The file is sparse, so it is
// cheap to create (text first, so it is not taken for binary), but reading it
// whole would allocate gigabytes.
func TestReadStreamsHugeFile(t *testing.T) {
	p := filepath.Join(t.TempDir(), "huge.log")
	f, err := os.Create(p)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString(strings.Repeat("log line\n", 1000)); err != nil {
		t.Fatal(err)
	}
	if err := f.Truncate(4 << 30); err != nil {
		t.Skip("cannot create a sparse file:", err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}

	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	out, isErr := read(context.Background(), map[string]any{"path": p, "offset": float64(10), "limit": float64(5)})
	runtime.ReadMemStats(&after)

	want := strings.Repeat("log line\n", 5) + "[truncated — showing lines 11–15; the file has more. Use offset=15 to read more.]"
	if isErr || out != want {
		t.Fatalf("got (%q, %v), want %q", out, isErr, want)
	}
	if alloc := after.TotalAlloc - before.TotalAlloc; alloc > 8<<20 {
		t.Fatalf("read allocated %d bytes for 5 lines", alloc)
	}
}

func TestReadOffsetPastEndCountsLines(t *testing.T) {
	out, _ := read(context.Background(), map[string]any{"path": writeTemp(t, "a\n\nb"), "offset": float64(5)})
	if want := "[File has 3 lines; offset 5 is out of range]"; out != want {
		t.Fatalf("got %q, want %q", out, want)
	}
}

func TestReadBinaryFile(t *testing.T) {
	out, _ := read(context.Background(), map[string]any{"path": writeTemp(t, "ab\x00cd\n")})
	if !strings.HasPrefix(out, "[Binary file:") {
		t.Fatalf("got %q", out)
	}
}
