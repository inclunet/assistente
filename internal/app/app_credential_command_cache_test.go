package app

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"assistente/internal/credentials"
	"assistente/internal/database"
	"github.com/google/uuid"
)

func TestCommandCacheSessionHelper(t *testing.T) {
	if os.Getenv("ASSISTENTE_COMMAND_CACHE_SESSION_HELPER") != "1" {
		return
	}
	fmt.Print(uuid.NewString())
	os.Exit(0)
}

func TestAuthSessionTransitionClearsCommandCredentialCache(t *testing.T) {
	t.Setenv("ASSISTENTE_COMMAND_CACHE_SESSION_HELPER", "1")
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	m := credentials.NewManager(nil)
	a := &App{credMgr: m}
	a.setCurrentUserID("cache-user")
	ctx := database.WithUserID(context.Background(), "cache-user")
	if err := m.RegisterPatternWithContext(ctx, "cache.example", &credentials.AuthConfig{Source: "command", Type: "bearer", SourceConfig: &credentials.SourceConfig{Command: exe, Args: []string{"-test.run=^TestCommandCacheSessionHelper$"}}}); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = fmt.Fprint(w, r.Header.Get("Authorization")) }))
	defer server.Close()
	client := credentials.NewHTTPClient(m, "cache.example", 0)
	get := func() string {
		t.Helper()
		req, _ := http.NewRequestWithContext(ctx, "GET", server.URL, nil)
		response, e := client.Do(req)
		if e != nil {
			t.Fatal(e)
		}
		defer func() { _ = response.Body.Close() }()
		body, e := io.ReadAll(response.Body)
		if e != nil {
			t.Fatal(e)
		}
		return string(body)
	}
	first := get()
	if get() != first {
		t.Fatal("token não reutilizado")
	}
	a.setCurrentUserID("")
	second := get()
	if second == first {
		t.Fatal("logout reteve token")
	}
	a.setCurrentUserID("another-user")
	if get() == second {
		t.Fatal("troca de usuário reteve token")
	}
}
