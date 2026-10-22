package tools

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/matteo-psnt/termwise/internal/provider"
)

const (
	defaultGrepHeadLimit = 250
	ripgrepMaxColumns    = "500"
)

func init() {
	stepFns[GrepDef.Name] = func(_ provider.ToolCall) Step {
		return Step{Kind: StepExecutor, Execute: executor(grep)}
	}
	detailFns[GrepDef.Name] = grepDetail
}

func grepDetail(tc provider.ToolCall) string {
	return patternInPath(grepPattern(tc), grepPath(tc))
}

var GrepDef = provider.ToolDef{
	Name: "grep",
	Description: `A search tool built on ripgrep.

Usage:
- ALWAYS use grep for content search. NEVER invoke rg or grep via the bash tool.
- Supports full regex syntax (e.g. "log.*Error", "function\\s+\\w+").
- Filter files with glob (e.g. "*.go", "**/*.{ts,tsx}") or type (e.g. "go", "py", "rust").
- output_mode: "files_with_matches" (default) lists file paths, "content" shows matching lines, "count" shows match counts per file.
- Pattern syntax is ripgrep (not POSIX grep) — literal braces need escaping (e.g. "interface\\{\\}" to match "interface{}").
- Set multiline=true to let . match newlines and patterns span lines.
- Respects .gitignore and skips .git/.svn/.hg by default.
- Pagination: head_limit caps results (default 250), offset skips entries; head_limit=0 means unlimited.`,
	InputSchema: map[string]any{
		"type": "object",
		"properties": map[string]any{
			"pattern": map[string]any{
				"type":        "string",
				"description": "Regular expression to search for in file contents.",
			},
			"path": map[string]any{
				"type":        "string",
				"description": "File or directory to search in. Defaults to the working directory.",
			},
			"glob": map[string]any{
				"type":        "string",
				"description": "Glob pattern(s) to filter files (e.g. \"*.go\", \"*.{ts,tsx}\"). Comma- or whitespace-separated.",
			},
			"output_mode": map[string]any{
				"type":        "string",
				"enum":        []string{"files_with_matches", "content", "count"},
				"description": "files_with_matches (default) lists file paths; content shows matching lines; count shows match counts per file.",
			},
			"-A": map[string]any{
				"type":        "integer",
				"description": "Lines of context after each match. content mode only.",
			},
			"-B": map[string]any{
				"type":        "integer",
				"description": "Lines of context before each match. content mode only.",
			},
			"-C": map[string]any{
				"type":        "integer",
				"description": "Lines of context before and after each match. content mode only.",
			},
			"-n": map[string]any{
				"type":        "boolean",
				"description": "Show line numbers. content mode only. Default: true.",
			},
			"-i": map[string]any{
				"type":        "boolean",
				"description": "Case-insensitive match.",
			},
			"type": map[string]any{
				"type":        "string",
				"description": "Restrict to a file type (rg --type), e.g. go, py, rust, js.",
			},
			"multiline": map[string]any{
				"type":        "boolean",
				"description": "Enable multiline mode (. matches newlines, patterns may span lines). Default: false.",
			},
			"head_limit": map[string]any{
				"type":        "integer",
				"description": "Limit output to first N entries. Default 250. Pass 0 for unlimited.",
			},
			"offset": map[string]any{
				"type":        "integer",
				"description": "Skip first N entries before applying head_limit. Default 0.",
			},
		},
		"required": []string{"pattern"},
	},
}

// grepPattern returns the pattern argument for a grep tool call.
func grepPattern(tc provider.ToolCall) string {
	return stringInput(tc.Input, "pattern")
}

// grepPath returns the path argument for a grep tool call (empty if unset).
func grepPath(tc provider.ToolCall) string {
	return stringInput(tc.Input, "path")
}

// grep runs ripgrep with arguments derived from input and returns a formatted
// text result. Returns (content, isError).
func grep(ctx context.Context, input map[string]any) (string, bool) {
	pattern := stringInput(input, "pattern")
	if pattern == "" {
		return "error: 'pattern' is required", true
	}

	cwd, _ := os.Getwd()
	pathArg := stringInput(input, "path")
	absPath, _, err := resolveSearchPath(pathArg, cwd)
	if err != nil {
		return fmt.Sprintf("error: path does not exist: %s", pathArg), true
	}

	outputMode := stringInput(input, "output_mode")
	if outputMode == "" {
		outputMode = "files_with_matches"
	}
	switch outputMode {
	case "files_with_matches", "content", "count":
	default:
		return fmt.Sprintf("error: invalid output_mode %q", outputMode), true
	}

	args := buildGrepArgs(input, pattern, outputMode, absPath)
	lines, runErr := runRipgrep(ctx, args)
	if runErr != nil {
		return fmt.Sprintf("error: %s", runErr), true
	}

	headLimit := defaultGrepHeadLimit
	if v, ok := intInput(input, "head_limit"); ok && v >= 0 {
		headLimit = v
	}
	offset := 0
	if v, ok := intInput(input, "offset"); ok && v > 0 {
		offset = v
	}

	switch outputMode {
	case "files_with_matches":
		return formatFilesWithMatches(lines, cwd, headLimit, offset), false
	case "content":
		return formatContent(lines, cwd, headLimit, offset, grepShowLineNumbers(input)), false
	case "count":
		return formatCount(lines, cwd, headLimit, offset), false
	}
	return "", false
}

func buildGrepArgs(input map[string]any, pattern, outputMode, absPath string) []string {
	args := []string{"--hidden", "--max-columns", ripgrepMaxColumns}
	for _, dir := range vcsExcludeGlobs {
		args = append(args, "--glob", "!"+dir)
	}
	if b, _ := input["multiline"].(bool); b {
		args = append(args, "-U", "--multiline-dotall")
	}
	if b, _ := input["-i"].(bool); b {
		args = append(args, "-i")
	}
	args = append(args, outputModeArgs(input, outputMode)...)
	if t, _ := input["type"].(string); t != "" {
		args = append(args, "--type", t)
	}
	if g, _ := input["glob"].(string); g != "" {
		for _, p := range splitGlobPatterns(g) {
			args = append(args, "--glob", p)
		}
	}
	// Use -e for dash-prefixed patterns to avoid rg flag parsing.
	if strings.HasPrefix(pattern, "-") {
		args = append(args, "-e", pattern)
	} else {
		args = append(args, pattern)
	}
	args = append(args, absPath)
	return args
}

// outputModeArgs returns rg flags specific to the requested output mode,
// including content-mode context and line-number controls.
func outputModeArgs(input map[string]any, outputMode string) []string {
	switch outputMode {
	case "files_with_matches":
		return []string{"-l"}
	case "count":
		// rg drops the "path:" prefix when searching a single file, and a
		// path can itself contain ':'. Force the name and end it with NUL.
		return []string{"-c", "--with-filename", "--null"}
	case "content":
		// Mark the end of the path with NUL so relativizeContentLines can find
		// it: rg ends it with ':' on match lines and '-' on context lines, and
		// either can appear inside a filename. -H keeps the path on output
		// for a single-file search too.
		args := []string{
			"--with-filename",
			"--field-match-separator", `\x00:`,
			"--field-context-separator", `\x00-`,
		}
		showLine := grepShowLineNumbers(input)
		if showLine {
			args = append(args, "-n")
		}
		if c, ok := intInput(input, "-C"); ok {
			return append(args, "-C", strconv.Itoa(c))
		}
		if b, ok := intInput(input, "-B"); ok {
			args = append(args, "-B", strconv.Itoa(b))
		}
		if a, ok := intInput(input, "-A"); ok {
			args = append(args, "-A", strconv.Itoa(a))
		}
		return args
	}
	return nil
}

// grepShowLineNumbers reports whether content mode prints line numbers; the
// -n argument defaults to true.
func grepShowLineNumbers(input map[string]any) bool {
	if v, ok := input["-n"].(bool); ok {
		return v
	}
	return true
}

func formatFilesWithMatches(lines []string, cwd string, headLimit, offset int) string {
	sortByMtimeDesc(lines)
	paged, truncated := applyHeadLimit(lines, headLimit, offset)
	rel := relativizeAll(paged, cwd)
	if len(rel) == 0 {
		return "No files found"
	}
	head := fmt.Sprintf("Found %d %s", len(rel), pluralFiles(len(rel)))
	if info := formatLimitInfo(headLimit, truncated, offset); info != "" {
		head += " " + info
	}
	return head + "\n" + strings.Join(rel, "\n")
}

func formatContent(lines []string, cwd string, headLimit, offset int, showLine bool) string {
	paged, truncated := applyHeadLimit(lines, headLimit, offset)
	rel := relativizeContentLines(paged, cwd, showLine)
	body := strings.Join(rel, "\n")
	if body == "" {
		body = "No matches found"
	}
	if info := formatLimitInfo(headLimit, truncated, offset); info != "" {
		body += "\n\n[Showing results with pagination = " + info + "]"
	}
	return body
}

func formatCount(lines []string, cwd string, headLimit, offset int) string {
	paged, truncated := applyHeadLimit(lines, headLimit, offset)
	totalFiles := 0
	totalMatches := 0
	relLines := make([]string, 0, len(paged))
	for _, line := range paged {
		// Lines are "path\x00count"; see outputModeArgs.
		path, count, ok := strings.Cut(line, "\x00")
		if !ok {
			continue
		}
		n, err := strconv.Atoi(count)
		if err != nil {
			continue
		}
		totalFiles++
		totalMatches += n
		relLines = append(relLines, relativizePath(path, cwd)+":"+count)
	}
	body := strings.Join(relLines, "\n")
	if body == "" {
		body = "No matches found"
	}
	summary := fmt.Sprintf("\n\nFound %d total %s across %d %s.",
		totalMatches, pluralOccurrences(totalMatches),
		totalFiles, pluralFiles(totalFiles))
	if info := formatLimitInfo(headLimit, truncated, offset); info != "" {
		summary += " with pagination = " + info
	}
	return body + summary
}
