package allowlist

import (
	"slices"
	"strings"
)

type Rule struct {
	Cmd   string    `yaml:"cmd"`
	Parse ParseRule `yaml:"parse"`
	Allow AllowRule `yaml:"allow"`
	Deny  DenyRule  `yaml:"deny"`
}

type ParseRule struct {
	SubcommandValueFlags []string `yaml:"subcommand_value_flags"`
	// BoolFlags are global flags that take no value, so they may appear
	// before the subcommand. Any other flag there, unless written
	// --flag=value, makes the subcommand unknown.
	BoolFlags []string `yaml:"bool_flags"`
}

type AllowRule struct {
	Subcommands []string `yaml:"subcommands"`
}

type DenyRule struct {
	Flags []string `yaml:"flags"`
}

type allowlistFile struct {
	Rules []Rule `yaml:"rules"`
}

func matchesAllowedSubcommand(rule Rule, args []string) bool {
	if len(rule.Allow.Subcommands) == 0 {
		return true
	}
	for _, candidate := range subcommandCandidates(rule, args) {
		for _, allowed := range rule.Allow.Subcommands {
			if candidate == allowed {
				return true
			}
		}
	}
	return false
}

func subcommandCandidates(rule Rule, args []string) []string {
	if len(args) == 0 {
		return nil
	}
	out := []string{args[0]}
	if sub := firstPositional(rule, args); sub != "" && sub != args[0] {
		out = append(out, sub)
	}
	return out
}

func firstPositional(rule Rule, args []string) string {
	if i := subcommandIndex(rule, args); i >= 0 {
		return args[i]
	}
	return ""
}

// subcommandIndex returns the index of the first positional argument, which
// for a rule with an allowed subcommand list is its subcommand, or -1 when
// there is none or it cannot be told.
//
// A global flag that takes a value but is not in subcommand_value_flags would
// make its value look like the subcommand: in git --attr-source status reset,
// git runs reset. So when the rule restricts subcommands, a flag before the
// subcommand must be known: a listed value flag, a listed bool flag, or a
// --flag=value whose value cannot be the next word. Anything else fails
// closed.
func subcommandIndex(rule Rule, args []string) int {
	strict := len(rule.Allow.Subcommands) > 0
	for i := 0; i < len(args); i++ {
		tok := args[i]
		if tok == "--" {
			if i+1 < len(args) {
				return i + 1
			}
			return -1
		}
		if !strings.HasPrefix(tok, "-") {
			return i
		}
		if consumes, known := subcommandValueFlag(rule.Parse.SubcommandValueFlags, tok); known {
			if consumes {
				i++
			}
			continue
		}
		if !strict {
			continue
		}
		if strings.HasPrefix(tok, "--") && strings.Contains(tok, "=") {
			continue
		}
		if slices.Contains(rule.Parse.BoolFlags, tok) {
			continue
		}
		return -1
	}
	return -1
}

// subcommandValueFlag reports whether token is one of the value flags, and if
// so whether its value is the next argument (-C dir) rather than attached
// (--git-dir=.git).
func subcommandValueFlag(valueFlags []string, token string) (consumesNext, known bool) {
	for _, flag := range valueFlags {
		if token == flag {
			return true, true
		}
		if name, _, ok := strings.Cut(token, "="); ok && name == flag {
			return false, true
		}
	}
	return false, false
}

// tokenMatchesDeniedFlag reports whether an argument turns on a denied flag,
// in any of the spellings the tool's own parser would accept:
//
//   - a long flag exactly, with =value attached, or abbreviated: getopt_long
//     takes any unambiguous prefix, so sort --out=f is sort --output=f;
//   - a short flag on its own, with its value attached (-ofile), or inside a
//     bundle (sort -uo f, fd -Hx rm). A bundle is scanned for the letter
//     anywhere, which can also hit an attached value that merely contains it.
//     That only costs an approval prompt, where missing a real flag would run
//     the command unasked.
//
// Single-dash multi-letter flags (find's -delete) are whole words and only
// match exactly.
func tokenMatchesDeniedFlag(token, blocked string) bool {
	if token == blocked {
		return true
	}
	if strings.HasPrefix(blocked, "--") {
		name, _, _ := strings.Cut(token, "=")
		return len(name) > 2 && strings.HasPrefix(name, "--") && strings.HasPrefix(blocked, name)
	}
	if len(blocked) == 2 && blocked[0] == '-' {
		if len(token) < 2 || token[0] != '-' || token[1] == '-' {
			return false
		}
		return strings.IndexByte(token[1:], blocked[1]) >= 0
	}
	return false
}
