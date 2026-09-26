package mcp

import (
	"assistente/internal/credentials"
	"assistente/internal/database"
	"assistente/internal/tools"
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestMCPAuthCommandHelper(t *testing.T) {
	if os.Getenv("MCP_AUTH_COMMAND_HELPER") != "1" {
		return
	}
	if err := os.WriteFile(os.Args[len(os.Args)-1], []byte("executed"), 0600); err != nil {
		os.Exit(2)
	}
	if _, err := os.Stdout.WriteString("token"); err != nil {
		os.Exit(3)
	}
	os.Exit(0)
}
func TestGetServerAuthInfoDoesNotExecuteSources(t *testing.T) {
	t.Setenv("MCP_AUTH_COMMAND_HELPER", "1")
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	for _, pattern := range []string{"example.com", clientCredPattern("test")} {
		t.Run(pattern, func(t *testing.T) {
			marker := filepath.Join(t.TempDir(), "command-executed")
			mgr := credentials.NewManager(nil)
			ctx := database.WithUserID(context.Background(), "auth-info-user")
			if err := mgr.RegisterPatternWithContext(ctx, pattern, &credentials.AuthConfig{Source: "command", Type: "bearer", SourceConfig: &credentials.SourceConfig{Command: executable, Args: []string{"-test.run=^TestMCPAuthCommandHelper$", "--", marker}}}); err != nil {
				t.Fatal(err)
			}
			m := NewManager(tools.NewRegistry(), mgr, func(string, any) {})
			m.SetAuthContextProvider(func() context.Context { return ctx })
			m.servers["test"] = &ServerStatus{Slug: "test", Config: ServerConfig{URL: "https://example.com/mcp"}}
			_, configured, err := m.GetServerAuthInfo("test")
			if err != nil || !configured {
				t.Fatalf("auth info: %v, %v", configured, err)
			}
			if _, err := os.Stat(marker); !os.IsNotExist(err) {
				t.Fatalf("metadata executed command: %v", err)
			}
			if _, err := mgr.GetByPatternWithContext(ctx, pattern); err != nil {
				t.Fatal(err)
			}
			if _, err := os.Stat(marker); err != nil {
				t.Fatal(err)
			}
		})
	}
}
