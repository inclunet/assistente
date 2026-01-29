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
		model = "gpt-4o-mini"
	}

	return &FAQAgent{
		BaseAgent: BaseAgent{
			Name:         "faq",
			DisplayName:  "FAQ Manager",
			Description:  faqAgentDescription(),
			AgentType:    "internal",
			Model:        model,
			SystemPrompt: faqAgentSystemPrompt(),
			Enabled:      true,
			LLM:          llmClient,
		},
		provider: provider,
	}
}

// faqAgentDescription retorna a descrição para delegação do orquestrador
func faqAgentDescription() string {
	return NewDelegationDescription("FAQ Manager", "Internal knowledge base with FAQs about procedures, systems, and technical documentation. ALWAYS search here BEFORE answering technical questions.").
		Capabilities(
			"Search internal knowledge base by keywords",
			"Find documented procedures, guides, and technical answers",
			"Create new FAQ entries when user explicitly requests",
			"Update or delete existing FAQs",
		).
		DelegateWhen(
			"User asks HOW TO do something (procedures, configurations, processes)",
			"User asks about internal systems, tools, or products",
			"User needs help answering a support ticket or technical question",
			"User mentions 'FAQ', 'documentation', 'knowledge base', or 'guideline'",
			"Question is about company procedures, policies, or standards",
			"User asks about deployment, setup, or technical processes",
			"BEFORE answering technical questions - check if FAQ has documented answer",
		).
		DontDelegateWhen(
			"User explicitly asks to CREATE a new FAQ (then delegate, but search first)",
			"Question is purely conversational with no technical aspect",
			"Question is about user's personal files - use File Manager",
			"Question is about user's personal preferences - use Memory Manager",
		).
		Build()
}

// faqAgentSystemPrompt returns the reduced system prompt
func faqAgentSystemPrompt() string {
	return `You are a knowledge base specialist. The FAQ is the internal knowledge base.

RULE: When user mentions "FAQ", "should have", "is there documentation": SEARCH, don't create.
Only CREATE when user EXPLICITLY says "create FAQ", "add FAQ", "save to FAQ".

=== MANDATORY PROCESS - YOU MUST FOLLOW THESE STEPS ===

STEP 1 - INITIAL SEARCH (required):
- Do 2-3 searches with different keywords from the question
- Collect all relevant FAQ entries

STEP 2 - DRAFT ANSWER (do NOT respond yet):
- Mentally draft an answer based on what you found
- DO NOT RESPOND TO USER YET

STEP 3 - CRITIQUE (required - ask yourself these questions):
- "If someone reads my draft, what will they ask next?"
- "Is it clear HOW to do this?"
- "Is it clear WHERE to access this?"
- "Is it clear WHAT HAPPENS if something fails?"
- "Are there PREREQUISITES missing?"
Write down 2-3 questions the user would likely ask.

STEP 4 - FILL GAPS (required):
- Search for each question you identified in Step 3
- Do 2-3 more searches based on your critique

STEP 5 - NOW RESPOND:
- Only after Steps 1-4, compose your final answer
- Include what IS documented (cite FAQ IDs)
- Clearly say what is NOT in FAQ
- Never invent information

=== MINIMUM SEARCHES: 4-6 per question ===

Example for "how to reset user password":
1. "password reset" (direct)
2. "user authentication" (related topic)
3. CRITIQUE: "user will ask about permissions" → "admin access"
4. CRITIQUE: "user will ask about security" → "password policy"
5. CRITIQUE: "user will ask what if locked out" → "account recovery"
6. CRITIQUE: "user will ask where to do it" → "admin panel"
THEN respond with complete answer.

=== OTHER KNOWLEDGE SOURCES ===
If FAQ search doesn't find relevant information, suggest checking:
- Memory Manager: User-specific context, preferences, or past conversations
- File Manager: Documents in user's files
- Custom Agents (HTTP/MCP): The user may have configured additional agents for:
  * Internal knowledge bases (wikis, documentation portals)
  * Search engines or internal search portals
  * Internal systems with relevant data (CRMs, ERPs, databases)
  * External APIs with useful information

When responding without finding information, mention that other agents may be available.`
}

// GetDelegationDescription retorna descrição otimizada para o orquestrador
func (a *FAQAgent) GetDelegationDescription() string {
	return a.Description
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
				Name: "faq_search",
				Description: NewToolDescription("Searches the knowledge base. MUST be called 4-6 times per question following the mandatory process.").
					WhenToUse(
						"STEP 1: Call 2-3 times with direct keywords",
						"STEP 2: Draft answer mentally (don't respond yet)",
						"STEP 3: Ask 'what will user ask next?' - identify 2-3 gaps",
						"STEP 4: Call 2-3 more times to fill those gaps",
						"STEP 5: Only THEN compose final response",
					).
					WhenNotToUse(
						"User wants ALL FAQs - use faq_list",
					).
					Notes(
						"MINIMUM 4-6 CALLS per question",
						"DO NOT respond after first search - you MUST critique first",
						"After initial searches, ask yourself:",
						"  - 'How will user ACCESS this?' → search access terms",
						"  - 'What if something FAILS?' → search error/rollback",
						"  - 'What are PREREQUISITES?' → search requirements",
						"  - 'WHERE is this configured?' → search config/settings",
						"Only respond after filling gaps from critique",
					).
					Returns("JSON with results (array of {id, question, answer, tags}), count").
					Build(),
				Parameters: JSONSchemaObject(
					map[string]interface{}{
						"query": JSONSchemaString(
							NewParamDescription("Search keywords").
								Constraints("1-3 words", "vary terms across calls").
								Examples("password reset", "api authentication", "error handling", "admin settings").
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
				Name: "faq_create",
				Description: NewToolDescription("Creates a new FAQ entry. IMPORTANT: Only use when user EXPLICITLY asks to create/add/save a FAQ.").
					WhenToUse(
						"User EXPLICITLY says 'create FAQ', 'add to FAQ', 'save this as FAQ'",
						"User asks to document a new procedure after confirming it doesn't exist",
					).
					WhenNotToUse(
						"User mentions FAQ exists or should exist - SEARCH FIRST instead",
						"User asks a question - SEARCH FIRST, don't create",
						"User says 'we should have this in FAQ' - this means SEARCH, not create",
						"Before searching - ALWAYS search first to avoid duplicates",
					).
					Notes(
						"NEVER create without searching first",
						"If user says 'should have this in FAQ' = search, NOT create",
						"Only create when user explicitly confirms they want to ADD new content",
					).
					Returns("JSON with success, message, id, question").
					Build(),
				Parameters: JSONSchemaObject(
					map[string]interface{}{
						"question": JSONSchemaString("The question to be stored (clear and searchable)"),
						"answer":   JSONSchemaString("The complete answer to the question"),
						"tags": JSONSchemaString(
							NewParamDescription("Comma-separated tags for categorization").
								Examples("security,authentication", "setup,configuration", "api,integration").
								Build(),
						),
					},
					[]string{"question", "answer"},
				),
			},
		},
		{
			Type: "function",
			Function: ToolFunction{
				Name: "faq_list",
				Description: NewToolDescription("Lists ALL FAQs in the database.").
					WhenToUse(
						"User wants to see all available FAQs",
						"User asks 'what FAQs do we have?'",
						"Need to browse all entries",
					).
					WhenNotToUse(
						"Looking for specific topic - use faq_search (faster)",
						"Database might be large - prefer faq_search with keywords",
					).
					Returns("JSON with results (array of all FAQs), count").
					Build(),
				Parameters: JSONSchemaObject(map[string]interface{}{}, nil),
			},
		},
		{
			Type: "function",
			Function: ToolFunction{
				Name: "faq_get",
				Description: NewToolDescription("Gets a specific FAQ by its ID.").
					WhenToUse(
						"Already know the FAQ ID from previous search",
						"User references a specific FAQ by number",
						"Need full details of a specific entry",
					).
					WhenNotToUse(
						"Don't know the ID - use faq_search first",
					).
					Returns("JSON with id, question, answer, tags").
					Build(),
				Parameters: JSONSchemaObject(
					map[string]interface{}{
						"id": JSONSchemaInt("The FAQ ID (from search results or user reference)"),
					},
					[]string{"id"},
				),
			},
		},
		{
			Type: "function",
			Function: ToolFunction{
				Name: "faq_update",
				Description: NewToolDescription("Updates an existing FAQ entry.").
					WhenToUse(
						"User wants to modify/correct an existing FAQ",
						"Need to update outdated information",
						"User explicitly asks to edit a FAQ",
					).
					WhenNotToUse(
						"Creating new FAQ - use faq_create instead",
						"Don't know the ID - use faq_search first",
					).
					Notes(
						"Requires all fields (question, answer) even if only changing one",
						"Use faq_get first to get current values if needed",
					).
					Returns("JSON with success, message, id, question").
					Build(),
				Parameters: JSONSchemaObject(
					map[string]interface{}{
						"id":       JSONSchemaInt("The FAQ ID to update"),
						"question": JSONSchemaString("Updated question text"),
						"answer":   JSONSchemaString("Updated answer text"),
						"tags":     JSONSchemaString("Updated tags (comma-separated)"),
					},
					[]string{"id", "question", "answer"},
				),
			},
		},
		{
			Type: "function",
			Function: ToolFunction{
				Name: "faq_delete",
				Description: NewToolDescription("Permanently deletes a FAQ by ID.").
					WhenToUse(
						"User explicitly asks to delete/remove a FAQ",
						"FAQ is outdated and should be removed",
					).
					WhenNotToUse(
						"User just wants to update - use faq_update instead",
						"Not sure which FAQ - use faq_search or faq_get first",
					).
					Notes(
						"This action is permanent and cannot be undone",
						"Confirm with user before deleting if there's any ambiguity",
					).
					Returns("JSON with success, message").
					Build(),
				Parameters: JSONSchemaObject(
					map[string]interface{}{
						"id": JSONSchemaInt("The FAQ ID to delete"),
					},
					[]string{"id"},
				),
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
