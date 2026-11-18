package app

import (
	"assistente/internal/commandcatalog"
	"assistente/internal/commandexecution"
	"assistente/internal/commandui"
	"assistente/internal/questionnaire"
	"context"
	"path/filepath"
	"strings"
)

const commandWorkspaceCreateID = "workspace.create"

func commandWorkspaceCreateRegistration() (commandcatalog.Definition, commandcatalog.HandlerContract) {
	contract := commandcatalog.HandlerContract{
		Effect:           commandcatalog.Write,
		HasMutableTarget: true,
		Route:            "contextual/workspace/create",
		Classification:   commandcatalog.HandlerBackend,
	}
	return commandcatalog.Definition{
		ID:               commandWorkspaceCreateID,
		Effect:           commandcatalog.Write,
		Decision:         commandcatalog.NoDecision,
		HasMutableTarget: true,
		AllowedSources:   []commandcatalog.Source{commandcatalog.Palette, commandcatalog.KeyboardLocal, commandcatalog.StreamDeck},
		Context: commandcatalog.ContextPolicy{Facts: []commandcatalog.ContextFact{{
			Provider: "workspace", Fact: "active_tab", Mode: commandcatalog.ExactVersion,
		}}},
		Presentation: &commandcatalog.Presentation{Version: "workspace-create-v1", Locales: map[string]commandcatalog.LocalizedMetadata{
			"pt-BR": {Name: "Criar workspace", Description: "Cria um novo workspace a partir do contexto ativo", Category: "Workspace", Aliases: []string{"novo workspace", "novo espaço de trabalho"}},
			"en":    {Name: "Create workspace", Description: "Creates a new workspace from the active context", Category: "Workspace", Aliases: []string{"new workspace", "workspace"}},
			"es":    {Name: "Crear espacio de trabajo", Description: "Crea un nuevo espacio de trabajo a partir del contexto activo", Category: "Workspace", Aliases: []string{"nuevo espacio de trabajo", "workspace"}},
		}},
		ArgumentsSchema:       &commandcatalog.Schema{Type: commandcatalog.SchemaObject},
		ResultSchema:          &commandcatalog.Schema{Type: commandcatalog.SchemaObject},
		Risk:                  commandcatalog.RiskLow,
		Persistence:           commandcatalog.PersistencePolicy{Arguments: commandcatalog.PersistenceNever, Result: commandcatalog.PersistenceSummary, Audit: commandcatalog.PersistenceRedacted},
		Scopes:                []commandcatalog.Scope{commandcatalog.ScopeWorkspace},
		Availability:          commandcatalog.Availability{Status: commandcatalog.Available},
		HandlerRoute:          contract.Route,
		HandlerClassification: contract.Classification,
	}, contract
}

// requestWorkspaceDestination coleta a escolha antes de qualquer escrita. O
// contexto admitido cancela a pergunta se sessão/segurança forem invalidadas.
func (a *App) requestWorkspaceDestination(ctx context.Context) (string, string, error) {
	if a.dialogPort == nil || a.questionnaireMgr == nil {
		return "", "", commandexecution.ErrDenied
	}
	directory, err := a.dialogPort.OpenDirectoryDialog()
	if err != nil {
		return "", "", err
	}
	if err := ctx.Err(); err != nil {
		return "", "", err
	}
	if directory == "" {
		return "", "", commandui.ErrCancelled
	}
	response, err := a.questionnaireMgr.RequestQuestionnaire(ctx, questionnaire.RequestPayload{
		Title:       questionnaire.Keyed("workspace.newWorkspace", "Novo workspace"),
		AllowCancel: true,
		SubmitLabel: questionnaire.Keyed("workspace.createInDirectory", "Criar workspace"),
		CancelLabel: questionnaire.Keyed("common.cancel", "Cancelar"),
		Questions: []questionnaire.Question{
			{ID: "directory", Type: "readonly_code", Prompt: questionnaire.Keyed("workspace.directory", "Pasta do workspace"), Content: directory},
			{ID: "name", Type: "text", Prompt: questionnaire.Keyed("workspace.renamePlaceholder", "Nome do workspace"), Required: true, Default: filepath.Base(directory), AutoFocus: true},
		},
	})
	if err != nil {
		return "", "", err
	}
	if response.Cancelled {
		return "", "", commandui.ErrCancelled
	}
	name, ok := response.Answers["name"].(string)
	if !ok || strings.TrimSpace(name) == "" {
		return "", "", commandexecution.ErrInvalidRequest
	}
	return strings.TrimSpace(name), directory, nil
}
