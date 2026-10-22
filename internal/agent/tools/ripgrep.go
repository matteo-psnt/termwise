package tools

// Ripgrep plumbing shared by the grep and glob tools.

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

// vcsExcludeGlobs are directories ripgrep skips by default to keep results free
// of version-control noise. Applied to grep and glob.
var vcsExcludeGlobs = []string{".git", ".svn", ".hg", ".bzr", ".jj", ".sl"}

// resolveSearchPath resolves raw (the user-supplied path argument, empty for
// the working directory) against cwd and stats it. On failure abs is empty if
// the path could not be made absolute, and set if only the stat failed; the
// error text is left to each caller, since grep and glob word it differently.
func resolveSearchPath(raw, cwd string) (abs string, info os.FileInfo, err error) {
	searchPath := raw
	if searchPath == "" {
		searchPath = cwd
	}
	abs, err = filepath.Abs(searchPath)
	if err != nil {
		return "", nil, err
	}
	info, err = os.Stat(abs)
	if err != nil {
		return abs, nil, err
	}
	return abs, info, nil
}

// runRipgrep executes `rg` with args and returns stdout lines.
// Exit code 1 (no matches) is treated as success with no results.
func runRipgrep(ctx context.Context, args []string) ([]string, error) {
	cmd := exec.CommandContext(ctx, "rg", args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err != nil {
		if exitErr, ok := errors.AsType[*exec.ExitError](err); ok {
			if exitErr.ExitCode() == 1 {
				return nil, nil
			}
			msg := strings.TrimSpace(stderr.String())
			if msg == "" {
				msg = exitErr.Error()
			}
			// The field separators grep relies on to split paths from
			// matches arrived in ripgrep 13; older ones reject them.
			if strings.Contains(msg, "field-match-separator") || strings.Contains(msg, "field-context-separator") {
				return nil, fmt.Errorf("this search needs ripgrep 13 or newer; upgrade rg (https://github.com/BurntSushi/ripgrep)")
			}
			return nil, fmt.Errorf("ripgrep failed: %s", msg)
		}
		if errors.Is(err, exec.ErrNotFound) {
			return nil, fmt.Errorf("ripgrep (rg) not found on PATH — install it from https://github.com/BurntSushi/ripgrep")
		}
		return nil, err
	}
	out := strings.TrimRight(stdout.String(), "\n")
	if out == "" {
		return nil, nil
	}
	return strings.Split(out, "\n"), nil
}

// globArgs returns rg --glob flags for the user's patterns followed by the
// VCS exclusions. The order matters: rg lets a later glob override an earlier
// one, so a broad pattern such as "*" placed after "!.git" matched .git and
// pulled it back in.
func globArgs(patterns []string) []string {
	var args []string
	for _, p := range patterns {
		args = append(args, "--glob", p)
	}
	for _, dir := range vcsExcludeGlobs {
		args = append(args, "--glob", "!"+dir)
	}
	return args
}

// splitGlobPatterns splits a user-supplied glob string on whitespace and commas
// while preserving brace expansions like "*.{ts,tsx}".
func splitGlobPatterns(s string) []string {
	var out []string
	for raw := range strings.FieldsSeq(s) {
		if strings.ContainsAny(raw, "{}") {
			out = append(out, raw)
			continue
		}
		for p := range strings.SplitSeq(raw, ",") {
			if p != "" {
				out = append(out, p)
			}
		}
	}
	return out
}

// sortByMtimeDesc sorts files in place by modification time (newest first),
// falling back to lexicographic order on ties or stat failures.
func sortByMtimeDesc(files []string) {
	type entry struct {
		path  string
		mtime int64
	}
	entries := make([]entry, len(files))
	for i, f := range files {
		if st, err := os.Stat(f); err == nil {
			entries[i] = entry{f, st.ModTime().UnixNano()}
		} else {
			entries[i] = entry{f, 0}
		}
	}
	sort.SliceStable(entries, func(i, j int) bool {
		if entries[i].mtime != entries[j].mtime {
			return entries[i].mtime > entries[j].mtime
		}
		return entries[i].path < entries[j].path
	})
	for i, e := range entries {
		files[i] = e.path
	}
}
