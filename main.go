// Command encapic is a dependency-less Go client for encapi, used as the Puppet
// exec External Node Classifier (ENC) on the Kubernetes compilers.
//
// It is a behavioural drop-in for the previous uv/python ENC script: invoked as
//
//	encapic <certname>
//
// it fetches ${ENCAPI_URL}/cblr/svc/op/puppet/hostname/<certname> (the
// cobbler-wire ENC document encapi serves for compatibility), applies the same
// normalisation the python script applied, and prints the reshaped ENC YAML to
// stdout. Any HTTP or parse failure (including a 404) exits non-zero so the
// puppet exec node_terminus fails safe rather than compiling an empty catalog.
//
// # Why the cobbler endpoint and hand-emitted YAML
//
// encapi already exposes /api/v1/nodes/<certname>/enc which serves the fully
// reshaped document. We deliberately consume the cobbler-wire endpoint instead
// and reshape it here so encapic reproduces the exact byte-for-byte output of
// the python script it replaces (python's yaml.dump: alphabetically sorted
// keys, block-style lists indented at the parent, two-space nesting). Matching
// that output means the swap changes nothing the puppet agent sees. The YAML we
// consume has a small, fixed shape, so it is hand-parsed; the YAML we emit is
// hand-written. This keeps the binary on the standard library only, which is
// the whole point of the rewrite (the python script's first-invocation
// dependency resolution failed on fresh compiler pods).
package main

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// version is overwritten at build time via -ldflags "-X main.version=...".
var version = "dev"

// defaultBaseURL is compiled in and points at the in-cluster encapi service. It
// is overridden by ENCAPI_URL from the environment or a conf file.
const defaultBaseURL = "http://encapi.encapi.svc.cluster.local"

// httpTimeout bounds the whole request; the puppet exec ENC must not hang.
const httpTimeout = 10 * time.Second

const usage = `usage:
  encapic <certname>                         print the Puppet ENC YAML for certname
  encapic set-testing <certname>             pin certname to the testing environment
  encapic init                               write ~/.config/encapi/encapic.conf
  encapic completion bash|zsh [--install]    print or install shell completion
  encapic -h | --help | help                 show this help
  encapic -v | --version                     show the version

ENCAPI_URL is read from the environment, then ~/.config/encapi/encapic.conf,
/etc/encapi/encapic.conf, /etc/encapic/encapic.conf, else the in-cluster default.
`

func main() {
	if err := run(os.Args, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// run is the testable entry point. It writes the ENC YAML to out and returns a
// non-nil error on any failure.
func run(args []string, out io.Writer) error {
	if len(args) == 2 {
		switch args[1] {
		case "-v", "--version":
			_, err := fmt.Fprintf(out, "encapic %s\n", version)
			return err
		case "-h", "--help", "help":
			_, err := io.WriteString(out, usage)
			return err
		case "init":
			return initConfig(os.Stdin, out)
		}
	}
	if len(args) >= 2 && args[1] == "completion" {
		return completion(args[2:], out)
	}
	if len(args) == 3 && args[1] == "set-testing" {
		return setTesting(args[2], out)
	}
	if len(args) != 2 || strings.HasPrefix(args[1], "-") {
		return fmt.Errorf("invalid arguments %q\n%s", args[1:], strings.TrimSuffix(usage, "\n"))
	}
	certname := args[1]
	baseURL := baseURL()

	body, err := fetch(baseURL, certname)
	if err != nil {
		return err
	}

	doc, err := parseCobbler(body)
	if err != nil {
		return fmt.Errorf("parse ENC for %q: %w", certname, err)
	}

	yaml, err := renderENC(doc)
	if err != nil {
		return fmt.Errorf("render ENC for %q: %w", certname, err)
	}
	_, err = io.WriteString(out, yaml)
	return err
}

// fetch retrieves the cobbler-wire ENC document for certname. A non-2xx
// response (notably 404 for an unknown node) is an error so puppet fails safe.
func fetch(baseURL, certname string) ([]byte, error) {
	url := strings.TrimRight(baseURL, "/") + "/cblr/svc/op/puppet/hostname/" + certname
	client := &http.Client{Timeout: httpTimeout}
	resp, err := client.Get(url)
	if err != nil {
		return nil, fmt.Errorf("request %s: %w", url, err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response from %s: %w", url, err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("request %s returned HTTP %d: %s",
			url, resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return body, nil
}

// cobblerDoc is the parsed cobbler-wire ENC document.
type cobblerDoc struct {
	// classes are the role names, in the order the wire document listed them.
	classes []string
	// environment is the node's environment; empty means the key was absent.
	environment string
	hasEnv      bool
	// parameters are any pre-existing top-level parameters (usually none).
	parameters map[string]string
	paramOrder []string
}
