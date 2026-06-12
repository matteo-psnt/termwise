package allowlist

import (
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"
	"mvdan.cc/sh/v3/syntax"
)

// Rule describes one entry in the allow-list.
// A command is auto-approved when all criteria match:
//   - Cmd matches the executable basename
//   - AllowSubs: if non-empty, the first positional arg (non-flag) must be in the list
//   - BlockFlags: if non-empty, none of these flag strings may appear in the command
type Rule struct {
	Cmd        string   `yaml:"cmd"`
	AllowSubs  []string `yaml:"allow_subs"`
	BlockFlags []string `yaml:"block_flags"`
}

// allowlistFile is the top-level structure of allowlist.yaml.
type allowlistFile struct {
	Rules []Rule `yaml:"rules"`
}

// ----------------------------------------------------------------------------
// Embedded YAML loader
// ----------------------------------------------------------------------------

var (
	once         sync.Once
	builtinRules []Rule
)

//go:embed allowlist.yaml
var builtinAllowlistYAML []byte

func loadBuiltin() {
	once.Do(func() {
		var f allowlistFile
		if err := yaml.Unmarshal(builtinAllowlistYAML, &f); err != nil {
			// Malformed embedded file is a build-time bug; degrade gracefully.
			builtinRules = nil
			return
		}
		builtinRules = f.Rules
	})
}

// ----------------------------------------------------------------------------
// Public API
// ----------------------------------------------------------------------------

// Parse parses an allow-list rule string into a Rule.
//
// Format: "cmd[:subs][:!flags]" where:
//   - cmd is the executable basename
//   - subs is an optional comma-separated list of allowed first positional args
//   - !flags is an optional comma-separated list of blocked flags (each prefixed with !)
//
// Examples:
//
//	"ls"                           → allow any ls invocation
//	"git:status,log,diff"          → allow git with these subcommands only
//	"git:status,log:!--force"      → allow git status/log, block --force
//	"find:!-exec,!-delete"         → allow find, block these flags
func Parse(s string) (Rule, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return Rule{}, false
	}

	parts := strings.SplitN(s, ":", 3)
	rule := Rule{Cmd: parts[0]}

	for _, part := range parts[1:] {
		items := strings.Split(part, ",")
		if len(items) == 0 {
			continue
		}
		if strings.HasPrefix(strings.TrimSpace(items[0]), "!") {
			// BlockFlags segment.
			for _, item := range items {
				item = strings.TrimSpace(item)
				if strings.HasPrefix(item, "!") {
					rule.BlockFlags = append(rule.BlockFlags, item[1:])
				}
			}
		} else {
			// AllowSubs segment.
			for _, item := range items {
				item = strings.TrimSpace(item)
				if item != "" {
					rule.AllowSubs = append(rule.AllowSubs, item)
				}
			}
		}
	}

	return rule, true
}

// Matches reports whether rule allows the given shell command.
func Matches(rule Rule, command string) bool {
	parsed, err := parseCommand(command)
	if err != nil || len(parsed) != 1 {
		return false
	}
	return matchesTokens(rule, parsed[0].tokens)
}

func matchesTokens(rule Rule, tokens []string) bool {
	if len(tokens) == 0 {
		return false
	}
	if filepath.Base(tokens[0]) != rule.Cmd {
		return false
	}

	// Check AllowSubs: accept either the first arg token (for flag-style
	// subcommands like `brew --version`) or the first non-flag positional
	// token (for commands like `git status` or `git --no-pager status`).
	if len(rule.AllowSubs) > 0 {
		found := false
		for _, candidate := range subcommandCandidates(tokens[1:]) {
			for _, s := range rule.AllowSubs {
				if s == candidate {
					found = true
					break
				}
			}
			if found {
				break
			}
		}
		if !found {
			return false
		}
	}

	if !matchesCommandSpecificPolicy(rule, tokens) {
		return false
	}

	// Check BlockFlags.
	for _, tok := range tokens[1:] {
		for _, blocked := range rule.BlockFlags {
			if tokenMatchesBlockedFlag(tok, blocked) {
				return false
			}
		}
	}

	return true
}

func matchesCommandSpecificPolicy(rule Rule, tokens []string) bool {
	switch rule.Cmd {
	case "find":
		return validateFindTokens(tokens)
	case "go":
		return validateGoTokens(tokens)
	case "sysctl":
		for _, tok := range tokens[1:] {
			if strings.Contains(tok, "=") {
				return false
			}
		}
	}
	return true
}

func tokenMatchesBlockedFlag(token, blocked string) bool {
	if token == blocked {
		return true
	}
	if strings.HasPrefix(blocked, "--") {
		return strings.HasPrefix(token, blocked+"=")
	}
	if strings.HasPrefix(blocked, "-") && !strings.HasPrefix(blocked, "--") && len(blocked) == 2 {
		return strings.HasPrefix(token, blocked)
	}
	return false
}

// NeedsApproval returns true if the command must be confirmed by the user.
// Returns false (auto-approved) if every parsed command segment matches a
// built-in or user rule and any redirections stay within approved safe paths.
func NeedsApproval(userRules []string, command string) bool {
	command = strings.TrimSpace(command)
	if command == "" {
		return true
	}

	segments, err := parseCommand(command)
	if err != nil {
		return true
	}

	loadBuiltin()

	for _, seg := range segments {
		if !matchesAny(userRules, seg.tokens) {
			return true
		}
	}
	return false
}

func matchesAny(userRules []string, tokens []string) bool {
	for _, rule := range builtinRules {
		if matchesTokens(rule, tokens) {
			return true
		}
	}
	for _, s := range userRules {
		rule, ok := Parse(s)
		if !ok {
			continue
		}
		if matchesTokens(rule, tokens) {
			return true
		}
	}
	return false
}

// BuildRuleFromCommand derives a sensible rule string from a shell command.
// If the command has a subcommand (first positional arg), returns "cmd:sub".
// Otherwise returns just "cmd".
// Used by the TUI when the user presses 'a' at an approval prompt.
func BuildRuleFromCommand(command string) string {
	parsed, err := parseCommand(command)
	if err != nil || len(parsed) != 1 {
		return ""
	}
	tokens := parsed[0].tokens
	cmd := filepath.Base(tokens[0])
	if sub := preferredRuleSubcommand(tokens[1:]); sub != "" {
		return cmd + ":" + sub
	}
	return cmd
}

func subcommandCandidates(args []string) []string {
	var out []string
	if len(args) > 0 && args[0] != "" {
		out = append(out, args[0])
	}
	if sub := firstPositional(args); sub != "" && (len(out) == 0 || out[0] != sub) {
		out = append(out, sub)
	}
	return out
}

func preferredRuleSubcommand(args []string) string {
	if sub := firstPositional(args); sub != "" {
		return sub
	}
	if len(args) > 0 {
		return args[0]
	}
	return ""
}

// firstPositional returns the first token in args that does not start with '-'.
func firstPositional(args []string) string {
	for _, tok := range args {
		if !strings.HasPrefix(tok, "-") {
			return tok
		}
	}
	return ""
}

type parsedCommand struct {
	tokens []string
}

func parseCommand(command string) ([]parsedCommand, error) {
	file, err := syntax.NewParser(syntax.Variant(syntax.LangBash)).Parse(strings.NewReader(command), "")
	if err != nil {
		return nil, err
	}
	if len(file.Stmts) != 1 {
		return nil, fmt.Errorf("multiple statements require approval")
	}
	return parseStmt(file.Stmts[0])
}

func parseStmt(stmt *syntax.Stmt) ([]parsedCommand, error) {
	if stmt == nil || stmt.Cmd == nil {
		return nil, fmt.Errorf("empty statement")
	}
	if stmt.Negated || stmt.Background || stmt.Coprocess || stmt.Disown {
		return nil, fmt.Errorf("compound statement requires approval")
	}
	if err := validateRedirects(stmt.Redirs); err != nil {
		return nil, err
	}

	switch cmd := stmt.Cmd.(type) {
	case *syntax.CallExpr:
		parsed, err := parseCallExpr(cmd)
		if err != nil {
			return nil, err
		}
		return []parsedCommand{parsed}, nil
	case *syntax.BinaryCmd:
		switch cmd.Op {
		case syntax.Pipe, syntax.AndStmt, syntax.OrStmt:
		default:
			return nil, fmt.Errorf("unsupported binary operator requires approval")
		}
		left, err := parseStmt(cmd.X)
		if err != nil {
			return nil, err
		}
		right, err := parseStmt(cmd.Y)
		if err != nil {
			return nil, err
		}
		return append(left, right...), nil
	default:
		return nil, fmt.Errorf("compound command requires approval")
	}
}

func parseCallExpr(cmd *syntax.CallExpr) (parsedCommand, error) {
	if len(cmd.Args) == 0 {
		return parsedCommand{}, fmt.Errorf("assignment-only command requires approval")
	}
	if err := validateAssignments(cmd.Assigns); err != nil {
		return parsedCommand{}, err
	}

	tokens := make([]string, 0, len(cmd.Args))
	for _, word := range cmd.Args {
		lit, ok := literalWord(word)
		if !ok {
			return parsedCommand{}, fmt.Errorf("dynamic word requires approval")
		}
		tokens = append(tokens, lit)
	}
	return parsedCommand{tokens: tokens}, nil
}

func validateAssignments(assigns []*syntax.Assign) error {
	for _, assign := range assigns {
		if assign == nil || assign.Append || assign.Naked || assign.Index != nil || assign.Array != nil || assign.Name == nil {
			return fmt.Errorf("dynamic assignment requires approval")
		}
		if assign.Value == nil {
			continue
		}
		if _, ok := literalWord(assign.Value); !ok {
			return fmt.Errorf("dynamic assignment requires approval")
		}
	}
	return nil
}

func validateRedirects(redirs []*syntax.Redirect) error {
	for _, redir := range redirs {
		if err := validateRedirect(redir); err != nil {
			return err
		}
	}
	return nil
}

func validateRedirect(redir *syntax.Redirect) error {
	if redir == nil || redir.Word == nil || redir.Hdoc != nil {
		return fmt.Errorf("unsupported redirect requires approval")
	}

	switch redir.Op {
	case syntax.RdrIn:
		fd := ""
		if redir.N != nil {
			fd = redir.N.Value
		}
		if fd != "" && fd != "0" {
			return fmt.Errorf("unsupported input redirect fd")
		}
		target, ok := literalWord(redir.Word)
		if !ok || target != "/dev/null" {
			return fmt.Errorf("input redirection requires approval")
		}
		return nil
	case syntax.RdrOut, syntax.AppOut:
		fd := ""
		if redir.N != nil {
			fd = redir.N.Value
		}
		if fd != "" && fd != "1" && fd != "2" {
			return fmt.Errorf("unsupported redirect fd")
		}
		target, ok := literalWord(redir.Word)
		if !ok || !isSafeRedirectTarget(target, redir.Op == syntax.AppOut) {
			return fmt.Errorf("output redirection requires approval")
		}
		return nil
	case syntax.DplOut:
		srcFD := "1"
		if redir.N != nil {
			srcFD = redir.N.Value
		}
		if srcFD != "1" && srcFD != "2" {
			return fmt.Errorf("unsupported duplicate output fd")
		}
		target, ok := literalWord(redir.Word)
		if !ok || (target != "1" && target != "2") {
			return fmt.Errorf("output duplication requires approval")
		}
		return nil
	default:
		return fmt.Errorf("unsupported redirect requires approval")
	}
}

func literalWord(word *syntax.Word) (string, bool) {
	if word == nil {
		return "", false
	}
	return literalWordParts(word.Parts)
}

func literalWordParts(parts []syntax.WordPart) (string, bool) {
	var b strings.Builder
	for _, part := range parts {
		switch part := part.(type) {
		case *syntax.Lit:
			b.WriteString(part.Value)
		case *syntax.SglQuoted:
			if part.Dollar {
				return "", false
			}
			b.WriteString(part.Value)
		case *syntax.DblQuoted:
			if part.Dollar {
				return "", false
			}
			s, ok := literalWordParts(part.Parts)
			if !ok {
				return "", false
			}
			b.WriteString(s)
		default:
			return "", false
		}
	}
	return b.String(), true
}

func validateFindTokens(tokens []string) bool {
	for i := 1; i < len(tokens); i++ {
		switch tokens[i] {
		case "-delete", "-ok", "-okdir", "-execdir":
			return false
		case "-exec":
			end := -1
			for j := i + 1; j < len(tokens); j++ {
				if tokens[j] == ";" || tokens[j] == `\;` || tokens[j] == "+" {
					end = j
					break
				}
			}
			if end == -1 || end == i+1 {
				return false
			}
			nested := tokens[i+1 : end]
			if !matchesBuiltinTokens(nested) {
				return false
			}
			i = end
		}
	}
	return true
}

func validateGoTokens(tokens []string) bool {
	if firstPositional(tokens[1:]) != "env" {
		return true
	}
	for _, tok := range tokens[1:] {
		if tokenMatchesBlockedFlag(tok, "-w") || tokenMatchesBlockedFlag(tok, "-u") {
			return false
		}
	}
	return true
}

func matchesBuiltinTokens(tokens []string) bool {
	loadBuiltin()
	for _, rule := range builtinRules {
		if matchesTokens(rule, tokens) {
			return true
		}
	}
	return false
}

func isSafeRedirectTarget(target string, _ bool) bool {
	if target == "/dev/null" {
		return true
	}
	if !filepath.IsAbs(target) {
		return false
	}
	if strings.ContainsAny(target, "*?[") {
		return false
	}

	cleaned := filepath.Clean(target)
	resolved, err := resolveExistingRedirectPath(cleaned)
	if err != nil {
		return false
	}
	for _, root := range allowedRedirectRoots() {
		if resolved == root || strings.HasPrefix(resolved, root+string(filepath.Separator)) {
			return true
		}
	}
	return false
}

func allowedRedirectRoots() []string {
	seen := map[string]struct{}{}
	var roots []string
	for _, root := range []string{"/tmp", "/private/tmp", os.TempDir()} {
		root = filepath.Clean(root)
		if root == "" || root == "." {
			continue
		}
		if _, ok := seen[root]; ok {
			continue
		}
		seen[root] = struct{}{}
		roots = append(roots, root)
	}
	return roots
}

func resolveExistingRedirectPath(target string) (string, error) {
	current := target
	for {
		info, err := os.Lstat(current)
		if err == nil {
			if info.Mode()&os.ModeSymlink != 0 {
				return filepath.EvalSymlinks(current)
			}
			return current, nil
		}
		if !os.IsNotExist(err) {
			return "", err
		}

		parent := filepath.Dir(current)
		if parent == current {
			return "", fmt.Errorf("no existing path component")
		}
		current = parent
	}
}
