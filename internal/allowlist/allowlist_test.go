package allowlist

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// matchesRule drives ruleReason the way builtinReason does, so this table keeps
// covering rule matching now that the exported Matches wrapper is gone.
func matchesRule(t *testing.T, rule Rule, command string) bool {
	t.Helper()
	e := engine()
	segments, err := e.parseCommand(command)
	if err != nil || len(segments) != 1 {
		return false
	}
	return e.ruleReason(rule, segments[0]) == ""
}

func TestMatchRuleSupportsFlagStyleAndPositionalSubcommands(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		rule    Rule
		command string
		want    bool
	}{
		{
			name:    "flag style subcommand",
			rule:    Rule{Cmd: "brew", Allow: AllowRule{Subcommands: []string{"--version"}}},
			command: "brew --version",
			want:    true,
		},
		{
			name:    "short flag subcommand",
			rule:    Rule{Cmd: "dpkg", Allow: AllowRule{Subcommands: []string{"-l"}}},
			command: "dpkg -l",
			want:    true,
		},
		{
			name:    "positional subcommand after option",
			rule:    Rule{Cmd: "git", Allow: AllowRule{Subcommands: []string{"status"}}},
			command: "git --no-pager status",
			want:    true,
		},
		{
			name:    "positional subcommand after option with value",
			rule:    Rule{Cmd: "git", Allow: AllowRule{Subcommands: []string{"status"}}},
			command: "git -C /tmp status",
			want:    true,
		},
		{
			name:    "docker context before subcommand",
			rule:    Rule{Cmd: "docker", Allow: AllowRule{Subcommands: []string{"ps"}}},
			command: "docker --context prod ps",
			want:    true,
		},
		{
			name:    "literal env assignment before command",
			rule:    Rule{Cmd: "rg"},
			command: "LC_ALL=C rg termwise README.md",
			want:    true,
		},
		{
			name:    "find exec ls is allowed",
			rule:    Rule{Cmd: "find"},
			command: `find . -type f -exec ls -la {} \;`,
			want:    true,
		},
		{
			name:    "sysctl read-only args are allowed",
			rule:    Rule{Cmd: "sysctl"},
			command: `sysctl -n hw.memsize hw.ncpu hw.model`,
			want:    true,
		},
		{
			name:    "blocked flag still denies",
			rule:    Rule{Cmd: "sed", Deny: DenyRule{Flags: []string{"-i"}}},
			command: "sed -i s/a/b/ file.txt",
			want:    false,
		},
		{
			name:    "blocked short flag with attached value denies",
			rule:    Rule{Cmd: "poetry", Deny: DenyRule{Flags: []string{"-o"}}},
			command: "poetry export -orequirements.txt",
			want:    false,
		},
		{
			name:    "blocked long flag with equals denies",
			rule:    Rule{Cmd: "poetry", Deny: DenyRule{Flags: []string{"--output"}}},
			command: "poetry export --output=requirements.txt",
			want:    false,
		},
		{
			name:    "blocked exec short flag denies",
			rule:    Rule{Cmd: "fd", Deny: DenyRule{Flags: []string{"-x"}}},
			command: "fd -x echo {}",
			want:    false,
		},
		{
			name:    "sysctl assignment style denies",
			rule:    Rule{Cmd: "sysctl"},
			command: `sysctl kern.maxfiles=10`,
			want:    false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := matchesRule(t, tc.rule, tc.command); got != tc.want {
				t.Fatalf("ruleReason(%q) == \"\" is %v, want %v", tc.command, got, tc.want)
			}
		})
	}
}

func TestNewRuleEngineClonesRules(t *testing.T) {
	t.Parallel()

	rules := []Rule{{
		Cmd:   "echo",
		Parse: ParseRule{SubcommandValueFlags: []string{"--context"}, BoolFlags: []string{"--debug"}},
		Deny:  DenyRule{Flags: []string{"-f"}},
	}}
	engine := newRuleEngine(rules)
	rules[0].Cmd = "rm"
	rules[0].Deny.Flags[0] = "-x"
	rules[0].Parse.SubcommandValueFlags[0] = "--project"
	rules[0].Parse.BoolFlags[0] = "--verbose"

	cloned := engine.ruleMetadata("echo")
	if cloned.Cmd != "echo" || cloned.Deny.Flags[0] != "-f" || cloned.Parse.SubcommandValueFlags[0] != "--context" || cloned.Parse.BoolFlags[0] != "--debug" {
		t.Fatalf("expected deep-cloned rule metadata, got %#v", cloned)
	}
}

func TestNeedsApprovalStructuredShellParsing(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		command string
		want    bool
	}{
		{
			name:    "quoted pipe stays safe",
			command: `rg "foo|bar" README.md`,
			want:    false,
		},
		{
			name:    "literal env assignment stays safe",
			command: `LC_ALL=C rg termwise README.md`,
			want:    false,
		},
		{
			name:    "cd and ls stays safe",
			command: `cd /Users/dev/Projects/termwise && ls -la`,
			want:    false,
		},
		{
			name:    "cd and grep pipeline stays safe",
			command: `cd /Users/dev/Projects/termwise && grep -r "glamour\|goldmark" --include="*.go" internal/ | head -10`,
			want:    false,
		},
		{
			name:    "read only or chain stays safe",
			command: `nvidia-smi 2>/dev/null || echo "No NVIDIA GPU detected"`,
			want:    false,
		},
		{
			name:    "sysctl df and fallback echo stays safe",
			command: `sysctl -n hw.memsize hw.ncpu hw.model && df -h / && nvidia-smi 2>/dev/null || echo "No NVIDIA GPU detected"`,
			want:    false,
		},
		{
			name:    "find exec ls pipeline stays safe",
			command: `find . -type f -exec ls -la {} \; | sort -k5 -nr | head -10`,
			want:    false,
		},
		{
			name:    "quoted semicolon stays safe",
			command: `printf ';'`,
			want:    false,
		},
		{
			name:    "pure pipeline stays safe",
			command: `printf "ok\n" | wc -c`,
			want:    false,
		},
		{
			name:    "null redirect with spaces stays safe",
			command: `rg termwise README.md > /dev/null`,
			want:    false,
		},
		{
			name:    "stderr null redirect stays safe",
			command: `git status 2>/dev/null`,
			want:    false,
		},
		{
			name:    "dev null input redirect stays safe",
			command: `cat </dev/null`,
			want:    false,
		},
		{
			name:    "tmp file redirect stays safe",
			command: `printf ok > /tmp/termwise-test.txt`,
			want:    false,
		},
		{
			name:    "tmp file append stays safe",
			command: `printf ok >> /tmp/termwise-test.txt`,
			want:    false,
		},
		{
			name:    "quoted tmp path stays safe",
			command: `printf ok > "/tmp/termwise test.txt"`,
			want:    false,
		},
		{
			name:    "tmp file redirect with stderr dup stays safe",
			command: `printf ok > /tmp/termwise-test.txt 2>&1`,
			want:    false,
		},
		{
			name:    "stderr dup before tmp redirect stays safe",
			command: `printf ok 2>&1 > /tmp/termwise-test.txt`,
			want:    false,
		},
		{
			name:    "escaped command substitution requires approval",
			command: `printf \$(whoami)`,
			want:    true,
		},
		{
			name:    "dynamic env assignment needs approval",
			command: `FOO=$(whoami) rg termwise README.md`,
			want:    true,
		},
		{
			name:    "process substitution needs approval",
			command: `cat <(pwd)`,
			want:    true,
		},
		{
			name:    "multiple statements need approval",
			command: "pwd\nwhoami",
			want:    true,
		},
		{
			name:    "logical and needs approval",
			command: `git status && pwd`,
			want:    false,
		},
		{
			name:    "git option value before subcommand stays safe",
			command: `git -C /tmp status`,
			want:    false,
		},
		{
			name:    "git option value matching subcommand still needs approval",
			command: `git -C status branch`,
			want:    true,
		},
		{
			name:    "go option value before env subcommand stays safe",
			command: `go -C . env GOMOD`,
			want:    false,
		},
		{
			name:    "kubectl context option before subcommand stays safe",
			command: `kubectl --context prod get pods`,
			want:    false,
		},
		{
			name:    "docker context option before subcommand stays safe",
			command: `docker --context prod ps`,
			want:    false,
		},
		{
			name:    "npm prefix option before subcommand stays safe",
			command: `npm --prefix /tmp ls`,
			want:    false,
		},
		{
			name:    "cargo toolchain prefix before subcommand stays safe",
			command: `cargo +nightly tree`,
			want:    true,
		},
		{
			name:    "file redirect needs approval",
			command: `echo ok > out.txt`,
			want:    true,
		},
		{
			name:    "cd and rm needs approval",
			command: `cd /tmp && rm -rf nope`,
			want:    true,
		},
		{
			name:    "find exec shell needs approval",
			command: `find . -type f -exec sh -c 'echo hi' \;`,
			want:    true,
		},
		{
			name:    "find execdir still needs approval",
			command: `find . -type f -execdir ls -la {} \;`,
			want:    true,
		},
		// xargs: safe patterns
		{
			name:    "xargs grep stays safe",
			command: `find . -name "*.go" -type f | xargs grep -l "pattern"`,
			want:    false,
		},
		{
			name:    "xargs grep original failing command stays safe",
			command: `find /Users/dev/Projects/termwise -name "*.go" -type f | xargs grep -l "WriteFile||os.Create" 2>/dev/null | head -5`,
			want:    false,
		},
		{
			name:    "xargs with no command stays safe",
			command: `find . -name "*.log" | xargs`,
			want:    false,
		},
		{
			name:    "xargs -0 stays safe",
			command: `find . -name "*.go" -print0 | xargs -0 grep pattern`,
			want:    false,
		},
		{
			name:    "xargs -n stays safe",
			command: `find . -type f | xargs -n 10 wc -l`,
			want:    false,
		},
		{
			name:    "xargs -n inline stays safe",
			command: `find . -type f | xargs -n10 wc -l`,
			want:    false,
		},
		{
			name:    "xargs -P stays safe",
			command: `find . -name "*.go" | xargs -P 4 grep pattern`,
			want:    false,
		},
		{
			name:    "xargs -I stays safe",
			command: `find . -name "*.go" | xargs -I {} grep pattern {}`,
			want:    false,
		},
		{
			name:    "xargs -I inline stays safe",
			command: `find . -name "*.go" | xargs -I{} grep pattern {}`,
			want:    false,
		},
		{
			name:    "xargs -r stays safe",
			command: `find . -name "*.go" | xargs -r grep pattern`,
			want:    false,
		},
		{
			name:    "xargs --null stays safe",
			command: `find . -name "*.go" -print0 | xargs --null grep pattern`,
			want:    false,
		},
		// xargs: unsafe patterns
		{
			name:    "xargs rm needs approval",
			command: `find . -name "*.tmp" | xargs rm -rf`,
			want:    true,
		},
		{
			name:    "xargs sh needs approval",
			command: `find . -type f | xargs sh -c 'echo hi'`,
			want:    true,
		},
		{
			name:    "xargs -a needs approval",
			command: `xargs -a /etc/passwd grep root`,
			want:    true,
		},
		{
			name:    "xargs unknown flag needs approval",
			command: `find . | xargs -z grep pattern`,
			want:    true,
		},
		{
			name:    "xargs combined flags needs approval",
			command: `find . | xargs -rn10 grep pattern`,
			want:    true,
		},
		{
			name:    "sysctl write flag needs approval",
			command: `sysctl -w kern.maxfiles=10`,
			want:    true,
		},
		{
			name:    "sysctl load file needs approval",
			command: `sysctl -p /etc/sysctl.conf`,
			want:    true,
		},
		{
			name:    "sysctl system flag needs approval",
			command: `sysctl --system`,
			want:    true,
		},
		{
			name:    "find fprint needs approval",
			command: `find . -type f -fprint /tmp/out.txt`,
			want:    true,
		},
		{
			name:    "tree output file needs approval",
			command: `tree -o report.txt`,
			want:    true,
		},
		{
			name:    "bat pager needs approval",
			command: `bat --pager="cat" README.md`,
			want:    true,
		},
		{
			name:    "less output log needs approval",
			command: `less -o session.log README.md`,
			want:    true,
		},
		{
			name:    "xxd reverse mode needs approval",
			command: `xxd -r dump.hex`,
			want:    true,
		},
		{
			name:    "rg preprocessor needs approval",
			command: `rg --pre cat needle README.md`,
			want:    true,
		},
		{
			name:    "fd exec needs approval",
			command: `fd -x echo {} \;`,
			want:    true,
		},
		{
			name:    "sort output file needs approval",
			command: `sort -o out.txt input.txt`,
			want:    true,
		},
		{
			name:    "diff output file needs approval",
			command: `diff --output=out.patch a.txt b.txt`,
			want:    true,
		},
		{
			name:    "delta pager needs approval",
			command: `delta --pager="cat"`,
			want:    true,
		},
		{
			name:    "xsv needs approval",
			command: `xsv index data.csv`,
			want:    true,
		},
		{
			name:    "sed needs approval",
			command: `sed 's/a/b/' README.md`,
			want:    true,
		},
		{
			name:    "go build needs approval",
			command: `go build ./...`,
			want:    true,
		},
		{
			name:    "cargo check needs approval",
			command: `cargo check`,
			want:    true,
		},
		{
			name:    "go env write flag needs approval",
			command: `go env -w GOPROXY=https://proxy.golang.org,direct`,
			want:    true,
		},
		{
			name:    "go env unset flag needs approval",
			command: `go env -u GOPROXY`,
			want:    true,
		},
		{
			name:    "go env option before subcommand write flag needs approval",
			command: `go -C . env -w GOPROXY=https://proxy.golang.org,direct`,
			want:    true,
		},
		{
			name:    "git remote add needs approval",
			command: `git remote add origin https://example.com/repo.git`,
			want:    true,
		},
		{
			name:    "git config env override needs approval",
			command: `git -c core.pager=cat status`,
			want:    true,
		},
		{
			name:    "git branch move needs approval",
			command: `git branch -m main trunk`,
			want:    true,
		},
		{
			name:    "git tag create needs approval",
			command: `git tag v1.2.3`,
			want:    true,
		},
		{
			name:    "git reflog expire needs approval",
			command: `git reflog expire --expire=now --all`,
			want:    true,
		},
		{
			name:    "uv pip install needs approval",
			command: `uv pip install requests`,
			want:    true,
		},
		{
			name:    "npm audit fix needs approval",
			command: `npm audit fix`,
			want:    true,
		},
		{
			name:    "npm version needs approval",
			command: `npm version patch`,
			want:    true,
		},
		{
			name:    "pnpm audit fix needs approval",
			command: `pnpm audit --fix`,
			want:    true,
		},
		{
			name:    "yarn version needs approval",
			command: `yarn version`,
			want:    true,
		},
		{
			name:    "bun repl needs approval",
			command: `bun repl`,
			want:    true,
		},
		{
			name:    "bun pm add needs approval",
			command: `bun pm add react`,
			want:    true,
		},
		{
			name:    "gh repo create needs approval",
			command: `gh repo create demo --public`,
			want:    true,
		},
		{
			name:    "gh api post needs approval",
			command: `gh api -X POST /user/repos`,
			want:    true,
		},
		{
			name:    "poetry export output file needs approval",
			command: `poetry export -o requirements.txt`,
			want:    true,
		},
		{
			name:    "poetry export output equals flag needs approval",
			command: `poetry export --output=requirements.txt`,
			want:    true,
		},
		{
			name:    "non dev null input redirect needs approval",
			command: `cat < README.md`,
			want:    true,
		},
		{
			name:    "non stdin input redirect fd needs approval",
			command: `cat 1</dev/null`,
			want:    true,
		},
		{
			name:    "arbitrary output fd duplication needs approval",
			command: `printf ok 3>&1`,
			want:    true,
		},
		{
			name:    "dup to arbitrary target fd needs approval",
			command: `printf ok 2>&3`,
			want:    true,
		},
		{
			name:    "path traversal out of tmp needs approval",
			command: `echo ok > /tmp/../etc/passwd`,
			want:    true,
		},
		{
			name:    "quoted path traversal out of tmp needs approval",
			command: `echo ok > "/tmp/../../etc/passwd"`,
			want:    true,
		},
		{
			name:    "home expansion target needs approval",
			command: `echo ok > ~/tmp/out.txt`,
			want:    true,
		},
		{
			name:    "glob temp target needs approval",
			command: `echo ok > /tmp/*`,
			want:    true,
		},
		{
			name:    "command substitution needs approval",
			command: `echo $(whoami)`,
			want:    true,
		},
		{
			name:    "backticks need approval",
			command: "echo `whoami`",
			want:    true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := NeedsApproval(tc.command); got != tc.want {
				t.Fatalf("NeedsApproval(%q) = %v, want %v", tc.command, got, tc.want)
			}
			// The approval prompt shows ApprovalReason, so it has to agree
			// with the decision on every case, not just the handful below.
			if reason := ApprovalReason(tc.command); (reason != "") != tc.want {
				t.Fatalf("ApprovalReason(%q) = %q, but NeedsApproval wants %v", tc.command, reason, tc.want)
			}
		})
	}
}

func TestNeedsApprovalRejectsSymlinkEscapeFromTempDir(t *testing.T) {
	t.Parallel()

	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	tmpDir := t.TempDir()
	link := filepath.Join(tmpDir, "escape-dir")
	if err := os.Symlink(wd, link); err != nil {
		t.Fatalf("symlink dir: %v", err)
	}

	command := `printf ok > "` + filepath.Join(link, "out.txt") + `"`
	if !NeedsApproval(command) {
		t.Fatalf("expected symlinked temp directory redirect to require approval: %q", command)
	}
}

func TestNeedsApprovalRejectsSymlinkedTempFile(t *testing.T) {
	t.Parallel()

	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	tmpDir := t.TempDir()
	link := filepath.Join(tmpDir, "escape-file")
	target := filepath.Join(wd, "README.md")
	if err := os.Symlink(target, link); err != nil {
		t.Fatalf("symlink file: %v", err)
	}

	command := `printf ok > "` + link + `"`
	if !NeedsApproval(command) {
		t.Fatalf("expected symlinked temp file redirect to require approval: %q", command)
	}
}

func TestApprovalReasonNamesTheOffendingCommand(t *testing.T) {
	cases := map[string]string{
		"git commit -m x": "git",
		"rm -rf /tmp/x":   "rm",
		"ls -la":          "",
		"git status":      "",
		"":                "the command is empty",
		"ls && git push":  "git",
		"ls; rm x":        "it is more than one statement",
	}
	for cmd, want := range cases {
		got := ApprovalReason(cmd)
		if want == "" {
			if got != "" {
				t.Errorf("ApprovalReason(%q) = %q, want no reason", cmd, got)
			}
			continue
		}
		if !strings.Contains(got, want) {
			t.Errorf("ApprovalReason(%q) = %q, want it to mention %q", cmd, got, want)
		}
	}
}

// Reason and decision must never disagree.
func TestApprovalReasonAgreesWithNeedsApproval(t *testing.T) {
	for _, cmd := range []string{
		"ls", "git status", "cat go.mod", "grep -rn x .",
		"git commit -m x", "rm file", "curl https://x", "npm install", "",
	} {
		needs := NeedsApproval(cmd)
		reason := ApprovalReason(cmd)
		if needs != (reason != "") {
			t.Errorf("%q: NeedsApproval=%v but reason=%q", cmd, needs, reason)
		}
	}
}

// Commands that would let a model run or write something without asking.
// Each was auto-approved before the fix named in its case.
func TestNeedsApprovalBlocksIndirectExecution(t *testing.T) {
	t.Parallel()

	needApproval := []string{
		// Environment variables that make an allowed command run something else.
		`GIT_EXTERNAL_DIFF=/tmp/x git diff`,
		`GIT_PAGER=/tmp/x git log`,
		`PAGER=/tmp/x git log`,
		`LESSOPEN='|/tmp/x %s' less README.md`,
		`LD_PRELOAD=/tmp/x.so ls`,
		`DYLD_INSERT_LIBRARIES=/tmp/x.dylib ls`,
		`BASH_ENV=/tmp/x ls`,
		`PATH=/tmp:/usr/bin ls`,
		`EDITOR=/tmp/x git log`,
		// A path names a different binary than the allowlisted one.
		`./ls`,
		`/tmp/x/cat README.md`,
		`bin/grep x .`,
		`ls | ./cat`,
		`xargs ./cat < files.txt`,
		// Denied short flags hidden in a bundle, or long flags abbreviated.
		`sort -uo /tmp/out file`,
		`sort -uo/tmp/out file`,
		`fd -Hx rm`,
		`xxd -pr dump bin`,
		`sort --out=/tmp/out file`,
		`sort --compress-program=/tmp/x file`,
		`sort --compress=/tmp/x file`,
		// git options that run helpers or write files.
		`git --exec-path=/tmp status`,
		`git diff --output=/tmp/out`,
		`git diff --ext-diff`,
		`git log --output /tmp/out`,
	}
	for _, cmd := range needApproval {
		if !NeedsApproval(cmd) {
			t.Errorf("NeedsApproval(%q) = false, want true", cmd)
		}
	}

	// Everyday read-only forms the fixes must not start prompting for.
	staySafe := []string{
		`LC_ALL=C rg termwise README.md`,
		`LANG=C.UTF-8 ls`,
		`TZ=UTC git log -1`,
		`NO_COLOR=1 git diff`,
		`COLUMNS=200 ls`,
		`sort -u file`,
		`sort -rn -k2 file`,
		`fd -t f main`,
		`less -R README.md`,
		`git log --oneline -n 5`,
		`git diff --stat`,
		`git status -sb`,
		`ls -la /tmp`,
	}
	for _, cmd := range staySafe {
		if NeedsApproval(cmd) {
			t.Errorf("NeedsApproval(%q) = true, want false (%s)", cmd, ApprovalReason(cmd))
		}
	}
}

// The rules are embedded, so a malformed file is a build defect. It must
// never reach users as an empty rule set.
func TestBuiltinRulesLoad(t *testing.T) {
	rules, err := parseRules(builtinAllowlistYAML)
	if err != nil {
		t.Fatalf("embedded allowlist.yaml does not parse: %v", err)
	}
	if len(rules) < 50 {
		t.Fatalf("embedded allowlist.yaml has %d rules; expected the full catalog", len(rules))
	}
}

func requireApproval(t *testing.T, cmds ...string) {
	t.Helper()
	for _, cmd := range cmds {
		if !NeedsApproval(cmd) {
			t.Errorf("NeedsApproval(%q) = false, want true", cmd)
		}
		if ApprovalReason(cmd) == "" {
			t.Errorf("ApprovalReason(%q) is empty, but the command needs approval", cmd)
		}
	}
}

func requireSafe(t *testing.T, cmds ...string) {
	t.Helper()
	for _, cmd := range cmds {
		if NeedsApproval(cmd) {
			t.Errorf("NeedsApproval(%q) = true, want false (%s)", cmd, ApprovalReason(cmd))
		}
	}
}

// Go's flag package takes -flag and --flag alike, with or without =value.
func TestGoFlagsMatchInEveryDashForm(t *testing.T) {
	t.Parallel()
	requireApproval(t,
		`go env --w GOFLAGS=x`,
		`go env --u GOPATH`,
		`go env -w=true GOFLAGS=x`,
		`go env --w=true GOFLAGS=x`,
		`go list -export -toolexec x ./...`,
		`go list --toolexec=x ./...`,
		`go vet -vettool=/tmp/v ./...`,
		`go vet --vettool /tmp/v ./...`,
		`go list -exec x ./...`,
		`go doc -http`,
		`go doc --http=:8080 fmt`,
	)
	requireSafe(t,
		`go env GOPATH`,
		`go env -json`,
		`go env --json GOMOD`,
		`go list -m all`,
		`go list -f '{{.Dir}}' ./...`,
		`go vet ./...`,
		`go version`,
		`go doc fmt.Println`,
	)
}

// A global flag that takes a value, but that the rule does not know about,
// makes its value look like the subcommand. Unknown flags before the
// subcommand must fail closed.
func TestUnknownGlobalFlagHidesSubcommand(t *testing.T) {
	t.Parallel()
	requireApproval(t,
		`git --attr-source status reset --hard`,
		`kubectl --as get delete pod web`,
		`docker --tlscacert ps rm -f web`,
		`npm --loglevel ls install evil`,
		`helm --burst-limit list uninstall web`,
		`cargo -Z tree install evil`,
		`pip --cache-dir list install evil`,
		`docker-compose --ansi ps down`,
		`go --C list build`,
		`git - status`,
	)
	requireSafe(t,
		`git --no-pager status`,
		`git --no-pager log --oneline -5`,
		`git -P diff`,
		`git --no-optional-locks status`,
		`git --attr-source=HEAD status`,
		`git -C /tmp status`,
		`kubectl --context prod get pods`,
		`kubectl --namespace=kube-system get pods`,
		`kubectl -n kube-system get pods`,
		`kubectl get pods --as admin`,
		`docker --context prod ps`,
		`docker --debug ps`,
		`npm --prefix /tmp ls`,
		`npm -g ls --depth=0`,
		`npm ls --loglevel silent`,
		`pnpm -r ls`,
		`cargo --locked metadata`,
		`cargo -q tree`,
		`pip -q list`,
		`pip --disable-pip-version-check list`,
		`ruff -q check .`,
		`dpkg -l`,
		`brew --version`,
		`rpm -qa`,
		`pacman -Qi bash`,
	)
}

// xargs appends words read at run time, so a denied flag can arrive on stdin.
// Only commands that are safe whatever their arguments may run under it.
func TestXargsOnlyRunsArgumentProofCommands(t *testing.T) {
	t.Parallel()
	requireApproval(t,
		`echo -delete | xargs find .`,
		`echo --output=/tmp/x | xargs git diff`,
		`echo --pre=/tmp/x | xargs rg needle`,
		`echo -o /tmp/x | xargs sort`,
		`echo -w kern.x=1 | xargs sysctl`,
		`echo GOFLAGS=x | xargs go env -w`,
		`echo x | xargs git status`,
		`echo x | xargs xargs cat`,
		`ls | xargs -I{} find {} -delete`,
	)
	requireSafe(t,
		`find . -name "*.go" | xargs grep -l pattern`,
		`find . -type f | xargs wc -l`,
		`find . -type f | xargs cat`,
		`find . -type f | xargs ls -la`,
		`find . -type f | xargs head -5`,
		`find . -type f -print0 | xargs -0 stat`,
		`printf 'a\nb\n' | xargs echo`,
	)
}

// Flags that load a config file able to name a program the tool runs
// (kubeconfig exec credential plugins, docker credential helpers, cargo's
// build.rustc) or that name the program directly.
func TestConfigFlagsThatRunProgramsNeedApproval(t *testing.T) {
	t.Parallel()
	requireApproval(t,
		`kubectl --kubeconfig /tmp/kc get pods`,
		`kubectl --kubeconfig=/tmp/kc get pods`,
		`kubectl get pods --kubeconfig /tmp/kc`,
		`helm --kubeconfig /tmp/kc list`,
		`helm list --kubeconfig=/tmp/kc`,
		`helm --registry-config /tmp/r.json show chart oci://x/y`,
		`helm show chart oci://x/y --registry-config=/tmp/r.json`,
		`docker --config /tmp/d ps`,
		`docker --config=/tmp/d ps`,
		`cargo --config build.rustc="/tmp/x" metadata`,
		`cargo metadata --config=build.rustc="/tmp/x"`,
		`pip --python /tmp/x list`,
		`pip3 list --python=/tmp/x`,
		`uv tree --python /tmp/x`,
		`uv --python=/tmp/x tree`,
		`podman --runtime=/tmp/x ps`,
		`podman --conmon=/tmp/x ps`,
		`podman --storage-opt=overlay.mount_program=/tmp/x ps`,
		`podman --module=/tmp/x.conf ps`,
	)
	requireSafe(t,
		`kubectl --context prod get pods`,
		`helm list -n web`,
		`helm --kube-context prod status web`,
		`docker --context prod ps`,
		`cargo metadata --format-version 1`,
		`pip list --outdated`,
		`uv tree --depth 1`,
		`podman ps -a`,
	)
}

// Flags and arguments that make an allowed tool write files or change system
// state.
func TestWritingFlagsOnAllowedToolsNeedApproval(t *testing.T) {
	t.Parallel()
	requireApproval(t,
		`ruff check --fix .`,
		`ruff check . --fix`,
		`ruff check --fix-only .`,
		`ruff check --unsafe-fixes --fix .`,
		`ruff check --add-noqa .`,
		`ruff check --output-file=/tmp/r .`,
		`ruff check -o /tmp/r .`,
		`poetry version patch`,
		`poetry version 1.2.3`,
		`poetry version --dry-run minor`,
		`poetry -q version major`,
		`tomlq -i '.a = 1' pyproject.toml`,
		`tomlq --in-place '.a = 1' pyproject.toml`,
		`yq -i '.a = 1' x.yaml`,
		`yq --inplace '.a = 1' x.yaml`,
		`yq --in-place '.a = 1' x.yaml`,
		`yq --split-exp '.name' x.yaml`,
		`yq -s '.name' x.yaml`,
		`yq --split-exp-file f x.yaml`,
		`dasel put -f x.json -t string -v v '.a'`,
		`dasel -f x.json put -t string -v v '.a'`,
		`dasel delete -f x.json '.a'`,
		`dasel -f x.json -o /tmp/out '.a'`,
		`less --log-file=/tmp/l README.md`,
		`less --LOG-FILE /tmp/l README.md`,
		`less -O /tmp/l README.md`,
		`less -o/tmp/l README.md`,
		`less --lesskey-src=/tmp/k README.md`,
		`less -k /tmp/k README.md`,
		`xxd in.bin out.hex`,
		`xxd -g 4 in.bin out.hex`,
		`xxd in.bin -r`,
		`xxd - out.hex`,
		`xxd -- in.bin out.hex`,
		`pacman --sync vim`,
		`pacman -S vim`,
		`pacman -Si vim -y`,
		`pacman -Ss vim --refresh`,
		`pacman -Si vim -u`,
		`pacman -Si vim -c`,
		`pacman -Qi bash --config /tmp/p.conf`,
		`rpm -q --eval '%(touch /tmp/x)' bash`,
		`rpm -q -E '%(id)' bash`,
		`rpm -q --define '_x %(id)' bash`,
		`rpm -qa --pipe 'sh -c id'`,
		`rpm -q --macros /tmp/m bash`,
		`nvidia-smi --gpu-reset`,
		`nvidia-smi -r`,
		`nvidia-smi -pm 1`,
		`nvidia-smi -pl 200`,
		`nvidia-smi -ac 5001,1590`,
		`nvidia-smi -rac`,
		`nvidia-smi -lgc 300,1500`,
		`nvidia-smi --persistence-mode=1`,
		`nvidia-smi -f /tmp/log -q`,
		`nvidia-smi mig -cgi 9`,
		`nvidia-smi -i 0 -c EXCLUSIVE_PROCESS`,
		`bat cache --clear`,
		`bat cache --build`,
		`bat cache -b`,
		`date -s '2020-01-01'`,
		`date --set='2020-01-01'`,
		`date --se='2020-01-01'`,
		`date -us '2020-01-01'`,
		`date 010112002020`,
		`date -u 010112002020`,
		`hostname evil`,
		`hostname -s evil`,
		`hostname -F /tmp/name`,
		`hostname --file=/tmp/name`,
		`rg --hostname-bin=/tmp/x needle`,
		`rg --hostname-bin /tmp/x needle`,
		// Same class, found while auditing the rules above.
		`tree -R -H . -L 1`,
		`uniq in.txt out.txt`,
		`uniq -c in.txt out.txt`,
		`file -C -m /tmp/magic`,
		`file --compile -m /tmp/magic`,
		`ss -K dst 10.0.0.1`,
		`ss --kill dst 10.0.0.1`,
		`ss -D /tmp/dump`,
	)
	requireSafe(t,
		`ruff check .`,
		`ruff check --no-fix --statistics .`,
		`ruff check --select E501 src`,
		`ruff --version`,
		`poetry version`,
		`poetry version -s`,
		`poetry version --short`,
		`poetry show --tree`,
		`tomlq '.tool.poetry' pyproject.toml`,
		`tomlq -r '.project.name' pyproject.toml`,
		`yq '.a' x.yaml`,
		`yq -o json x.yaml`,
		`yq e '.a' x.yaml`,
		`dasel -f x.json '.a'`,
		`dasel -f x.json -r json -w yaml`,
		`less -R README.md`,
		`less -N README.md`,
		`xxd in.bin`,
		`xxd -l 64 in.bin`,
		`xxd -c 16 in.bin`,
		`xxd -s 0x100 -l 32 in.bin`,
		`xxd in.bin -`,
		`pacman -Qi bash`,
		`pacman -Ql bash`,
		`pacman -Qs python`,
		`pacman -Si vim`,
		`pacman -Ss vim`,
		`rpm -qa`,
		`rpm -qi bash`,
		`rpm -q --qf '%{NAME}\n' bash`,
		`nvidia-smi`,
		`nvidia-smi -L`,
		`nvidia-smi -q`,
		`nvidia-smi -q -d MEMORY`,
		`nvidia-smi -i 0 -q`,
		`nvidia-smi --query-gpu=name,memory.used --format=csv`,
		`nvidia-smi --query-gpu=name --format=csv,noheader`,
		`nvidia-smi --format csv --query-gpu name`,
		`nvidia-smi -x -q`,
		`nvidia-smi topo -m`,
		`bat README.md`,
		`bat -n src/main.rs`,
		`bat -l c src/cache.c`,
		`date`,
		`date +%s`,
		`date -u +%Y-%m-%dT%H:%M:%SZ`,
		`date -R`,
		`date -Iseconds`,
		`date -d yesterday +%F`,
		`date --date='2 days ago'`,
		`date -r 1700000000`,
		`date -v-1d +%F`,
		`date -j -f %Y-%m-%d 2020-01-01 +%s`,
		`hostname`,
		`hostname -s`,
		`hostname -f`,
		`hostname --fqdn`,
		`hostname -I`,
		`rg needle`,
		`rg -n --hidden needle src`,
		`tree -L 2`,
		`tree -a -d`,
		`uniq -c in.txt`,
		`sort file | uniq -c`,
		`uniq -f 2 in.txt`,
		`file README.md`,
		`file -b --mime-type README.md`,
		`ss -tlnp`,
	)
}

// Read-only forms that bundle scanning wrongly caught, without reopening the
// bundle gap.
func TestBundleScanFalsePositivesStaySafe(t *testing.T) {
	t.Parallel()
	requireSafe(t,
		`git log -S'func main'`,
		`git log -Gconfig`,
		`git log -L10,20:main.c`,
		`git log -c`,
		`git -C /tmp log -Scount`,
		`tree -I node_modules -L 2`,
		`tree -P '*.go' -I vendor`,
		`tree -L2 -a`,
		`xxd -groupsize 4 f`,
		`xxd -postscript f`,
		`xxd -p f`,
		`xxd -ps -c 32 f`,
	)
	requireApproval(t,
		// git -c before the subcommand still sets config.
		`git -c core.pager=/tmp/x log`,
		`git -C /tmp -c core.fsmonitor=/tmp/x status`,
		`git --no-pager -c diff.external=/tmp/x diff`,
		// tree takes -I's value from the next word and keeps reading the
		// bundle, so -Inode_modules a b is -I a -n -o b. It stays denied.
		`tree -Inode_modules a b`,
		`tree -Inode_modules`,
		`tree -RI vendor`,
		`tree -ao out.txt`,
		// xxd reads flags by their leading letter, so every spelling of -r
		// is revert.
		`xxd -r dump.hex`,
		`xxd -revert dump.hex`,
		`xxd --revert dump.hex`,
		`xxd -rp dump.hex`,
		`xxd -r -p dump.hex`,
	)
}

// PAGER and GIT_PAGER name a program to run, so only "cat" and the empty
// value (no pager) are harmless.
func TestPagerVariablesAllowOnlyCat(t *testing.T) {
	t.Parallel()
	requireSafe(t,
		`GIT_PAGER=cat git log`,
		`PAGER=cat git log -5`,
		`GIT_PAGER= git log`,
		`GIT_PAGER='' git diff`,
		`PAGER="cat" git show HEAD`,
	)
	requireApproval(t,
		`GIT_PAGER=less git log`,
		`PAGER=/tmp/x git log`,
		`GIT_PAGER='cat /etc/passwd; sh' git log`,
		`GIT_PAGER=./cat git log`,
		`PAGER=$(echo cat) git log`,
		`GIT_PAGER+=cat git log`,
		`MANPAGER=cat git log`,
	)
}

// The approval prompt shows ApprovalReason, so it has to name the real cause.
func TestApprovalReasonNamesTheCause(t *testing.T) {
	t.Parallel()
	cases := map[string]string{
		`LD_PRELOAD=/tmp/x.so ls`:               "setting LD_PRELOAD requires approval",
		`PAGER=less git log`:                    "setting PAGER",
		`FOO=$(whoami) ls`:                      "setting FOO requires approval",
		`LANG=$(whoami) ls`:                     "only known when it runs",
		`echo $(whoami)`:                        "only known when it runs",
		`echo ok > out.txt`:                     "writes output to out.txt",
		`cat < README.md`:                       "reads input from README.md",
		`ls &`:                                  "background",
		`ls; rm x`:                              "more than one statement",
		`ls 'unterminated`:                      "could not be parsed",
		`for f in *; do cat $f; done`:           "loop, subshell, function",
		`git diff --output=/tmp/out`:            "git's --output flag requires approval",
		`sort -uo /tmp/out file`:                "sort's -o flag requires approval",
		`git -c core.pager=x log`:               "git's -c flag requires approval",
		`git reset --hard`:                      "git reset is not on the read-only allowlist",
		`git --attr-source status reset --hard`: "--attr-source",
		`git`:                                   "git without a read-only subcommand",
		`find . -delete`:                        "find's -delete",
		`find . -exec sh -c x \;`:               "sh is not on the read-only allowlist",
		`go env -w GOFLAGS=x`:                   "go env -w",
		`go vet -vettool=/tmp/v ./...`:          "go's -vettool flag",
		`echo -delete | xargs find .`:           "xargs",
		`xargs -a /etc/passwd grep root`:        "xargs -a",
		`poetry version patch`:                  "pyproject.toml",
		`xxd in.bin out.hex`:                    "output file",
		`date -s 2020-01-01`:                    "sets the clock",
		`hostname evil`:                         "hostname",
		`nvidia-smi -pm 1`:                      "-pm",
		`sysctl kern.maxfiles=10`:               "sysctl",
		`curl https://x`:                        "curl is not on the read-only allowlist",
	}
	for cmd, want := range cases {
		got := ApprovalReason(cmd)
		if !strings.Contains(got, want) {
			t.Errorf("ApprovalReason(%q) = %q, want it to mention %q", cmd, got, want)
		}
		if !NeedsApproval(cmd) {
			t.Errorf("NeedsApproval(%q) = false, but it has a reason", cmd)
		}
	}
}

// Everyday read-only commands an agent runs constantly. Hardening the rules
// must not start prompting for any of them.
func TestEverydayReadOnlyCommandsStayApproved(t *testing.T) {
	t.Parallel()
	for _, cmd := range []string{
		`ls -la`,
		`ls -R src | head -50`,
		`pwd`,
		`cat go.mod`,
		`head -n 40 README.md`,
		`tail -n 100 /var/log/system.log`,
		`wc -l internal/*.go`,
		`grep -rn "TODO" --include='*.go' .`,
		`rg -n "func main" -g '*.go'`,
		`rg --files | head`,
		`fd -e go internal`,
		`find . -name '*.go' -not -path './vendor/*'`,
		`find . -type f -newer go.mod`,
		`tree -L 2 -I node_modules`,
		`stat go.mod`,
		`file bin/termwise`,
		`du -sh .`,
		`df -h`,
		`which go`,
		`uname -a`,
		`whoami`,
		`date +%Y-%m-%d`,
		`echo hello`,
		`printenv HOME`,
		`ps aux | grep termwise`,
		`sort -u names.txt | uniq -c`,
		`cut -d, -f1 data.csv | sort | uniq -c | sort -rn`,
		`diff -u a.txt b.txt`,
		`jq '.name' package.json`,
		`yq '.services' docker-compose.yml`,
		`xxd -l 64 bin/termwise`,
		`less -R README.md`,
		`bat --style=plain README.md`,
		`git status`,
		`git status --porcelain`,
		`git log --oneline -20`,
		`git log -p -S'needle' -- internal/`,
		`git diff HEAD~1 --stat`,
		`git diff --cached`,
		`git show HEAD:go.mod`,
		`git blame -L 10,20 main.go`,
		`git rev-parse --abbrev-ref HEAD`,
		`git ls-files | wc -l`,
		`git --no-pager log -1 --format=%H`,
		`GIT_PAGER=cat git log -3`,
		`git -C ../other status -sb`,
		`go env GOPATH GOMOD`,
		`go list -m all`,
		`go vet ./...`,
		`go doc net/http.Client`,
		`go version`,
		`cargo tree --depth 1`,
		`cargo metadata --no-deps --format-version 1`,
		`npm ls --depth=0`,
		`npm view react version`,
		`pnpm why lodash`,
		`pip list --outdated`,
		`pip show requests`,
		`poetry show --tree`,
		`ruff check src`,
		`docker ps -a`,
		`docker logs --tail 50 web`,
		`kubectl get pods -n kube-system`,
		`kubectl describe pod web`,
		`helm list -A`,
		`brew list --versions`,
		`sysctl -n hw.ncpu`,
		`nvidia-smi --query-gpu=name,memory.total --format=csv`,
		`find . -name '*.go' | xargs grep -l "context"`,
		`find . -name '*.go' -print0 | xargs -0 wc -l`,
	} {
		if NeedsApproval(cmd) {
			t.Errorf("NeedsApproval(%q) = true, want false (%s)", cmd, ApprovalReason(cmd))
		}
	}
}
