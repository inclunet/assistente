package agents

import (
	"context"
	"fmt"
	"sync"

	"assistente/internal/llm"
)

// Registry gerencia todos os agentes disponíveis
type Registry struct {
	agents map[string]Agent
	mu     sync.RWMutex
}

// NewRegistry cria um novo registry de agentes
func NewRegistry() *Registry {
	return &Registry{
		agents: make(map[string]Agent),
	}
}

// Register adiciona um agente ao registry
func (r *Registry) Register(agent Agent) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.agents[agent.GetName()] = agent
}

// Unregister remove um agente do registry
func (r *Registry) Unregister(name string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.agents, name)
}

// Get retorna um agente pelo nome
func (r *Registry) Get(name string) Agent {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.agents[name]
}

// GetAll retorna todos os agentes registrados
func (r *Registry) GetAll() []Agent {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]Agent, 0, len(r.agents))
	for _, agent := range r.agents {
		result = append(result, agent)
	}
	return result
}

// GetEnabled retorna apenas os agentes habilitados
func (r *Registry) GetEnabled() []Agent {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]Agent, 0)
	for _, agent := range r.agents {
		if agent.IsEnabled() {
			result = append(result, agent)
		}
	}
	return result
}

// GetDelegationTools retorna as tools de delegação para o orquestrador
// Cada agente habilitado gera uma tool "delegate_to_<name>"
func (r *Registry) GetDelegationTools() []Tool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var tools []Tool
	for _, agent := range r.agents {
		if !agent.IsEnabled() {
			continue
		}

		// Usa descrição otimizada para delegação se o agente implementar a interface
		description := getDelegationDescription(agent)

		tool := Tool{
			Type: "function",
			Function: ToolFunction{
				Name:        fmt.Sprintf("delegate_to_%s", agent.GetName()),
				Description: description,
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"task": map[string]interface{}{
							"type":        "string",
							"description": "Detailed task description in natural language. Be specific about what you need the agent to do.",
						},
					},
					"required": []string{"task"},
				},
			},
		}
		tools = append(tools, tool)
	}

	return tools
}

// getDelegationDescription obtém a descrição otimizada para delegação
// Usa DelegationDescriptionProvider se implementado, senão usa formato padrão
func getDelegationDescription(agent Agent) string {
	// Verifica se o agente implementa a interface de descrição de delegação
	if provider, ok := agent.(DelegationDescriptionProvider); ok {
		return provider.GetDelegationDescription()
	}
	// Fallback para formato padrão
	return fmt.Sprintf("[%s] %s", agent.GetDisplayName(), agent.GetDescription())
}

// ExecuteDelegation executa uma delegação para um agente específico
func (r *Registry) ExecuteDelegation(ctx context.Context, agentName, task string) (string, error) {
	agent := r.Get(agentName)
	if agent == nil {
		return "", fmt.Errorf("agente não encontrado: %s", agentName)
	}

	if !agent.IsEnabled() {
		return "", fmt.Errorf("agente desabilitado: %s", agentName)
	}

	return agent.Execute(ctx, task)
}

// SetAgentConversationContext configura o contexto de conversa para um agente
// Isso permite que o agente salve suas mensagens internas no histórico
// Funciona para QUALQUER agente que implemente ConversationContextSetter (todos os agentes baseados em BaseAgent)
func (r *Registry) SetAgentConversationContext(agentName string, conversationID uint, saver llm.MessageSaver) {
	agent := r.Get(agentName)
	if agent == nil {
		return
	}

	// Usa type assertion para verificar se o agente suporta contexto de conversa
	// Isso funciona automaticamente para todos os agentes que herdam BaseAgent,
	// incluindo HTTP/MCP agents criados dinamicamente
	if contextSetter, ok := agent.(ConversationContextSetter); ok {
		contextSetter.SetConversationContext(conversationID, saver)
	}
}

// =====================================================
// Métodos legados (para compatibilidade durante transição)
// =====================================================

// GetAllTools retorna todas as tools de todos os agentes habilitados
// DEPRECATED: Use GetDelegationTools() para a nova arquitetura
func (r *Registry) GetAllTools() []Tool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var tools []Tool
	for _, agent := range r.agents {
		if agent.IsEnabled() {
			tools = append(tools, agent.GetTools()...)
		}
	}
	return tools
}

// FindAgentForTool encontra o agente que pode executar uma tool
// DEPRECATED: Use ExecuteDelegation() para a nova arquitetura
func (r *Registry) FindAgentForTool(toolName string) Agent {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for _, agent := range r.agents {
		if agent.IsEnabled() && agent.CanHandle(toolName) {
			return agent
		}
	}
	return nil
}

// ExecuteTool executa uma tool encontrando o agente apropriado
// DEPRECATED: Use ExecuteDelegation() para a nova arquitetura
func (r *Registry) ExecuteTool(toolCall ToolCall) (string, error) {
	agent := r.FindAgentForTool(toolCall.Function.Name)
	if agent == nil {
		return "", fmt.Errorf("nenhum agente encontrado para a tool: %s", toolCall.Function.Name)
	}
	return agent.ExecuteTool(toolCall)
}

// =====================================================
// Métodos para MCP Agents
// =====================================================

// RegisterMCPAgent registra um agente MCP e tenta conectar ao servidor
// O agente é registrado SEMPRE, mesmo se a conexão inicial falhar.
// A conexão pode ser tentada novamente quando o agente for usado (lazy connection).
func (r *Registry) RegisterMCPAgent(agent *MCPAgent) error {
	// Registra o agente primeiro (sempre disponível como ferramenta)
	r.Register(agent)

	// Tenta conectar (se falhar, o agente ainda está registrado)
	if err := agent.Connect(); err != nil {
		fmt.Printf("⚠️ [Registry] Erro ao conectar MCP agent %s: %v (agente registrado, conexão será tentada novamente no uso)\n", agent.GetName(), err)
		return err // Retorna erro para logging, mas agente está registrado
	}

	return nil
}

// DisconnectMCPAgents desconecta todos os agentes MCP
func (r *Registry) DisconnectMCPAgents() {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for _, agent := range r.agents {
		if mcpAgent, ok := agent.(*MCPAgent); ok {
			mcpAgent.Disconnect()
		}
	}
}

// GetMCPAgent retorna um agente MCP pelo nome
func (r *Registry) GetMCPAgent(name string) *MCPAgent {
	agent := r.Get(name)
	if agent == nil {
		return nil
	}
	mcpAgent, ok := agent.(*MCPAgent)
	if !ok {
		return nil
	}
	return mcpAgent
}
