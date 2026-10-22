package tools

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/matteo-psnt/termwise/internal/provider"
)

const defaultGlobLimit = 100

func init() {
	stepFns[GlobDef.Name] = func(_ provider.ToolCall) Step {
		return Step{Kind: StepExecutor, Execute: executor(Glob)}
	}
	detailFns[GlobDef.Name] = globDetail
}

func globDetail(tc provider.ToolCall) string {
	return patternInPath(GlobPattern(tc), GlobPath(tc))
}

var GlobDef = provider.ToolDef{
	Name: "glob",
	Description: `Fast file pattern matching backed by ripgrep.

Usage:
- Use glob to find files by name pattern (e.g. "**/*.go", "src/**/*.ts").
- Supports brace expansion (e.g. "**/*.{ts,tsx}").
- Returns matching file paths sorted by modification time (newest first), capped at 100.
- Respects .gitignore and skips .git/.svn/.hg by default.
- For open-ended searches that require multiple rounds of globbing/grepping, use grep with the glob filter directly.`,
	InputSchema: map[string]any{
		"type": "object",
		"properties": map[string]any{
			"pattern": map[string]any{
				"type":        "string",
				"description": "Glob pattern to match files (e.g. \"**/*.go\", \"src/**/*.{ts,tsx}\").",
			},
			"path": map[string]any{
				"type":        "string",
				"description": "Directory to search in. Defaults to the working directory.",
			},
		},
		"required": []string{"pattern"},
	},
}

// GlobPattern returns the pattern argument for a glob tool call.
func GlobPattern(tc provider.ToolCall) string {
	return stringInput(tc.Input, "pattern")
}

// GlobPath returns the path argument for a glob tool call (empty if unset).
func GlobPath(tc provider.ToolCall) string {
	return stringInput(tc.Input, "path")
}

// Glob lists files matching a glob pattern, sorted by modification time.
// Returns (content, isError).
func Glob(ctx context.Context, input map[string]any) (string, bool) {
	pattern := stringInput(input, "pattern")
	if pattern == "" {
		return "error: 'pattern' is required", true
	}

	cwd, _ := os.Getwd()
	pathArg := stringInput(input, "path")
	absPath, info, err := resolveSearchPath(pathArg, cwd)
	if err != nil {
		if absPath == "" {
			return fmt.Sprintf("error: %s", err), true
		}
		return fmt.Sprintf("error: path does not exist: %s", pathArg), true
	}
	if !info.IsDir() {
		return fmt.Sprintf("error: path is not a directory: %s", pathArg), true
	}

	args := []string{"--files", "--hidden"}
	for _, dir := range vcsExcludeGlobs {
		args = append(args, "--glob", "!"+dir)
	}
	for _, p := range splitGlobPatterns(pattern) {
		args = append(args, "--glob", p)
	}
	args = append(args, absPath)

	lines, runErr := runRipgrep(ctx, args)
	if runErr != nil {
		return fmt.Sprintf("error: %s", runErr), true
	}

	sortByMtimeDesc(lines)
	truncated := len(lines) > defaultGlobLimit
	if truncated {
		lines = lines[:defaultGlobLimit]
	}
	rel := relativizeAll(lines, cwd)

	if len(rel) == 0 {
		return "No files found", false
	}

	head := fmt.Sprintf("Found %d %s", len(rel), pluralFiles(len(rel)))
	body := head + "\n" + strings.Join(rel, "\n")
	if truncated {
		body += "\n(Results truncated at 100. Use a more specific pattern or path.)"
	}
	return body, false
}
