package main

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func fakeEncapiDelete(t *testing.T, status int, body string, seen *seenPut) *httptest.Server {
	t.Helper()
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*seen = seenPut{r.Method + " " + r.URL.Path, r.Header.Get("Authorization"), ""}
		w.WriteHeader(status)
		_, _ = io.WriteString(w, body)
	}))
	t.Cleanup(srv.Close)
	trustServer(t, srv)
	return srv
}

func runUnset(t *testing.T, status int, body string) (string, seenPut, error) {
	t.Helper()
	var seen seenPut
	t.Setenv("VAULT_ADDR", fakeVault(t, 200, `{"data":{"token":"jwt123"}}`).URL)
	t.Setenv("VAULT_TOKEN", "vt")
	t.Setenv("ENCAPI_URL", fakeEncapiDelete(t, status, body, &seen).URL)
	var out bytes.Buffer
	err := run([]string{"encapic", "unset-testing", "host.example"}, &out)
	return out.String(), seen, err
}

func TestUnsetTestingSuccess(t *testing.T) {
	out, seen, err := runUnset(t, 200, `{"certname":"host.example","environment":"production"}`)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	want := seenPut{"DELETE /api/v1/nodes/host.example/environment/testing", "Bearer jwt123", ""}
	if seen != want {
		t.Errorf("request = %+v, want %+v", seen, want)
	}
	if out != "host.example restored to production\n" {
		t.Errorf("out = %q", out)
	}
}

func TestUnsetTestingErrors(t *testing.T) {
	for _, tc := range []struct {
		status     int
		body, want string
	}{
		{409, `{"error":"node is not in testing"}`, "HTTP 409: {\"error\":\"node is not in testing\"}"},
		{404, `{"error":"node not found"}`, "HTTP 404"},
		{200, `{"certname":"host.example"}`, "no environment"},
		{200, `not json`, "decode encapi node response"},
	} {
		out, _, err := runUnset(t, tc.status, tc.body)
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%d %s: err = %v, want %q", tc.status, tc.body, err, tc.want)
		}
		if out != "" {
			t.Errorf("%d %s: out = %q, want none", tc.status, tc.body, out)
		}
	}
}

func TestUnsetTestingRefusesPlainHTTP(t *testing.T) {
	t.Setenv("ENCAPI_URL", "http://encapi.example")
	err := run([]string{"encapic", "unset-testing", "host.example"}, io.Discard)
	if err == nil || !strings.Contains(err.Error(), "requires an https ENCAPI_URL") {
		t.Fatalf("err = %v, want https refusal", err)
	}
}
