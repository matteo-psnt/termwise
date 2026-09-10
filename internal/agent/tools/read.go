package tools

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/matteo-psnt/termwise/internal/provider"
)

// ExecuteRead runs the read tool for a tool call and packages the result.
// The ctx argument is unused; it's present to give read the same signature as
// other executors so they can plug into Step.Execute uniformly.
func ExecuteRead(_ context.Context, tc provider.ToolCall) provider.ToolResult {
	content, isErr := Read(tc.Input)
	return provider.ToolResult{ToolCallID: tc.ID, Content: content, IsError: isErr}
}

const (
	maxReadLines      = 2000
	maxDirListEntries = 500
)

func init() {
	stepFns[ReadDef.Name] = func(_ provider.ToolCall) Step {
		return Step{Kind: StepExecutor, Execute: ExecuteRead}
	}
	detailFns[ReadDef.Name] = ReadPath
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
				"description": "Line number to start from (default: 0). File mode only.",
			},
			"limit": map[string]any{
				"type":        "integer",
				"description": "Max lines to return (default: 2000, max: 2000). File mode only.",
			},
		},
		"required": []string{"path"},
	},
}

// ReadPath returns the path argument for a read tool call.
func ReadPath(tc provider.ToolCall) string {
	p, _ := tc.Input["path"].(string)
	return p
}

// Read executes the read tool. When path points at a directory, it returns a
// listing of immediate children; otherwise it returns file contents.
// Returns (content, isError).
func Read(input map[string]any) (string, bool) {
	path, _ := input["path"].(string)
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

	offset := 0
	if v, ok := input["offset"].(float64); ok && v >= 0 {
		offset = int(v)
	}
	limit := maxReadLines
	if v, ok := input["limit"].(float64); ok && v > 0 && int(v) < maxReadLines {
		limit = int(v)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Sprintf("error: %s", err), true
	}

	if isBinary(data) {
		return formatBinaryNotice(path, info.Size()), false
	}

	lines := strings.Split(string(data), "\n")
	total := len(lines)

	if offset >= total {
		return fmt.Sprintf("[File has %d lines; offset %d is out of range]", total, offset), false
	}

	end := min(offset+limit, total)

	result := strings.Join(lines[offset:end], "\n")
	if end < total {
		result += fmt.Sprintf(
			"\n[truncated — showing lines %d–%d of %d. Use offset/limit to read more.]",
			offset+1, end, total,
		)
	}
	return result, false
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

func pluralEntries(n int) string {
	if n == 1 {
		return "entry"
	}
	return "entries"
}

// isBinary reports whether data appears to be binary content.
func isBinary(data []byte) bool {
	if len(data) == 0 {
		return false
	}
	sample := data
	if len(sample) > 8192 {
		sample = sample[:8192]
	}
	for _, b := range sample {
		if b == 0 {
			return true
		}
	}
	nonPrintable := 0
	for _, b := range sample {
		if b < 8 || (b > 13 && b < 32) {
			nonPrintable++
		}
	}
	return float64(nonPrintable)/float64(len(sample)) > 0.1
}
