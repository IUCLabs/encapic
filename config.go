package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

const initDefaultURL = "https://encapi"

// systemConfPaths follow the user conf in precedence; the legacy path is a
// read-only fallback for hosts whose puppet still manages it.
var systemConfPaths = []string{"/etc/encapi/encapic.conf", "/etc/encapic/encapic.conf"}

func xdgDir(env, fallback string) string {
	if d := os.Getenv(env); d != "" {
		return d
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, fallback)
}

func userConfPath() string {
	d := xdgDir("XDG_CONFIG_HOME", ".config")
	if d == "" {
		return ""
	}
	return filepath.Join(d, "encapi", "encapic.conf")
}

// resolveBaseURL returns the encapi URL and whether it came from env or a conf
// file rather than the compiled default.
func resolveBaseURL() (string, bool) {
	if u := os.Getenv("ENCAPI_URL"); u != "" {
		return u, true
	}
	for _, p := range append([]string{userConfPath()}, systemConfPaths...) {
		if u := readConfURL(p); u != "" {
			return u, true
		}
	}
	return defaultBaseURL, false
}

func baseURL() string {
	u, found := resolveBaseURL()
	if !found {
		noConfigHint(os.Stderr)
	}
	return u
}

func noConfigHint(f *os.File) {
	if fi, err := f.Stat(); err == nil && fi.Mode()&os.ModeCharDevice != 0 {
		_, _ = fmt.Fprintln(f, "hint: no config found, run 'encapic init'")
	}
}

func readConfURL(path string) string {
	if path == "" {
		return ""
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	u := ""
	for _, line := range strings.Split(string(b), "\n") {
		k, v, ok := strings.Cut(strings.TrimSpace(line), "=")
		if ok && strings.TrimSpace(strings.TrimPrefix(k, "export ")) == "ENCAPI_URL" {
			u = strings.Trim(strings.TrimSpace(v), `"'`)
		}
	}
	return u
}

func validURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return fmt.Errorf("invalid encapi URL %q: want http(s)://host", raw)
	}
	return nil
}

func prompt(r *bufio.Reader, out io.Writer, q string) (string, error) {
	if _, err := fmt.Fprint(out, q); err != nil {
		return "", err
	}
	line, err := r.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return "", err
	}
	return strings.TrimSpace(line), nil
}

func initConfig(in io.Reader, out io.Writer) error {
	path := userConfPath()
	if path == "" {
		return errors.New("cannot determine user config directory")
	}
	r := bufio.NewReader(in)
	u, err := prompt(r, out, fmt.Sprintf("encapi URL [%s]: ", initDefaultURL))
	if err != nil {
		return err
	}
	if u == "" {
		u = initDefaultURL
	}
	if err := validURL(u); err != nil {
		return err
	}

	write, verb := true, "wrote"
	if _, err := os.Stat(path); err == nil {
		ans, err := prompt(r, out, path+" exists, overwrite? [y/N] ")
		if err != nil {
			return err
		}
		write = strings.EqualFold(ans, "y") || strings.EqualFold(ans, "yes")
		if !write {
			verb = "kept"
		}
	}
	if write {
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			return err
		}
		if err := os.WriteFile(path, []byte("ENCAPI_URL="+u+"\n"), 0o644); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprintf(out, "%s %s\n", verb, path); err != nil {
		return err
	}

	ans, err := prompt(r, out, "Install shell completion? [Y/n] ")
	if err != nil {
		return err
	}
	if ans != "" && !strings.EqualFold(ans, "y") && !strings.EqualFold(ans, "yes") {
		return nil
	}
	shell := filepath.Base(os.Getenv("SHELL"))
	if _, ok := completionScripts[shell]; !ok {
		_, err := fmt.Fprintf(out, "skipping completion: unsupported shell %q (bash|zsh)\n", shell)
		return err
	}
	return installCompletion(shell, out)
}
