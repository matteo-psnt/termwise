package allowlist

import (
	_ "embed"
	"slices"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"
)

type validator func(*ruleEngine, parsedCommand) bool

type ruleEngine struct {
	builtinRules  []Rule
	validators    map[string]validator
	redirectRoots []string
}

var packageRuleEngine = sync.OnceValue(func() *ruleEngine {
	return newRuleEngine(loadBuiltinRules())
})

//go:embed allowlist.yaml
var builtinAllowlistYAML []byte

// Parse parses an allow-list rule string into a Rule.
func Parse(s string) (Rule, bool) {
	return parseRule(s)
}

// Matches reports whether rule allows the given shell command.
func Matches(rule Rule, command string) bool {
	return packageRuleEngine().Matches(rule, command)
}

// NeedsApproval reports whether the command must be confirmed by the user.
func NeedsApproval(userRules []string, command string) bool {
	return packageRuleEngine().NeedsApproval(userRules, command)
}

// BuildRuleFromCommand derives a sensible rule string from a shell command.
func BuildRuleFromCommand(command string) string {
	return packageRuleEngine().BuildRuleFromCommand(command)
}

func newRuleEngine(rules []Rule) *ruleEngine {
	return &ruleEngine{
		builtinRules:  rules,
		validators:    defaultValidators(),
		redirectRoots: defaultRedirectRoots(),
	}
}

func (e *ruleEngine) Matches(rule Rule, command string) bool {
	segments, err := e.parseCommand(command)
	if err != nil || len(segments) != 1 {
		return false
	}
	return e.matchRule(rule, segments[0])
}

func (e *ruleEngine) NeedsApproval(userRules []string, command string) bool {
	command = strings.TrimSpace(command)
	if command == "" {
		return true
	}

	segments, err := e.parseCommand(command)
	if err != nil {
		return true
	}

	extraRules := compileRules(userRules)
	for _, seg := range segments {
		if !e.matchesAny(extraRules, seg) {
			return true
		}
	}
	return false
}

func (e *ruleEngine) BuildRuleFromCommand(command string) string {
	segments, err := e.parseCommand(command)
	if err != nil || len(segments) != 1 {
		return ""
	}

	cmd := segments[0].Name
	if sub := preferredRuleSubcommand(segments[0].Args); sub != "" {
		return cmd + ":" + sub
	}
	return cmd
}

func (e *ruleEngine) matchRule(rule Rule, cmd parsedCommand) bool {
	if cmd.Name == "" {
		return false
	}
	if cmd.Name != rule.Cmd {
		return false
	}

	if len(rule.AllowSubs) > 0 {
		if !slices.ContainsFunc(subcommandCandidates(cmd.Args), func(c string) bool {
			return slices.Contains(rule.AllowSubs, c)
		}) {
			return false
		}
	}

	if validator := e.validators[rule.Cmd]; validator != nil && !validator(e, cmd) {
		return false
	}

	for _, tok := range cmd.Args {
		for _, blocked := range rule.BlockFlags {
			if tokenMatchesBlockedFlag(tok, blocked) {
				return false
			}
		}
	}

	return true
}

func (e *ruleEngine) matchesAny(userRules []Rule, cmd parsedCommand) bool {
	if e.matchesBuiltin(cmd) {
		return true
	}
	for _, rule := range userRules {
		if e.matchRule(rule, cmd) {
			return true
		}
	}
	return false
}

func (e *ruleEngine) matchesBuiltin(cmd parsedCommand) bool {
	for _, rule := range e.builtinRules {
		if e.matchRule(rule, cmd) {
			return true
		}
	}
	return false
}

func loadBuiltinRules() []Rule {
	var f allowlistFile
	if err := yaml.Unmarshal(builtinAllowlistYAML, &f); err != nil {
		return nil
	}
	return f.Rules
}
