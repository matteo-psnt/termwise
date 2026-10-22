package shellrc

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInstallThenRemoveRestoresFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".zshrc")
	orig := "export PATH=$HOME/bin:$PATH\nalias ll='ls -l'\n"
	if err := os.WriteFile(path, []byte(orig), 0o600); err != nil {
		t.Fatal(err)
	}

	added, err := Install(path, "zsh")
	if err != nil || !added {
		t.Fatalf("Install = %v, %v; want true, nil", added, err)
	}
	if ok, _ := Installed(path); !ok {
		t.Fatal("Installed = false after Install")
	}

	added, err = Install(path, "zsh")
	if err != nil || added {
		t.Fatalf("second Install = %v, %v; want false, nil", added, err)
	}

	removed, err := Remove(path)
	if err != nil || !removed {
		t.Fatalf("Remove = %v, %v; want true, nil", removed, err)
	}
	got, _ := os.ReadFile(path)
	if string(got) != orig {
		t.Errorf("file after Remove = %q, want %q", got, orig)
	}
	if info, _ := os.Stat(path); info.Mode().Perm() != 0o600 {
		t.Errorf("Remove changed mode to %v", info.Mode().Perm())
	}
}

func TestInstallCreatesMissingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "fish", "config.fish")
	if _, err := Install(path, "fish"); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(path)
	want := marker + "\ntermwise init fish | source\n"
	if string(got) != want {
		t.Errorf("new file = %q, want %q", got, want)
	}
}

func TestUserLineCountsAsInstalledButIsNotRemoved(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".bashrc")
	orig := "eval \"$(termwise init bash)\"\n"
	if err := os.WriteFile(path, []byte(orig), 0o644); err != nil {
		t.Fatal(err)
	}
	if ok, _ := Installed(path); !ok {
		t.Error("Installed = false for a hand-written line")
	}
	if removed, _ := Remove(path); removed {
		t.Error("Remove deleted a line it did not add")
	}
}

func TestCommentedLineIsNotInstalled(t *testing.T) {
	if loadsIntegration([]byte("# eval \"$(termwise init zsh)\"\n")) {
		t.Error("a commented-out line counts as installed")
	}
}

func TestPathRespectsZDOTDIR(t *testing.T) {
	t.Setenv("ZDOTDIR", "/tmp/zdot")
	got, err := Path("zsh")
	if err != nil || got != "/tmp/zdot/.zshrc" {
		t.Errorf("Path(zsh) = %q, %v", got, err)
	}
}

func TestInstallThenRemoveRestoresFileWithoutFinalNewline(t *testing.T) {
	for _, orig := range []string{"alias ll='ls -l'", "a\r\nb", "a\n\nb"} {
		path := filepath.Join(t.TempDir(), ".zshrc")
		if err := os.WriteFile(path, []byte(orig), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := Install(path, "zsh"); err != nil {
			t.Fatal(err)
		}
		if ok, _ := Installed(path); !ok {
			t.Fatalf("%q: Installed = false after Install", orig)
		}
		// Still newline-terminated, so a later `echo … >>` starts a line.
		if got, _ := os.ReadFile(path); !strings.HasSuffix(string(got), "\n") {
			t.Errorf("%q: installed file %q has no final newline", orig, got)
		}
		if _, err := Remove(path); err != nil {
			t.Fatal(err)
		}
		if got, _ := os.ReadFile(path); string(got) != orig {
			t.Errorf("file after Remove = %q, want %q", got, orig)
		}
	}
}

func TestInstallUsesCRLFInCRLFFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".bashrc")
	orig := "export A=1\r\n"
	if err := os.WriteFile(path, []byte(orig), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Install(path, "bash"); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(path)
	want := orig + "\r\n" + marker + "\r\n" + Line("bash") + "\r\n"
	if string(got) != want {
		t.Errorf("after Install = %q, want %q", got, want)
	}
	if _, err := Remove(path); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(path); string(got) != orig {
		t.Errorf("after Remove = %q, want %q", got, orig)
	}
}
