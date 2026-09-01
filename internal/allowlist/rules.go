package allowlist

import "strings"

type Rule struct {
	Cmd   string    `yaml:"cmd"`
	Parse ParseRule `yaml:"parse"`
	Allow AllowRule `yaml:"allow"`
	Deny  DenyRule  `yaml:"deny"`
}

type ParseRule struct {
	SubcommandValueFlags []string `yaml:"subcommand_value_flags"`
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

// Parse parses an allow-list rule string into a Rule. The second return is
// false when the input is empty or unparseable.
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
			for _, item := range items {
				item = strings.TrimSpace(item)
				if strings.HasPrefix(item, "!") {
					rule.Deny.Flags = append(rule.Deny.Flags, item[1:])
				}
			}
			continue
		}
		for _, item := range items {
			item = strings.TrimSpace(item)
			if item != "" {
				rule.Allow.Subcommands = append(rule.Allow.Subcommands, item)
			}
		}
	}

	return rule, true
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

func preferredRuleSubcommand(rule Rule, args []string) string {
	if sub := firstPositional(rule, args); sub != "" {
		return sub
	}
	if len(args) > 0 {
		return args[0]
	}
	return ""
}

func firstPositional(rule Rule, args []string) string {
	for i := 0; i < len(args); i++ {
		tok := args[i]
		if tok == "--" {
			if i+1 < len(args) {
				return args[i+1]
			}
			return ""
		}
		if strings.HasPrefix(tok, "-") {
			if consumesSubcommandValue(rule.Parse.SubcommandValueFlags, tok) {
				i++
			}
			continue
		}
		return tok
	}
	return ""
}

func consumesSubcommandValue(valueFlags []string, token string) bool {
	if len(valueFlags) == 0 {
		return false
	}
	for _, flag := range valueFlags {
		if token == flag {
			return true
		}
		if name, _, ok := strings.Cut(token, "="); ok && name == flag {
			return true
		}
	}
	return false
}

func tokenMatchesDeniedFlag(token, blocked string) bool {
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
