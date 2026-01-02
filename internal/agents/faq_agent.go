package agents

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"assistente/internal/faq"
)

// FAQAgent é um agente inteligente que gerencia FAQs
type FAQAgent struct {
	BaseAgent
	provider faq.Provider
}

// NewFAQAgent cria um novo FAQAgent inteligente
func NewFAQAgent(provider faq.Provider, llmClient LLMClient, model string) *FAQAgent {
	if model == "" {
		model = "gpt-4o-mini" // Modelo padrão para agentes
	}

	return &FAQAgent{
		BaseAgent: BaseAgent{
			Name:        "faq",
			DisplayName: "FAQ Manager",
			Description: "Gerencia perguntas frequentes (FAQs). Use para criar, buscar, listar, atualizar ou deletar FAQs. Ideal para perguntas sobre documentação, procedimentos, ou informações que precisam ser consultadas frequentemente.",
			AgentType:   "internal",
			Model:       model,
			SystemPrompt: `Você é um especialista em gerenciamento de FAQs (Perguntas Frequentes).

## Suas capacidades:
- faq_create: Criar nova FAQ com pergunta, resposta e tags opcionais
- faq_search: Buscar FAQs por termo (busca em pergunta, resposta e tags)
- faq_list: Listar todas as FAQs cadastradas
- faq_get: Obter uma FAQ específica por ID
- faq_update: Atualizar pergunta, resposta ou tags de uma FAQ
- faq_delete: Deletar uma FAQ por ID

## Estratégia de Busca Inteligente:

Quando precisar encontrar informações no FAQ, **faça múltiplas buscas** com variações:

1. **Extraia palavras-chave**: Identifique os termos mais importantes da pergunta
2. **Busque por sinônimos**: Se "configurar" não encontrar, tente "config", "setup", "ajustar"
3. **Busque por conceitos**: Se procura "como instalar", tente também "instalação", "install"
4. **Seja incremental**: Comece específico, depois generalize
   - Primeira busca: termo exato ou específico
   - Segunda busca: palavras-chave principais separadas
   - Terceira busca: termo mais genérico ou relacionado

**Exemplo**: Para "como faço pra mudar a senha do wifi?"
- Busca 1: "senha wifi"
- Busca 2: "wifi" 
- Busca 3: "rede" ou "wireless"
- Busca 4: "senha" ou "password"

**IMPORTANTE**: Não desista após uma busca vazia. Tente pelo menos 3 variações diferentes antes de concluir que não há FAQ relevante.

## Instruções:
1. Analise a tarefa recebida
2. Se for busca, use a estratégia de busca inteligente acima
3. Execute as tools necessárias (pode ser múltiplas buscas)
4. Se encontrar FAQ relevante, use a resposta para ajudar o usuário
5. Retorne uma resposta clara e útil

Seja conciso e direto. Quando encontrar FAQ relevante, adapte a resposta ao contexto da pergunta original.`,
			Enabled: true,
			LLM:     llmClient,
		},
		provider: provider,
	}
}

// Execute recebe uma tarefa em linguagem natural e usa o LLM para decidir como resolver
func (a *FAQAgent) Execute(ctx context.Context, task string) (string, error) {
	if a.LLM == nil {
		return "", fmt.Errorf("LLM client não configurado para o agente %s", a.Name)
	}

	fmt.Printf("🤖 [FAQ Agent] Recebeu tarefa: %s\n", task)

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
		return "", fmt.Errorf("erro no FAQ Agent: %w", err)
	}

	fmt.Printf("✅ [FAQ Agent] Resposta: %s\n", truncate(result, 100))
	return result, nil
}

// GetTools retorna as tools disponíveis do FAQAgent
func (a *FAQAgent) GetTools() []Tool {
	return []Tool{
		{
			Type: "function",
			Function: ToolFunction{
				Name:        "faq_create",
				Description: "Cria uma nova FAQ com pergunta e resposta",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"question": map[string]interface{}{
							"type":        "string",
							"description": "A pergunta a ser cadastrada",
						},
						"answer": map[string]interface{}{
							"type":        "string",
							"description": "A resposta para a pergunta",
						},
						"tags": map[string]interface{}{
							"type":        "string",
							"description": "Tags separadas por vírgula (opcional)",
						},
					},
					"required": []string{"question", "answer"},
				},
			},
		},
		{
			Type: "function",
			Function: ToolFunction{
				Name:        "faq_search",
				Description: "Busca FAQs por termo na pergunta, resposta ou tags. Use palavras-chave simples. Pode ser chamada múltiplas vezes com diferentes termos para encontrar resultados. Ex: primeiro 'senha wifi', depois 'wifi', depois 'rede'.",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"query": map[string]interface{}{
							"type":        "string",
							"description": "Termo de busca - use 1-3 palavras-chave. Evite frases longas.",
						},
					},
					"required": []string{"query"},
				},
			},
		},
		{
			Type: "function",
			Function: ToolFunction{
				Name:        "faq_list",
				Description: "Lista todas as FAQs cadastradas",
				Parameters: map[string]interface{}{
					"type":       "object",
					"properties": map[string]interface{}{},
				},
			},
		},
		{
			Type: "function",
			Function: ToolFunction{
				Name:        "faq_get",
				Description: "Obtém uma FAQ específica por ID",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"id": map[string]interface{}{
							"type":        "integer",
							"description": "ID da FAQ",
						},
					},
					"required": []string{"id"},
				},
			},
		},
		{
			Type: "function",
			Function: ToolFunction{
				Name:        "faq_update",
				Description: "Atualiza uma FAQ existente",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"id": map[string]interface{}{
							"type":        "integer",
							"description": "ID da FAQ a atualizar",
						},
						"question": map[string]interface{}{
							"type":        "string",
							"description": "Nova pergunta",
						},
						"answer": map[string]interface{}{
							"type":        "string",
							"description": "Nova resposta",
						},
						"tags": map[string]interface{}{
							"type":        "string",
							"description": "Novas tags",
						},
					},
					"required": []string{"id", "question", "answer"},
				},
			},
		},
		{
			Type: "function",
			Function: ToolFunction{
				Name:        "faq_delete",
				Description: "Deleta uma FAQ por ID",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"id": map[string]interface{}{
							"type":        "integer",
							"description": "ID da FAQ a deletar",
						},
					},
					"required": []string{"id"},
				},
			},
		},
	}
}

// CanHandle verifica se o agente pode executar a tool (usado internamente)
func (a *FAQAgent) CanHandle(toolName string) bool {
	return strings.HasPrefix(toolName, "faq_")
}

// ExecuteTool executa uma tool do FAQAgent
func (a *FAQAgent) ExecuteTool(toolCall ToolCall) (string, error) {
	var args map[string]interface{}
	if err := json.Unmarshal([]byte(toolCall.Function.Arguments), &args); err != nil {
		return "", fmt.Errorf("erro ao parsear argumentos: %v", err)
	}

	switch toolCall.Function.Name {
	case "faq_create":
		return a.executeCreate(args)
	case "faq_search":
		return a.executeSearch(args)
	case "faq_list":
		return a.executeList()
	case "faq_get":
		return a.executeGet(args)
	case "faq_update":
		return a.executeUpdate(args)
	case "faq_delete":
		return a.executeDelete(args)
	default:
		return "", fmt.Errorf("tool desconhecida: %s", toolCall.Function.Name)
	}
}

func (a *FAQAgent) executeCreate(args map[string]interface{}) (string, error) {
	question, _ := args["question"].(string)
	answer, _ := args["answer"].(string)
	tags, _ := args["tags"].(string)

	if question == "" || answer == "" {
		return "Erro: pergunta e resposta são obrigatórios", nil
	}

	faqData, err := a.provider.Create(question, answer, tags)
	if err != nil {
		return fmt.Sprintf("Erro ao criar FAQ: %v", err), nil
	}

	result := map[string]interface{}{
		"success":  true,
		"message":  "FAQ criada com sucesso",
		"id":       faqData.ID,
		"question": faqData.Question,
	}
	jsonResult, _ := json.Marshal(result)
	return string(jsonResult), nil
}

func (a *FAQAgent) executeSearch(args map[string]interface{}) (string, error) {
	query, _ := args["query"].(string)

	if query == "" {
		return "Erro: termo de busca é obrigatório", nil
	}

	faqs, err := a.provider.Search(query)
	if err != nil {
		return fmt.Sprintf("Erro ao buscar FAQs: %v", err), nil
	}

	if len(faqs) == 0 {
		return `{"results": [], "message": "Nenhuma FAQ encontrada para a busca"}`, nil
	}

	results := make([]map[string]interface{}, len(faqs))
	for i, f := range faqs {
		results[i] = map[string]interface{}{
			"id":       f.ID,
			"question": f.Question,
			"answer":   f.Answer,
			"tags":     f.Tags,
		}
	}

	result := map[string]interface{}{
		"results": results,
		"count":   len(faqs),
	}
	jsonResult, _ := json.Marshal(result)
	return string(jsonResult), nil
}

func (a *FAQAgent) executeList() (string, error) {
	faqs, err := a.provider.GetAll()
	if err != nil {
		return fmt.Sprintf("Erro ao listar FAQs: %v", err), nil
	}

	if len(faqs) == 0 {
		return `{"results": [], "message": "Nenhuma FAQ cadastrada"}`, nil
	}

	results := make([]map[string]interface{}, len(faqs))
	for i, f := range faqs {
		results[i] = map[string]interface{}{
			"id":       f.ID,
			"question": f.Question,
			"answer":   f.Answer,
			"tags":     f.Tags,
		}
	}

	result := map[string]interface{}{
		"results": results,
		"count":   len(faqs),
	}
	jsonResult, _ := json.Marshal(result)
	return string(jsonResult), nil
}

func (a *FAQAgent) executeGet(args map[string]interface{}) (string, error) {
	id, ok := args["id"].(float64)
	if !ok {
		return "Erro: ID é obrigatório", nil
	}

	faqData, err := a.provider.Get(uint(id))
	if err != nil {
		return fmt.Sprintf("Erro: FAQ com ID %d não encontrada", int(id)), nil
	}

	result := map[string]interface{}{
		"id":       faqData.ID,
		"question": faqData.Question,
		"answer":   faqData.Answer,
		"tags":     faqData.Tags,
	}
	jsonResult, _ := json.Marshal(result)
	return string(jsonResult), nil
}

func (a *FAQAgent) executeUpdate(args map[string]interface{}) (string, error) {
	id, ok := args["id"].(float64)
	if !ok {
		return "Erro: ID é obrigatório", nil
	}

	question, _ := args["question"].(string)
	answer, _ := args["answer"].(string)
	tags, _ := args["tags"].(string)

	if question == "" || answer == "" {
		return "Erro: pergunta e resposta são obrigatórios", nil
	}

	faqData, err := a.provider.Update(uint(id), question, answer, tags)
	if err != nil {
		return fmt.Sprintf("Erro ao atualizar FAQ: %v", err), nil
	}

	result := map[string]interface{}{
		"success":  true,
		"message":  "FAQ atualizada com sucesso",
		"id":       faqData.ID,
		"question": faqData.Question,
	}
	jsonResult, _ := json.Marshal(result)
	return string(jsonResult), nil
}

func (a *FAQAgent) executeDelete(args map[string]interface{}) (string, error) {
	id, ok := args["id"].(float64)
	if !ok {
		return "Erro: ID é obrigatório", nil
	}

	err := a.provider.Delete(uint(id))
	if err != nil {
		return fmt.Sprintf("Erro ao excluir FAQ: %v", err), nil
	}

	result := map[string]interface{}{
		"success": true,
		"message": fmt.Sprintf("FAQ com ID %d excluída com sucesso", int(id)),
	}
	jsonResult, _ := json.Marshal(result)
	return string(jsonResult), nil
}

// truncate trunca uma string para exibição em logs
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
