package allowlist

import (
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"
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
	return matchesTokens(rule, parsed[0])
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

	// Check BlockFlags: no token may equal a blocked flag.
	for _, tok := range tokens[1:] {
		for _, blocked := range rule.BlockFlags {
			if tok == blocked {
				return false
			}
		}
	}

	return true
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
		if !matchesAny(userRules, seg) {
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
	tokens := parsed[0]
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

type shellParser struct {
	command  string
	buf      strings.Builder
	tokens   []string
	segments [][]string
	state    shellState
	i        int
}

func (p *shellParser) flushToken() {
	if p.buf.Len() == 0 {
		return
	}
	p.tokens = append(p.tokens, p.buf.String())
	p.buf.Reset()
}

func (p *shellParser) flushSegment() error {
	p.flushToken()
	if len(p.tokens) == 0 {
		return fmt.Errorf("empty pipeline segment")
	}
	p.segments = append(p.segments, p.tokens)
	p.tokens = nil
	return nil
}

func (p *shellParser) handleSingle(ch byte) {
	if ch == '\'' {
		p.state = shellStateNormal
		return
	}
	p.buf.WriteByte(ch)
}

func (p *shellParser) handleDouble() error {
	ch := p.command[p.i]
	switch ch {
	case '"':
		p.state = shellStateNormal
	case '\\':
		if p.i+1 >= len(p.command) {
			return fmt.Errorf("dangling escape")
		}
		p.i++
		p.buf.WriteByte(p.command[p.i])
	case '`':
		return fmt.Errorf("backticks require approval")
	case '$':
		if p.i+1 < len(p.command) && p.command[p.i+1] == '(' {
			return fmt.Errorf("command substitution requires approval")
		}
		p.buf.WriteByte(ch)
	default:
		p.buf.WriteByte(ch)
	}
	return nil
}

func (p *shellParser) handleRedirect() error {
	fd := ""
	if p.buf.Len() > 0 {
		word := p.buf.String()
		p.buf.Reset()
		if isDigits(word) {
			fd = word
		} else {
			p.tokens = append(p.tokens, word)
		}
	}
	appendMode := false
	if p.i+1 < len(p.command) && p.command[p.i+1] == '>' {
		appendMode = true
		p.i++
	}
	if fd != "" && fd != "1" && fd != "2" {
		return fmt.Errorf("unsupported redirect fd")
	}
	target, next, err := parseWord(p.command, p.i+1)
	if err != nil {
		return err
	}
	if !isSafeRedirectTarget(target, appendMode) {
		return fmt.Errorf("output redirection requires approval")
	}
	p.i = next - 1
	return nil
}

func (p *shellParser) handleNormal() error {
	ch := p.command[p.i]
	switch ch {
	case ' ', '\t':
		p.flushToken()
	case '\\':
		if p.i+1 >= len(p.command) {
			return fmt.Errorf("dangling escape")
		}
		p.i++
		p.buf.WriteByte(p.command[p.i])
	case '\'':
		p.state = shellStateSingle
	case '"':
		p.state = shellStateDouble
	case '|':
		if p.i+1 < len(p.command) && p.command[p.i+1] == '|' {
			return fmt.Errorf("logical OR requires approval")
		}
		return p.flushSegment()
	case '&':
		return fmt.Errorf("backgrounding or logical AND requires approval")
	case ';', '\n', '\r':
		return fmt.Errorf("command chaining requires approval")
	case '`':
		return fmt.Errorf("backticks require approval")
	case '$':
		if p.i+1 < len(p.command) && p.command[p.i+1] == '(' {
			return fmt.Errorf("command substitution requires approval")
		}
		p.buf.WriteByte(ch)
	case '<':
		return fmt.Errorf("input redirection requires approval")
	case '>':
		return p.handleRedirect()
	case '(', ')':
		if p.buf.Len() == 0 {
			return fmt.Errorf("subshell syntax requires approval")
		}
		p.buf.WriteByte(ch)
	default:
		p.buf.WriteByte(ch)
	}
	return nil
}

func parseCommand(command string) ([][]string, error) {
	p := &shellParser{command: command}
	for p.i = 0; p.i < len(command); p.i++ {
		var err error
		switch p.state {
		case shellStateSingle:
			p.handleSingle(command[p.i])
		case shellStateDouble:
			err = p.handleDouble()
		default:
			err = p.handleNormal()
		}
		if err != nil {
			return nil, err
		}
	}
	if p.state != shellStateNormal {
		return nil, fmt.Errorf("unterminated quote")
	}
	if err := p.flushSegment(); err != nil {
		return nil, err
	}
	return p.segments, nil
}

type shellState uint8

const (
	shellStateNormal shellState = iota
	shellStateSingle
	shellStateDouble
)

func parseWord(s string, start int) (string, int, error) {
	var (
		buf   strings.Builder
		state = shellStateNormal
		i     = start
	)

	for i < len(s) && (s[i] == ' ' || s[i] == '\t') {
		i++
	}
	if i >= len(s) {
		return "", i, fmt.Errorf("missing redirect target")
	}

	for ; i < len(s); i++ {
		ch := s[i]
		switch state {
		case shellStateSingle:
			if ch == '\'' {
				state = shellStateNormal
				continue
			}
			buf.WriteByte(ch)

		case shellStateDouble:
			switch ch {
			case '"':
				state = shellStateNormal
			case '\\':
				if i+1 >= len(s) {
					return "", i, fmt.Errorf("dangling escape")
				}
				i++
				buf.WriteByte(s[i])
			case '`':
				return "", i, fmt.Errorf("backticks require approval")
			case '$':
				if i+1 < len(s) && s[i+1] == '(' {
					return "", i, fmt.Errorf("command substitution requires approval")
				}
				buf.WriteByte(ch)
			default:
				buf.WriteByte(ch)
			}

		default:
			switch ch {
			case ' ', '\t':
				if buf.Len() == 0 {
					continue
				}
				return buf.String(), i, nil
			case '\\':
				if i+1 >= len(s) {
					return "", i, fmt.Errorf("dangling escape")
				}
				i++
				buf.WriteByte(s[i])
			case '\'':
				state = shellStateSingle
			case '"':
				state = shellStateDouble
			case '|', '&', ';', '<', '>', '\n', '\r', '`':
				return "", i, fmt.Errorf("invalid redirect target")
			case '$':
				if i+1 < len(s) && s[i+1] == '(' {
					return "", i, fmt.Errorf("command substitution requires approval")
				}
				buf.WriteByte(ch)
			default:
				buf.WriteByte(ch)
			}
		}
	}

	if state != shellStateNormal {
		return "", i, fmt.Errorf("unterminated quote")
	}
	if buf.Len() == 0 {
		return "", i, fmt.Errorf("missing redirect target")
	}
	return buf.String(), i, nil
}

func isDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, ch := range s {
		if ch < '0' || ch > '9' {
			return false
		}
	}
	return true
}

func isSafeRedirectTarget(target string, _ bool) bool {
	if target == "/dev/null" {
		return true
	}
	if !filepath.IsAbs(target) {
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
