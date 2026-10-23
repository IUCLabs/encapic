package main

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func fakeVault(t *testing.T, status int, body string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/v1/identity/oidc/token/encapi-testing" {
			http.NotFound(w, r)
			return
		}
		if r.Header.Get("X-Vault-Token") != "vt" {
			http.Error(w, `{"errors":["permission denied"]}`, http.StatusForbidden)
			return
		}
		w.WriteHeader(status)
		_, _ = io.WriteString(w, body)
	}))
	t.Cleanup(srv.Close)
	return srv
}

type seenPut struct{ path, auth, body string }

func fakeEncapi(t *testing.T, status int, seen *seenPut) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		if r.Method == http.MethodPut {
			*seen = seenPut{r.URL.Path, r.Header.Get("Authorization"), string(b)}
		}
		w.WriteHeader(status)
		_, _ = io.WriteString(w, `{"error":"forbidden"}`)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestSetTestingSuccess(t *testing.T) {
	var seen seenPut
	t.Setenv("VAULT_ADDR", fakeVault(t, 200, `{"data":{"token":"jwt123"}}`).URL)
	t.Setenv("VAULT_TOKEN", "vt")
	t.Setenv("ENCAPI_URL", fakeEncapi(t, 200, &seen).URL)

	var out bytes.Buffer
	if err := run([]string{"encapic", "set-testing", "host.example"}, &out); err != nil {
		t.Fatalf("run: %v", err)
	}
	want := seenPut{"/api/v1/nodes/host.example/environment", "Bearer jwt123", `{"environment":"testing"}`}
	if seen != want {
		t.Errorf("PUT = %+v, want %+v", seen, want)
	}
}

func TestSetTestingVaultTokenFile(t *testing.T) {
	var seen seenPut
	home := t.TempDir()
	if err := os.WriteFile(filepath.Join(home, ".vault-token"), []byte("vt\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	t.Setenv("VAULT_TOKEN", "")
	t.Setenv("VAULT_ADDR", fakeVault(t, 200, `{"data":{"token":"jwt123"}}`).URL)
	t.Setenv("ENCAPI_URL", fakeEncapi(t, 204, &seen).URL)

	if err := run([]string{"encapic", "set-testing", "host.example"}, io.Discard); err != nil {
		t.Fatalf("run: %v", err)
	}
	if seen.auth != "Bearer jwt123" {
		t.Errorf("auth = %q", seen.auth)
	}
}

func TestSetTestingVaultError(t *testing.T) {
	var seen seenPut
	t.Setenv("VAULT_ADDR", fakeVault(t, 400, `{"errors":["role not found"]}`).URL)
	t.Setenv("VAULT_TOKEN", "vt")
	t.Setenv("ENCAPI_URL", fakeEncapi(t, 200, &seen).URL)

	err := run([]string{"encapic", "set-testing", "host.example"}, io.Discard)
	if err == nil || !strings.Contains(err.Error(), "HTTP 400") || !strings.Contains(err.Error(), "role not found") {
		t.Fatalf("err = %v, want HTTP 400 with body", err)
	}
	if seen.path != "" {
		t.Errorf("encapi called despite vault failure: %+v", seen)
	}
}

func TestSetTestingEncapiForbidden(t *testing.T) {
	var seen seenPut
	t.Setenv("VAULT_ADDR", fakeVault(t, 200, `{"data":{"token":"jwt123"}}`).URL)
	t.Setenv("VAULT_TOKEN", "vt")
	t.Setenv("ENCAPI_URL", fakeEncapi(t, 403, &seen).URL)

	err := run([]string{"encapic", "set-testing", "host.example"}, io.Discard)
	if err == nil || !strings.Contains(err.Error(), "HTTP 403") || !strings.Contains(err.Error(), "forbidden") {
		t.Fatalf("err = %v, want HTTP 403 with body", err)
	}
}
