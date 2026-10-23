package main

import (
	"bytes"
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
	client, err := vaultClient()
	if err != nil {
		return err
	}
	jwt, err := mintToken(client)
	if err != nil {
		return err
	}

	baseURL := os.Getenv("ENCAPI_URL")
	if baseURL == "" {
		baseURL = defaultBaseURL
	}
	u := strings.TrimRight(baseURL, "/") + "/api/v1/nodes/" + url.PathEscape(certname) + "/environment"
	req, err := http.NewRequest(http.MethodPut, u, bytes.NewBufferString(`{"environment":"testing"}`))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+jwt)
	req.Header.Set("Content-Type", "application/json")
	if _, err := do(client, req); err != nil {
		return err
	}
	_, err = fmt.Fprintf(out, "%s set to testing\n", certname)
	return err
}

func mintToken(client *http.Client) (string, error) {
	addr := os.Getenv("VAULT_ADDR")
	if addr == "" {
		addr = defaultVaultAddr
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
	client := &http.Client{Timeout: httpTimeout}
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
