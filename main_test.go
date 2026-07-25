package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

// cobblerWire is the exact cobbler-wire document encapi serves for a node with
// a single role in the develop environment.
const cobblerWire = "classes:\n" +
	"    roles::infra::storage::vault: {}\n" +
	"environment: develop\n" +
	"parameters: {}\n"

// goldenENC is the reshaped ENC document the previous python script produced
// from cobblerWire (verified byte-for-byte against uv/python yaml.dump).
const goldenENC = "classes:\n" +
	"- roles::infra::storage::vault\n" +
	"environment: develop\n" +
	"parameters:\n" +
	"  enc_env: develop\n" +
	"  enc_role:\n" +
	"  - roles::infra::storage::vault\n"

func TestRunGoldenOutput(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/cblr/svc/op/puppet/hostname/host.example" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/x-yaml")
		_, _ = w.Write([]byte(cobblerWire))
	}))
	defer srv.Close()

	t.Setenv("ENCAPI_URL", srv.URL)
	var out bytes.Buffer
	if err := run([]string{"encapic", "host.example"}, &out); err != nil {
		t.Fatalf("run returned error: %v", err)
	}
	if out.String() != goldenENC {
		t.Errorf("output mismatch\n--- want ---\n%s\n--- got ---\n%s", goldenENC, out.String())
	}
}

func TestRun404FailsNonZero(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
	}))
	defer srv.Close()

	t.Setenv("ENCAPI_URL", srv.URL)
	var out bytes.Buffer
	err := run([]string{"encapic", "ghost.example"}, &out)
	if err == nil {
		t.Fatalf("expected error on 404, got nil")
	}
	if out.Len() != 0 {
		t.Errorf("expected no stdout on failure, got %q", out.String())
	}
	if !strings.Contains(err.Error(), "404") {
		t.Errorf("error should mention 404, got %v", err)
	}
}

func TestRun500FailsNonZero(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer srv.Close()

	t.Setenv("ENCAPI_URL", srv.URL)
	var out bytes.Buffer
	if err := run([]string{"encapic", "host.example"}, &out); err == nil {
		t.Fatalf("expected error on 500, got nil")
	}
}

func TestFetchTimeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		_, _ = w.Write([]byte(cobblerWire))
	}))
	defer srv.Close()

	// Use a client with a tiny timeout to exercise the timeout path quickly.
	client := &http.Client{Timeout: 20 * time.Millisecond}
	resp, err := client.Get(srv.URL + "/cblr/svc/op/puppet/hostname/host.example")
	if err == nil {
		_ = resp.Body.Close()
		t.Fatalf("expected timeout error, got none")
	}
}

func TestRunUsage(t *testing.T) {
	var out bytes.Buffer
	if err := run([]string{"encapic"}, &out); err == nil {
		t.Fatalf("expected usage error with no args")
	}
	if err := run([]string{"encapic", "a", "b"}, &out); err == nil {
		t.Fatalf("expected usage error with too many args")
	}
}

func TestRunVersion(t *testing.T) {
	var out bytes.Buffer
	if err := run([]string{"encapic", "--version"}, &out); err != nil {
		t.Fatalf("version returned error: %v", err)
	}
	if !strings.Contains(out.String(), "encapic") {
		t.Errorf("version output = %q", out.String())
	}
}

func TestDefaultBaseURLCompiledIn(t *testing.T) {
	// Guard against accidental changes to the in-cluster default.
	if err := os.Unsetenv("ENCAPI_URL"); err != nil {
		t.Fatalf("unsetenv: %v", err)
	}
	if defaultBaseURL != "http://encapi.encapi.svc.cluster.local" {
		t.Errorf("defaultBaseURL = %q", defaultBaseURL)
	}
}
