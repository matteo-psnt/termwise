package allowlist

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMatchesSupportsFlagStyleAndPositionalSubcommands(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		rule    Rule
		command string
		want    bool
	}{
		{
			name:    "flag style subcommand",
			rule:    Rule{Cmd: "brew", AllowSubs: []string{"--version"}},
			command: "brew --version",
			want:    true,
		},
		{
			name:    "short flag subcommand",
			rule:    Rule{Cmd: "dpkg", AllowSubs: []string{"-l"}},
			command: "dpkg -l",
			want:    true,
		},
		{
			name:    "positional subcommand after option",
			rule:    Rule{Cmd: "git", AllowSubs: []string{"status"}},
			command: "git --no-pager status",
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
			rule:    Rule{Cmd: "sed", BlockFlags: []string{"-i"}},
			command: "sed -i s/a/b/ file.txt",
			want:    false,
		},
		{
			name:    "blocked short flag with attached value denies",
			rule:    Rule{Cmd: "poetry", BlockFlags: []string{"-o"}},
			command: "poetry export -orequirements.txt",
			want:    false,
		},
		{
			name:    "blocked long flag with equals denies",
			rule:    Rule{Cmd: "poetry", BlockFlags: []string{"--output"}},
			command: "poetry export --output=requirements.txt",
			want:    false,
		},
		{
			name:    "blocked exec short flag denies",
			rule:    Rule{Cmd: "fd", BlockFlags: []string{"-x"}},
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
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := Matches(tc.rule, tc.command); got != tc.want {
				t.Fatalf("Matches(%q) = %v, want %v", tc.command, got, tc.want)
			}
		})
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
			command: `cd /Users/matteopesenti/Projects/termwise && ls -la`,
			want:    false,
		},
		{
			name:    "cd and grep pipeline stays safe",
			command: `cd /Users/matteopesenti/Projects/termwise && grep -r "glamour\|goldmark" --include="*.go" internal/ | head -10`,
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
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := NeedsApproval(nil, tc.command); got != tc.want {
				t.Fatalf("NeedsApproval(%q) = %v, want %v", tc.command, got, tc.want)
			}
		})
	}
}

func TestBuildRuleFromCommandUsesStructuredTokens(t *testing.T) {
	t.Parallel()

	tests := []struct {
		command string
		want    string
	}{
		{command: `brew --version`, want: `brew:--version`},
		{command: `git --no-pager status`, want: `git:status`},
		{command: `LC_ALL=C git --no-pager status`, want: `git:status`},
		{command: `printf ';'`, want: `printf:;`},
		{command: `printf "ok" | wc -c`, want: ``},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.command, func(t *testing.T) {
			t.Parallel()
			if got := BuildRuleFromCommand(tc.command); got != tc.want {
				t.Fatalf("BuildRuleFromCommand(%q) = %q, want %q", tc.command, got, tc.want)
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
	if !NeedsApproval(nil, command) {
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
	if !NeedsApproval(nil, command) {
		t.Fatalf("expected symlinked temp file redirect to require approval: %q", command)
	}
}
