package shell

import (
	"assistente/internal/terminal"
	"assistente/internal/tools"
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// TerminalSessionManager expõe apenas o ciclo de vida necessário à tool.
type TerminalSessionManager interface {
	List() []terminal.SessionInfo
	CreateInfo(name, workDir string) (terminal.SessionInfo, error)
	Interrupt(sessionID string) error
	Close(sessionID string) error
}

// TerminalSession permite ao chat administrar explicitamente os PTYs vivos.
type TerminalSession struct {
	manager TerminalSessionManager
	workDir string
}

func NewTerminalSession(manager TerminalSessionManager, workDir string) *TerminalSession {
	return &TerminalSession{manager: manager, workDir: workDir}
}

func (t *TerminalSession) Name() string { return "terminal_session" }

func (t *TerminalSession) Description() string {
	return `Lists and manages live terminal sessions. Use action=list to inspect available terminal IDs, create to start a new terminal, interrupt to send Ctrl+C, and close to terminate the PTY. Closing is permanent; a dead terminal cannot be reopened.`
}

func (t *TerminalSession) CatalogMetadata() tools.CatalogMetadata {
	return tools.CatalogMetadata{Category: "shell", Class: "run_commands", Package: "coding_edit", Risk: "shell"}
}

func (t *TerminalSession) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"action": {
				"type": "string",
				"enum": ["list", "create", "interrupt", "close"]
			},
			"terminal_id": {
				"type": "string",
				"description": "Required for interrupt and close."
			},
			"name": {
				"type": "string",
				"description": "Optional display name when creating a terminal."
			},
			"working_directory": {
				"type": "string",
				"description": "Optional working directory when creating a terminal."
			}
		},
		"required": ["action"],
		"additionalProperties": false
	}`)
}

type terminalSessionArgs struct {
	Action           string `json:"action"`
	TerminalID       string `json:"terminal_id"`
	Name             string `json:"name"`
	WorkingDirectory string `json:"working_directory"`
}

func (t *TerminalSession) Execute(_ context.Context, raw json.RawMessage) (tools.ToolResult, error) {
	if t.manager == nil {
		return tools.ToolResult{Content: "Gerenciador de terminais indisponível", IsError: true}, nil
	}

	var args terminalSessionArgs
	if err := json.Unmarshal(raw, &args); err != nil {
		return tools.ToolResult{Content: "Parâmetros inválidos: " + err.Error(), IsError: true}, nil
	}

	switch args.Action {
	case "list":
		sessions := t.manager.List()
		payload, err := json.Marshal(sessions)
		if err != nil {
			return tools.ToolResult{
				Content: "Erro ao serializar terminais: " + err.Error(),
				IsError: true,
			}, nil
		}
		return tools.ToolResult{
			Content: string(payload),
			Metadata: map[string]any{
				"count": len(sessions),
			},
		}, nil

	case "create":
		workDir := strings.TrimSpace(args.WorkingDirectory)
		if workDir == "" {
			workDir = t.workDir
		} else {
			resolvedWorkDir, resolveErr := resolveProjectWorkDir(t.workDir, workDir)
			if resolveErr != nil {
				return tools.ToolResult{
					Content: "working_directory inválido: " + resolveErr.Error(),
					IsError: true,
				}, nil
			}
			workDir = resolvedWorkDir
		}
		info, err := t.manager.CreateInfo(strings.TrimSpace(args.Name), workDir)
		if err != nil {
			return tools.ToolResult{Content: "Erro ao criar terminal: " + err.Error(), IsError: true}, nil
		}
		link := fmt.Sprintf("assistente://terminal/%s", info.ID)
		displayName := strings.TrimSpace(info.Name)
		createdLabel := ""
		if displayName == "" {
			displayName = info.ID
			createdLabel = displayName
		} else {
			createdLabel = fmt.Sprintf("%s (%s)", displayName, info.ID)
		}
		return tools.ToolResult{
			Content: fmt.Sprintf("Terminal criado: %s\nAbrir para inspeção: %s", createdLabel, link),
			Metadata: map[string]any{
				"terminalId": info.ID,
				"sessionId":  info.ID,
				"deepLink":   link,
				"state":      info.State,
			},
		}, nil

	case "interrupt", "close":
		id := strings.TrimSpace(args.TerminalID)
		if id == "" {
			return tools.ToolResult{Content: "terminal_id é obrigatório para " + args.Action, IsError: true}, nil
		}
		var err error
		if args.Action == "interrupt" {
			err = t.manager.Interrupt(id)
		} else {
			err = t.manager.Close(id)
		}
		if err != nil {
			return tools.ToolResult{Content: fmt.Sprintf("Erro ao executar %s no terminal %s: %v", args.Action, id, err), IsError: true}, nil
		}
		return tools.ToolResult{
			Content: fmt.Sprintf("Terminal %s: ação %s concluída", id, args.Action),
			Metadata: map[string]any{
				"terminalId": id,
				"action":     args.Action,
			},
		}, nil

	default:
		return tools.ToolResult{Content: "action deve ser list, create, interrupt ou close", IsError: true}, nil
	}
}
