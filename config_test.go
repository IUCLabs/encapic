package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "encapic-test")
	if err != nil {
		panic(err)
	}
	systemConfPath = ""
	_ = os.Unsetenv("ENCAPI_URL")
	_ = os.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, "config"))
	_ = os.Setenv("XDG_DATA_HOME", filepath.Join(dir, "data"))
	code := m.Run()
	_ = os.RemoveAll(dir)
	os.Exit(code)
}

func writeConf(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestResolveBaseURLPrecedence(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, "user"))
	user := filepath.Join(dir, "user", "encapi", "encapic.conf")
	system := filepath.Join(dir, "etc", "encapi", "encapic.conf")
	old := systemConfPath
	systemConfPath = system
	t.Cleanup(func() { systemConfPath = old })

	check := func(want string, wantFound bool) {
		t.Helper()
		got, found := resolveBaseURL()
		if got != want || found != wantFound {
			t.Errorf("resolveBaseURL() = %q, %v; want %q, %v", got, found, want, wantFound)
		}
	}

	t.Setenv("ENCAPI_URL", "")
	check(defaultBaseURL, false)
	writeConf(t, system, "# comment\nENCAPI_URL=\"https://system\"\n")
	check("https://system", true)
	writeConf(t, user, "OTHER=x\nexport ENCAPI_URL='https://user'\n")
	check("https://user", true)
	t.Setenv("ENCAPI_URL", "https://env")
	check("https://env", true)

	t.Setenv("ENCAPI_URL", "")
	writeConf(t, user, "OTHER=x\n")
	check("https://system", true)
}

func TestInitConfig(t *testing.T) {
	cases := []struct {
		name, input, want string
		wantErr           bool
	}{
		{"default", "\nn\n", "ENCAPI_URL=https://encapi.example.com\n", false},
		{"custom", "http://encapi.example:8080\nn\n", "ENCAPI_URL=http://encapi.example:8080\n", false},
		{"invalid", "ftp://nope\n", "", true},
		{"no host", "encapi.example\n", "", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("XDG_CONFIG_HOME", t.TempDir())
			var out bytes.Buffer
			err := initConfig(strings.NewReader(tc.input), &out)
			if (err != nil) != tc.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tc.wantErr)
			}
			b, rerr := os.ReadFile(userConfPath())
			if tc.wantErr {
				if rerr == nil {
					t.Errorf("conf written despite invalid URL: %q", b)
				}
				return
			}
			if string(b) != tc.want {
				t.Errorf("conf = %q, want %q", b, tc.want)
			}
			if !strings.Contains(out.String(), "encapi URL [https://encapi.example.com]: ") {
				t.Errorf("missing URL prompt in %q", out.String())
			}
			fi, err := os.Stat(filepath.Dir(userConfPath()))
			if err != nil || fi.Mode().Perm() != 0o700 {
				t.Errorf("conf dir mode = %v, %v; want 0700", fi.Mode().Perm(), err)
			}
			t.Setenv("ENCAPI_URL", "")
			if got, found := resolveBaseURL(); !found || got+"\n" != strings.TrimPrefix(tc.want, "ENCAPI_URL=") {
				t.Errorf("resolveBaseURL after init = %q", got)
			}
		})
	}
}

func TestInitConfigOverwrite(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	writeConf(t, userConfPath(), "ENCAPI_URL=https://keep\n")

	var out bytes.Buffer
	if err := initConfig(strings.NewReader("https://new\n\nn\n"), &out); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(userConfPath()); string(b) != "ENCAPI_URL=https://keep\n" {
		t.Errorf("conf overwritten without consent: %q", b)
	}
	if !strings.Contains(out.String(), "overwrite? [y/N]") {
		t.Errorf("no overwrite prompt in %q", out.String())
	}

	if err := initConfig(strings.NewReader("https://new\ny\nn\n"), &out); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(userConfPath()); string(b) != "ENCAPI_URL=https://new\n" {
		t.Errorf("conf not overwritten after yes: %q", b)
	}
}

func TestInitInstallsCompletion(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())

	t.Setenv("SHELL", "/bin/bash")
	var out bytes.Buffer
	if err := initConfig(strings.NewReader("\n\n"), &out); err != nil {
		t.Fatal(err)
	}
	path, _ := completionPath("bash")
	if b, err := os.ReadFile(path); err != nil || string(b) != bashCompletion {
		t.Errorf("bash completion not installed at %s: %v", path, err)
	}

	t.Setenv("SHELL", "/usr/bin/fish")
	out.Reset()
	if err := initConfig(strings.NewReader("\ny\n"), &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `unsupported shell "fish"`) {
		t.Errorf("no skip note for fish in %q", out.String())
	}
}

func TestNoConfigHintSilentWithoutTTY(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "stderr")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	noConfigHint(f)
	if fi, err := f.Stat(); err != nil || fi.Size() != 0 {
		t.Errorf("hint written to non-TTY stderr (size %d, %v)", fi.Size(), err)
	}
}
