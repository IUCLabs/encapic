package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCompletionPrint(t *testing.T) {
	for shell, marker := range map[string]string{"bash": "complete -F _encapic encapic", "zsh": "#compdef encapic"} {
		var out bytes.Buffer
		if err := run([]string{"encapic", "completion", shell}, &out); err != nil {
			t.Fatalf("%s: %v", shell, err)
		}
		if !strings.Contains(out.String(), marker) {
			t.Errorf("%s script missing %q", shell, marker)
		}
		for _, word := range []string{"init", "set-testing", "unset-testing", "completion", "bash zsh", "--install"} {
			if !strings.Contains(out.String(), word) {
				t.Errorf("%s script does not complete %q", shell, word)
			}
		}
	}
}

func TestCompletionErrors(t *testing.T) {
	for _, args := range [][]string{{}, {"fish"}, {"bash", "--bogus"}, {"bash", "--install", "x"}} {
		if err := completion(args, &bytes.Buffer{}); err == nil {
			t.Errorf("completion(%q) succeeded, want error", args)
		}
	}
}

func TestCompletionInstall(t *testing.T) {
	data := t.TempDir()
	t.Setenv("XDG_DATA_HOME", data)
	zshDir := filepath.Join(data, "zsh", "site-functions")

	cases := []struct {
		shell, path, fpath string
		hint               bool
	}{
		{"bash", filepath.Join(data, "bash-completion", "completions", "encapic"), "", false},
		{"zsh", filepath.Join(zshDir, "_encapic"), "/usr/share/zsh/site-functions", true},
		{"zsh", filepath.Join(zshDir, "_encapic"), "/a:" + zshDir + ":/b", false},
	}
	for _, tc := range cases {
		t.Setenv("FPATH", tc.fpath)
		var out bytes.Buffer
		if err := run([]string{"encapic", "completion", tc.shell, "--install"}, &out); err != nil {
			t.Fatalf("%s install: %v", tc.shell, err)
		}
		b, err := os.ReadFile(tc.path)
		if err != nil || string(b) != completionScripts[tc.shell] {
			t.Errorf("%s: script not at %s: %v", tc.shell, tc.path, err)
		}
		if got := strings.Contains(out.String(), "fpath=("+zshDir); got != tc.hint {
			t.Errorf("%s FPATH=%q: hint = %v, want %v (%q)", tc.shell, tc.fpath, got, tc.hint, out.String())
		}
	}
}

func TestCompletionInstallDefaultsToLocalShare(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_DATA_HOME", "")
	path, err := completionPath("bash")
	if err != nil || path != filepath.Join(home, ".local", "share", "bash-completion", "completions", "encapic") {
		t.Errorf("completionPath = %q, %v", path, err)
	}
}
