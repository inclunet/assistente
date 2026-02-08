package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"

	"assistente/internal/agents"
	"assistente/internal/database"
	"assistente/internal/llm"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// ToolResult representa o resultado de uma execução de ferramenta
type ToolResult struct {
	ToolCallID string `json:"tool_call_id"`
	Role       string `json:"role"` // sempre "tool"
	Content    string `json:"content"`
}

// GetToolsForAPI retorna todas as tools disponíveis para o orquestrador
func (a *App) GetToolsForAPI() []llm.Tool {
	var result []llm.Tool

	// 1. Tools genéricas (executadas diretamente, sem sub-LLMs)
	result = append(result, a.getGenericTools()...)

	// 2. Tools de delegação dos agentes MCP (mantido para MCP agents que expõem tools nativas)
	if a.registry != nil {
		agentTools := a.registry.GetDelegationTools()
		for _, t := range agentTools {
			// Filtra: só mantém delegation tools de agentes MCP
			name := t.Function.Name
			if strings.HasPrefix(name, "delegate_to_mcp_") || strings.HasPrefix(name, "delegate_to_websearch") {
				result = append(result, llm.Tool{
					Type: t.Type,
					Function: llm.ToolFunction{
						Name:        t.Function.Name,
						Description: t.Function.Description,
						Parameters:  t.Function.Parameters,
					},
				})
			}
		}
	}

	return result
}

// ExecuteTool executa uma tool de delegação
func (a *App) ExecuteTool(toolCall llm.ToolCall) (string, error) {
	toolName := toolCall.Function.Name

	// Tools genéricas (executadas diretamente, sem sub-LLMs)
	if result, handled, err := a.executeGenericTool(toolCall); handled {
		return result, err
	}

	// Verifica se é uma tool de delegação (delegate_to_*)
	if strings.HasPrefix(toolName, "delegate_to_") {
		agentName := strings.TrimPrefix(toolName, "delegate_to_")

		// Extrai a tarefa dos argumentos
		var args map[string]interface{}
		if err := json.Unmarshal([]byte(toolCall.Function.Arguments), &args); err != nil {
			return "", fmt.Errorf("erro ao parsear argumentos: %v", err)
		}

		task, ok := args["task"].(string)
		if !ok || task == "" {
			return "", fmt.Errorf("tarefa não especificada para o agente %s", agentName)
		}

		fmt.Printf("📤 [ORQUESTRADOR] Delegando para agente '%s': %s\n", agentName, truncateStr(task, 80))
		fmt.Printf("📤 [ORQUESTRADOR] currentDelegationID=%d, conversationID=%d\n", a.currentDelegationID, a.currentConversationID)

		// Configura o saver para salvar mensagens internas do agente
		if a.currentConversationID > 0 && a.currentDelegationID > 0 {
			a.registry.SetAgentConversationContext(agentName, a.currentConversationID, a.createAgentMessageSaver())
		} else {
			fmt.Printf("⚠️ [ORQUESTRADOR] Saver não criado: conversationID=%d, delegationID=%d\n",
				a.currentConversationID, a.currentDelegationID)
		}

		// Executa delegação para o agente
		result, err := a.registry.ExecuteDelegation(context.Background(), agentName, task)
		if err != nil {
			return "", fmt.Errorf("erro na delegação para %s: %v", agentName, err)
		}

		fmt.Printf("📥 [ORQUESTRADOR] Resposta do agente '%s': %s\n", agentName, truncateStr(result, 80))
		return result, nil
	}

	// Fallback para tools legadas (compatibilidade durante transição)
	agentToolCall := agents.ToolCall{
		ID:   toolCall.ID,
		Type: toolCall.Type,
		Function: agents.FunctionCall{
			Name:      toolCall.Function.Name,
			Arguments: toolCall.Function.Arguments,
		},
	}
	return a.registry.ExecuteTool(agentToolCall)
}

// createAgentMessageSaver cria um callback para salvar mensagens internas dos agentes
// NOVA ARQUITETURA v2:
// - n0: user/assistant
// - n1: assistant chamando agent (parentId=userMessage)
// - n2: agent interagindo com tools (parentId=mensagem de tool_calls do assistant)
func (a *App) createAgentMessageSaver() llm.MessageSaver {
	// NÃO captura parentID aqui - usa sempre o valor atual de currentDelegationID
	// Isso garante que múltiplas interações usem o parentID correto

	fmt.Printf("📝 [SAVER CRIADO] delegationID atual: %d\n", a.currentDelegationID)

	return func(msg llm.AgentMessage) error {
		// Usa o valor ATUAL de currentDelegationID (não capturado)
		parentID := a.currentDelegationID

		if a.currentConversationID == 0 || parentID == 0 {
			fmt.Printf("⚠️ [AGENT SAVER] Sem contexto: conversationID=%d, delegationID=%d\n",
				a.currentConversationID, parentID)
			return nil
		}

		// Serializa tool calls se houver
		var toolCallsJSON string
		if len(msg.ToolCalls) > 0 {
			if data, err := json.Marshal(msg.ToolCalls); err == nil {
				toolCallsJSON = string(data)
			}
		}

		// Para mensagens de tool_calls (agente chamando tools),
		// formata conteúdo markdown com as chamadas
		content := msg.Content
		if len(msg.ToolCalls) > 0 && content == "" {
			var contentBuilder strings.Builder

			if len(msg.ToolCalls) == 1 {
				// Uma única tool call
				tc := msg.ToolCalls[0]
				contentBuilder.WriteString(fmt.Sprintf("🔧 **Executando:** `%s`\n\n", tc.Function.Name))
				contentBuilder.WriteString("**Parâmetros:**\n```json\n")

				// Formata JSON
				var prettyArgs bytes.Buffer
				if err := json.Indent(&prettyArgs, []byte(tc.Function.Arguments), "", "  "); err == nil {
					contentBuilder.WriteString(prettyArgs.String())
				} else {
					contentBuilder.WriteString(tc.Function.Arguments)
				}
				contentBuilder.WriteString("\n```")
			} else {
				// Múltiplas tool calls
				contentBuilder.WriteString(fmt.Sprintf("🔧 **Executando %d ferramentas:**\n\n", len(msg.ToolCalls)))
				for i, tc := range msg.ToolCalls {
					contentBuilder.WriteString(fmt.Sprintf("%d. **%s**\n```json\n", i+1, tc.Function.Name))
					var prettyArgs bytes.Buffer
					if err := json.Indent(&prettyArgs, []byte(tc.Function.Arguments), "", "  "); err == nil {
						contentBuilder.WriteString(prettyArgs.String())
					} else {
						contentBuilder.WriteString(tc.Function.Arguments)
					}
					contentBuilder.WriteString("\n```\n")
					if i < len(msg.ToolCalls)-1 {
						contentBuilder.WriteString("\n")
					}
				}
			}

			content = contentBuilder.String()
		}

		fmt.Printf("💾 [AGENT SAVER] Salvando: role=%s, agent=%s, parentID=%d (nível 2)\n",
			msg.Role, msg.AgentName, parentID)

		// Salva como filho da mensagem de tool_calls do assistant (nível 2)
		savedMsg, err := database.AddChildMessage(
			a.currentConversationID,
			parentID,
			msg.Role,
			content,
			toolCallsJSON,
			msg.ToolCallID,
			msg.AgentName,
			msg.Model,
		)
		if err != nil {
			// Se a conversa foi deletada ou mensagem pai não existe, retorna erro especial para abortar
			if errors.Is(err, database.ErrConversationDeleted) || errors.Is(err, database.ErrParentMessageDeleted) {
				fmt.Printf("🛑 [AGENT SAVER] Conversa %d foi deletada/limpa - abortando\n", a.currentConversationID)
				return database.ErrConversationDeleted // Retorna mesmo erro para simplificar tratamento upstream
			}
			fmt.Printf("❌ [AGENT SAVER] Erro ao salvar: %v\n", err)
			return err
		}
		fmt.Printf("✅ [AGENT SAVER] Salvo: ID=%d, parentID=%d\n", savedMsg.ID, parentID)

		// Emite evento em tempo real para o frontend
		runtime.EventsEmit(a.ctx, "chat:agent_message", map[string]interface{}{
			"id":         savedMsg.ID,
			"parentId":   parentID,
			"role":       msg.Role,
			"content":    content,
			"agentName":  msg.AgentName,
			"toolCalls":  toolCallsJSON,
			"toolCallId": msg.ToolCallID,
		})

		return nil
	}
}

// truncateStr trunca uma string para exibição em logs
func truncateStr(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// AgentInfo representa informações de um agente para a UI
type AgentInfo struct {
	Name         string `json:"name"`
	DisplayName  string `json:"display_name"`
	Description  string `json:"description"`
	AgentType    string `json:"agent_type"`
	Model        string `json:"model"`
	SystemPrompt string `json:"system_prompt"`
	Enabled      bool   `json:"enabled"`
}

// GetRegisteredAgents retorna informações dos agentes registrados
func (a *App) GetRegisteredAgents() []AgentInfo {
	if a.registry == nil {
		return []AgentInfo{}
	}

	registeredAgents := a.registry.GetAll()
	result := make([]AgentInfo, 0, len(registeredAgents))

	for _, agent := range registeredAgents {
		result = append(result, AgentInfo{
			Name:         agent.GetName(),
			DisplayName:  agent.GetDisplayName(),
			Description:  agent.GetDescription(),
			AgentType:    agent.GetType(),
			Model:        agent.GetModel(),
			SystemPrompt: agent.GetSystemPrompt(),
			Enabled:      agent.IsEnabled(),
		})
	}

	return result
}

// TestAgent testa um agente com uma tarefa (playground)
func (a *App) TestAgent(agentName, task string) (string, error) {
	if a.registry == nil {
		return "", fmt.Errorf("registry não inicializado")
	}

	agent := a.registry.Get(agentName)
	if agent == nil {
		return "", fmt.Errorf("agente não encontrado: %s", agentName)
	}

	if !agent.IsEnabled() {
		return "", fmt.Errorf("agente desabilitado: %s", agentName)
	}

	fmt.Printf("🧪 [PLAYGROUND] Testando agente '%s' com: %s\n", agentName, truncateStr(task, 80))

	result, err := agent.Execute(context.Background(), task)
	if err != nil {
		return "", fmt.Errorf("erro ao executar agente: %v", err)
	}

	fmt.Printf("✅ [PLAYGROUND] Resultado: %s\n", truncateStr(result, 100))
	return result, nil
}

// ReloadMCPAgent recarrega um MCP Agent no registry (hot reload)
func (a *App) ReloadMCPAgent(mcpAgentID uint) error {
	mcpAgent, err := a.GetMCPAgent(mcpAgentID)
	if err != nil {
		return fmt.Errorf("erro ao buscar MCP agent: %w", err)
	}

	agentConfig, err := a.GetAgentConfigByID(mcpAgent.AgentConfigID)
	if err != nil {
		return fmt.Errorf("erro ao buscar config: %w", err)
	}

	// Remove do registry se já estiver conectado
	existing := a.registry.GetMCPAgent(agentConfig.Name)
	if existing != nil {
		existing.Disconnect()
		a.registry.Unregister(agentConfig.Name)
	}

	// Só recarrega se estiver habilitado e autoConnect
	if !agentConfig.Enabled || !mcpAgent.AutoConnect {
		log.Printf("[ReloadMCPAgent] Agente %s não será carregado (enabled=%v, autoConnect=%v)",
			agentConfig.Name, agentConfig.Enabled, mcpAgent.AutoConnect)
		return nil
	}

	// Registra e conecta
	if err := a.registerMCPAgentInRegistry(agentConfig, mcpAgent); err != nil {
		return fmt.Errorf("erro ao registrar no registry: %w", err)
	}

	log.Printf("[ReloadMCPAgent] ✅ Agente %s recarregado com sucesso", agentConfig.Name)
	return nil
}

