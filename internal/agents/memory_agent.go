package agents

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"assistente/internal/memory"
)

// MemoryAgent é um agente inteligente que gerencia memórias
type MemoryAgent struct {
	BaseAgent
	provider memory.Provider
}

// NewMemoryAgent cria um novo MemoryAgent inteligente
func NewMemoryAgent(provider memory.Provider, llmClient LLMClient, model string) *MemoryAgent {
	if model == "" {
		model = "gpt-4o-mini" // Modelo padrão para agentes
	}

	return &MemoryAgent{
		BaseAgent: BaseAgent{
			Name:        "memory",
			DisplayName: "Memory Manager",
			Description: "Gerencia memórias persistentes sobre o usuário. Use para salvar informações importantes, preferências, contexto de projetos, ou qualquer coisa que deva ser lembrada entre conversas.",
			AgentType:   "internal",
			Model:       model,
			SystemPrompt: `Você é um especialista em gerenciamento de memórias persistentes.

Suas capacidades:
- memory_save: Salvar uma nova memória com título, conteúdo e categoria
- memory_search: Buscar memórias por termo
- memory_list: Listar todas as memórias salvas
- memory_delete: Deletar uma memória por ID

Categorias disponíveis:
- "core": Informações críticas (nome do usuário, necessidades de acessibilidade) - usar com moderação
- "usuario": Informações pessoais do usuário
- "preferencia": Preferências e configurações
- "projeto": Informações sobre projetos em andamento
- "contexto": Contexto geral de conversas anteriores

Instruções:
1. Analise a tarefa recebida
2. Decida qual(is) tool(s) usar
3. Execute as tools necessárias
4. Retorne uma resposta clara e útil

Seja conciso. Ao listar memórias, organize por categoria quando possível.`,
			Enabled: true,
			LLM:     llmClient,
		},
		provider: provider,
	}
}

// Execute recebe uma tarefa em linguagem natural e usa o LLM para decidir como resolver
func (a *MemoryAgent) Execute(ctx context.Context, task string) (string, error) {
	if a.LLM == nil {
		return "", fmt.Errorf("LLM client não configurado para o agente %s", a.Name)
	}

	fmt.Printf("🧠 [Memory Agent] Recebeu tarefa: %s\n", task)

	// Usa o método com saver se disponível
	var result string
	var err error
	if a.MessageSaver != nil {
		result, err = a.LLM.ChatWithToolsAndSaver(
			ctx,
			a.Model,
			a.SystemPrompt,
			task,
			a.GetTools(),
			a.ExecuteTool,
			a.Name,
			a.MessageSaver,
		)
	} else {
		result, err = a.LLM.ChatWithTools(
			ctx,
			a.Model,
			a.SystemPrompt,
			task,
			a.GetTools(),
			a.ExecuteTool,
		)
	}

	if err != nil {
		return "", fmt.Errorf("erro no Memory Agent: %w", err)
	}

	fmt.Printf("✅ [Memory Agent] Resposta: %s\n", truncate(result, 100))
	return result, nil
}

// GetTools retorna as tools disponíveis do MemoryAgent
func (a *MemoryAgent) GetTools() []Tool {
	return []Tool{
		{
			Type: "function",
			Function: ToolFunction{
				Name:        "memory_save",
				Description: "Salva uma nova memória persistente",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"title": map[string]interface{}{
							"type":        "string",
							"description": "Título curto para identificar a memória",
						},
						"content": map[string]interface{}{
							"type":        "string",
							"description": "Conteúdo da memória",
						},
						"category": map[string]interface{}{
							"type":        "string",
							"description": "Categoria: core, usuario, preferencia, projeto, contexto",
						},
					},
					"required": []string{"title", "content"},
				},
			},
		},
		{
			Type: "function",
			Function: ToolFunction{
				Name:        "memory_search",
				Description: "Busca memórias por termo",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"query": map[string]interface{}{
							"type":        "string",
							"description": "Termo de busca",
						},
					},
					"required": []string{"query"},
				},
			},
		},
		{
			Type: "function",
			Function: ToolFunction{
				Name:        "memory_list",
				Description: "Lista todas as memórias salvas",
				Parameters: map[string]interface{}{
					"type":       "object",
					"properties": map[string]interface{}{},
				},
			},
		},
		{
			Type: "function",
			Function: ToolFunction{
				Name:        "memory_delete",
				Description: "Deleta uma memória por ID",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"id": map[string]interface{}{
							"type":        "integer",
							"description": "ID da memória a deletar",
						},
					},
					"required": []string{"id"},
				},
			},
		},
	}
}

// CanHandle verifica se o agente pode executar a tool (usado internamente)
func (a *MemoryAgent) CanHandle(toolName string) bool {
	return strings.HasPrefix(toolName, "memory_")
}

// ExecuteTool executa uma tool do MemoryAgent
func (a *MemoryAgent) ExecuteTool(toolCall ToolCall) (string, error) {
	var args map[string]interface{}
	if err := json.Unmarshal([]byte(toolCall.Function.Arguments), &args); err != nil {
		return "", fmt.Errorf("erro ao parsear argumentos: %v", err)
	}

	switch toolCall.Function.Name {
	case "memory_save":
		return a.executeSave(args)
	case "memory_search":
		return a.executeSearch(args)
	case "memory_list":
		return a.executeList()
	case "memory_delete":
		return a.executeDelete(args)
	default:
		return "", fmt.Errorf("tool desconhecida: %s", toolCall.Function.Name)
	}
}

func (a *MemoryAgent) executeSave(args map[string]interface{}) (string, error) {
	title, _ := args["title"].(string)
	content, _ := args["content"].(string)
	category, _ := args["category"].(string)

	if title == "" || content == "" {
		return "Erro: título e conteúdo são obrigatórios", nil
	}

	if category == "" {
		category = "geral"
	}

	memData, err := a.provider.Create(title, content, category)
	if err != nil {
		return fmt.Sprintf("Erro ao salvar memória: %v", err), nil
	}

	result := map[string]interface{}{
		"success":  true,
		"message":  "Memória salva com sucesso",
		"id":       memData.ID,
		"title":    memData.Title,
		"category": memData.Category,
	}
	jsonResult, _ := json.Marshal(result)
	return string(jsonResult), nil
}

func (a *MemoryAgent) executeSearch(args map[string]interface{}) (string, error) {
	query, _ := args["query"].(string)

	if query == "" {
		return "Erro: termo de busca é obrigatório", nil
	}

	memories, err := a.provider.Search(query)
	if err != nil {
		return fmt.Sprintf("Erro ao buscar memórias: %v", err), nil
	}

	if len(memories) == 0 {
		return `{"results": [], "message": "Nenhuma memória encontrada"}`, nil
	}

	results := make([]map[string]interface{}, len(memories))
	for i, m := range memories {
		results[i] = map[string]interface{}{
			"id":       m.ID,
			"title":    m.Title,
			"content":  m.Content,
			"category": m.Category,
		}
	}

	result := map[string]interface{}{
		"results": results,
		"count":   len(memories),
	}
	jsonResult, _ := json.Marshal(result)
	return string(jsonResult), nil
}

func (a *MemoryAgent) executeList() (string, error) {
	memories, err := a.provider.GetAll()
	if err != nil {
		return fmt.Sprintf("Erro ao listar memórias: %v", err), nil
	}

	if len(memories) == 0 {
		return `{"results": [], "message": "Nenhuma memória salva"}`, nil
	}

	results := make([]map[string]interface{}, len(memories))
	for i, m := range memories {
		results[i] = map[string]interface{}{
			"id":       m.ID,
			"title":    m.Title,
			"content":  m.Content,
			"category": m.Category,
		}
	}

	result := map[string]interface{}{
		"results": results,
		"count":   len(memories),
	}
	jsonResult, _ := json.Marshal(result)
	return string(jsonResult), nil
}

func (a *MemoryAgent) executeDelete(args map[string]interface{}) (string, error) {
	id, ok := args["id"].(float64)
	if !ok {
		return "Erro: ID é obrigatório", nil
	}

	err := a.provider.Delete(uint(id))
	if err != nil {
		return fmt.Sprintf("Erro ao excluir memória: %v", err), nil
	}

	result := map[string]interface{}{
		"success": true,
		"message": fmt.Sprintf("Memória com ID %d excluída com sucesso", int(id)),
	}
	jsonResult, _ := json.Marshal(result)
	return string(jsonResult), nil
}
