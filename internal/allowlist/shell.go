package allowlist

import (
	"fmt"
	"path/filepath"
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
		return nil, err
	}
	if len(file.Stmts) != 1 {
		return nil, fmt.Errorf("multiple statements require approval")
	}
	return e.parseStmt(file.Stmts[0])
}

func (e *ruleEngine) parseStmt(stmt *syntax.Stmt) ([]parsedCommand, error) {
	if stmt == nil || stmt.Cmd == nil {
		return nil, fmt.Errorf("empty statement")
	}
	if stmt.Negated || stmt.Background || stmt.Coprocess || stmt.Disown {
		return nil, fmt.Errorf("compound statement requires approval")
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
			return nil, fmt.Errorf("unsupported binary operator requires approval")
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
		return nil, fmt.Errorf("compound command requires approval")
	}
}

func parseCallExpr(cmd *syntax.CallExpr) (parsedCommand, error) {
	if len(cmd.Args) == 0 {
		return parsedCommand{}, fmt.Errorf("assignment-only command requires approval")
	}
	if err := validateAssignments(cmd.Assigns); err != nil {
		return parsedCommand{}, err
	}

	tokens := make([]string, 0, len(cmd.Args))
	for _, word := range cmd.Args {
		lit, ok := literalWord(word)
		if !ok {
			return parsedCommand{}, fmt.Errorf("dynamic word requires approval")
		}
		tokens = append(tokens, lit)
	}
	parsed, ok := parsedCommandFromTokens(tokens)
	if !ok {
		return parsedCommand{}, fmt.Errorf("assignment-only command requires approval")
	}
	return parsed, nil
}

func validateAssignments(assigns []*syntax.Assign) error {
	for _, assign := range assigns {
		if assign == nil || assign.Append || assign.Naked || assign.Index != nil || assign.Array != nil || assign.Name == nil {
			return fmt.Errorf("dynamic assignment requires approval")
		}
		if !harmlessEnvVar(assign.Name.Value) {
			return fmt.Errorf("setting %s requires approval", assign.Name.Value)
		}
		if assign.Value == nil {
			continue
		}
		if _, ok := literalWord(assign.Value); !ok {
			return fmt.Errorf("dynamic assignment requires approval")
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
	return parsedCommand{
		Name: filepath.Base(tokens[0]),
		Args: append([]string(nil), tokens[1:]...),
	}, true
}
