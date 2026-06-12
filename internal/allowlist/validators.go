package allowlist

import (
	"path/filepath"
	"strings"
)

func defaultValidators() map[string]validator {
	return map[string]validator{
		"find":   validateFindCommand,
		"go":     validateGoCommand,
		"sysctl": validateSysctlCommand,
	}
}

func validateFindCommand(e *ruleEngine, cmd parsedCommand) bool {
	for i := 0; i < len(cmd.Args); i++ {
		switch cmd.Args[i] {
		case "-delete", "-ok", "-okdir", "-execdir":
			return false
		case "-exec":
			end := -1
			for j := i + 1; j < len(cmd.Args); j++ {
				if cmd.Args[j] == ";" || cmd.Args[j] == `\;` || cmd.Args[j] == "+" {
					end = j
					break
				}
			}
			if end == -1 || end == i+1 {
				return false
			}
			if !e.matchesBuiltin(parsedCommand{Name: filepath.Base(cmd.Args[i+1]), Args: cmd.Args[i+2 : end]}) {
				return false
			}
			i = end
		}
	}
	return true
}

func validateGoCommand(_ *ruleEngine, cmd parsedCommand) bool {
	if firstPositional(cmd.Args) != "env" {
		return true
	}
	for _, tok := range cmd.Args {
		if tokenMatchesBlockedFlag(tok, "-w") || tokenMatchesBlockedFlag(tok, "-u") {
			return false
		}
	}
	return true
}

func validateSysctlCommand(_ *ruleEngine, cmd parsedCommand) bool {
	for _, tok := range cmd.Args {
		if strings.Contains(tok, "=") {
			return false
		}
	}
	return true
}
