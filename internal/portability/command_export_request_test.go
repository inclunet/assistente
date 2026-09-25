package portability

import (
	"errors"
	"testing"

	"assistente/internal/commandportability"
	"github.com/google/uuid"
)

func TestValidateCommandExportRequestAcceptsGlobal(t *testing.T) {
	err := ValidateCommandExportRequest(ExportRequest{
		ExplicitSelection:    true,
		IncludeCommandLayers: true,
		OutputFormat:         FormatJSON,
	})
	if err != nil {
		t.Fatalf("export global inválido: %v", err)
	}
}

func TestValidateCommandExportRequestAcceptsAllAccessibleWorkspaces(t *testing.T) {
	err := ValidateCommandExportRequest(ExportRequest{
		ExplicitSelection:    true,
		IncludeCommandLayers: true,
		IncludeWorkspace:     true,
	})
	if err != nil {
		t.Fatalf("export global+workspaces inválido: %v", err)
	}
}

func TestValidateCommandExportRequestAcceptsExplicitIDs(t *testing.T) {
	err := ValidateCommandExportRequest(ExportRequest{
		ExplicitSelection:    true,
		IncludeCommandLayers: true,
		IncludeWorkspace:     true,
		CommandLayerIDs: []string{
			"01926b90-7a5a-7c4e-8d3f-000000000001",
			"01926b90-7a5a-7c4e-8d3f-000000000002",
		},
	})
	if err != nil {
		t.Fatalf("export por IDs inválido: %v", err)
	}
}

func TestValidateCommandExportRequestRejectsUnsupportedAndAmbiguousInputs(t *testing.T) {
	trueValue := true
	tests := []struct {
		name string
		req  ExportRequest
		want error
	}{
		{"sem seleção explícita", ExportRequest{IncludeCommandLayers: true}, commandportability.ErrInvalid},
		{"include commandLayers ausente", ExportRequest{ExplicitSelection: true}, commandportability.ErrInvalid},
		{"all", ExportRequest{ExplicitSelection: true, IncludeCommandLayers: true, All: true}, commandportability.ErrUnsupported},
		{"outro recurso", ExportRequest{ExplicitSelection: true, IncludeCommandLayers: true, ConversationIDs: []string{"conversation"}}, commandportability.ErrUnsupported},
		{"contatos", ExportRequest{ExplicitSelection: true, IncludeCommandLayers: true, IncludeContacts: true}, commandportability.ErrUnsupported},
		{"áudio", ExportRequest{ExplicitSelection: true, IncludeCommandLayers: true, IncludeAudio: true}, commandportability.ErrUnsupported},
		{"toggle rich", ExportRequest{ExplicitSelection: true, IncludeCommandLayers: true, IncludeMetadata: &trueValue}, commandportability.ErrUnsupported},
		{"credenciais", ExportRequest{ExplicitSelection: true, IncludeCommandLayers: true, IncludeCredentials: true}, commandportability.ErrUnsupported},
		{"senha", ExportRequest{ExplicitSelection: true, IncludeCommandLayers: true, CredentialExportPassword: "segredo"}, commandportability.ErrUnsupported},
		{"senha somente com espaços", ExportRequest{ExplicitSelection: true, IncludeCommandLayers: true, CredentialExportPassword: "   "}, commandportability.ErrUnsupported},
		{"formato html", ExportRequest{ExplicitSelection: true, IncludeCommandLayers: true, OutputFormat: FormatHTML}, commandportability.ErrUnsupported},
		{"UUID não v7", ExportRequest{ExplicitSelection: true, IncludeCommandLayers: true, CommandLayerIDs: []string{"01926b90-7a5a-6c4e-8d3f-000000000001"}}, commandportability.ErrInvalid},
		{"UUID duplicado", ExportRequest{ExplicitSelection: true, IncludeCommandLayers: true, CommandLayerIDs: []string{"01926b90-7a5a-7c4e-8d3f-000000000001", "01926b90-7a5a-7c4e-8d3f-000000000001"}}, commandportability.ErrInvalid},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if err := ValidateCommandExportRequest(tc.req); !errors.Is(err, tc.want) {
				t.Fatalf("erro = %v, want %v", err, tc.want)
			}
		})
	}
}

func TestValidateCommandExportRequestRejectsMoreThan64IDs(t *testing.T) {
	ids := make([]string, maxCommandLayerExportIDs)
	for i := range ids {
		ids[i] = uuid.Must(uuid.NewV7()).String()
	}
	validRequest := ExportRequest{ExplicitSelection: true, IncludeCommandLayers: true, CommandLayerIDs: ids}
	if err := ValidateCommandExportRequest(validRequest); err != nil {
		t.Fatalf("64 UUIDv7 válidos retornaram erro: %v", err)
	}

	ids = append(ids, uuid.Must(uuid.NewV7()).String())
	validRequest.CommandLayerIDs = ids
	if err := ValidateCommandExportRequest(validRequest); !errors.Is(err, commandportability.ErrInvalid) {
		t.Fatalf("erro = %v, want %v", err, commandportability.ErrInvalid)
	}
}
