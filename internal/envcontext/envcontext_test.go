package envcontext

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestRenderIncludesEverythingTheModelNeeds(t *testing.T) {
	c := Context{
		OS:       "macOS",
		Shell:    "/bin/zsh",
		WorkDir:  "/Users/x/proj",
		Project:  []string{"Go (go.mod)", "Makefile"},
		PkgMgrs:  []string{"brew", "npm", "bun"},
		CLITools: []string{"rg", "gh"},
		Git:      &GitInfo{Branch: "main", Dirty: true, Changed: 3, Recent: []string{"fix thing", "add other"}},
	}
	out := c.Render()
	for _, want := range []string{
		"OS: macOS",
		"Working directory: /Users/x/proj",
		"Project: Go (go.mod), Makefile",
		"Package managers installed: brew, npm, bun",
		"CLI tools available: rg, gh",
		"branch main, uncommitted changes in 3 files",
		"Recent commits: fix thing | add other",
		"Only propose commands that use the package managers and CLI tools listed above",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("Render() missing %q\n---\n%s", want, out)
		}
	}
}

func TestRenderOmitsEmptySections(t *testing.T) {
	out := Context{OS: "Linux", Shell: "/bin/sh", WorkDir: "/tmp"}.Render()
	if strings.Contains(out, "Project:") || strings.Contains(out, "CLI tools available:") {
		t.Errorf("empty sections were rendered:\n%s", out)
	}
	if !strings.Contains(out, "Git: not a repository") {
		t.Errorf("non-repo state not reported:\n%s", out)
	}
}

func TestRenderPluralisesChangedFiles(t *testing.T) {
	c := Context{Git: &GitInfo{Branch: "main", Dirty: true, Changed: 1}}
	if !strings.Contains(c.Render(), "in 1 file\n") {
		t.Errorf("singular file count not handled: %s", c.Render())
	}
}

func TestDetectFindsProjectMarkersAndGit(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	dir := t.TempDir()
	for _, f := range []string{"go.mod", "Makefile"} {
		if err := os.WriteFile(filepath.Join(dir, f), []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	for _, args := range [][]string{
		{"init"}, {"config", "user.email", "t@t"}, {"config", "user.name", "t"},
		{"add", "."}, {"commit", "-m", "initial commit"},
	} {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}

	t.Chdir(dir)
	c := Detect(context.Background(), Options{})

	if len(c.Project) != 2 {
		t.Errorf("Project = %v, want go.mod and Makefile", c.Project)
	}
	if c.Git == nil {
		t.Fatal("Git was nil inside a repository")
	}
	if c.Git.Branch == "" {
		t.Error("branch not detected")
	}
	if len(c.Git.Recent) == 0 || c.Git.Recent[0] != "initial commit" {
		t.Errorf("Recent = %v", c.Git.Recent)
	}
	if c.Git.Dirty {
		t.Errorf("fresh commit reported dirty (%d changed)", c.Git.Changed)
	}
}

func TestDetectOutsideRepoReportsNoGit(t *testing.T) {
	t.Chdir(t.TempDir())
	if g := Detect(context.Background(), Options{}).Git; g != nil {
		t.Errorf("Git = %+v outside a repository, want nil", g)
	}
}

// The probe deadline must hold even when git leaves a child process holding
// its stdout open, which would otherwise keep Output blocked until that
// child exits.
func TestDetectGitIsBoundedWhenChildHoldsOutput(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses a shell script as a fake git")
	}
	bin := t.TempDir()
	script := "#!/bin/sh\n/bin/sleep 20 &\necho true\n"
	if err := os.WriteFile(filepath.Join(bin, "git"), []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin)

	start := time.Now()
	detectGit(context.Background(), t.TempDir())
	if d := time.Since(start); d > 3*probeTimeout {
		t.Fatalf("detectGit took %v; the probe timeout is %v", d, probeTimeout)
	}
}

// PATH probing must not invent tools that aren't there.
func TestOnPathOnlyReturnsRealBinaries(t *testing.T) {
	got := onPath([]string{"sh", "definitely-not-a-real-binary-xyz"})
	for _, g := range got {
		if g == "definitely-not-a-real-binary-xyz" {
			t.Error("reported a binary that does not exist")
		}
	}
}

func TestRecentCommandsFiltersSecretsAndSelfCalls(t *testing.T) {
	dir := t.TempDir()
	hist := filepath.Join(dir, ".zsh_history")
	body := strings.Join([]string{
		": 1774724113:0;git status",
		": 1774724114:0;export OPENAI_API_KEY=sk-abcdef123456",
		": 1774724115:0;tw ask what port is this",
		": 1774724116:0;npm run dev",
		": 1774724117:0;npm run dev",
		": 1774724118:0;curl -H 'Authorization: Bearer xyz' https://api.example.com",
		": 1774724119:0;docker compose up",
	}, "\n") + "\n"
	if err := os.WriteFile(hist, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HISTFILE", hist)

	got := RecentCommands("/bin/zsh")
	want := []string{"git status", "npm run dev", "docker compose up"}
	if len(got) != len(want) {
		t.Fatalf("RecentCommands() = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("[%d] = %q, want %q", i, got[i], want[i])
		}
	}
	for _, g := range got {
		if strings.Contains(g, "sk-") || strings.Contains(g, "Bearer") {
			t.Errorf("a secret survived filtering: %q", g)
		}
	}
}

func TestRecentCommandsWindowsToTheTail(t *testing.T) {
	dir := t.TempDir()
	hist := filepath.Join(dir, ".bash_history")
	var lines []string
	for i := range 200 {
		lines = append(lines, "echo "+string(rune('a'+i%26))+strings.Repeat("x", i%3))
	}
	if err := os.WriteFile(hist, []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HISTFILE", hist)
	got := RecentCommands("/bin/bash")
	if len(got) != recentCommandCount {
		t.Fatalf("got %d commands, want %d", len(got), recentCommandCount)
	}
	if got[len(got)-1] != lines[len(lines)-1] {
		t.Errorf("last command = %q, want %q", got[len(got)-1], lines[len(lines)-1])
	}
}

// zsh writes each byte >= 0x83 as 0x83 followed by the byte XOR 0x20.
func metafy(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] >= 0x83 {
			b.WriteByte(0x83)
			b.WriteByte(s[i] ^ 0x20)
			continue
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

func TestRecentCommandsUnmetafiesZshHistory(t *testing.T) {
	cmd := "cd ~/Documents/résumé && ls 写真"
	hist := filepath.Join(t.TempDir(), ".zsh_history")
	if err := os.WriteFile(hist, []byte(metafy(": 1774724113:0;"+cmd)+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HISTFILE", hist)
	if got := RecentCommands("/bin/zsh"); len(got) != 1 || got[0] != cmd {
		t.Fatalf("RecentCommands() = %q, want [%q]", got, cmd)
	}

	// bash stores UTF-8 as is; 0x83 there is part of a character, not a marker.
	bashCmd := "ls ă"
	bashHist := filepath.Join(t.TempDir(), ".bash_history")
	if err := os.WriteFile(bashHist, []byte(bashCmd+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HISTFILE", bashHist)
	if got := RecentCommands("/bin/bash"); len(got) != 1 || got[0] != bashCmd {
		t.Fatalf("RecentCommands() = %q, want [%q]", got, bashCmd)
	}
}

func TestRecentCommandsAbsentFileIsSilent(t *testing.T) {
	t.Setenv("HISTFILE", filepath.Join(t.TempDir(), "nope"))
	if got := RecentCommands("/bin/zsh"); got != nil {
		t.Errorf("RecentCommands() = %v, want nil", got)
	}
}

// Shell history must stay out of the prompt unless it was explicitly enabled.
func TestDetectOmitsShellHistoryByDefault(t *testing.T) {
	t.Chdir(t.TempDir())
	if got := Detect(context.Background(), Options{}); len(got.Recent) != 0 {
		t.Errorf("Recent = %v with ShellHistory off", got.Recent)
	}
}
