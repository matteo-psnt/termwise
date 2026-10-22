package allowlist

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"mvdan.cc/sh/v3/syntax"
)

func (e *ruleEngine) validateRedirects(redirs []*syntax.Redirect) error {
	for _, redir := range redirs {
		if err := e.validateRedirect(redir); err != nil {
			return err
		}
	}
	return nil
}

func (e *ruleEngine) validateRedirect(redir *syntax.Redirect) error {
	if redir == nil || redir.Word == nil || redir.Hdoc != nil {
		return errors.New("it uses a redirect other than > to /dev/null or a temp file")
	}

	switch redir.Op {
	case syntax.RdrIn:
		fd := ""
		if redir.N != nil {
			fd = redir.N.Value
		}
		if fd != "" && fd != "0" {
			return errors.New("it redirects input into a descriptor other than stdin")
		}
		target, ok := literalWord(redir.Word)
		if !ok {
			return errors.New("it reads input from a path only known when it runs")
		}
		if target != "/dev/null" {
			return fmt.Errorf("it reads input from %s", target)
		}
		return nil

	case syntax.RdrOut, syntax.AppOut:
		fd := ""
		if redir.N != nil {
			fd = redir.N.Value
		}
		if fd != "" && fd != "1" && fd != "2" {
			return errors.New("it redirects a descriptor other than stdout or stderr")
		}
		target, ok := literalWord(redir.Word)
		if !ok {
			return errors.New("it writes output to a path only known when it runs")
		}
		if !e.isSafeRedirectTarget(target) {
			return fmt.Errorf("it writes output to %s, outside the temp directory", target)
		}
		return nil

	case syntax.DplOut:
		srcFD := "1"
		if redir.N != nil {
			srcFD = redir.N.Value
		}
		if srcFD != "1" && srcFD != "2" {
			return errors.New("it duplicates a descriptor other than stdout or stderr")
		}
		target, ok := literalWord(redir.Word)
		if !ok || (target != "1" && target != "2") {
			return errors.New("it duplicates output into a descriptor other than stdout or stderr")
		}
		return nil
	default:
		return errors.New("it uses a redirect other than > to /dev/null or a temp file")
	}
}

func (e *ruleEngine) isSafeRedirectTarget(target string) bool {
	if target == "/dev/null" {
		return true
	}
	if !filepath.IsAbs(target) {
		return false
	}
	if strings.ContainsAny(target, "*?[") {
		return false
	}

	cleaned := filepath.Clean(target)
	resolved, err := resolveExistingRedirectPath(cleaned)
	if err != nil {
		return false
	}
	for _, root := range e.redirectRoots {
		if resolved == root || strings.HasPrefix(resolved, root+string(filepath.Separator)) {
			return true
		}
	}
	return false
}

func defaultRedirectRoots() []string {
	seen := map[string]struct{}{}
	var roots []string
	for _, root := range []string{"/tmp", "/private/tmp", os.TempDir()} {
		root = filepath.Clean(root)
		if root == "" || root == "." {
			continue
		}
		if _, ok := seen[root]; ok {
			continue
		}
		seen[root] = struct{}{}
		roots = append(roots, root)
	}
	return roots
}

func resolveExistingRedirectPath(target string) (string, error) {
	current := target
	for {
		info, err := os.Lstat(current)
		if err == nil {
			if info.Mode()&os.ModeSymlink != 0 {
				return filepath.EvalSymlinks(current)
			}
			return current, nil
		}
		if !os.IsNotExist(err) {
			return "", err
		}

		parent := filepath.Dir(current)
		if parent == current {
			return "", errors.New("no existing path component")
		}
		current = parent
	}
}
