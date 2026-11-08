package main

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

const defaultVaultAddr = "https://vault.example.com:8200"

// setTesting pins certname to the testing environment via encapi, using a
// Vault identity OIDC token for the encapi-testing role.
func setTesting(certname string, out io.Writer) error {
	if _, err := encapiEnvironment(http.MethodPut, certname, "", `{"environment":"testing"}`); err != nil {
		return err
	}
	_, err := fmt.Fprintf(out, "%s set to testing\n", certname)
	return err
}

// unsetTesting restores certname to the environment it had before set-testing.
func unsetTesting(certname string, out io.Writer) error {
	body, err := encapiEnvironment(http.MethodDelete, certname, "/testing", "")
	if err != nil {
		return err
	}
	var node struct {
		Environment string `json:"environment"`
	}
	if err := json.Unmarshal(body, &node); err != nil {
		return fmt.Errorf("decode encapi node response: %w", err)
	}
	if node.Environment == "" {
		return errors.New("encapi returned no environment")
	}
	_, err = fmt.Fprintf(out, "%s restored to %s\n", certname, node.Environment)
	return err
}

// encapiEnvironment sends an authenticated request to the node's
// /environment<suffix> endpoint and returns the response body.
func encapiEnvironment(method, certname, suffix, body string) ([]byte, error) {
	baseURL := baseURL()
	if err := requireHTTPS("ENCAPI_URL", baseURL); err != nil {
		return nil, err
	}
	client, err := vaultClient()
	if err != nil {
		return nil, err
	}
	jwt, err := mintToken(client)
	if err != nil {
		return nil, err
	}

	u := strings.TrimRight(baseURL, "/") + "/api/v1/nodes/" + url.PathEscape(certname) + "/environment" + suffix
	req, err := http.NewRequest(method, u, strings.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+jwt)
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	return do(client, req)
}

func mintToken(client *http.Client) (string, error) {
	addr := os.Getenv("VAULT_ADDR")
	if addr == "" {
		addr = defaultVaultAddr
	}
	if err := requireHTTPS("VAULT_ADDR", addr); err != nil {
		return "", err
	}
	role := os.Getenv("ENCAPI_OIDC_ROLE")
	if role == "" {
		role = "encapi-testing"
	}
	vaultToken, err := readVaultToken()
	if err != nil {
		return "", err
	}
	req, err := http.NewRequest(http.MethodGet, strings.TrimRight(addr, "/")+"/v1/identity/oidc/token/"+url.PathEscape(role), nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("X-Vault-Token", vaultToken)
	body, err := do(client, req)
	if err != nil {
		return "", err
	}
	var resp struct {
		Data struct {
			Token string `json:"token"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return "", fmt.Errorf("decode vault token response: %w", err)
	}
	if resp.Data.Token == "" {
		return "", errors.New("vault returned no OIDC token")
	}
	return resp.Data.Token, nil
}

func requireHTTPS(name, raw string) error {
	if parsed, err := url.Parse(raw); err != nil || parsed.Scheme != "https" {
		return fmt.Errorf("encapic requires an https %s, got %q", name, raw)
	}
	return nil
}

func readVaultToken() (string, error) {
	if t := os.Getenv("VAULT_TOKEN"); t != "" {
		return t, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	b, err := os.ReadFile(filepath.Join(home, ".vault-token"))
	if err != nil {
		return "", fmt.Errorf("no VAULT_TOKEN and cannot read ~/.vault-token: %w", err)
	}
	return strings.TrimSpace(string(b)), nil
}

func vaultClient() (*http.Client, error) {
	client := &http.Client{
		Timeout: httpTimeout,
		// Never follow redirects: they would re-send the vault token or JWT.
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	caFile := os.Getenv("VAULT_CACERT")
	if caFile == "" {
		return client, nil
	}
	pem, err := os.ReadFile(caFile)
	if err != nil {
		return nil, fmt.Errorf("read VAULT_CACERT: %w", err)
	}
	pool, err := x509.SystemCertPool()
	if err != nil {
		pool = x509.NewCertPool()
	}
	if !pool.AppendCertsFromPEM(pem) {
		return nil, fmt.Errorf("no certificates in VAULT_CACERT %s", caFile)
	}
	client.Transport = &http.Transport{
		Proxy:           http.ProxyFromEnvironment,
		TLSClientConfig: &tls.Config{RootCAs: pool, MinVersion: tls.VersionTLS12},
	}
	return client, nil
}

func do(client *http.Client, req *http.Request) ([]byte, error) {
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%s %s: %w", req.Method, req.URL, err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response from %s: %w", req.URL, err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("%s %s returned HTTP %d: %s",
			req.Method, req.URL, resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return body, nil
}
