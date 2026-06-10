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
			name:    "blocked flag still denies",
			rule:    Rule{Cmd: "sed", BlockFlags: []string{"-i"}},
			command: "sed -i s/a/b/ file.txt",
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
			name:    "escaped command substitution stays safe",
			command: `printf \$(whoami)`,
			want:    false,
		},
		{
			name:    "logical and needs approval",
			command: `git status && pwd`,
			want:    true,
		},
		{
			name:    "file redirect needs approval",
			command: `echo ok > out.txt`,
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
