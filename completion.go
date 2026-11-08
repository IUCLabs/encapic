package main

import (
	_ "embed"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

//go:embed completions/encapic.bash
var bashCompletion string

//go:embed completions/_encapic
var zshCompletion string

var completionScripts = map[string]string{"bash": bashCompletion, "zsh": zshCompletion}

func completionPath(shell string) (string, error) {
	d := xdgDir("XDG_DATA_HOME", filepath.Join(".local", "share"))
	if d == "" {
		return "", fmt.Errorf("cannot determine user data directory")
	}
	if shell == "zsh" {
		return filepath.Join(d, "zsh", "site-functions", "_encapic"), nil
	}
	return filepath.Join(d, "bash-completion", "completions", "encapic"), nil
}

func installCompletion(shell string, out io.Writer) error {
	path, err := completionPath(shell)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(path, []byte(completionScripts[shell]), 0o644); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(out, "installed %s completion to %s\n", shell, path); err != nil {
		return err
	}
	dir := filepath.Dir(path)
	if shell == "zsh" && !strings.Contains(":"+os.Getenv("FPATH")+":", ":"+dir+":") {
		_, err = fmt.Fprintf(out, "hint: add to ~/.zshrc before compinit: fpath=(%s $fpath)\n", dir)
	}
	return err
}

func completion(args []string, out io.Writer) error {
	if len(args) < 1 || len(args) > 2 || (len(args) == 2 && args[1] != "--install") {
		return fmt.Errorf("usage: encapic completion bash|zsh [--install]")
	}
	script, ok := completionScripts[args[0]]
	if !ok {
		return fmt.Errorf("unsupported shell %q: want bash or zsh", args[0])
	}
	if len(args) == 2 {
		return installCompletion(args[0], out)
	}
	_, err := io.WriteString(out, script)
	return err
}
