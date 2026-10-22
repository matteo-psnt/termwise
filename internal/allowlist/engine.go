package allowlist

import (
	_ "embed"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"
)

// validator checks what a rule's flag lists cannot express. It returns why the
// command needs approval, or "" when it is fine.
type validator func(*ruleEngine, parsedCommand) string

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
		// Every error parseCommand returns is phrased as the reason.
		return err.Error()
	}

	for _, seg := range segments {
		if reason := e.builtinReason(seg); reason != "" {
			return reason
		}
	}
	return ""
}

// ruleReason returns why cmd does not satisfy rule, or "" when it does.
func (e *ruleEngine) ruleReason(rule Rule, cmd parsedCommand) string {
	if cmd.Name == "" {
		return "part of it is not a plain command"
	}
	if cmd.Name != rule.Cmd {
		return cmd.Name + " is not on the read-only allowlist"
	}
	rule = e.effectiveRule(rule)

	if reason := subcommandReason(rule, cmd.Args); reason != "" {
		return reason
	}

	if validator := e.validators[rule.Cmd]; validator != nil {
		if reason := validator(e, cmd); reason != "" {
			return reason
		}
	}

	for _, tok := range cmd.Args {
		for _, blocked := range rule.Deny.Flags {
			if ruleDeniesToken(rule, tok, blocked) {
				return deniedFlagReason(rule.Cmd, blocked)
			}
		}
	}

	if len(rule.Deny.GlobalFlags) > 0 {
		global := cmd.Args
		if i := subcommandIndex(rule, cmd.Args); i >= 0 {
			global = cmd.Args[:i]
		}
		for _, tok := range global {
			for _, blocked := range rule.Deny.GlobalFlags {
				if ruleDeniesToken(rule, tok, blocked) {
					return deniedFlagReason(rule.Cmd, blocked)
				}
			}
		}
	}

	return ""
}

func deniedFlagReason(cmd, flag string) string {
	return cmd + "'s " + flag + " flag requires approval"
}

// builtinReason returns why no builtin rule approves cmd, or "" when one
// does. The reason comes from the command's own rule when it has one, so it
// names the subcommand, flag or argument at fault.
func (e *ruleEngine) builtinReason(cmd parsedCommand) string {
	if cmd.Name == "" {
		return "part of it is not a plain command"
	}
	first := ""
	for _, rule := range e.builtinRules {
		if rule.Cmd != cmd.Name {
			continue
		}
		reason := e.ruleReason(rule, cmd)
		if reason == "" {
			return ""
		}
		if first == "" {
			first = reason
		}
	}
	if first == "" {
		return cmd.Name + " is not on the read-only allowlist"
	}
	return first
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
	rule.Parse.WordFlags = rule.Parse.WordFlags || meta.Parse.WordFlags
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
				WordFlags:            rule.Parse.WordFlags,
			},
			Allow: AllowRule{
				Subcommands: append([]string(nil), rule.Allow.Subcommands...),
			},
			Deny: DenyRule{
				Flags:       append([]string(nil), rule.Deny.Flags...),
				GlobalFlags: append([]string(nil), rule.Deny.GlobalFlags...),
			},
		}
	}
	return out
}
