package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestConfigWithBrokenFileAndNoTTYDoesNotOpenEditor(t *testing.T) {
	cfgPath := isolateConfigEnv(t)
	writeTestConfig(t, cfgPath, "selected_provider = \n")

	marker := filepath.Join(t.TempDir(), "editor-ran")
	editor := filepath.Join(t.TempDir(), "editor")
	if err := os.WriteFile(editor, []byte("#!/bin/sh\ntouch '"+marker+"'\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("EDITOR", editor)

	// go test runs without a terminal on stdin, which is the case under test.
	err := configCmd.RunE(configCmd, nil)
	if err == nil || !strings.Contains(err.Error(), "config file is invalid") || !strings.Contains(err.Error(), "--edit") {
		t.Errorf("err = %v, want the parse error and the repair hint", err)
	}
	if _, statErr := os.Stat(marker); statErr == nil {
		t.Error("$EDITOR was started without a terminal")
	}
}
