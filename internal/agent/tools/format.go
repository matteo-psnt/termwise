package tools

// Result-formatting helpers shared by the grep, glob and read tools.

import (
	"fmt"
	"path/filepath"
	"strings"
)

// patternInPath formats a "<pattern> in <path>" detail line, dropping the
// suffix when path is empty. Shared by grep and glob.
func patternInPath(pattern, path string) string {
	if path != "" {
		return pattern + " in " + path
	}
	return pattern
}

// applyHeadLimit returns at most limit items starting at offset.
// limit=0 means unlimited. The second return reports whether truncation occurred.
func applyHeadLimit(items []string, limit, offset int) ([]string, bool) {
	if offset > 0 {
		if offset >= len(items) {
			return nil, false
		}
		items = items[offset:]
	}
	if limit == 0 {
		return items, false
	}
	if len(items) <= limit {
		return items, false
	}
	return items[:limit], true
}

func formatLimitInfo(limit int, truncated bool, offset int) string {
	var parts []string
	if truncated {
		parts = append(parts, fmt.Sprintf("limit: %d", limit))
	}
	if offset > 0 {
		parts = append(parts, fmt.Sprintf("offset: %d", offset))
	}
	return strings.Join(parts, ", ")
}

func relativizeAll(paths []string, cwd string) []string {
	out := make([]string, len(paths))
	for i, p := range paths {
		out[i] = relativizePath(p, cwd)
	}
	return out
}

// relativizePath returns p relative to cwd when p is under cwd, otherwise p.
func relativizePath(p, cwd string) string {
	if cwd == "" {
		return p
	}
	rel, err := filepath.Rel(cwd, p)
	if err != nil || strings.HasPrefix(rel, "..") {
		return p
	}
	return rel
}

// relativizeContentLines rewrites the path on rg's content-mode lines so users
// see paths relative to cwd. grep runs rg with NUL-prefixed field separators,
// so a line is "path\x00:line\x00:text" for a match and "path\x00-line\x00-text"
// for context (without the line field when showLine is false). The NULs are
// dropped to give rg's usual "path:line:text" / "path-line-text". Lines with no
// NUL, such as the "--" between context groups, pass through.
func relativizeContentLines(lines []string, cwd string, showLine bool) []string {
	out := make([]string, len(lines))
	for i, line := range lines {
		path, rest, ok := strings.Cut(line, "\x00")
		if !ok {
			out[i] = line
			continue
		}
		if showLine {
			// Only the separator after the line number; any later NUL is
			// part of the matched text.
			rest = strings.Replace(rest, "\x00", "", 1)
		}
		out[i] = relativizePath(path, cwd) + rest
	}
	return out
}

func pluralFiles(n int) string {
	if n == 1 {
		return "file"
	}
	return "files"
}

func pluralOccurrences(n int) string {
	if n == 1 {
		return "occurrence"
	}
	return "occurrences"
}

func pluralEntries(n int) string {
	if n == 1 {
		return "entry"
	}
	return "entries"
}
