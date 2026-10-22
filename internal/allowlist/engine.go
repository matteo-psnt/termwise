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
// NeedsApproval is derived from this; exposing the reason lets the approval
// prompt say more than "run command?".
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

// needsApproval is defined by approvalReason so the decision and the reason
// shown in the approval prompt cannot drift apart. Every rejecting path in
// approvalReason returns a non-empty reason; "" means every segment matched.
func (e *ruleEngine) needsApproval(command string) bool {
	return e.approvalReason(command) != ""
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

// loadBuiltinRules parses the embedded rules. They are compiled in, so a
// parse failure is a defect in the file, not something a user can cause or
// fix. Returning no rules would fail closed but silently (every command would
// suddenly need approval with no hint why), so panic and let the first test
// run catch it, as internal/models does for its embedded catalog.
func loadBuiltinRules() []Rule {
	rules, err := parseRules(builtinAllowlistYAML)
	if err != nil {
		panic("embedded allowlist.yaml is malformed: " + err.Error())
	}
	return rules
}

func parseRules(data []byte) ([]Rule, error) {
	var f allowlistFile
	if err := yaml.Unmarshal(data, &f); err != nil {
		return nil, err
	}
	return f.Rules, nil
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
	if len(rule.Parse.BoolFlags) == 0 {
		rule.Parse.BoolFlags = append([]string(nil), meta.Parse.BoolFlags...)
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
				BoolFlags:            append([]string(nil), rule.Parse.BoolFlags...),
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
