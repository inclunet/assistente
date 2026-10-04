package wailsapi

import (
	"assistente/controllers"
	"assistente/internal/apidto"
	mcpmgr "assistente/internal/mcp"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestMCPNotWired(t *testing.T) {
	t.Parallel()
	api := NewMCP()
	if err := api.SaveMCPServerWithCredential("test", mcpmgr.ServerConfig{}, apidto.CredentialInput{}); !errors.Is(err, ErrMCPNotWired) {
		t.Fatalf("SaveMCPServerWithCredential: %v", err)
	}
	if _, err := api.InspectMCPOAuthInventory(); !errors.Is(err, ErrMCPNotWired) {
		t.Fatalf("InspectMCPOAuthInventory: got %v", err)
	}
	if _, err := api.ListMCPServers(); !errors.Is(err, ErrMCPNotWired) {
		t.Fatalf("ListMCPServers: got %v", err)
	}
	if err := api.ConnectMCPServer("x"); !errors.Is(err, ErrMCPNotWired) {
		t.Fatalf("ConnectMCPServer: got %v", err)
	}
	if _, err := api.GetMCPServerAuthInfo("x"); !errors.Is(err, ErrMCPNotWired) {
		t.Fatalf("GetMCPServerAuthInfo: got %v", err)
	}
	if _, err := api.DiscoverMCPServerAuth("https://example.com"); !errors.Is(err, ErrMCPNotWired) {
		t.Fatalf("DiscoverMCPServerAuth: got %v", err)
	}
}

func TestMCPOAuthInventoryRequiresSession(t *testing.T) {
	want := errors.New("no session")
	api := NewMCP()
	AttachMCP(api, stubSession{err: want}, controllers.NewMCPController(nil, nil, nil))
	if _, err := api.InspectMCPOAuthInventory(); !errors.Is(err, want) {
		t.Fatalf("got %v", err)
	}
}

func TestMCPOAuthSnapshotsRequireSession(t *testing.T) {
	want := errors.New("no session")
	api := NewMCP()
	AttachMCP(api, stubSession{err: want}, controllers.NewMCPController(nil, nil, nil))
	if err := api.ReconnectMCPOAuthSnapshot("snapshot", "none"); !errors.Is(err, want) {
		t.Fatalf("reconnect: %v", err)
	}
	if err := api.ConvertMCPOAuthClientSnapshot("snapshot", "client_secret_post"); !errors.Is(err, want) {
		t.Fatalf("convert: %v", err)
	}
	if _, err := api.ListMCPOAuthSnapshots(); !errors.Is(err, want) {
		t.Fatalf("list: %v", err)
	}
	if _, err := api.CreateMCPOAuthSnapshot("consumer"); !errors.Is(err, want) {
		t.Fatalf("create: %v", err)
	}
	if err := api.RestoreMCPOAuthSnapshot("snapshot"); !errors.Is(err, want) {
		t.Fatalf("restore: %v", err)
	}
	if err := api.DiscardMCPOAuthSnapshot("snapshot", true); !errors.Is(err, want) {
		t.Fatalf("discard: %v", err)
	}
}

func TestMCPUsesWithUserNotRequireAuth(t *testing.T) {
	t.Parallel()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller")
	}
	src, err := os.ReadFile(filepath.Join(filepath.Dir(thisFile), "mcp.go"))
	if err != nil {
		t.Fatal(err)
	}
	body := string(src)
	if strings.Contains(body, "requireAuthenticatedContext(") {
		t.Fatal("mcp.go não deve chamar requireAuthenticatedContext(; use WithUser")
	}
	if !strings.Contains(body, "WithUser(") {
		t.Fatal("mcp.go deve chamar WithUser(")
	}
}

func TestMCPCredentialSaveRequiresSession(t *testing.T) {
	want := errors.New("no session")
	api := NewMCP()
	AttachMCP(api, stubSession{err: want}, controllers.NewMCPController(nil, nil, nil))
	if err := api.SaveMCPServerWithCredential("test", mcpmgr.ServerConfig{}, apidto.CredentialInput{}); !errors.Is(err, want) {
		t.Fatalf("got %v", err)
	}
}
