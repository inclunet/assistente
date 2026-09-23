package portability

import (
	"encoding/json"
	"errors"
	"testing"

	cp "assistente/internal/commandportability"
)

const commandImportTestLayerID = "01926b90-7a5a-7c4e-8d3f-000000000001"

func commandImportTestEnvelope(t *testing.T, layers []cp.LayerExport) string {
	t.Helper()
	raw, err := json.Marshal(ExportFile{
		Version: ExportVersion,
		Options: ExportOptions{},
		Resources: ExportResources{
			CommandLayers: layers,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func commandImportTestLayer(id, workspace string) cp.LayerExport {
	scope := cp.PortableScope{Kind: cp.GlobalScope}
	if workspace != "" {
		scope = cp.PortableScope{Kind: cp.WorkspaceScope, WorkspaceID: workspace}
	}
	return cp.LayerExport{ID: id, Scope: scope, Name: "Camada", Enabled: true}
}

func TestCommandImportOptionsConverteResolucaoCompleta(t *testing.T) {
	const sourceWorkspace = "workspace-origem"
	raw := commandImportTestEnvelope(t, []cp.LayerExport{commandImportTestLayer(commandImportTestLayerID, sourceWorkspace)})
	options, err := CommandImportOptions(ImportRequest{
		JSONData: raw,
		Resolutions: []ImportResolution{
			{ResourceType: "commandLayers", Identifier: "*", Strategy: ConflictResolutionRename},
			{ResourceType: "commandLayerName", Identifier: commandImportTestLayerID, Strategy: ConflictResolutionRename, RenameValue: "Camada nova"},
			{ResourceType: "commandWorkspace", Identifier: sourceWorkspace, Strategy: ConflictResolutionRename, RenameValue: "workspace-destino"},
		},
	})
	if err != nil {
		t.Fatalf("CommandImportOptions: %v", err)
	}
	if options.Mode != cp.CopyMode || options.RenameByLayer[commandImportTestLayerID] != "Camada nova" || options.WorkspaceMap[sourceWorkspace] != "workspace-destino" {
		t.Fatalf("opções convertidas incorretamente: %+v", options)
	}
}

func TestCommandImportOptionsRejeitaOpcoesInvalidas(t *testing.T) {
	const sourceWorkspace = "workspace-origem"
	raw := commandImportTestEnvelope(t, []cp.LayerExport{commandImportTestLayer(commandImportTestLayerID, sourceWorkspace)})
	tests := map[string]ImportRequest{
		"sem decisão global": {JSONData: raw},
		"senha": {
			JSONData:                 raw,
			CredentialExportPassword: "não pode",
			Resolutions: []ImportResolution{{ResourceType: "commandLayers", Identifier: "*", Strategy: ConflictResolutionSkip}},
		},
		"entrada desconhecida": {
			JSONData: raw,
			Resolutions: []ImportResolution{
				{ResourceType: "outro", Identifier: "*", Strategy: ConflictResolutionSkip},
				{ResourceType: "commandLayers", Identifier: "*", Strategy: ConflictResolutionSkip},
			},
		},
		"identifier fora do arquivo": {
			JSONData: raw,
			Resolutions: []ImportResolution{
				{ResourceType: "commandLayers", Identifier: "*", Strategy: ConflictResolutionSkip},
				{ResourceType: "commandLayerName", Identifier: "missing-layer", Strategy: ConflictResolutionRename, RenameValue: "Novo"},
			},
		},
		"workspace fora do arquivo": {
			JSONData: raw,
			Resolutions: []ImportResolution{
				{ResourceType: "commandLayers", Identifier: "*", Strategy: ConflictResolutionSkip},
				{ResourceType: "commandWorkspace", Identifier: "missing-workspace", Strategy: ConflictResolutionRename, RenameValue: "destino"},
			},
		},
		"duplicada": {
			JSONData: raw,
			Resolutions: []ImportResolution{
				{ResourceType: "commandLayers", Identifier: "*", Strategy: ConflictResolutionSkip},
				{ResourceType: "commandLayers", Identifier: "*", Strategy: ConflictResolutionSkip},
			},
		},
		"estratégia de nome incorreta": {
			JSONData: raw,
			Resolutions: []ImportResolution{
				{ResourceType: "commandLayers", Identifier: "*", Strategy: ConflictResolutionSkip},
				{ResourceType: "commandLayerName", Identifier: commandImportTestLayerID, Strategy: ConflictResolutionSkip, RenameValue: "Novo"},
			},
		},
	}
	for name, req := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := CommandImportOptions(req); !errors.Is(err, cp.ErrInvalid) {
				t.Fatalf("erro=%v, want cp.ErrInvalid", err)
			}
		})
	}
}

func TestCommandImportOptionsRejeitaMaisDe64Camadas(t *testing.T) {
	layers := make([]cp.LayerExport, 65)
	for i := range layers {
		layers[i] = commandImportTestLayer("01926b90-7a5a-7c4e-8d3f-"+formatCommandImportTestID(i+1), "")
	}
	raw := commandImportTestEnvelope(t, layers)
	if _, err := CommandImportOptions(ImportRequest{
		JSONData:    raw,
		Resolutions: []ImportResolution{{ResourceType: "commandLayers", Identifier: "*", Strategy: ConflictResolutionSkip}},
	}); !errors.Is(err, cp.ErrInvalid) {
		t.Fatalf("limite de camadas: erro=%v, want cp.ErrInvalid", err)
	}
}

func TestParseCommandImportEnvelopeRejeitaEnvelopeMistoEDuplicata(t *testing.T) {
	mixed := `{"version":2,"options":{},"resources":{"commandLayers":[{"id":"` + commandImportTestLayerID + `","scope":{"kind":"global"},"name":"Camada","enabled":true}],"conversations":[]}}`
	if _, err := ParseCommandImportEnvelope([]byte(mixed)); !errors.Is(err, cp.ErrUnsupported) {
		t.Fatalf("envelope misto: erro=%v, want cp.ErrUnsupported", err)
	}
	duplicate := `{"version":2,"options":{},"resources":{"commandLayers":[{"id":"` + commandImportTestLayerID + `","scope":{"kind":"global"},"name":"Camada","enabled":true}],"commandLayers":[]}}`
	if _, err := ParseCommandImportEnvelope([]byte(duplicate)); err == nil {
		t.Fatal("chave commandLayers duplicada foi aceita")
	}
}

func TestHasCommandLayersPercorreResourcesDuplicadosECases(t *testing.T) {
	raw := `{"Resources":{"CommandLayers":[]},"resources":{"conversations":[]}}`
	if !HasCommandLayers(raw) {
		t.Fatal("resources/commandLayers não foi encontrado sem diferenciar maiúsculas")
	}
}

func TestHasCommandLayersRetornaFalseSemChave(t *testing.T) {
	for _, raw := range []string{
		`{}`,
		`{"resources":{"conversations":[]}}`,
	} {
		if HasCommandLayers(raw) {
			t.Fatalf("payload sem commandLayers foi roteado: %s", raw)
		}
	}
}

func formatCommandImportTestID(n int) string {
	return string(rune('a'+n/16)) + string(rune('a'+n%16)) + "000000000000"
}
