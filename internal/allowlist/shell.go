package allowlist

import (
	"errors"
	"fmt"
	"strings"

	"mvdan.cc/sh/v3/syntax"
)

type parsedCommand struct {
	Name string
	Args []string
}

func (e *ruleEngine) parseCommand(command string) ([]parsedCommand, error) {
	file, err := syntax.NewParser(syntax.Variant(syntax.LangBash)).Parse(strings.NewReader(command), "")
	if err != nil {
		return nil, errors.New("it could not be parsed")
	}
	if len(file.Stmts) != 1 {
		return nil, errors.New("it is more than one statement")
	}
	return e.parseStmt(file.Stmts[0])
}

func (e *ruleEngine) parseStmt(stmt *syntax.Stmt) ([]parsedCommand, error) {
	if stmt == nil || stmt.Cmd == nil {
		return nil, errors.New("part of it is empty")
	}
	if stmt.Negated || stmt.Background || stmt.Coprocess || stmt.Disown {
		return nil, errors.New("it negates a command or runs one in the background")
	}
	if err := e.validateRedirects(stmt.Redirs); err != nil {
		return nil, err
	}

	switch cmd := stmt.Cmd.(type) {
	case *syntax.CallExpr:
		segment, err := parseCallExpr(cmd)
		if err != nil {
			return nil, err
		}
		return []parsedCommand{segment}, nil
	case *syntax.BinaryCmd:
		switch cmd.Op {
		case syntax.Pipe, syntax.AndStmt, syntax.OrStmt:
		default:
			return nil, errors.New("it joins commands with an operator other than |, && or ||")
		}

		left, err := e.parseStmt(cmd.X)
		if err != nil {
			return nil, err
		}
		right, err := e.parseStmt(cmd.Y)
		if err != nil {
			return nil, err
		}
		return append(left, right...), nil
	default:
		return nil, errors.New("it uses a loop, subshell, function or other compound command")
	}
}

func parseCallExpr(cmd *syntax.CallExpr) (parsedCommand, error) {
	if len(cmd.Args) == 0 {
		return parsedCommand{}, errors.New("it only sets variables")
	}
	if err := validateAssignments(cmd.Assigns); err != nil {
		return parsedCommand{}, err
	}

	tokens := make([]string, 0, len(cmd.Args))
	for _, word := range cmd.Args {
		lit, ok := literalWord(word)
		if !ok {
			return parsedCommand{}, errors.New("it uses a value only known when it runs, such as $VAR or $(...)")
		}
		tokens = append(tokens, lit)
	}
	parsed, ok := parsedCommandFromTokens(tokens)
	if !ok {
		return parsedCommand{}, errors.New("it only sets variables")
	}
	return parsed, nil
}

func validateAssignments(assigns []*syntax.Assign) error {
	for _, assign := range assigns {
		if assign == nil || assign.Append || assign.Naked || assign.Index != nil || assign.Array != nil || assign.Name == nil {
			return errors.New("it sets a variable in a way only known when it runs")
		}
		name := assign.Name.Value
		pager := isPagerVar(name)
		if !harmlessEnvVar(name) && !pager {
			return fmt.Errorf("setting %s requires approval", name)
		}
		value := ""
		if assign.Value != nil {
			lit, ok := literalWord(assign.Value)
			if !ok {
				return errors.New("it sets a variable in a way only known when it runs")
			}
			value = lit
		}
		// The pager variables name a program. Only cat (found on PATH, which
		// cannot be set here) and the empty value, which turns the pager
		// off, are harmless; this allows the common GIT_PAGER=cat git log.
		if pager && value != "" && value != "cat" {
			return fmt.Errorf("setting %s to anything but cat requires approval", name)
		}
	}
	return nil
}

// harmlessEnvVar reports whether a variable is safe to set in front of an
// allowlisted command. A literal value is not enough: PAGER, GIT_EXTERNAL_DIFF,
// LESSOPEN, LD_PRELOAD, BASH_ENV, PATH and many more make a read-only command
// run whatever the value names. So this is an allow list of variables that
// only change formatting or locale, and everything else needs approval.
func harmlessEnvVar(name string) bool {
	switch name {
	case "LANG", "LANGUAGE", "TZ", "NO_COLOR", "CLICOLOR", "CLICOLOR_FORCE", "COLUMNS", "LINES", "TERM":
		return true
	}
	return strings.HasPrefix(name, "LC_")
}

func isPagerVar(name string) bool {
	return name == "PAGER" || name == "GIT_PAGER"
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

func parsedCommandFromTokens(tokens []string) (parsedCommand, bool) {
	if len(tokens) == 0 {
		return parsedCommand{}, false
	}
	// The name is kept exactly as written. Rules are keyed by bare names that
	// the shell looks up on PATH; a word with a slash runs that file instead,
	// so ./ls or /tmp/x/cat is some other program that only shares a name.
	// Leaving the slash in means no rule matches and it needs approval.
	return parsedCommand{
		Name: tokens[0],
		Args: append([]string(nil), tokens[1:]...),
	}, true
}
