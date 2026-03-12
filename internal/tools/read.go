package tools

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const maxReadLines = 2000

// Read executes the read tool with the given input map.
// Returns (content, isError).
func Read(input map[string]any) (string, bool) {
	path, _ := input["path"].(string)
	if path == "" {
		return "error: 'path' is required", true
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
		info, _ := os.Stat(path)
		size := "unknown size"
		if info != nil {
			kb := info.Size() / 1024
			if kb == 0 {
				size = fmt.Sprintf("%dB", info.Size())
			} else {
				size = fmt.Sprintf("%dKB", kb)
			}
		}
		ext := filepath.Ext(path)
		if ext == "" {
			ext = "file"
		}
		return fmt.Sprintf("[Binary file: %s %s — cannot read as text]", size, ext), false
	}

	lines := strings.Split(string(data), "\n")
	total := len(lines)

	if offset >= total {
		return fmt.Sprintf("[File has %d lines; offset %d is out of range]", total, offset), false
	}

	end := offset + limit
	if end > total {
		end = total
	}

	result := strings.Join(lines[offset:end], "\n")
	if end < total {
		result += fmt.Sprintf(
			"\n[truncated — showing lines %d–%d of %d. Use offset/limit to read more.]",
			offset+1, end, total,
		)
	}
	return result, false
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
