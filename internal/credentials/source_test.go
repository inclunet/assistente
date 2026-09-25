package credentials

import (
	"assistente/internal/database"
	"context"
	"errors"
	"fmt"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

func TestCredentialCommandHelper(t *testing.T) {
	if os.Getenv("ASSISTENTE_CREDENTIAL_TEST_HELPER") != "1" {
		return
	}
	mode := os.Args[len(os.Args)-1]
	switch mode {
	case "success":
		fmt.Print("  token-value\n")
	case "fail":
		fmt.Fprint(os.Stderr, "SECRET-STDERR")
		fmt.Print("SECRET-STDOUT")
		os.Exit(3)
	case "empty":
	case "multiline":
		fmt.Print("token\nsecond")
	case "large":
		fmt.Print(strings.Repeat("x", maxCommandOutput+1))
	case "sleep":
		time.Sleep(5 * time.Second)
	default:
		fmt.Print(mode)
	}
	os.Exit(0)
}

func commandTestConfig(t *testing.T, mode string) *SourceConfig {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	return &SourceConfig{Command: exe, Args: []string{"-test.run=^TestCredentialCommandHelper$", "--", mode}, TimeoutSeconds: 1}
}

func TestCommandSource(t *testing.T) {
	t.Setenv("ASSISTENTE_CREDENTIAL_TEST_HELPER", "1")
	for _, mode := range []string{"success", "fail", "empty", "multiline", "large", "sleep", "a b;$(command)"} {
		t.Run(mode, func(t *testing.T) {
			auth, err := ResolveSource(context.Background(), &AuthConfig{Source: "command", Type: "bearer", SourceConfig: commandTestConfig(t, mode)})
			if mode == "success" || mode == "a b;$(command)" {
				expected := mode
				if mode == "success" {
					expected = "token-value"
				}
				if err != nil || auth.Token != expected {
					t.Fatalf("result=%v err=%v", auth, err)
				}
			} else if err == nil {
				t.Fatal("expected error")
			} else if strings.Contains(err.Error(), "SECRET") {
				t.Fatal("secret in error")
			}
		})
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := ResolveSource(ctx, &AuthConfig{Source: "command", Type: "bearer", SourceConfig: commandTestConfig(t, "sleep")})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel: %v", err)
	}
}

func TestSourcePersistenceAndIsolation(t *testing.T) {
	setupScopedCredentialStoreTestDB(t)
	key := []byte("01234567890123456789012345678901")
	store := NewDBStore()
	mgr := NewManagerWithStoreAndPersistence(key, store, true)
	ctx := database.WithUserID(context.Background(), "source-user")
	t.Setenv("SOURCE_TEST_TOKEN", "first")
	for _, scheme := range []string{"bearer", "basic", "custom"} {
		auth := &AuthConfig{Source: "env", Type: scheme, Username: "user", SourceConfig: &SourceConfig{Env: "SOURCE_TEST_TOKEN"}}
		if scheme == "custom" {
			auth.Headers = map[string]string{"X-Token": ""}
		}
		if err := mgr.RegisterPatternWithContext(ctx, scheme+".example", auth); err != nil {
			t.Fatal(err)
		}
	}
	rows, err := store.ListCredentials(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range rows {
		if row.Auth.Source != "env" || row.Auth.SourceConfig != nil || row.Auth.SourceConfigEnc == "" || strings.Contains(row.Auth.SourceConfigEnc, "SOURCE_TEST_TOKEN") {
			t.Fatal("config not encrypted")
		}
		if row.Auth.Token != "" || row.Auth.Password != "" {
			t.Fatal("resolved value persisted")
		}
	}
	mgr = NewManagerWithStoreAndPersistence(key, store, true)
	if err := mgr.LoadUserCredentials(ctx, "source-user"); err != nil {
		t.Fatal(err)
	}
	for _, scheme := range []string{"bearer", "basic", "custom"} {
		auth, err := mgr.GetByPatternWithContext(ctx, scheme+".example")
		if err != nil || ResolveSecretFromAuth(auth) != "first" {
			t.Fatalf("scheme %s: %v", scheme, err)
		}
	}
	t.Setenv("SOURCE_TEST_TOKEN", "second")
	auth, err := mgr.GetByPatternWithContext(ctx, "bearer.example")
	if err != nil || auth.Token != "second" {
		t.Fatalf("refresh: %v", err)
	}
	auth, err = mgr.GetByPatternWithContext(database.WithUserID(context.Background(), "another-user"), "bearer.example")
	if err != nil || auth != nil {
		t.Fatal("cross-user leak")
	}
}

func TestListingDoesNotExecuteCommand(t *testing.T) {
	mgr := NewManager(nil)
	if err := mgr.RegisterPattern("example", &AuthConfig{Source: "command", Type: "bearer", SourceConfig: &SourceConfig{Command: "nonexistent-command"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := mgr.ListCredentials(); err != nil {
		t.Fatal(err)
	}
	if _, err := mgr.GetByPattern("example"); err == nil {
		t.Fatal("expected execution error")
	}
}

func TestOAuthSourceUnavailable(t *testing.T) {
	_, err := ResolveSource(context.Background(), &AuthConfig{Source: "oauth", Type: "bearer", SourceConfig: &SourceConfig{OAuth: &OAuthSourceConfig{Issuer: "https://example", ClientID: "client"}}})
	if !errors.Is(err, ErrOAuthSourceUnavailable) {
		t.Fatal(err)
	}
}

func TestAuthNoneRemovesRealHeaderWithoutMutatingCaller(t *testing.T) {
	capture := &captureTransport{}
	transport := &CredentialTransport{Base: capture, AuthMode: AuthNone}
	req := httptest.NewRequest("GET", "http://localhost", nil)
	req.Header.Set("Authorization", "Bearer real-token")
	if _, err := transport.RoundTrip(req); err != nil {
		t.Fatal(err)
	}
	if capture.captured.Header.Get("Authorization") != "" {
		t.Fatal("authorization leaked")
	}
	if req.Header.Get("Authorization") != "Bearer real-token" {
		t.Fatal("mutated caller request")
	}
}
