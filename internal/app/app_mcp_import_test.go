package app

import (
	"context"
	"encoding/json"
	"testing"

	"assistente/internal/credentials"
	"assistente/internal/database"
	"assistente/internal/mcp"
	"assistente/internal/portability"
)

func TestMCPImportPublishesDataFacadeWithoutRestart(t *testing.T) {
	for _, mode := range []string{"data", "resolutions", "partial", "historical"} {
		t.Run(mode, func(t *testing.T) {
			a := readyCommandProduct(t)
			if err := database.DB().AutoMigrate(&database.MCPServer{}, &database.CredentialEntry{}); err != nil {
				t.Fatal(err)
			}
			a.credMgr = credentials.NewManagerWithStoreAndPersistence([]byte("01234567890123456789012345678901"), credentials.NewDBStore(), true)
			a.mcpMgr = mcp.NewManager(nil, a.credMgr, nil)
			a.mcpMgr.SetRepository(mcp.NewDBRepository(database.DB()))
			ctx := database.WithUserID(context.Background(), a.currentUserID)
			a.mcpMgr.SetAuthContextProvider(func() context.Context { return ctx })
			t.Cleanup(a.mcpMgr.CloseAll)
			a.wireExportImport()
			payload := `{"mcpServers":{"remote":{"url":"https://unreachable.invalid/mcp"}}}`
			if mode == "partial" {
				payload = `{"mcpServers":{"remote":{"url":"https://unreachable.invalid/mcp"},"broken":{}}}`
			}
			if mode == "historical" {
				raw, err := json.Marshal(portability.ExportFile{Version: portability.ExportVersion, Resources: portability.ExportResources{MCPServers: []portability.MCPServerExport{{Slug: "remote", URL: "https://unreachable.invalid/mcp", Transport: "streamable", AuthType: "oauth2_pkce", OAuth2ClientID: "historical-client", Enabled: true, AutoConnect: true}}}})
				if err != nil {
					t.Fatal(err)
				}
				payload = string(raw)
			}
			var result *portability.ImportResult
			var err error
			if mode == "resolutions" {
				result, err = a.exportImportAPI.ImportDataWithResolutions(portability.ImportRequest{JSONData: payload})
			} else {
				result, err = a.exportImportAPI.ImportData(payload, "")
			}
			warnings := 0
			if mode == "historical" {
				warnings = 1
			}
			if err != nil || result.Imported != 1 || len(result.Warnings) != warnings {
				t.Fatalf("import: %v %+v", err, result)
			}
			if mode == "partial" && result.Failed != 1 {
				t.Fatal("missing partial failure")
			}
			servers := a.mcpMgr.List()
			if len(servers) != 1 || servers[0].Slug != "remote" || servers[0].Status != mcp.StatusDisconnected || servers[0].AutoConnect {
				t.Fatal("runtime did not publish pending import without connecting")
			}
			cfg, err := a.mcpMgr.GetConfig("remote")
			if mode == "historical" {
				if err != nil || cfg.Enabled || cfg.AutoConnect || cfg.OAuthManaged || cfg.OAuth2ClientID != "historical-client" || result.Warnings[0].Code != "mcpServer.oauthRecoveryRequired" {
					t.Fatal("historical import was not staged for recovery", err)
				}
				a.mcpMgr.AutoConnectAll(ctx)
				if a.mcpMgr.List()[0].Status != mcp.StatusDisconnected {
					t.Fatal("historical import connected automatically")
				}
				return
			}
			if err != nil || cfg.OAuthAuthorizationID == "" {
				t.Fatal("authorization unavailable", err)
			}
			a.setCurrentUserID("another-user")
			if err := a.reloadMCPAfterImport(ctx); err == nil {
				t.Fatal("old session published runtime")
			}
		})
	}
}
