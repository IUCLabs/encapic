package main

import (
	"bytes"
	"encoding/pem"
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
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		if r.Method == http.MethodPut {
			*seen = seenPut{r.URL.Path, r.Header.Get("Authorization"), string(b)}
		}
		w.WriteHeader(status)
		_, _ = io.WriteString(w, `{"error":"forbidden"}`)
	}))
	t.Cleanup(srv.Close)
	trustServer(t, srv)
	return srv
}

// trustServer points VAULT_CACERT at the TLS server's certificate.
func trustServer(t *testing.T, srv *httptest.Server) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "ca.pem")
	b := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: srv.Certificate().Raw})
	if err := os.WriteFile(path, b, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("VAULT_CACERT", path)
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

func TestSetTestingRefusesPlainHTTP(t *testing.T) {
	vaultHit := false
	vault := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { vaultHit = true }))
	t.Cleanup(vault.Close)
	t.Setenv("VAULT_ADDR", vault.URL)
	t.Setenv("VAULT_TOKEN", "vt")

	for _, encapiURL := range []string{"http://encapi.example", "", "encapi.example", "://bad"} {
		t.Setenv("ENCAPI_URL", encapiURL)
		err := run([]string{"encapic", "set-testing", "host.example"}, io.Discard)
		if err == nil || !strings.Contains(err.Error(), "requires an https ENCAPI_URL") {
			t.Errorf("ENCAPI_URL=%q: err = %v, want https refusal", encapiURL, err)
		}
	}
	if vaultHit {
		t.Error("vault called despite non-https ENCAPI_URL")
	}
}

func TestSetTestingVaultNoToken(t *testing.T) {
	for _, body := range []string{`{"data":{"token":""}}`, `{"data":{}}`, `{}`} {
		var seen seenPut
		t.Setenv("VAULT_ADDR", fakeVault(t, 200, body).URL)
		t.Setenv("VAULT_TOKEN", "vt")
		t.Setenv("ENCAPI_URL", fakeEncapi(t, 200, &seen).URL)

		err := run([]string{"encapic", "set-testing", "host.example"}, io.Discard)
		if err == nil || !strings.Contains(err.Error(), "no OIDC token") {
			t.Errorf("body %s: err = %v, want no OIDC token", body, err)
		}
		if seen.path != "" {
			t.Errorf("body %s: encapi called: %+v", body, seen)
		}
	}
}

func TestSetTestingNoVaultToken(t *testing.T) {
	var seen seenPut
	t.Setenv("HOME", t.TempDir())
	t.Setenv("VAULT_TOKEN", "")
	t.Setenv("VAULT_ADDR", fakeVault(t, 200, `{"data":{"token":"jwt123"}}`).URL)
	t.Setenv("ENCAPI_URL", fakeEncapi(t, 200, &seen).URL)

	err := run([]string{"encapic", "set-testing", "host.example"}, io.Discard)
	if err == nil || !strings.Contains(err.Error(), "no VAULT_TOKEN and cannot read ~/.vault-token") {
		t.Fatalf("err = %v, want missing vault token", err)
	}
	if seen.path != "" {
		t.Errorf("encapi called: %+v", seen)
	}
}

func TestVaultClientCACertUnreadable(t *testing.T) {
	t.Setenv("VAULT_CACERT", filepath.Join(t.TempDir(), "missing.pem"))
	if _, err := vaultClient(); err == nil || !strings.Contains(err.Error(), "read VAULT_CACERT") {
		t.Fatalf("err = %v, want read VAULT_CACERT", err)
	}
}

func TestVaultClientCACertNoPEM(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ca.pem")
	if err := os.WriteFile(path, []byte("not a certificate\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("VAULT_CACERT", path)
	if _, err := vaultClient(); err == nil || !strings.Contains(err.Error(), "no certificates in VAULT_CACERT") {
		t.Fatalf("err = %v, want no certificates", err)
	}
}

func TestVaultClientCACertTrustsServer(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	t.Cleanup(srv.Close)

	t.Setenv("VAULT_CACERT", "")
	plain, err := vaultClient()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := plain.Get(srv.URL); err == nil {
		t.Fatal("untrusted TLS server accepted without VAULT_CACERT")
	}

	trustServer(t, srv)
	client, err := vaultClient()
	if err != nil {
		t.Fatalf("vaultClient: %v", err)
	}
	resp, err := client.Get(srv.URL)
	if err != nil {
		t.Fatalf("GET with VAULT_CACERT: %v", err)
	}
	_ = resp.Body.Close()
}
