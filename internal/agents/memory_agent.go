package agents

import (
	"context"
	"encoding/json"
	"fmt"

	"assistente/internal/memory"
)

// ContextSearcher define operações de busca em contexto (abas e histórico)
type ContextSearcher interface {
	SearchOpenTabs(query string, minSimilarity float32) ([]OpenTabResult, error)
	SearchConversationHistory(query string, topK int, minSimilarity float32) ([]ConversationResult, error)
}

// MemoryAgent é um agente inteligente que gerencia memórias
type MemoryAgent struct {
	BaseAgent
	provider        memory.Provider
	contextSearcher ContextSearcher
}

// NewMemoryAgent cria um novo MemoryAgent inteligente
func NewMemoryAgent(provider memory.Provider, llmClient LLMClient, model string) *MemoryAgent {
	if model == "" {
		model = "gpt-4o-mini"
	}

	return &MemoryAgent{
		BaseAgent: BaseAgent{
			Name:         "memory",
			DisplayName:  "Memory Manager",
			Description:  memoryAgentDescription(),
			AgentType:    "internal",
			Model:        model,
			SystemPrompt: memoryAgentSystemPrompt(),
			Enabled:      true,
			LLM:          llmClient,
		},
		provider: provider,
	}
}

// SetContextSearcher configura o buscador de contexto (abas e histórico)
func (a *MemoryAgent) SetContextSearcher(searcher ContextSearcher) {
	a.contextSearcher = searcher
}

// memoryAgentDescription returns the delegation description for the orchestrator
func memoryAgentDescription() string {
	return NewDelegationDescription("Memory Manager", "Persistent memory about the USER. Also searches open tabs and conversation history.").
		Capabilities(
			"Search and recall saved information about the user",
			"Save personal info, preferences, interests, context",
			"List all known information about the user",
			"Update or delete outdated information",
			"Search conversations in open tabs by topic",
			"Search past conversations from history",
		).
		DelegateWhen(
			"User asks 'do you know my...', 'what is my...', 'who is my...'",
			"User asks 'do you remember...', 'did I tell you...'",
			"User asks about their name, family, work, preferences",
			"BEFORE saying 'I don't know' about personal info - CHECK MEMORY FIRST",
			"User reveals INTERESTS: 'I like...', 'I love...', 'I enjoy...'",
			"User reveals PREFERENCES: 'I prefer...', 'I usually...', 'my favorite is...'",
			"User shares PERSONAL INFO: name, family, job, background",
			"User says 'remember this', 'save this', 'don't forget'",
			"User corrects info ('actually...', 'I changed...')",
			"User shares hobbies, passions, or areas of interest (save for future personalization)",
			"User asks if there's an open tab about something: 'is there a tab...', 'do I have open...'",
			"User asks about past conversations: 'we talked about...', 'remember when we discussed...'",
			"User references a previous discussion",
		).
		DontDelegateWhen(
			"Question is about procedures/documentation - use FAQ Manager",
			"Question is about files - use File Manager",
			"User explicitly says 'don't save this'",
			"Information is trivial/temporary (e.g., 'I'm tired today')",
			"User wants to NAVIGATE to a tab or open a conversation - use Chat Manager",
		).
		Build()
}

// memoryAgentSystemPrompt returns the system prompt
func memoryAgentSystemPrompt() string {
	return `You manage memories and context recall. This includes:
1. Persistent memories (saved info about the user)
2. Conversations in open tabs
3. Past conversation history

=== PERSISTENT MEMORIES ===
CATEGORIES (choose appropriate one):
- "core": Critical info (user's name, accessibility needs) - use sparingly
- "usuario": Personal info (role, team, background)
- "preferencia": Preferences (communication style, tools, settings)
- "projeto": Ongoing projects, current work context
- "contexto": General context from conversations

WHEN SAVING: Use clear titles, include keywords, choose correct category.
WHEN SEARCHING: Try multiple keyword variations.

=== CONTEXT SEARCH (tabs and history) ===
You can also search for context in:
- OPEN TABS: Use search_open_tabs when user asks "is there a tab about...", "do I have open..."
- PAST CONVERSATIONS: Use search_conversations when user says "we talked about...", "remember when we discussed..."

IMPORTANT:
- For user preferences/info → use memory_search first
- For "do you remember our conversation about X" → use search_conversations
- For "is there a tab open about X" → use search_open_tabs
- Remember the results (tab_id, conversation_id) so Chat Manager can use them if user wants to go there

RESPONSE STYLE:
- Be conversational and natural
- Describe what you found in a friendly way
- If you find a relevant conversation, mention it so user can ask Chat Manager to open it

=== OTHER KNOWLEDGE SOURCES ===
If nothing found, suggest checking FAQ Manager, File Manager, or custom agents.`
}

// GetDelegationDescription retorna descrição otimizada para o orquestrador
func (a *MemoryAgent) GetDelegationDescription() string {
	return a.Description
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
				Name: "memory_save",
				Description: NewToolDescription("Saves information about the user for future personalization. Be PROACTIVE - save when user reveals interests/preferences.").
					WhenToUse(
						"User reveals INTERESTS: 'I like...', 'I love...', 'I enjoy...', 'fascinates me'",
						"User reveals PREFERENCES: 'I prefer...', 'I usually...', 'my favorite...'",
						"User shares PERSONAL INFO: name, family, job, background, hobbies",
						"User says 'remember this', 'save this'",
						"User corrects previous info ('actually...', 'I changed...')",
						"BE PROACTIVE: Save interests/hobbies even if user doesn't ask",
					).
					WhenNotToUse(
						"Information is trivial/temporary ('I'm hungry', 'it's late')",
						"It's documentation/procedure - use FAQ instead",
						"User explicitly says don't save",
						"Similar memory might exist - search first to avoid duplicates",
					).
					Notes(
						"PROACTIVE SAVING: When user reveals interests/hobbies → save them!",
						"Search first if similar memory might exist",
						"Use clear titles: 'Interest: [topic]', 'Preference: [what they prefer]'",
						"Good categories: 'preferencia' for likes, 'usuario' for personal info",
					).
					Returns("JSON with success, message, id, title, category").
					Build(),
				Parameters: JSONSchemaObject(
					map[string]interface{}{
						"title": JSONSchemaString(
							NewParamDescription("Short, searchable title").
								Examples("Interest: cooking", "Preference: detailed explanations", "Works at: [company name]").
								Constraints("Include key terms", "prefix with type: Interest/Preference/Works at").
								Build(),
						),
						"content": JSONSchemaString("Detailed information to remember"),
						"category": JSONSchemaStringEnum(
							"Category: 'preferencia' for interests/likes, 'usuario' for personal info, 'projeto' for work context",
							[]string{"core", "usuario", "preferencia", "projeto", "contexto"},
						),
					},
					[]string{"title", "content"},
				),
			},
		},
		{
			Type: "function",
			Function: ToolFunction{
				Name: "memory_search",
				Description: NewToolDescription("Searches saved memories about the user. ALWAYS search before saying 'I don't know' about user info.").
					WhenToUse(
						"User asks 'do you know my...', 'what is my...', 'who is my...'",
						"User asks about their name, family, work, preferences",
						"User says 'you should know this', 'I already told you'",
						"BEFORE saying 'I don't know' - search first!",
						"Need to recall user preference or context",
					).
					WhenNotToUse(
						"Want to see ALL memories - use memory_list instead",
					).
					Notes(
						"Search is TEXT-BASED - use exact words likely in memory",
						"Try multiple searches with different terms",
						"For family info try: 'wife', 'spouse', 'family', 'children'",
						"For work info try: 'job', 'work', 'company', 'role'",
						"Try category names: 'usuario', 'preferencia', 'projeto'",
					).
					Returns("JSON with results (array of {id, title, content, category}), count").
					Build(),
				Parameters: JSONSchemaObject(
					map[string]interface{}{
						"query": JSONSchemaString(
							NewParamDescription("Search keywords").
								Constraints("Use words likely in memory title/content", "try multiple variations").
								Examples("name", "family", "work", "preference", "interest").
								Build(),
						),
					},
					[]string{"query"},
				),
			},
		},
		{
			Type: "function",
			Function: ToolFunction{
				Name: "memory_list",
				Description: NewToolDescription("Lists ALL saved memories, organized by category.").
					WhenToUse(
						"User asks 'what do you know about me?'",
						"User asks 'list my memories', 'show everything saved'",
						"Need overview of all stored information",
					).
					WhenNotToUse(
						"Looking for specific memory - use memory_search (faster)",
					).
					Returns("JSON with results (all memories), count").
					Build(),
				Parameters: JSONSchemaObject(map[string]interface{}{}, nil),
			},
		},
		{
			Type: "function",
			Function: ToolFunction{
				Name: "memory_delete",
				Description: NewToolDescription("Permanently deletes a memory by ID.").
					WhenToUse(
						"User asks to forget something specific",
						"User says information is outdated/wrong and should be removed",
						"Cleaning up duplicate or obsolete memories",
					).
					WhenNotToUse(
						"User wants to UPDATE info - delete old + save new",
						"Not sure which memory - search/list first",
					).
					Notes(
						"Deletion is permanent",
						"Use search or list to find the correct ID first",
					).
					Returns("JSON with success, message").
					Build(),
				Parameters: JSONSchemaObject(
					map[string]interface{}{
						"id": JSONSchemaInt("The memory ID to delete (from search/list results)"),
					},
					[]string{"id"},
				),
			},
		},
		// === Context Search Tools ===
		{
			Type: "function",
			Function: ToolFunction{
				Name: "search_open_tabs",
				Description: NewToolDescription("Searches conversations in currently open tabs by topic similarity.").
					WhenToUse(
						"User asks 'is there a tab about...', 'do I have a tab open...'",
						"User asks 'where was I talking about...'",
						"User mentions something they might have in another open tab",
					).
					WhenNotToUse(
						"User wants saved personal info - use memory_search",
						"User wants past conversations - use search_conversations",
					).
					Returns("List of matching open tabs with title, summary and tab_id").
					Build(),
				Parameters: JSONSchemaObject(
					map[string]interface{}{
						"query": JSONSchemaString("Topic or keywords to search for in open tabs"),
					},
					[]string{"query"},
				),
			},
		},
		{
			Type: "function",
			Function: ToolFunction{
				Name: "search_conversations",
				Description: NewToolDescription("Searches past conversations by topic. Finds discussions from history that are NOT currently open.").
					WhenToUse(
						"User asks 'do you remember when we talked about...'",
						"User says 'we discussed this before', 'I told you about this...'",
						"User wants to find a past conversation",
						"User references a previous discussion that's not in open tabs",
					).
					WhenNotToUse(
						"User wants saved personal info - use memory_search",
						"User asks about currently open tabs - use search_open_tabs",
					).
					Returns("List of matching past conversations with title, summary, date and conversation_id").
					Build(),
				Parameters: JSONSchemaObject(
					map[string]interface{}{
						"query": JSONSchemaString("Topic or keywords to search for in conversation history"),
					},
					[]string{"query"},
				),
			},
		},
	}
}

// CanHandle verifica se o agente pode executar a tool (usado internamente)
func (a *MemoryAgent) CanHandle(toolName string) bool {
	switch toolName {
	case "memory_save", "memory_search", "memory_list", "memory_delete",
		"search_open_tabs", "search_conversations":
		return true
	}
	return false
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
	case "search_open_tabs":
		return a.executeSearchOpenTabs(args)
	case "search_conversations":
		return a.executeSearchConversations(args)
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

func (a *MemoryAgent) executeSearchOpenTabs(args map[string]interface{}) (string, error) {
	if a.contextSearcher == nil {
		return `{"error": "Busca em abas não configurada"}`, nil
	}

	query, _ := args["query"].(string)
	if query == "" {
		return `{"error": "query é obrigatório"}`, nil
	}

	results, err := a.contextSearcher.SearchOpenTabs(query, 0.5)
	if err != nil {
		return fmt.Sprintf(`{"error": "Erro ao buscar: %v"}`, err), nil
	}

	if len(results) == 0 {
		return `{"results": [], "message": "Nenhuma aba aberta encontrada sobre esse assunto"}`, nil
	}

	items := make([]map[string]interface{}, len(results))
	for i, r := range results {
		items[i] = map[string]interface{}{
			"tab_id":    r.TabID,
			"title":     r.Title,
			"summary":   r.Summary,
			"is_active": r.IsActive,
		}
	}

	result := map[string]interface{}{
		"results": items,
		"count":   len(results),
	}
	jsonResult, _ := json.Marshal(result)
	return string(jsonResult), nil
}

func (a *MemoryAgent) executeSearchConversations(args map[string]interface{}) (string, error) {
	if a.contextSearcher == nil {
		return `{"error": "Busca em histórico não configurada"}`, nil
	}

	query, _ := args["query"].(string)
	if query == "" {
		return `{"error": "query é obrigatório"}`, nil
	}

	results, err := a.contextSearcher.SearchConversationHistory(query, 5, 0.5)
	if err != nil {
		return fmt.Sprintf(`{"error": "Erro ao buscar: %v"}`, err), nil
	}

	if len(results) == 0 {
		return `{"results": [], "message": "Nenhuma conversa passada encontrada sobre esse assunto"}`, nil
	}

	items := make([]map[string]interface{}, len(results))
	for i, r := range results {
		items[i] = map[string]interface{}{
			"conversation_id": r.ConversationID,
			"title":           r.Title,
			"summary":         r.Summary,
			"date":            r.CreatedAt,
		}
	}

	result := map[string]interface{}{
		"results": items,
		"count":   len(results),
	}
	jsonResult, _ := json.Marshal(result)
	return string(jsonResult), nil
}
