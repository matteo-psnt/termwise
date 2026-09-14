package allowlist

import (
	_ "embed"
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

var engine = sync.OnceValue(func() *ruleEngine {
	return newRuleEngine(loadBuiltinRules())
})

//go:embed allowlist.yaml
var builtinAllowlistYAML []byte

// NeedsApproval reports whether the command must be confirmed by the user.
func NeedsApproval(command string) bool {
	return engine().needsApproval(command)
}

// ApprovalReason explains, in one phrase, why a command needs confirmation,
// naming the part that triggered it. Returns "" when no approval is needed.
//
// NeedsApproval already computes this; discarding it left the approval prompt
// unable to say anything beyond "run command?".
func ApprovalReason(command string) string {
	return engine().approvalReason(command)
}

func newRuleEngine(rules []Rule) *ruleEngine {
	return &ruleEngine{
		builtinRules:  cloneRules(rules),
		validators:    defaultValidators(),
		redirectRoots: defaultRedirectRoots(),
	}
}

func (e *ruleEngine) needsApproval(command string) bool {
	command = strings.TrimSpace(command)
	if command == "" {
		return true
	}

	segments, err := e.parseCommand(command)
	if err != nil {
		return true
	}

	for _, seg := range segments {
		if !e.matchesBuiltin(seg) {
			return true
		}
	}
	return false
}

func (e *ruleEngine) approvalReason(command string) string {
	command = strings.TrimSpace(command)
	if command == "" {
		return "the command is empty"
	}

	segments, err := e.parseCommand(command)
	if err != nil {
		return "it is more than one statement, or could not be parsed"
	}

	for _, seg := range segments {
		if !e.matchesBuiltin(seg) {
			if seg.Name == "" {
				return "part of it is not a plain command"
			}
			return seg.Name + " is not on the read-only allowlist"
		}
	}
	return ""
}

func (e *ruleEngine) matchRule(rule Rule, cmd parsedCommand) bool {
	if cmd.Name == "" {
		return false
	}
	if cmd.Name != rule.Cmd {
		return false
	}
	rule = e.effectiveRule(rule)

	if !matchesAllowedSubcommand(rule, cmd.Args) {
		return false
	}

	if validator := e.validators[rule.Cmd]; validator != nil && !validator(e, cmd) {
		return false
	}

	for _, tok := range cmd.Args {
		for _, blocked := range rule.Deny.Flags {
			if tokenMatchesDeniedFlag(tok, blocked) {
				return false
			}
		}
	}

	return true
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

func (e *ruleEngine) ruleMetadata(command string) Rule {
	for _, rule := range e.builtinRules {
		if rule.Cmd == command {
			return rule
		}
	}
	return Rule{Cmd: command}
}

func (e *ruleEngine) effectiveRule(rule Rule) Rule {
	meta := e.ruleMetadata(rule.Cmd)
	if len(rule.Parse.SubcommandValueFlags) == 0 {
		rule.Parse.SubcommandValueFlags = append([]string(nil), meta.Parse.SubcommandValueFlags...)
	}
	return rule
}

func cloneRules(rules []Rule) []Rule {
	out := make([]Rule, len(rules))
	for i, rule := range rules {
		out[i] = Rule{
			Cmd: rule.Cmd,
			Parse: ParseRule{
				SubcommandValueFlags: append([]string(nil), rule.Parse.SubcommandValueFlags...),
			},
			Allow: AllowRule{
				Subcommands: append([]string(nil), rule.Allow.Subcommands...),
			},
			Deny: DenyRule{
				Flags: append([]string(nil), rule.Deny.Flags...),
			},
		}
	}
	return out
}
