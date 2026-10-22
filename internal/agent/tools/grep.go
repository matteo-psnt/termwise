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
		return Step{Kind: StepExecutor, Execute: ExecuteGrep}
	}
	detailFns[GrepDef.Name] = grepDetail
}

func grepDetail(tc provider.ToolCall) string {
	return patternInPath(GrepPattern(tc), GrepPath(tc))
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

// GrepPattern returns the pattern argument for a grep tool call.
func GrepPattern(tc provider.ToolCall) string {
	p, _ := tc.Input["pattern"].(string)
	return p
}

// GrepPath returns the path argument for a grep tool call (empty if unset).
func GrepPath(tc provider.ToolCall) string {
	p, _ := tc.Input["path"].(string)
	return p
}

// ExecuteGrep runs the grep tool for a tool call and packages the result.
func ExecuteGrep(ctx context.Context, tc provider.ToolCall) provider.ToolResult {
	content, isErr := Grep(ctx, tc.Input)
	return provider.ToolResult{ToolCallID: tc.ID, Content: content, IsError: isErr}
}

// Grep runs ripgrep with arguments derived from input and returns a formatted
// text result. Returns (content, isError).
func Grep(ctx context.Context, input map[string]any) (string, bool) {
	pattern, _ := input["pattern"].(string)
	if pattern == "" {
		return "error: 'pattern' is required", true
	}

	cwd, _ := os.Getwd()
	pathArg, _ := input["path"].(string)
	absPath, _, err := resolveSearchPath(pathArg, cwd)
	if err != nil {
		return fmt.Sprintf("error: path does not exist: %s", pathArg), true
	}

	outputMode, _ := input["output_mode"].(string)
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
		return formatContent(lines, cwd, headLimit, offset), false
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
		return []string{"-c"}
	case "content":
		args := []string{}
		showLine := true
		if v, ok := input["-n"].(bool); ok {
			showLine = v
		}
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

func formatContent(lines []string, cwd string, headLimit, offset int) string {
	paged, truncated := applyHeadLimit(lines, headLimit, offset)
	rel := relativizeContentLines(paged, cwd)
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
		idx := strings.LastIndex(line, ":")
		if idx <= 0 {
			continue
		}
		n, err := strconv.Atoi(line[idx+1:])
		if err != nil {
			continue
		}
		totalFiles++
		totalMatches += n
		relLines = append(relLines, relativizePath(line[:idx], cwd)+line[idx:])
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

func intInput(input map[string]any, key string) (int, bool) {
	switch v := input[key].(type) {
	case float64:
		return int(v), true
	case int:
		return v, true
	}
	return 0, false
}
