package allowlist

import (
	"slices"
	"strings"
)

func defaultValidators() map[string]validator {
	return map[string]validator{
		"bat":        validateBatCommand,
		"dasel":      validateDaselCommand,
		"date":       validateDateCommand,
		"find":       validateFindCommand,
		"go":         validateGoCommand,
		"hostname":   validateHostnameCommand,
		"nvidia-smi": validateNvidiaSmiCommand,
		"poetry":     validatePoetryCommand,
		"sysctl":     validateSysctlCommand,
		"uniq":       validateUniqCommand,
		"xargs":      validateXargsCommand,
		"xxd":        validateXxdCommand,
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
	isEnv := firstPositional(e.ruleMetadata(cmd.Name), cmd.Args) == "env"
	for _, tok := range cmd.Args {
		switch goFlagName(tok) {
		// -toolexec, -vettool and -exec name a program the go command runs
		// in place of (or around) its own tools.
		case "toolexec", "vettool", "exec":
			return false
		// go doc -http downloads and runs a documentation server and opens a
		// browser.
		case "http":
			return false
		// go env -w and -u write the user's go env file.
		case "w", "u":
			if isEnv {
				return false
			}
		}
	}
	return true
}

// goFlagName returns the name of a Go-style flag: the flag package accepts
// -name and --name alike, with an optional =value. Non-flags give "".
func goFlagName(tok string) string {
	if len(tok) < 2 || tok[0] != '-' || tok == "--" {
		return ""
	}
	name := strings.TrimPrefix(strings.TrimPrefix(tok, "-"), "-")
	name, _, _ = strings.Cut(name, "=")
	return name
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
// run is itself approved by the builtin allowlist and is argument-proof (see
// argumentProof).
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
	return e.argumentProof(nested.Name) && e.matchesBuiltin(nested)
}

// argumentProof reports whether a command is safe whatever arguments it is
// given: its rule has no denied flags, no validator and no subcommand list.
//
// xargs appends words read from stdin at run time (or splices them in with
// -I), so echo -delete | xargs find . runs find . -delete. The words a command
// line shows are not the words the nested command gets, so only a command no
// argument can make unsafe may run under xargs.
func (e *ruleEngine) argumentProof(name string) bool {
	for _, rule := range e.builtinRules {
		if rule.Cmd != name {
			continue
		}
		return len(rule.Deny.Flags) == 0 && len(rule.Allow.Subcommands) == 0 && e.validators[name] == nil
	}
	return false
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

// validateBatCommand rejects bat cache, whose --build and --clear rewrite or
// delete bat's syntax cache. bat only treats cache as a subcommand in first
// place, but rejecting the word anywhere keeps this simple; a file named
// cache costs one prompt.
func validateBatCommand(_ *ruleEngine, cmd parsedCommand) bool {
	return !slices.Contains(cmd.Args, "cache")
}

// validateDaselCommand rejects dasel's write subcommands. cobra finds a
// subcommand after root flags whose values it knows and this does not, so
// the words are rejected wherever they appear.
func validateDaselCommand(_ *ruleEngine, cmd parsedCommand) bool {
	for _, tok := range cmd.Args {
		if tok == "put" || tok == "delete" {
			return false
		}
	}
	return true
}

// validatePoetryCommand keeps poetry version read-only: with an argument
// (patch, 1.2.3, ...) it rewrites the version in pyproject.toml.
func validatePoetryCommand(e *ruleEngine, cmd parsedCommand) bool {
	i := subcommandIndex(e.ruleMetadata(cmd.Name), cmd.Args)
	if i < 0 || cmd.Args[i] != "version" {
		return true
	}
	for _, tok := range cmd.Args[i+1:] {
		if !strings.HasPrefix(tok, "-") || tok == "-" {
			return false
		}
	}
	return true
}

// validateXxdCommand rejects a second operand, which xxd writes its output
// to (a second operand of "-" is stdout). xxd stops reading options at the
// first operand, so every word after it is an operand, dashes or not.
func validateXxdCommand(_ *ruleEngine, cmd parsedCommand) bool {
	var operands []string
	for i := 0; i < len(cmd.Args); i++ {
		tok := cmd.Args[i]
		if tok == "--" {
			operands = append(operands, cmd.Args[i+1:]...)
			break
		}
		if len(tok) < 2 || tok[0] != '-' {
			operands = append(operands, cmd.Args[i:]...)
			break
		}
		// xxd reads --flag as -flag. Only a flag written exactly takes the
		// next word as its value; when unsure, the word counts as an operand,
		// which can only cost a prompt.
		word := tok
		if strings.HasPrefix(word, "--") {
			word = word[1:]
		}
		switch word {
		case "-c", "-cols", "-g", "-groupsize", "-l", "-len", "-n", "-name", "-o", "-offset", "-s", "-seek", "-R":
			i++
		}
	}
	return len(operands) < 2 || (len(operands) == 2 && operands[1] == "-")
}

// validateUniqCommand rejects a second operand, which uniq writes its output
// to. Only exact value flags skip their value; anything unsure counts as an
// operand.
func validateUniqCommand(_ *ruleEngine, cmd parsedCommand) bool {
	operands := 0
	for i := 0; i < len(cmd.Args); i++ {
		tok := cmd.Args[i]
		if tok == "--" {
			operands += len(cmd.Args) - i - 1
			break
		}
		if len(tok) < 2 || tok[0] != '-' {
			operands++
			continue
		}
		switch tok {
		case "-f", "-s", "-w", "--skip-fields", "--skip-chars", "--check-chars":
			i++
		}
	}
	return operands < 2
}

// validateDateCommand rejects setting the clock: -s/--set, or an operand that
// is not a +FORMAT, which GNU and BSD date both take as the new time (BSD
// date -j only parses it, so operands are fine there).
//
// Short flags are scanned as getopt bundles. d, f, r, v and z take a value
// (the rest of the bundle, or the next word); I takes only an attached one,
// so -Iseconds does not hide an s.
func validateDateCommand(_ *ruleEngine, cmd parsedCommand) bool {
	noSet := false
	var operands []string
	for i := 0; i < len(cmd.Args); i++ {
		tok := cmd.Args[i]
		if tok == "--" {
			operands = append(operands, cmd.Args[i+1:]...)
			break
		}
		if len(tok) < 2 || tok[0] != '-' {
			operands = append(operands, tok)
			continue
		}
		if strings.HasPrefix(tok, "--") {
			if tokenMatchesDeniedFlag(tok, "--set") {
				return false
			}
			switch tok {
			case "--date", "--file", "--reference":
				i++
			}
			continue
		}
	scan:
		for j := 1; j < len(tok); j++ {
			switch tok[j] {
			case 's':
				return false
			case 'j':
				noSet = true
			case 'I':
				break scan
			case 'd', 'f', 'r', 'v', 'z':
				if j == len(tok)-1 {
					i++
				}
				break scan
			}
		}
	}
	if noSet {
		return true
	}
	for _, op := range operands {
		if !strings.HasPrefix(op, "+") {
			return false
		}
	}
	return true
}

// validateHostnameCommand allows only flags that print a name or address.
// An operand, -F/--file or -b/--boot sets the hostname, so anything not
// known to read fails closed.
func validateHostnameCommand(_ *ruleEngine, cmd parsedCommand) bool {
	for _, tok := range cmd.Args {
		switch tok {
		case "-s", "--short", "-f", "--fqdn", "--long", "-d", "--domain",
			"-i", "--ip-address", "-I", "--all-ip-addresses", "-A", "--all-fqdns",
			"-a", "--alias", "-y", "--yp", "--nis", "-v", "--verbose",
			"-h", "--help", "-V", "--version":
		default:
			return false
		}
	}
	return true
}

// validateNvidiaSmiCommand allows only nvidia-smi's query forms. Most of its
// other flags (-r, -pm, -pl, -ac, -lgc, -c, -e, -mig, ...) and subcommands
// change GPU state, and -f writes a log file, so anything not known to read
// fails closed.
func validateNvidiaSmiCommand(_ *ruleEngine, cmd parsedCommand) bool {
	args := cmd.Args
	if len(args) > 0 && args[0] == "topo" {
		for _, tok := range args[1:] {
			if tok != "-m" && tok != "--matrix" {
				return false
			}
		}
		return true
	}
	for i := 0; i < len(args); i++ {
		tok := args[i]
		name, _, hasValue := strings.Cut(tok, "=")
		switch name {
		case "-L", "--list-gpus", "-q", "--query", "-x", "--xml-format",
			"-u", "--unit", "-h", "--help", "--version":
			if hasValue {
				return false
			}
		case "-d", "--display", "-i", "--id", "-l", "--loop", "-lms", "--loop-ms",
			"--format", "--query-gpu", "--query-compute-apps", "--query-accounted-apps",
			"--query-retired-pages", "--query-remapped-rows", "--query-supported-clocks":
			if !hasValue {
				if i+1 >= len(args) || strings.HasPrefix(args[i+1], "-") {
					return false
				}
				i++
			}
		default:
			return false
		}
	}
	return true
}
