package allowlist

import (
	"strings"
)

func defaultValidators() map[string]validator {
	return map[string]validator{
		"find":   validateFindCommand,
		"go":     validateGoCommand,
		"sysctl": validateSysctlCommand,
		"xargs":  validateXargsCommand,
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
			nested, ok := parsedCommandFromTokens(cmd.Args[i+1 : end])
			if !ok || !e.matchesBuiltin(nested) {
				return false
			}
			i = end
		}
	}
	return true
}

func validateGoCommand(e *ruleEngine, cmd parsedCommand) bool {
	if firstPositional(e.ruleMetadata(cmd.Name), cmd.Args) != "env" {
		return true
	}
	for _, tok := range cmd.Args {
		if tokenMatchesDeniedFlag(tok, "-w") || tokenMatchesDeniedFlag(tok, "-u") {
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

// validateXargsCommand approves xargs only when the nested command it would
// run is itself approved by the builtin allowlist.
//
// The validator parses xargs's own flags to locate the nested command start
// index. Any flag not in the known set causes an immediate rejection — this
// produces false negatives (safe commands require manual approval) but never
// false positives (dangerous commands auto-approved).
//
// Safety constraints:
//   - -a / --arg-file is always rejected: it reads input from a file instead
//     of the pipeline, changing the input source in a way we cannot reason about.
//   - Combined short flags (e.g. -rn10) are rejected: getopt combining semantics
//     are ambiguous in terms of which flags consume values.
//   - Unknown flags are rejected rather than assumed safe.
//
//nolint:gocyclo // xargs flag-parsing state machine; complexity is intrinsic to flag semantics
func validateXargsCommand(e *ruleEngine, cmd parsedCommand) bool {
	i := 0
	for i < len(cmd.Args) {
		tok := cmd.Args[i]

		// "--" ends options; everything after is the nested command.
		if tok == "--" {
			i++
			break
		}
		// Non-flag token: the nested command starts here.
		if !strings.HasPrefix(tok, "-") {
			break
		}

		// -a / --arg-file: reads input from a file rather than the pipeline.
		if tok == "-a" || tok == "--arg-file" || strings.HasPrefix(tok, "--arg-file=") {
			return false
		}

		// --replace[=replstr] and deprecated -i[replstr]: the replacement string
		// is optional and always inline — both forms consume exactly one token.
		if tok == "--replace" || strings.HasPrefix(tok, "--replace=") ||
			tok == "-i" || (len(tok) > 2 && tok[0] == '-' && tok[1] == 'i' && tok[2] != '-') {
			i++
			continue
		}

		// Long flags (--flag, --flag=value, or --flag value).
		if strings.HasPrefix(tok, "--") {
			if idx := strings.IndexByte(tok, '='); idx >= 0 {
				// --flag=value: inline, one token.
				if isXargsLongValueFlag(tok[:idx]) {
					i++
					continue
				}
				return false // unknown long flag with value
			}
			// --flag without "=".
			if isXargsLongValueFlag(tok) {
				if i+1 >= len(cmd.Args) {
					return false // truncated: missing value
				}
				i += 2 // value is the next token
				continue
			}
			if isXargsStandaloneFlag(tok) {
				i++
				continue
			}
			return false // unknown long flag
		}

		// Short flags.
		// A two-byte token is the flag letter alone (e.g. -n).
		// A longer token uses the first two bytes as the flag and the rest
		// as an inline value (e.g. -n10, -I{}, -P4).
		// Combined flags like -rn10 have an unknown flag letter in position 2
		// (after -r) and are rejected conservatively.
		if len(tok) < 2 {
			return false // bare "-"
		}
		flagLetter := tok[:2]
		if isXargsShortValueFlag(flagLetter) {
			if len(tok) > 2 {
				i++ // inline value (e.g. -n10)
			} else {
				if i+1 >= len(cmd.Args) {
					return false // truncated: missing value
				}
				i += 2 // separate value (e.g. -n 10)
			}
			continue
		}
		if isXargsStandaloneFlag(tok) {
			i++
			continue
		}
		return false // unknown or combined short flag
	}

	// No nested command: xargs defaults to running echo, which is safe.
	if i >= len(cmd.Args) {
		return true
	}

	nested, ok := parsedCommandFromTokens(cmd.Args[i:])
	if !ok {
		return false
	}
	return e.matchesBuiltin(nested)
}

// isXargsShortValueFlag reports whether the two-byte string (e.g. "-n") is a
// short xargs flag that takes a value as the next token or inline.
func isXargsShortValueFlag(flag string) bool {
	switch flag {
	case "-d", // --delimiter
		"-E",       // end-of-file string
		"-I",       // replacement string (see also -i handled separately)
		"-L", "-l", // --max-lines
		"-n", // --max-args
		"-P", // --max-procs
		"-s": // --max-chars
		return true
	}
	return false
}

// isXargsLongValueFlag reports whether the long flag name (without "=") takes
// a value as the next token or inline after "=".
func isXargsLongValueFlag(flag string) bool {
	switch flag {
	case "--delimiter",
		"--eof",
		"--max-lines",
		"--max-args",
		"--max-procs",
		"--max-chars":
		return true
	}
	return false
}

// isXargsStandaloneFlag reports whether the flag takes no value.
func isXargsStandaloneFlag(flag string) bool {
	switch flag {
	case "-r", "--no-run-if-empty",
		"-t", "--verbose",
		"-x", "--exit",
		"-0", "--null",
		"-p", "--interactive",
		"-o", "--open-tty",
		"--show-limits",
		"--version",
		"--help":
		return true
	}
	return false
}
