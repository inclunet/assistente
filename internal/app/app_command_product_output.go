package app

import (
	"encoding/json"
	"strings"

	"assistente/internal/commandcatalog"
	"assistente/internal/commandexecution"
	"assistente/internal/commandledger"
)

// CommandOutput é a união fechada de resultados ao vivo permitidos na borda
// desktop. Não faz parte do ledger, da auditoria ou da consulta de histórico.
// Um novo comando precisa de DTO e revisão próprios antes de expor seu output.
type CommandOutput struct {
	Kind       string                     `json:"kind"`
	Workspaces []CommandWorkspaceMetadata `json:"workspaces"`
}

// Resultado vivo não é extraído do ledger. Reautenticar após a espera evita
// entregá-lo à sessão/workspace que substituiu a composição solicitante.
func (a *App) commandProductResultWithOutput(p *commandProductRuntime, commandID string, record commandledger.FullRecord, raw json.RawMessage) (CommandExecutionResult, error) {
	current, err := a.authenticatedCommandProduct()
	if err != nil || current != p || !commandRecordMatchesPrincipal(record, p.principal) {
		return CommandExecutionResult{}, commandexecution.ErrDenied
	}
	result := commandProductResult(record)
	if record.Status != commandledger.Succeeded || len(raw) == 0 {
		return result, nil
	}
	if record.Envelope.CommandID == nil || *record.Envelope.CommandID != commandID {
		return result, commandexecution.ErrDenied
	}
	definition, ok := p.registry.Lookup(commandID)
	if !ok {
		return result, commandexecution.ErrExecution
	}
	result.Output, err = commandProductOutput(definition, raw)
	return result, err
}

func commandProductOutput(definition commandcatalog.Definition, raw json.RawMessage) (*CommandOutput, error) {
	if isCommandToolExecutionID(definition.ID) && definition.HandlerClassification == commandcatalog.HandlerTool {
		if _, err := definition.ValidateResult(raw); err != nil {
			return nil, commandexecution.ErrExecution
		}
		// Tool output contém material arbitrário. O sucesso fica visível, mas
		// o payload efêmero não atravessa a API desktop.
		return nil, nil
	}
	if isCommandLayerAction(definition.ID) && definition.HandlerClassification == commandcatalog.HandlerBackend {
		if _, err := definition.ValidateResult(raw); err != nil {
			return nil, commandexecution.ErrExecution
		}
		// Claims e referências da configuração não são payload visual.
		return nil, nil
	}
	if definition.ID != commandProductWorkspaceListID || definition.HandlerClassification != commandcatalog.HandlerBackend {
		return nil, commandexecution.ErrDenied
	}
	validated, err := definition.ValidateResult(raw)
	if err != nil {
		return nil, commandexecution.ErrExecution
	}
	var result workspaceListResult
	if err := json.Unmarshal(validated, &result); err != nil || result.Workspaces == nil {
		return nil, commandexecution.ErrExecution
	}
	ids := make(map[string]bool, len(result.Workspaces))
	active := false
	for _, item := range result.Workspaces {
		if strings.TrimSpace(item.ID) == "" || strings.TrimSpace(item.Name) == "" || item.TabCount < 0 || ids[item.ID] || active && item.IsActive {
			return nil, commandexecution.ErrExecution
		}
		ids[item.ID] = true
		active = active || item.IsActive
	}
	return &CommandOutput{Kind: definition.ID, Workspaces: result.Workspaces}, nil
}
