package portability

import (
	"bytes"
	"context"
	"encoding/json"
	"time"

	"assistente/internal/commandconfig"
	"assistente/internal/commandjson"
	"assistente/internal/commandportability"
	"gorm.io/gorm"
)

// ExportCommandEnvelope é uma composição interna para exatamente um escopo.
// Scope e referências devem vir do host após autorização de leitura; esta
// função não autentica user IDs. Workspace não inclui globals herdados.
// Usa o envelope AEP0047 e limita o JSON a 64 KiB pelo parser commandjson.
// Não habilita o export genérico nem inclui credenciais criptografadas.
func ExportCommandEnvelope(ctx context.Context, db *gorm.DB, scope commandconfig.Scope, refs commandportability.ReferencePort) ([]byte, error) {
	if ctx == nil || db == nil {
		return nil, commandportability.ErrInvalid
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if scope.WorkspaceID != nil {
		if refs.Workspace == nil {
			return nil, commandportability.ErrWorkspaceResolution
		}
		id, err := refs.Workspace(ctx, *scope.WorkspaceID)
		if err != nil || id != *scope.WorkspaceID || id == "" {
			return nil, commandportability.ErrWorkspaceResolution
		}
	}
	layers, err := commandportability.ExportScopeFromStore(ctx, db, scope, refs)
	if err != nil {
		return nil, err
	}
	return commandjson.Marshal(ExportFile{Version: ExportVersion, ExportedAt: time.Now().UTC(), Resources: ExportResources{CommandLayers: layers}})
}

// ApplyCommandEnvelope decodifica o envelope canônico v1/v2 e usa exclusivamente
// ApplyPlanImport, com autenticação, decisão e CAS do serviço comum. Portas são
// do host, nunca do arquivo. Rejeita recursos adicionais para não anunciar um
// restore parcial. Lotes multi-escopo continuam sujeitos à recusa do writer.
func ApplyCommandEnvelope(ctx context.Context, service *commandconfig.CompleteMutationService, token string, workspace *string, raw []byte, options commandportability.PlanOptions, owner commandportability.OwnershipPort, refs commandportability.ReferencePort) (commandconfig.MutationDiff, error) {
	canonical, err := commandjson.Canonicalize(raw)
	if err != nil {
		return commandconfig.MutationDiff{}, err
	}
	var strict ExportFile
	decoder := json.NewDecoder(bytes.NewReader(canonical))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&strict); err != nil {
		return commandconfig.MutationDiff{}, commandportability.ErrInvalid
	}
	file, unsupported, err := parseExportFile(string(canonical))
	if err != nil {
		return commandconfig.MutationDiff{}, err
	}
	r := file.Resources
	if len(unsupported) != 0 || len(r.Conversations) != 0 || len(r.Providers) != 0 || len(r.MCPServers) != 0 || len(r.TaskLists) != 0 || len(r.MemoryRecords) != 0 || r.Credentials != nil || file.Options.IncludeCredentials {
		return commandconfig.MutationDiff{}, commandportability.ErrUnsupported
	}
	return commandportability.ApplyPlanImport(ctx, service, token, workspace, r.CommandLayers, options, owner, refs)
}
