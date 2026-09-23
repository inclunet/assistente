package portability

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"assistente/internal/commandportability"
	"assistente/internal/database"
	"github.com/google/uuid"
)

func TestCommandLayersRoundTripNoEnvelope(t *testing.T) {
	layerID, err := uuid.NewV7()
	if err != nil {
		t.Fatal(err)
	}
	bindingID, err := uuid.NewV7()
	if err != nil {
		t.Fatal(err)
	}
	command := "workspace.tab.new"
	want := &ExportFile{
		Version: ExportVersion,
		Options: ExportOptions{},
		Resources: ExportResources{CommandLayers: []commandportability.LayerExport{{
			ID:                 layerID.String(),
			Scope:              commandportability.PortableScope{Kind: commandportability.GlobalScope},
			Name:               "Atalhos",
			Enabled:            true,
			ResolutionPriority: 1,
			Bindings: []commandportability.BindingExport{{
				ID:                 bindingID.String(),
				LayerRefKind:       "user",
				LayerRef:           layerID.String(),
				TriggerType:        "keyboard.local",
				TriggerSpec:        `{}`,
				CommandID:          &command,
				Arguments:          `{}`,
				Condition:          `{}`,
				Effect:             "execute",
				Enabled:            true,
				ResolutionPriority: 1,
				ReviewStatus:       "active",
				Presentation:       `{}`,
			}},
		}}},
	}
	raw, err := json.Marshal(want)
	if err != nil {
		t.Fatal(err)
	}
	got, unsupported, err := parseExportFile(string(raw))
	if err != nil {
		t.Fatal(err)
	}
	if len(unsupported) != 0 || len(got.Resources.CommandLayers) != 1 || got.Resources.CommandLayers[0].ID != layerID.String() || len(got.Resources.CommandLayers[0].Bindings) != 1 {
		t.Fatalf("commandLayers não fez round-trip: unsupported=%v got=%+v", unsupported, got.Resources.CommandLayers)
	}
}

func TestImportCommandLayersNaoFingeSucessoSemWriter(t *testing.T) {
	file := &ExportFile{Version: ExportVersion, Resources: ExportResources{CommandLayers: []commandportability.LayerExport{{ID: "01a0a4b1-191a-7354-9630-9eb5d9b0883a", Scope: commandportability.PortableScope{Kind: commandportability.GlobalScope}, Name: "Atalhos", Enabled: true}}}}
	raw, err := json.Marshal(file)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ImportConversationsWithContext(nil, string(raw), nil, ""); !errors.Is(err, commandportability.ErrUnsupported) { //nolint:staticcheck // A API legada aceita nil; isso não deve contornar a indisponibilidade do writer.
		t.Fatalf("import sem writer não foi bloqueado: %v", err)
	}
}

func TestPlanCommandLayersImportRecusaEnvelopeMisto(t *testing.T) {
	ctx := database.WithUserID(context.Background(), "plan-user")
	raw := `{"version":2,"options":{},"resources":{"commandLayers":[],"conversations":[]}}`
	if _, err := PlanCommandLayersImport(ctx, raw, commandportability.PlanOptions{}, nil, commandportability.ReferencePort{}); !errors.Is(err, commandportability.ErrInvalid) {
		t.Fatalf("envelope commandLayers vazio/misto não foi rejeitado: %v", err)
	}

	raw = `{"version":2,"options":{},"resources":{"commandLayers":[{"id":"01a0a4b1-191a-7354-9630-9eb5d9b0883a","scope":{"kind":"global"},"name":"Atalhos","enabled":true}],"conversations":[{"id":"conversation-forbidden"}]}}`
	if _, err := PlanCommandLayersImport(ctx, raw, commandportability.PlanOptions{}, nil, commandportability.ReferencePort{}); !errors.Is(err, commandportability.ErrUnsupported) {
		t.Fatalf("envelope misto foi aceito: %v", err)
	}
}

func TestExportCommandLayersWithContextRecusaWorkspaceAmbiguo(t *testing.T) {
	if _, err := ExportCommandLayersWithContext(context.Background(), commandportability.ReferencePort{}, nil, true); !errors.Is(err, commandportability.ErrWorkspaceResolution) {
		t.Fatalf("export workspace sem escopo exato não foi rejeitado: %v", err)
	}
}

func TestCanonicalVersionOneNormalizaParaAtualSemAlterarIDs(t *testing.T) {
	raw := `{
		"version": 1,
		"exportedAt": "2026-09-15T12:00:00Z",
		"appVersion": "baseline",
		"options": {},
		"resources": {
			"conversations": [{"id": "conversation-baseline", "title": "baseline", "messages": []}]
		}
	}`
	file, unsupported, err := parseExportFile(raw)
	if err != nil {
		t.Fatal(err)
	}
	if file.Version != ExportVersion || file.Resources.Conversations[0].ID != "conversation-baseline" || len(unsupported) != 0 {
		t.Fatalf("version 1 não normalizada sem preservar conteúdo: version=%d id=%q unsupported=%v", file.Version, file.Resources.Conversations[0].ID, unsupported)
	}
}

func TestCanonicalVersionFuturaContinuaRejeitada(t *testing.T) {
	raw := `{"version":999,"options":{},"resources":{"conversations":[]}}`
	if _, _, err := parseExportFile(raw); err == nil {
		t.Fatal("versão futura foi aceita")
	}
}
