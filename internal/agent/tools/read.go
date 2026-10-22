package tools

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/matteo-psnt/termwise/internal/provider"
)

const (
	maxReadLines      = 2000
	maxDirListEntries = 500
	// maxReadLineBytes caps each line. A minified bundle or data file can be
	// one multi-megabyte line, which would flood the context on its own.
	maxReadLineBytes = 2000
	// binarySniffBytes is how much of the start of a file isBinary inspects.
	binarySniffBytes = 8192
	readBufferSize   = 64 * 1024
)

func init() {
	stepFns[ReadDef.Name] = func(_ provider.ToolCall) Step {
		return Step{Kind: StepExecutor, Execute: executor(read)}
	}
	detailFns[ReadDef.Name] = readPath
}

var ReadDef = provider.ToolDef{
	Name: "read",
	Description: `Read a file's contents, or list a directory's immediate entries.

Pass a directory path to list its immediate children; pass a file path to read it. Use offset and limit to page through a large file rather than reading it whole — the default limit is 2000 lines and output above that is truncated.

Prefer this over ` + "`bash cat`" + ` or ` + "`bash ls`" + `: it is not gated, the output is already formatted for you, and it will not surprise the user with an approval prompt.`,
	InputSchema: map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path": map[string]any{
				"type":        "string",
				"description": "Absolute or relative path to the file or directory.",
			},
			"offset": map[string]any{
				"type":        "integer",
				"description": "0-based index of the first line to return (default: 0). File mode only.",
			},
			"limit": map[string]any{
				"type":        "integer",
				"description": "Max lines to return (default: 2000, max: 2000). File mode only.",
			},
		},
		"required": []string{"path"},
	},
}

// readPath returns the path argument for a read tool call.
func readPath(tc provider.ToolCall) string {
	return stringInput(tc.Input, "path")
}

// read executes the read tool. When path points at a directory, it returns a
// listing of immediate children; otherwise it returns file contents.
// Returns (content, isError). The ctx argument is unused; it's present to give
// read the same signature as the other tools so it can go through executor.
func read(_ context.Context, input map[string]any) (string, bool) {
	path := stringInput(input, "path")
	if path == "" {
		return "error: 'path' is required", true
	}

	info, err := os.Stat(path)
	if err != nil {
		return fmt.Sprintf("error: %s", err), true
	}
	if info.IsDir() {
		return readDir(path)
	}
	// Reading a FIFO blocks until a writer appears and a device such as
	// /dev/zero never ends, so either would hang the agent loop.
	if !info.Mode().IsRegular() {
		return fmt.Sprintf("error: %s is not a regular file", path), true
	}

	offset := 0
	if v, ok := intInput(input, "offset"); ok && v >= 0 {
		offset = v
	}
	limit := maxReadLines
	if v, ok := intInput(input, "limit"); ok && v > 0 && v < maxReadLines {
		limit = v
	}

	f, err := os.Open(path)
	if err != nil {
		return fmt.Sprintf("error: %s", err), true
	}
	defer func() { _ = f.Close() }()
	// Stream rather than read the file whole: a multi-gigabyte log should
	// cost only the lines asked for, not its full size in memory.
	r := bufio.NewReaderSize(f, readBufferSize)

	head, err := r.Peek(binarySniffBytes)
	if err != nil && !errors.Is(err, io.EOF) && !errors.Is(err, bufio.ErrBufferFull) {
		return fmt.Sprintf("error: %s", err), true
	}
	if len(head) == 0 {
		// Say so explicitly: an empty result reads as a failed tool call.
		return "[File is empty]", false
	}
	if isBinary(head) {
		return formatBinaryNotice(path, info.Size()), false
	}

	var shown []string
	n := 0
	for len(shown) < limit {
		line, size, err := readCappedLine(r)
		if err != nil {
			return fmt.Sprintf("error: %s", err), true
		}
		if line == nil {
			break // EOF
		}
		if n >= offset {
			shown = append(shown, truncateLine(line, size))
		}
		n++
	}

	if len(shown) == 0 {
		return fmt.Sprintf("[File has %d lines; offset %d is out of range]", n, offset), false
	}

	result := strings.Join(shown, "\n")
	// Any byte left over starts at least one more line. Counting them all
	// would mean reading the rest of the file, which is what streaming avoids.
	if more, _ := r.Peek(1); len(more) > 0 {
		end := offset + len(shown)
		// Lines are shown 1-based but offset is 0-based, so name the exact
		// offset to continue from rather than leave the model to convert.
		result += fmt.Sprintf(
			"\n[truncated — showing lines %d–%d; the file has more. Use offset=%d to read more.]",
			offset+1, end, end,
		)
	}
	return result, false
}

// readCappedLine reads one line without its newline, keeping at most
// maxReadLineBytes+1 bytes of it (enough for truncateLine to find a rune
// boundary) so one enormous line cannot be buffered whole. size is the line's
// full length. It returns a nil line at EOF. A final newline terminates the
// last line rather than starting another, so it is not counted as a line.
func readCappedLine(r *bufio.Reader) (line []byte, size int, err error) {
	line = []byte{}
	for {
		chunk, err := r.ReadSlice('\n')
		chunk = bytes.TrimSuffix(chunk, []byte("\n"))
		size += len(chunk)
		if keep := maxReadLineBytes + 1 - len(line); keep > 0 {
			line = append(line, chunk[:min(keep, len(chunk))]...)
		}
		switch {
		case err == nil:
			return line, size, nil
		case errors.Is(err, bufio.ErrBufferFull):
			continue
		case errors.Is(err, io.EOF):
			if size == 0 {
				return nil, 0, nil
			}
			return line, size, nil
		default:
			return nil, 0, err
		}
	}
}

// truncateLine cuts line, whose full length is size, to maxReadLineBytes on a
// rune boundary.
func truncateLine(line []byte, size int) string {
	if size <= maxReadLineBytes {
		return string(line)
	}
	cut := maxReadLineBytes
	for cut > 0 && !utf8.RuneStart(line[cut]) {
		cut--
	}
	return string(line[:cut]) + fmt.Sprintf("… [line truncated, %d bytes total]", size)
}

func formatBinaryNotice(path string, size int64) string {
	var sizeStr string
	switch {
	case size == 0:
		sizeStr = "unknown size"
	case size < 1024:
		sizeStr = fmt.Sprintf("%dB", size)
	default:
		sizeStr = fmt.Sprintf("%dKB", size/1024)
	}
	ext := filepath.Ext(path)
	if ext == "" {
		ext = "file"
	}
	return fmt.Sprintf("[Binary file: %s %s — cannot read as text]", sizeStr, ext)
}

// readDir returns a sorted listing of immediate children with a trailing slash
// on subdirectories. Capped at maxDirListEntries with a truncation note.
func readDir(path string) (string, bool) {
	entries, err := os.ReadDir(path)
	if err != nil {
		return fmt.Sprintf("error: %s", err), true
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Name() < entries[j].Name()
	})

	total := len(entries)
	truncated := false
	if total > maxDirListEntries {
		entries = entries[:maxDirListEntries]
		truncated = true
	}

	names := make([]string, len(entries))
	for i, e := range entries {
		name := e.Name()
		if e.IsDir() {
			name += "/"
		}
		names[i] = name
	}

	head := fmt.Sprintf("Directory: %s (%d %s)", path, total, pluralEntries(total))
	body := head + "\n" + strings.Join(names, "\n")
	if truncated {
		body += fmt.Sprintf("\n[truncated — showing first %d of %d entries. Use glob or bash ls for more.]", maxDirListEntries, total)
	}
	return body, false
}

// isBinary reports whether data appears to be binary content.
func isBinary(data []byte) bool {
	if len(data) == 0 {
		return false
	}
	sample := data
	if len(sample) > binarySniffBytes {
		sample = sample[:binarySniffBytes]
	}
	nonPrintable := 0
	for _, b := range sample {
		if b == 0 {
			return true
		}
		if b < 8 || (b > 13 && b < 32) {
			nonPrintable++
		}
	}
	return float64(nonPrintable)/float64(len(sample)) > 0.1
}
