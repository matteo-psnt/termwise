package allowlist

import (
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
		return fmt.Errorf("unsupported redirect requires approval")
	}

	switch redir.Op {
	case syntax.RdrIn:
		fd := ""
		if redir.N != nil {
			fd = redir.N.Value
		}
		if fd != "" && fd != "0" {
			return fmt.Errorf("unsupported input redirect fd")
		}
		target, ok := literalWord(redir.Word)
		if !ok || target != "/dev/null" {
			return fmt.Errorf("input redirection requires approval")
		}
		return nil

	case syntax.RdrOut, syntax.AppOut:
		fd := ""
		if redir.N != nil {
			fd = redir.N.Value
		}
		if fd != "" && fd != "1" && fd != "2" {
			return fmt.Errorf("unsupported redirect fd")
		}
		target, ok := literalWord(redir.Word)
		if !ok || !e.isSafeRedirectTarget(target) {
			return fmt.Errorf("output redirection requires approval")
		}
		return nil

	case syntax.DplOut:
		srcFD := "1"
		if redir.N != nil {
			srcFD = redir.N.Value
		}
		if srcFD != "1" && srcFD != "2" {
			return fmt.Errorf("unsupported duplicate output fd")
		}
		target, ok := literalWord(redir.Word)
		if !ok || (target != "1" && target != "2") {
			return fmt.Errorf("output duplication requires approval")
		}
		return nil
	default:
		return fmt.Errorf("unsupported redirect requires approval")
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
			return "", fmt.Errorf("no existing path component")
		}
		current = parent
	}
}
