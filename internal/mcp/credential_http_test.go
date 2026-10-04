package mcp

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"assistente/internal/credentials"
	"assistente/internal/database"
)

func TestMCPCredentialCommandHelper(t *testing.T) {
	if os.Getenv("MCP_CREDENTIAL_HELPER") != "1" {
		return
	}
	file, err := os.OpenFile(os.Getenv("MCP_CREDENTIAL_CALLS"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		os.Exit(2)
	}
	_, _ = file.WriteString("call\n")
	_ = file.Close()
	value, err := os.ReadFile(os.Getenv("MCP_CREDENTIAL_VALUE"))
	if err != nil {
		os.Exit(3)
	}
	fmt.Print(string(value))
	os.Exit(0)
}

func TestMCPCredentialTransportResolvesEnvPerRequest(t *testing.T) {
	for _, scheme := range []AuthType{AuthBearer, AuthBasic} {
		t.Run(string(scheme), func(t *testing.T) {
			t.Setenv("MCP_TEST_SECRET", "first")
			manager := &Manager{credMgr: newTestCredMgr()}
			auth := &credentials.AuthConfig{Type: string(scheme), Source: "env", Username: "user", SourceConfig: &credentials.SourceConfig{Env: "MCP_TEST_SECRET"}}
			if err := manager.credMgr.RegisterPattern("127.0.0.1", auth); err != nil {
				t.Fatal(err)
			}
			expected := "first"
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if scheme == AuthBearer {
					if r.Header.Get("Authorization") != "Bearer "+expected {
						t.Error("stale bearer")
					}
				} else {
					user, password, ok := r.BasicAuth()
					if !ok || user != "user" || password != expected {
						t.Error("stale basic")
					}
				}
				w.WriteHeader(http.StatusOK)
			}))
			defer server.Close()
			client := manager.buildAuthHTTPClient(context.Background(), "test", ServerConfig{URL: server.URL, AuthType: scheme})
			for _, value := range []string{"first", "second"} {
				t.Setenv("MCP_TEST_SECRET", value)
				expected = value
				response, err := client.Get(server.URL)
				if err != nil {
					t.Fatal(err)
				}
				_ = response.Body.Close()
			}
			if err := manager.credMgr.DeletePattern(context.Background(), "127.0.0.1"); err != nil {
				t.Fatal(err)
			}
			if _, err := client.Get(server.URL); !errors.Is(err, credentials.ErrCredentialResolution) {
				t.Fatalf("missing credential: %v", err)
			}
		})
	}
}

func TestMCPCredentialCommandCacheAndOrigin(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	calls := filepath.Join(dir, "calls")
	value := filepath.Join(dir, "value")
	t.Setenv("MCP_CREDENTIAL_HELPER", "1")
	t.Setenv("MCP_CREDENTIAL_CALLS", calls)
	t.Setenv("MCP_CREDENTIAL_VALUE", value)
	if err := os.WriteFile(value, []byte("first"), 0600); err != nil {
		t.Fatal(err)
	}
	owner := "owner"
	manager := &Manager{credMgr: newTestCredMgr(), authContext: func() context.Context { return database.WithUserID(context.Background(), owner) }}
	auth := &credentials.AuthConfig{Type: "bearer", Source: "command", SourceConfig: &credentials.SourceConfig{Command: exe, Args: []string{"-test.run=^TestMCPCredentialCommandHelper$"}, TimeoutSeconds: 30}}
	if err := manager.credMgr.RegisterPatternWithContext(manager.credentialContext(), "127.0.0.1", auth); err != nil {
		t.Fatal(err)
	}
	expected := "first"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+expected {
			w.WriteHeader(401)
			return
		}
		w.WriteHeader(200)
	}))
	defer server.Close()
	client := manager.buildAuthHTTPClient(context.Background(), "test", ServerConfig{URL: server.URL, AuthType: AuthBearer})
	count := func() int { data, _ := os.ReadFile(calls); return strings.Count(string(data), "call\n") }
	if count() != 0 {
		t.Fatal("building client executed command")
	}
	for i := 0; i < 2; i++ {
		resp, err := client.Get(server.URL)
		if err != nil {
			t.Fatal(err)
		}
		_ = resp.Body.Close()
		if resp.StatusCode != 200 {
			t.Fatal(resp.StatusCode)
		}
	}
	if count() != 1 {
		t.Fatalf("cache calls: %d", count())
	}
	expected = "second"
	if err := os.WriteFile(value, []byte(expected), 0600); err != nil {
		t.Fatal(err)
	}
	resp, err := client.Get(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != 200 || count() != 2 {
		t.Fatalf("renewal status=%d calls=%d", resp.StatusCode, count())
	}
	leaked := false
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { leaked = true }))
	defer other.Close()
	if _, err = client.Get(other.URL); !errors.Is(err, credentials.ErrCredentialResolution) || leaked || count() != 2 {
		t.Fatalf("foreign destination: %v", err)
	}
	owner = "other"
	if _, err = client.Get(server.URL); !errors.Is(err, credentials.ErrCredentialResolution) || count() != 2 {
		t.Fatalf("foreign user: %v", err)
	}
}

func TestMCPCredentialRedirectKeepsOrigin(t *testing.T) {
	leaked := false
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { leaked = true }))
	defer other.Close()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/external" {
			http.Redirect(w, r, other.URL, http.StatusTemporaryRedirect)
			return
		}
		if r.URL.Path == "/local" {
			http.Redirect(w, r, "/ok", http.StatusTemporaryRedirect)
			return
		}
		if r.Header.Get("Authorization") != "Bearer token" {
			t.Error("missing auth")
		}
		_, _ = io.WriteString(w, "ok")
	}))
	defer server.Close()
	manager := &Manager{credMgr: newTestCredMgr()}
	if err := manager.credMgr.RegisterPattern("127.0.0.1", &credentials.AuthConfig{Type: "bearer", Source: "static", Token: "token"}); err != nil {
		t.Fatal(err)
	}
	client := manager.buildAuthHTTPClient(context.Background(), "test", ServerConfig{URL: server.URL, AuthType: AuthBearer})
	resp, err := client.Get(server.URL + "/local")
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	resp, err = client.Get(server.URL + "/external")
	if resp != nil {
		_ = resp.Body.Close()
	}
	if !errors.Is(err, credentials.ErrCredentialResolution) || leaked {
		t.Fatalf("redirect: %v", err)
	}
}

func TestMCPCredentialRejectsIncompatibleScheme(t *testing.T) {
	for _, scheme := range []AuthType{AuthBearer, AuthBasic} {
		t.Run(string(scheme), func(t *testing.T) {
			manager := &Manager{credMgr: newTestCredMgr()}
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++; w.WriteHeader(http.StatusOK) }))
			defer server.Close()
			client := manager.buildAuthHTTPClient(context.Background(), "test", ServerConfig{URL: server.URL, AuthType: scheme})
			for _, actual := range []string{"bearer", "basic", "custom"} {
				if err := manager.credMgr.RegisterPattern("127.0.0.1", &credentials.AuthConfig{Source: "static", Type: actual, Token: "secret", Username: "user", Password: "secret", Headers: map[string]string{"X-Key": "secret"}}); err != nil {
					t.Fatal(err)
				}
				before := calls
				response, err := client.Get(server.URL)
				if actual == string(scheme) {
					if err != nil {
						t.Fatal(err)
					}
					_ = response.Body.Close()
					if calls != before+1 {
						t.Fatal("compatible credential was not sent")
					}
				} else if !errors.Is(err, credentials.ErrCredentialResolution) || calls != before {
					t.Fatalf("scheme %s sent for %s: calls=%d err=%v", actual, scheme, calls-before, err)
				}
			}
		})
	}
}

func TestMCPCredentialPreservesBearerNormalization(t *testing.T) {
	for _, value := range []string{" token ", " Bearer token ", " bearer token ", " bEaReR token "} {
		t.Run(value, func(t *testing.T) {
			manager := &Manager{credMgr: newTestCredMgr()}
			if err := manager.credMgr.RegisterPattern("example.com", &credentials.AuthConfig{Source: "static", Type: "bearer", Token: value}); err != nil {
				t.Fatal(err)
			}
			client := manager.buildAuthHTTPClient(context.Background(), "test", ServerConfig{URL: "https://example.com/mcp", AuthType: AuthBearer})
			calls := 0
			client.Transport.(*mcpCredentialTransport).base = roundTripFunc(func(req *http.Request) (*http.Response, error) {
				calls++
				if !strings.EqualFold(req.Header.Get("Authorization"), "Bearer token") {
					t.Fatalf("unexpected Authorization %q", req.Header.Get("Authorization"))
				}
				return &http.Response{StatusCode: http.StatusOK, Body: http.NoBody, Header: make(http.Header)}, nil
			})
			response, err := client.Get("https://example.com/mcp")
			if err != nil {
				t.Fatal(err)
			}
			_ = response.Body.Close()
			if calls != 1 {
				t.Fatal("request not sent")
			}
		})
	}
}
