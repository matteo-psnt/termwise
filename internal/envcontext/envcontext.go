// Package envcontext gathers the facts about the user's machine that the model
// needs in order to propose a command that will actually run.
//
// Every failure mode this package exists to prevent is one the model hit by
// guessing: proposing `npm uninstall` for a Homebrew-installed binary, or
// `brew install <name>` for a formula that does not exist. Detection is cheap —
// PATH lookups need no subprocess, and the git probes are bounded by a short
// timeout — so it runs once per session and is injected into the system prompt.
package envcontext

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// probeTimeout bounds the git probes. A slow or huge repository must never
// delay the first prompt; a missing git block is better than a stalled session.
const probeTimeout = 1500 * time.Millisecond

// Context is the resolved environment for one session.
type Context struct {
	OS      string
	Shell   string
	WorkDir string

	Git      *GitInfo // nil when the working directory is not a git repository
	Project  []string // detected project markers, e.g. "Go (go.mod)"
	PkgMgrs  []string // package managers found on PATH
	CLITools []string // notable CLI tools found on PATH

	// Recent is the tail of the user's shell history, oldest first. Populated
	// only when Options.ShellHistory is set — it is opt-in because history can
	// contain secrets.
	Recent []string
}

// Options controls what Detect gathers.
type Options struct {
	// ShellHistory includes recent shell commands. Off unless the user has
	// turned on the shell_context setting.
	ShellHistory bool
}

// GitInfo describes the repository containing the working directory.
type GitInfo struct {
	Branch  string
	Dirty   bool
	Changed int      // files with uncommitted changes
	Recent  []string // most recent commit subjects, newest first
}

// pkgManagers are probed in this order; the order is what the model sees, so
// it doubles as a weak preference hint on macOS and Linux.
var pkgManagers = []string{
	"brew", "apt", "dnf", "pacman", "port",
	"npm", "pnpm", "yarn", "bun",
	"pip3", "pipx", "uv", "cargo", "go", "gem",
}

// cliTools are probed because a proposed command that uses one of these is only
// correct if the user actually has it. `rg` vs `grep` and `fd` vs `find` are the
// common cases; the rest change what a plausible answer looks like.
var cliTools = []string{
	"rg", "fd", "jq", "yq", "gh", "git",
	"docker", "kubectl", "tmux", "fzf", "bat", "eza", "tree",
	"make", "just", "curl", "http",
}

// projectMarkers maps a file or directory to the stack it identifies.
var projectMarkers = []struct{ path, label string }{
	{"go.mod", "Go (go.mod)"},
	{"package.json", "Node (package.json)"},
	{"pnpm-lock.yaml", "pnpm workspace"},
	{"bun.lockb", "Bun"},
	{"Cargo.toml", "Rust (Cargo.toml)"},
	{"pyproject.toml", "Python (pyproject.toml)"},
	{"requirements.txt", "Python (requirements.txt)"},
	{"Gemfile", "Ruby (Gemfile)"},
	{"pom.xml", "Java (Maven)"},
	{"build.gradle", "Java (Gradle)"},
	{"Makefile", "Makefile"},
	{"justfile", "justfile"},
	{"docker-compose.yml", "Docker Compose"},
	{"compose.yaml", "Docker Compose"},
	{"Dockerfile", "Dockerfile"},
}

// Detect gathers the environment. It never returns an error: any probe that
// fails is simply omitted, because a partial environment block is still far
// better than none.
func Detect(ctx context.Context, opts Options) Context {
	cwd, _ := os.Getwd()
	c := Context{
		OS:      osName(),
		Shell:   shellPath(),
		WorkDir: cwd,
	}

	for _, m := range projectMarkers {
		if _, err := os.Stat(filepath.Join(cwd, m.path)); err == nil {
			c.Project = appendUnique(c.Project, m.label)
		}
	}
	c.PkgMgrs = onPath(pkgManagers)
	c.CLITools = onPath(cliTools)
	c.Git = detectGit(ctx, cwd)
	if opts.ShellHistory {
		c.Recent = RecentCommands(c.Shell)
	}
	return c
}

// Render returns the environment block for the system prompt. It is empty-safe:
// sections with nothing to report are omitted rather than rendered blank.
func (c Context) Render() string {
	var b strings.Builder
	b.WriteString("- OS: " + c.OS + "\n")
	b.WriteString("- Shell: " + c.Shell + "\n")
	b.WriteString("- Working directory: " + c.WorkDir + "\n")

	if len(c.Project) > 0 {
		b.WriteString("- Project: " + strings.Join(c.Project, ", ") + "\n")
	}
	if g := c.Git; g != nil {
		state := "clean"
		if g.Dirty {
			state = "uncommitted changes in " + plural(g.Changed, "file")
		}
		b.WriteString("- Git: branch " + g.Branch + ", " + state + "\n")
		if len(g.Recent) > 0 {
			b.WriteString("- Recent commits: " + strings.Join(g.Recent, " | ") + "\n")
		}
	} else {
		b.WriteString("- Git: not a repository\n")
	}
	if len(c.PkgMgrs) > 0 {
		b.WriteString("- Package managers installed: " + strings.Join(c.PkgMgrs, ", ") + "\n")
	}
	if len(c.CLITools) > 0 {
		b.WriteString("- CLI tools available: " + strings.Join(c.CLITools, ", ") + "\n")
	}
	if len(c.Recent) > 0 {
		b.WriteString("\nRecent shell commands (oldest first) — use these to resolve references like \"that\", \"the last one\", or \"fix that command\":\n")
		for _, cmd := range c.Recent {
			b.WriteString("  " + cmd + "\n")
		}
	}
	b.WriteString("Only propose commands that use the package managers and CLI tools listed above. " +
		"If a command needs something that is not listed, say so instead of assuming it is installed.")
	return b.String()
}

func onPath(names []string) []string {
	var found []string
	for _, n := range names {
		if _, err := exec.LookPath(n); err == nil {
			found = append(found, n)
		}
	}
	return found
}

func detectGit(ctx context.Context, dir string) *GitInfo {
	ctx, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()

	if out, err := gitOut(ctx, dir, "rev-parse", "--is-inside-work-tree"); err != nil || out != "true" {
		return nil
	}

	g := &GitInfo{}
	g.Branch, _ = gitOut(ctx, dir, "rev-parse", "--abbrev-ref", "HEAD")
	if g.Branch == "" {
		g.Branch = "(detached)"
	}
	if status, err := gitOut(ctx, dir, "status", "--porcelain"); err == nil && status != "" {
		g.Dirty = true
		g.Changed = len(strings.Split(status, "\n"))
	}
	if log, err := gitOut(ctx, dir, "log", "-3", "--format=%s"); err == nil && log != "" {
		g.Recent = strings.Split(log, "\n")
	}
	return g
}

func gitOut(ctx context.Context, dir string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func appendUnique(s []string, v string) []string {
	for _, existing := range s {
		if existing == v {
			return s
		}
	}
	return append(s, v)
}

func plural(n int, noun string) string {
	if n == 1 {
		return "1 " + noun
	}
	return itoa(n) + " " + noun + "s"
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var digits []byte
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	return string(digits)
}

func osName() string {
	switch runtime.GOOS {
	case "darwin":
		return "macOS"
	case "linux":
		return "Linux"
	default:
		return runtime.GOOS
	}
}

func shellPath() string {
	if s := os.Getenv("SHELL"); s != "" {
		return s
	}
	return "/bin/sh"
}
