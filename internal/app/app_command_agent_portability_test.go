package app

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"assistente/internal/commandconfig"
	"assistente/internal/commanddecision"
	"assistente/internal/database"
	"assistente/internal/portability"
	commandtool "assistente/internal/tools/command"
)

func TestCommandAgentPortabilityScopeAndConfirmedRoundTrip(t *testing.T) {
	a, decisions := settingsSecurityFixture(t)
	ctx := commandAgentTestContext(t, a, commandtool.ConfigName)
	b := commandAgentTools{app: a}
	workspace := a.commandProduct.Load().workspaceID
	now := time.Now().UTC()
	layers := []commandconfig.Layer{
		{ID: appCommandPortabilityUUID(t), UserID: a.currentUserID, Name: "agent portable global", Enabled: true, Source: "user", CreatedAt: now, UpdatedAt: now},
		{ID: appCommandPortabilityUUID(t), UserID: a.currentUserID, WorkspaceID: &workspace, Name: "agent portable workspace", Enabled: true, Source: "user", CreatedAt: now, UpdatedAt: now},
	}
	if err := database.DB().Create(&layers).Error; err != nil {
		t.Fatal(err)
	}
	for _, scope := range []string{"global", "workspace"} {
		value, err := b.Config(ctx, commandtool.Request{Action: "config_export", Scope: scope})
		if err != nil {
			t.Fatal(err)
		}
		file, ok := value.(portability.ExportFile)
		if !ok || len(file.Resources.CommandLayers) != 1 || string(file.Resources.CommandLayers[0].Scope.Kind) != scope {
			t.Fatalf("wrong scope export: %+v", value)
		}
		raw, err := json.Marshal(file)
		if err != nil {
			t.Fatal(err)
		}
		for _, forbidden := range []string{"authorizationDecisionId", "grantFingerprint", "claims", "userId", "sessionId"} {
			if strings.Contains(string(raw), forbidden) {
				t.Fatalf("nonportable field %q leaked", forbidden)
			}
		}
		if scope != "global" {
			continue
		}
		payload, err := json.Marshal(struct {
			JSONData    string                         `json:"jsonData"`
			Resolutions []portability.ImportResolution `json:"resolutions"`
		}{
			string(raw), []portability.ImportResolution{
				{ResourceType: "commandLayers", Identifier: "*", Strategy: portability.ConflictResolutionRename},
				{ResourceType: "commandLayerName", Identifier: layers[0].ID, Strategy: portability.ConflictResolutionRename, RenameValue: "agent confirmed copy"},
			},
		})
		if err != nil {
			t.Fatal(err)
		}
		type outcome struct {
			value any
			err   error
		}
		done := make(chan outcome, 1)
		go func() {
			value, err := b.Config(ctx, commandtool.Request{Action: "config_import", Scope: "global", Payload: payload})
			done <- outcome{value, err}
		}()
		appCommandImportWailsRespond(t, a, decisions, commanddecision.ApplyAction)
		select {
		case result := <-done:
			if result.err != nil {
				t.Fatal(result.err)
			}
			imported, ok := result.value.(*portability.ImportResult)
			if !ok || !imported.Success || imported.Imported != 1 {
				t.Fatalf("import failed: %+v", result.value)
			}
		case <-a.ctx.Done():
			t.Fatal(a.ctx.Err())
		}
		var count int64
		if err := database.DB().Model(&commandconfig.Layer{}).Where("user_id = ? AND name = ?", a.currentUserID, "agent confirmed copy").Count(&count).Error; err != nil || count != 1 {
			t.Fatalf("copy notpersisted: count=%d err=%v", count, err)
		}
		if _, err := b.Config(ctx, commandtool.Request{Action: "config_import", Scope: "workspace", Payload: payload}); err == nil {
			t.Fatal("global import escaped selected workspace scope")
		}
		file.Resources.CommandLayers[0].Name = "agent replaced original"
		changed, err := json.Marshal(file)
		if err != nil {
			t.Fatal(err)
		}
		for _, strategy := range []portability.ConflictResolutionStrategy{portability.ConflictResolutionSkip, portability.ConflictResolutionOverwrite} {
			request := struct {
				JSONData    string                         `json:"jsonData"`
				Resolutions []portability.ImportResolution `json:"resolutions"`
			}{string(changed), []portability.ImportResolution{{ResourceType: "commandLayers", Identifier: "*", Strategy: strategy}}}
			payload, err := json.Marshal(request)
			if err != nil {
				t.Fatal(err)
			}
			done := make(chan outcome, 1)
			go func() {
				value, err := b.Config(ctx, commandtool.Request{Action: "config_import", Scope: "global", Payload: payload})
				done <- outcome{value, err}
			}()
			if strategy == portability.ConflictResolutionOverwrite {
				appCommandImportWailsRespond(t, a, decisions, commanddecision.ApplyAction)
			}
			select {
			case result := <-done:
				if result.err != nil {
					t.Fatal(result.err)
				}
				if imported, ok := result.value.(*portability.ImportResult); !ok || !imported.Success {
					t.Fatalf("%s result: %+v", strategy, result.value)
				}
			case <-a.ctx.Done():
				t.Fatal(a.ctx.Err())
			}
			var actual commandconfig.Layer
			if err := database.DB().First(&actual, "id = ?", layers[0].ID).Error; err != nil {
				t.Fatal(err)
			}
			want := layers[0].Name
			if strategy == portability.ConflictResolutionOverwrite {
				want = "agent replaced original"
			}
			if actual.Name != want {
				t.Fatalf("%s applied wrong name %q; want %q", strategy, actual.Name, want)
			}
			select {
			case extra := <-decisions:
				t.Fatalf("unexpected extra decision: %+v", extra)
			default:
			}
		}
	}
	for _, payload := range []string{`{"includeCredentials":true}`, `{"credentialExportPassword":"secret"}`, `{"owner":"other"}`} {
		if _, err := b.Config(ctx, commandtool.Request{Action: "config_export", Scope: "global", Payload: json.RawMessage(payload)}); err == nil {
			t.Fatalf("unsafe export accepted: %s", payload)
		}
	}
}
