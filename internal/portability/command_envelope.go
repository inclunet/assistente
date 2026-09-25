package portability

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
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
	if ctx == nil || service == nil || len(raw) == 0 {
		return commandconfig.MutationDiff{}, commandportability.ErrInvalid
	}
	file, err := parseCommandLayersEnvelope(raw)
	if err != nil {
		return commandconfig.MutationDiff{}, err
	}
	if err := validateCommandEnvelopeTarget(ctx, file.Resources.CommandLayers, workspace, options, owner, refs); err != nil {
		return commandconfig.MutationDiff{}, err
	}
	return commandportability.ApplyPlanImport(ctx, service, token, workspace, file.Resources.CommandLayers, options, owner, refs)
}

// ApplyCommandEnvelopeBatch mantém o mesmo parser estrito, mas permite os
// escopos explicitamente resolvidos no destino. Todos precisam ser autorizados
// e confirmados antes de uma única transação. Não habilita o import genérico
// nem mistura credenciais/outros recursos no lote de configuração.
func ApplyCommandEnvelopeBatch(ctx context.Context, service *commandconfig.CompleteMutationService, token string, raw []byte, options commandportability.PlanOptions, owner commandportability.OwnershipPort, refs commandportability.ReferencePort) (commandportability.BatchImportResult, error) {
	if ctx == nil || service == nil || len(raw) == 0 {
		return commandportability.BatchImportResult{}, commandportability.ErrInvalid
	}
	file, err := parseCommandLayersEnvelope(raw)
	if err != nil {
		return commandportability.BatchImportResult{}, err
	}
	return commandportability.ApplyPlanImportBatch(ctx, service, token, file.Resources.CommandLayers, options, owner, refs)
}

func parseCommandLayersEnvelope(raw []byte) (*ExportFile, error) {
	canonical, err := commandjson.Canonicalize(raw)
	if err != nil {
		return nil, err
	}
	// A rota dedicada é exclusiva mesmo quando outro recurso está vazio.
	// Não aceitar um restore parcial nem aliases de casing do decoder Go.
	var shape map[string]json.RawMessage
	if err := json.Unmarshal(canonical, &shape); err != nil {
		return nil, commandportability.ErrInvalid
	}
	for key := range shape {
		if strings.EqualFold(key, "resources") && key != "resources" {
			return nil, commandportability.ErrInvalid
		}
	}
	var resourceShape map[string]json.RawMessage
	if err := json.Unmarshal(shape["resources"], &resourceShape); err != nil {
		return nil, commandportability.ErrInvalid
	}
	var strict ExportFile
	decoder := json.NewDecoder(bytes.NewReader(canonical))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&strict); err != nil {
		return nil, commandportability.ErrInvalid
	}
	if len(strict.Resources.CommandLayers) == 0 {
		return nil, commandportability.ErrInvalid
	}
	for key := range resourceShape {
		if key != "commandLayers" {
			return nil, commandportability.ErrUnsupported
		}
	}
	file, unsupported, err := parseExportFile(string(canonical))
	if err != nil {
		return nil, err
	}
	resources := file.Resources
	if len(unsupported) != 0 || len(resources.Conversations) != 0 || len(resources.Providers) != 0 || len(resources.MCPServers) != 0 || len(resources.TaskLists) != 0 || len(resources.MemoryRecords) != 0 || resources.Credentials != nil || file.Options.IncludeCredentials {
		return nil, commandportability.ErrUnsupported
	}
	if len(resources.CommandLayers) == 0 {
		return nil, commandportability.ErrInvalid
	}
	return file, nil
}

// validateCommandEnvelopeTarget faz o preflight puro do mesmo plano que será
// refeito pelo ApplyPlanImport. O envelope é para um escopo por aplicação:
// escopo global só pode ser aplicado no global, e escopo workspace precisa
// terminar exatamente no workspace autorizado pelo host. WorkspaceMap é uma
// decisão de remapeamento, não uma autorização para trocar o alvo pedido.
//
// A validação ocorre antes do writer e do presenter. O commit continua
// exclusivamente em ApplyPlanImport, que revalida tudo dentro do gate final.
func validateCommandEnvelopeTarget(ctx context.Context, layers []commandportability.LayerExport, workspace *string, options commandportability.PlanOptions, owner commandportability.OwnershipPort, refs commandportability.ReferencePort) error {
	if len(layers) == 0 || owner == nil {
		return commandportability.ErrInvalid
	}
	if workspace != nil {
		if strings.TrimSpace(*workspace) != *workspace || *workspace == "" || refs.Workspace == nil {
			return commandportability.ErrWorkspaceResolution
		}
		canonical, err := refs.Workspace(ctx, *workspace)
		if err != nil || canonical != *workspace {
			return commandportability.ErrWorkspaceResolution
		}
	}
	plan, err := commandportability.PlanImport(ctx, layers, options, owner, refs)
	if err != nil {
		return err
	}
	for _, item := range plan.Layers {
		if workspace == nil {
			if item.TargetScope.Kind != commandportability.GlobalScope {
				return commandportability.ErrWorkspaceResolution
			}
			continue
		}
		if item.TargetScope.Kind != commandportability.WorkspaceScope || item.TargetScope.WorkspaceID != *workspace {
			return commandportability.ErrWorkspaceResolution
		}
	}
	return nil
}
