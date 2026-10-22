// Package shellrc adds and removes the line that loads termwise's shell
// integration from a shell's startup file.
//
// `termwise init <shell>` only prints the integration; something still has to
// run it on every shell start. This package writes that one line, so setup can
// offer to do it rather than leaving the user to find the right rc file.
package shellrc

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// Supported lists the shells with an integration, in the order help text
// names them.
var Supported = []string{"zsh", "bash", "fish"}

// marker is written above the line so Remove can tell what Install added.
const marker = "# termwise shell integration (added by `termwise init --install`)"

// Detect returns the user's shell from $SHELL, if it is one termwise supports.
func Detect() (string, bool) {
	name := filepath.Base(os.Getenv("SHELL"))
	for _, s := range Supported {
		if name == s {
			return s, true
		}
	}
	return "", false
}

// Line is the rc-file line that loads the integration for shell.
func Line(shell string) string {
	if shell == "fish" {
		return "termwise init fish | source"
	}
	return fmt.Sprintf(`eval "$(termwise init %s)"`, shell)
}

// Path returns the startup file that should load the integration for shell.
func Path(shell string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("could not determine home directory: %w", err)
	}
	switch shell {
	case "zsh":
		dir := os.Getenv("ZDOTDIR")
		if dir == "" {
			dir = home
		}
		return filepath.Join(dir, ".zshrc"), nil
	case "bash":
		return bashPath(home), nil
	case "fish":
		dir := os.Getenv("XDG_CONFIG_HOME")
		if dir == "" {
			dir = filepath.Join(home, ".config")
		}
		return filepath.Join(dir, "fish", "config.fish"), nil
	}
	return "", fmt.Errorf("unsupported shell %q — supported: %s", shell, strings.Join(Supported, ", "))
}

// bashPath picks ~/.bashrc, except on macOS where terminals start login
// shells: those read ~/.bash_profile, which only sees ~/.bashrc if it sources
// it.
func bashPath(home string) string {
	rc := filepath.Join(home, ".bashrc")
	if runtime.GOOS != "darwin" {
		return rc
	}
	profile := filepath.Join(home, ".bash_profile")
	data, err := os.ReadFile(profile)
	if err != nil || bytes.Contains(data, []byte(".bashrc")) {
		return rc
	}
	return profile
}

// Installed reports whether the file at path already loads the integration,
// whether Install put it there or the user did.
func Installed(path string) (bool, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return loadsIntegration(data), nil
}

func loadsIntegration(data []byte) bool {
	sc := bufio.NewScanner(bytes.NewReader(data))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if !strings.HasPrefix(line, "#") && strings.Contains(line, "termwise init") {
			return true
		}
	}
	return false
}

// Install appends the integration line for shell to path, creating the file
// and its directory if needed. It reports false when the file already loads
// the integration and nothing was written.
func Install(path, shell string) (bool, error) {
	data, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return false, err
	}
	if loadsIntegration(data) {
		return false, nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return false, err
	}

	// Always "\n", even in a CRLF file: shells do not strip a "\r", so zsh
	// would bind the key to "_termwise_widget\r" and bash's `bind -x` would
	// get the same trailing "\r". Remove still restores the file exactly.
	block := marker + "\n" + Line(shell) + "\n"
	if len(data) > 0 {
		// A file with no final newline gets just that newline here, not a
		// blank separator line: its absence is how Remove knows to take
		// the added newline back off. The block itself still ends in one,
		// so a later `echo >>` does not run into the integration line.
		block = "\n" + block
	}

	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return false, err
	}
	if _, err := f.WriteString(block); err != nil {
		_ = f.Close()
		return false, err
	}
	return true, f.Close()
}

// Remove deletes what Install added to path: the marker comment and the line
// below it. A line the user wrote themselves is left alone. It reports false
// when there was nothing to remove.
func Remove(path string) (bool, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}

	lines := strings.SplitAfter(string(data), "\n")
	out := make([]string, 0, len(lines))
	removed := false
	for i := 0; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) != marker {
			out = append(out, lines[i])
			continue
		}
		removed = true
		markerLine := lines[i]
		if i+1 < len(lines) && strings.Contains(lines[i+1], "termwise init") {
			i++
		}
		n := len(out)
		// Install writes "\n" after the file's content. Older releases
		// wrote "\r\n" in a CRLF file, which shows on the marker line;
		// take back whichever was added.
		added := "\n"
		if strings.HasSuffix(markerLine, "\r\n") {
			added = "\r\n"
		}
		switch {
		case n > 0 && out[n-1] == added:
			// Drop the blank separator Install put above the marker.
			out = out[:n-1]
		case n > 0 && strings.Join(lines[i+1:], "") == "" && strings.HasSuffix(out[n-1], added):
			// No separator, and the block was last: Install added it to
			// a file with no final newline, so take that newline off.
			out[n-1] = strings.TrimSuffix(out[n-1], added)
		}
	}
	if !removed {
		return false, nil
	}

	info, err := os.Stat(path)
	if err != nil {
		return false, err
	}
	return true, os.WriteFile(path, []byte(strings.Join(out, "")), info.Mode().Perm())
}
