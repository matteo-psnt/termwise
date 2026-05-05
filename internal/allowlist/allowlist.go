package allowlist

import (
	_ "embed"
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
	tokens := strings.Fields(command)
	if len(tokens) == 0 {
		return false
	}

	if filepath.Base(tokens[0]) != rule.Cmd {
		return false
	}

	// Check AllowSubs: find the first non-flag positional arg.
	if len(rule.AllowSubs) > 0 {
		sub := firstPositional(tokens[1:])
		found := false
		for _, s := range rule.AllowSubs {
			if s == sub {
				found = true
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
// Returns false (auto-approved) if any built-in or user rule matches AND
// the command contains no shell metacharacters.
func NeedsApproval(userRules []string, command string) bool {
	command = strings.TrimSpace(command)
	if command == "" {
		return true
	}

	// Pure pipelines (| only — no ||, ;, &, etc.) are approved when every
	// segment individually passes the allowlist.
	if segs, ok := splitPipeline(command); ok {
		for _, seg := range segs {
			if NeedsApproval(userRules, seg) {
				return true
			}
		}
		return false
	}

	// All other shell metacharacters allow chaining or redirection that
	// bypasses flag-level checks — always require approval.
	if containsShellMetachar(command) {
		return true
	}

	loadBuiltin()

	for _, rule := range builtinRules {
		if Matches(rule, command) {
			return false
		}
	}

	for _, s := range userRules {
		rule, ok := Parse(s)
		if !ok {
			continue
		}
		if Matches(rule, command) {
			return false
		}
	}

	return true
}

// splitPipeline splits command on | if and only if it is a pure pipeline —
// single pipes only, no || (logical OR) or any other metacharacter.
// Returns (nil, false) if the command cannot be safely treated as a pipeline.
func splitPipeline(command string) ([]string, bool) {
	stripped := stripNullRedirects(command)
	if !strings.Contains(stripped, "|") {
		return nil, false
	}
	if strings.Contains(stripped, "||") {
		return nil, false
	}
	// Reject any other hard metacharacters.
	for i, ch := range stripped {
		switch ch {
		case ';', '<', '>':
			return nil, false
		case '&':
			return nil, false
		case '`':
			return nil, false
		case '\n', '\r':
			return nil, false
		case '$':
			if i+1 < len(stripped) && stripped[i+1] == '(' {
				return nil, false
			}
		}
	}
	var segs []string
	for _, p := range strings.Split(stripped, "|") {
		p = strings.TrimSpace(p)
		if p != "" {
			segs = append(segs, p)
		}
	}
	if len(segs) == 0 {
		return nil, false
	}
	return segs, true
}

// stripNullRedirects removes harmless >/dev/null redirections before metachar
// scanning. Patterns like 2>/dev/null, 1>/dev/null, and >/dev/null discard
// output and pose no risk, so stripping them avoids a false positive on '>'.
func stripNullRedirects(command string) string {
	for _, pat := range []string{"2>/dev/null", "1>/dev/null", ">/dev/null"} {
		command = strings.ReplaceAll(command, pat, "")
	}
	return command
}

// containsShellMetachar reports whether command contains shell control
// characters that could chain or redirect execution.
// We intentionally do not attempt to parse quoting — any occurrence of
// these characters triggers approval regardless of context.
func containsShellMetachar(command string) bool {
	command = stripNullRedirects(command)
	for i, ch := range command {
		switch ch {
		case '|', ';', '<', '>':
			return true
		case '&':
			return true
		case '`':
			return true
		case '\n', '\r':
			return true
		case '$':
			if i+1 < len(command) && command[i+1] == '(' {
				return true
			}
		}
	}
	return false
}

// BuildRuleFromCommand derives a sensible rule string from a shell command.
// If the command has a subcommand (first positional arg), returns "cmd:sub".
// Otherwise returns just "cmd".
// Used by the TUI when the user presses 'a' at an approval prompt.
func BuildRuleFromCommand(command string) string {
	tokens := strings.Fields(command)
	if len(tokens) == 0 {
		return ""
	}
	cmd := filepath.Base(tokens[0])
	sub := firstPositional(tokens[1:])
	if sub != "" {
		return cmd + ":" + sub
	}
	return cmd
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
