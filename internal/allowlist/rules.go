package allowlist

import "strings"

type Rule struct {
	Cmd        string   `yaml:"cmd"`
	AllowSubs  []string `yaml:"allow_subs"`
	BlockFlags []string `yaml:"block_flags"`
}

type allowlistFile struct {
	Rules []Rule `yaml:"rules"`
}

func parseRule(s string) (Rule, bool) {
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
					rule.BlockFlags = append(rule.BlockFlags, item[1:])
				}
			}
			continue
		}
		for _, item := range items {
			item = strings.TrimSpace(item)
			if item != "" {
				rule.AllowSubs = append(rule.AllowSubs, item)
			}
		}
	}

	return rule, true
}

func compileRules(userRules []string) []Rule {
	rules := make([]Rule, 0, len(userRules))
	for _, raw := range userRules {
		rule, ok := parseRule(raw)
		if ok {
			rules = append(rules, rule)
		}
	}
	return rules
}

func subcommandCandidates(args []string) []string {
	if len(args) == 0 {
		return nil
	}
	out := []string{args[0]}
	if sub := firstPositional(args); sub != "" && sub != args[0] {
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

func firstPositional(args []string) string {
	for _, tok := range args {
		if !strings.HasPrefix(tok, "-") {
			return tok
		}
	}
	return ""
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
