package agents

import (
	"context"
	"encoding/json"
	"fmt"
)

// ChatManager define as operações de gerenciamento de chat
type ChatManager interface {
	// Navegação
	SwitchToTab(tabID uint) error
	OpenConversationInNewTab(conversationID uint) (uint, error)
	OpenConversationInCurrentTab(conversationID uint) error

	// Gerenciamento de Abas
	CloseTab(tabID uint) error
	RenameTab(tabID uint, newTitle string) error
	GetCurrentTabID() (uint, error)
	GetCurrentConversationID() (uint, error)

	// Gerenciamento de Conversas
	CreateNewConversation(title string) (uint, error)
	RenameConversation(conversationID uint, newTitle string) error
	DeleteConversation(conversationID uint) error
	ClearConversation(conversationID uint) error
	DeleteMessages(conversationID uint, messageIDs []uint) error
	GetConversationSummary(conversationID uint) (string, error)
}

// ChatManagerAgent gerencia conversas, abas e navegação
type ChatManagerAgent struct {
	BaseAgent
	manager ChatManager
}

// NewChatManagerAgent cria um novo ChatManagerAgent
func NewChatManagerAgent(manager ChatManager, llmClient LLMClient, model string) *ChatManagerAgent {
	if model == "" {
		model = "gpt-4o-mini"
	}

	return &ChatManagerAgent{
		BaseAgent: BaseAgent{
			Name:         "chatmanager",
			DisplayName:  "Chat Manager",
			Description:  chatManagerAgentDescription(),
			AgentType:    "internal",
			Model:        model,
			SystemPrompt: chatManagerAgentSystemPrompt(),
			Enabled:      true,
			LLM:          llmClient,
		},
		manager: manager,
	}
}

func chatManagerAgentDescription() string {
	return NewDelegationDescription("Chat Manager", "ACTIONS on tabs and conversations: navigate, create, rename, delete, clear, summarize.").
		Capabilities(
			"Navigate between open tabs",
			"Open past conversations in new or current tab",
			"Close tabs",
			"Create new conversations",
			"Rename tabs and conversations",
			"Delete conversations permanently",
			"Clear conversation messages",
			"Delete specific messages",
			"Summarize conversations",
		).
		DelegateWhen(
			"User wants to DELETE/REMOVE a conversation: 'delete', 'remove', 'apaga', 'exclui'",
			"User wants to CLEAR messages: 'clear', 'limpa', 'start over'",
			"User wants to navigate: 'go to that tab', 'switch to', 'show me that tab'",
			"User wants to open a conversation: 'open that', 'open it', 'abre'",
			"User wants to close: 'close this tab', 'close it', 'fecha'",
			"User wants to create: 'new conversation', 'new chat', 'nova conversa'",
			"User wants to rename: 'rename', 'change title', 'renomeia'",
			"User wants to delete messages: 'delete those messages', 'remove last N'",
			"User wants a summary: 'summarize', 'what did we discuss', 'resume'",
		).
		DontDelegateWhen(
			"User wants to SEARCH for tabs or conversations - use Memory Manager",
			"User wants to save/recall personal memories - use Memory Manager",
			"User just wants information, not actions on conversations",
		).
		Build()
}

func chatManagerAgentSystemPrompt() string {
	return `You are a COMMAND EXECUTOR for chat management. You receive tasks and EXECUTE them immediately.

CRITICAL: You are NOT a conversational agent. Do NOT ask for confirmation. The orchestrator has already confirmed with the user before delegating to you.

When you receive a task:
1. IDENTIFY which tool to use
2. CALL THE TOOL IMMEDIATELY
3. Report the result

TOOL MAPPING:
- "delete conversation" / "apagar conversa" → call delete_conversation (NO conversation_id needed, uses current)
- "clear conversation" / "limpar conversa" → call clear_conversation (NO conversation_id needed, uses current)
- "close tab" / "fechar aba" → call close_tab
- "switch to tab" / "ir para aba" → call switch_to_tab
- "new conversation" / "nova conversa" → call new_conversation
- "rename" → call rename_tab or rename_conversation
- "open conversation" → call open_conversation
- "summarize" / "resumir" → call summarize_conversation

NEVER ask "are you sure?" or "do you confirm?" - just EXECUTE.

After execution, respond in Portuguese with the result.`
}

// GetDelegationDescription retorna descrição otimizada para o orquestrador
func (a *ChatManagerAgent) GetDelegationDescription() string {
	return a.Description
}

// Execute recebe uma tarefa em linguagem natural
func (a *ChatManagerAgent) Execute(ctx context.Context, task string) (string, error) {
	if a.LLM == nil {
		return "", fmt.Errorf("LLM client não configurado para o agente %s", a.Name)
	}

	fmt.Printf("💬 [Chat Manager] Recebeu tarefa: %s\n", task)

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
		return "", fmt.Errorf("erro no Chat Manager: %w", err)
	}

	fmt.Printf("✅ [Chat Manager] Resposta: %s\n", truncate(result, 100))
	return result, nil
}

// GetTools retorna as tools disponíveis
func (a *ChatManagerAgent) GetTools() []Tool {
	return []Tool{
		// === NAVIGATION ===
		{
			Type: "function",
			Function: ToolFunction{
				Name: "switch_to_tab",
				Description: NewToolDescription("Switches to a specific open tab.").
					WhenToUse(
						"User says 'go to that tab', 'switch to it', 'show me that tab'",
						"User says 'go there', 'take me there' (for open tabs)",
					).
					Returns("Confirmation message").
					Build(),
				Parameters: JSONSchemaObject(
					map[string]interface{}{
						"tab_id": JSONSchemaInt("The tab ID to switch to"),
					},
					[]string{"tab_id"},
				),
			},
		},
		{
			Type: "function",
			Function: ToolFunction{
				Name: "open_conversation",
				Description: NewToolDescription("Opens a past conversation.").
					WhenToUse(
						"User says 'open that conversation', 'show me that discussion'",
						"User wants to see a past conversation from history",
					).
					Notes(
						"Use in_new_tab=true to open in new tab, false to replace current",
					).
					Returns("Confirmation message").
					Build(),
				Parameters: JSONSchemaObject(
					map[string]interface{}{
						"conversation_id": JSONSchemaInt("The conversation ID to open"),
						"in_new_tab":      JSONSchemaBool("Open in new tab (true) or current tab (false). Default: true"),
					},
					[]string{"conversation_id"},
				),
			},
		},
		// === TAB MANAGEMENT ===
		{
			Type: "function",
			Function: ToolFunction{
				Name: "close_tab",
				Description: NewToolDescription("CLOSES the tab (window). Conversation stays in history, NOT deleted.").
					WhenToUse(
						"User says 'fecha', 'close', 'fechar aba', 'close tab'",
						"User just wants to close the window, NOT delete anything",
					).
					Notes(
						"This does NOT delete the conversation - it stays in history",
						"Just closes the tab/window",
					).
					Returns("Success message").
					Build(),
				Parameters: JSONSchemaObject(
					map[string]interface{}{
						"tab_id": JSONSchemaInt("Optional: leave empty to close current tab"),
					},
					nil,
				),
			},
		},
		{
			Type: "function",
			Function: ToolFunction{
				Name: "rename_tab",
				Description: NewToolDescription("Renames a tab.").
					WhenToUse(
						"User says 'rename this tab', 'change tab name to X'",
						"User wants to change the title of a tab",
					).
					Returns("Confirmation message").
					Build(),
				Parameters: JSONSchemaObject(
					map[string]interface{}{
						"tab_id":    JSONSchemaInt("The tab ID to rename (optional, defaults to current tab)"),
						"new_title": JSONSchemaString("The new title for the tab"),
					},
					[]string{"new_title"},
				),
			},
		},
		// === CONVERSATION MANAGEMENT ===
		{
			Type: "function",
			Function: ToolFunction{
				Name: "new_conversation",
				Description: NewToolDescription("Creates a new conversation and opens it in a new tab.").
					WhenToUse(
						"User says 'new conversation', 'new chat', 'start fresh'",
						"User wants to start a new chat",
					).
					Returns("Confirmation with new conversation ID").
					Build(),
				Parameters: JSONSchemaObject(
					map[string]interface{}{
						"title": JSONSchemaString("Optional title for the new conversation"),
					},
					nil,
				),
			},
		},
		{
			Type: "function",
			Function: ToolFunction{
				Name: "rename_conversation",
				Description: NewToolDescription("Renames a conversation.").
					WhenToUse(
						"User says 'rename this conversation', 'change the title'",
						"User wants to change the conversation title",
					).
					Returns("Confirmation message").
					Build(),
				Parameters: JSONSchemaObject(
					map[string]interface{}{
						"conversation_id": JSONSchemaInt("The conversation ID (optional, defaults to current)"),
						"new_title":       JSONSchemaString("The new title for the conversation"),
					},
					[]string{"new_title"},
				),
			},
		},
		{
			Type: "function",
			Function: ToolFunction{
				Name: "delete_conversation",
				Description: NewToolDescription("DELETES the current conversation permanently. Just call it - no parameters needed.").
					WhenToUse(
						"User wants to delete: 'apaga', 'delete', 'remove', 'exclui'",
						"User confirmed: 'sim', 'yes', 'confirmo', 'pode'",
					).
					Notes(
						"JUST CALL THIS TOOL - no confirmation parameter needed",
						"Deletes the current conversation automatically",
					).
					Returns("Success or error message").
					Build(),
				Parameters: JSONSchemaObject(
					map[string]interface{}{
						"conversation_id": JSONSchemaInt("Optional: leave empty to delete current conversation"),
					},
					nil,
				),
			},
		},
		{
			Type: "function",
			Function: ToolFunction{
				Name: "clear_conversation",
				Description: NewToolDescription("CLEARS all messages but keeps the conversation. Like erasing a whiteboard.").
					WhenToUse(
						"User says 'limpa', 'clear', 'limpar mensagens', 'apagar mensagens'",
						"User wants to remove messages but keep the conversation",
					).
					Notes(
						"Messages are deleted, but conversation stays",
						"Different from delete_conversation which removes everything",
					).
					Returns("Success message").
					Build(),
				Parameters: JSONSchemaObject(
					map[string]interface{}{
						"conversation_id": JSONSchemaInt("Optional: leave empty to clear current conversation"),
					},
					nil,
				),
			},
		},
		{
			Type: "function",
			Function: ToolFunction{
				Name: "delete_messages",
				Description: NewToolDescription("Deletes specific messages from a conversation.").
					WhenToUse(
						"User says 'delete those messages', 'remove the last 3 messages'",
						"User wants to remove specific messages",
					).
					Notes(
						"message_ids should be the IDs of messages to delete",
						"For 'last N messages', first get message IDs from context",
					).
					Returns("Confirmation message").
					Build(),
				Parameters: JSONSchemaObject(
					map[string]interface{}{
						"conversation_id": JSONSchemaInt("The conversation ID (optional, defaults to current)"),
						"message_ids":     JSONSchemaArrayInt("Array of message IDs to delete"),
					},
					[]string{"message_ids"},
				),
			},
		},
		{
			Type: "function",
			Function: ToolFunction{
				Name: "summarize_conversation",
				Description: NewToolDescription("Gets a summary of a conversation.").
					WhenToUse(
						"User says 'summarize this', 'what did we discuss', 'give me an overview'",
						"User wants an overview of what was discussed",
					).
					Notes(
						"Uses the pre-generated summary if available",
						"Covers main topics, decisions, and key points",
					).
					Returns("A natural language summary of the conversation").
					Build(),
				Parameters: JSONSchemaObject(
					map[string]interface{}{
						"conversation_id": JSONSchemaInt("The conversation ID (optional, defaults to current)"),
					},
					nil,
				),
			},
		},
	}
}

// CanHandle verifica se o agente pode executar a tool
func (a *ChatManagerAgent) CanHandle(toolName string) bool {
	switch toolName {
	case "switch_to_tab", "open_conversation", "close_tab", "rename_tab",
		"new_conversation", "rename_conversation", "delete_conversation",
		"clear_conversation", "delete_messages", "summarize_conversation":
		return true
	}
	return false
}

// ExecuteTool executa uma tool do ChatManagerAgent
func (a *ChatManagerAgent) ExecuteTool(toolCall ToolCall) (string, error) {
	fmt.Printf("💬 [Chat Manager] ExecuteTool chamado: %s\n", toolCall.Function.Name)
	fmt.Printf("💬 [Chat Manager] Argumentos: %s\n", toolCall.Function.Arguments)

	var args map[string]interface{}
	if err := json.Unmarshal([]byte(toolCall.Function.Arguments), &args); err != nil {
		return "", fmt.Errorf("erro ao parsear argumentos: %v", err)
	}

	switch toolCall.Function.Name {
	case "switch_to_tab":
		return a.executeSwitchToTab(args)
	case "open_conversation":
		return a.executeOpenConversation(args)
	case "close_tab":
		return a.executeCloseTab(args)
	case "rename_tab":
		return a.executeRenameTab(args)
	case "new_conversation":
		return a.executeNewConversation(args)
	case "rename_conversation":
		return a.executeRenameConversation(args)
	case "delete_conversation":
		return a.executeDeleteConversation(args)
	case "clear_conversation":
		return a.executeClearConversation(args)
	case "delete_messages":
		return a.executeDeleteMessages(args)
	case "summarize_conversation":
		return a.executeSummarizeConversation(args)
	default:
		return "", fmt.Errorf("tool desconhecida: %s", toolCall.Function.Name)
	}
}

// === Tool Implementations ===

func (a *ChatManagerAgent) executeSwitchToTab(args map[string]interface{}) (string, error) {
	tabID, ok := args["tab_id"].(float64)
	if !ok {
		return `{"error": "tab_id é obrigatório"}`, nil
	}

	err := a.manager.SwitchToTab(uint(tabID))
	if err != nil {
		return fmt.Sprintf(`{"error": "Erro ao mudar de aba: %v"}`, err), nil
	}

	return `{"success": true, "message": "Mudei para a aba solicitada"}`, nil
}

func (a *ChatManagerAgent) executeOpenConversation(args map[string]interface{}) (string, error) {
	convID, ok := args["conversation_id"].(float64)
	if !ok {
		return `{"error": "conversation_id é obrigatório"}`, nil
	}

	inNewTab := true
	if val, exists := args["in_new_tab"].(bool); exists {
		inNewTab = val
	}

	if inNewTab {
		newTabID, err := a.manager.OpenConversationInNewTab(uint(convID))
		if err != nil {
			return fmt.Sprintf(`{"error": "Erro ao abrir conversa: %v"}`, err), nil
		}
		result := map[string]interface{}{
			"success":    true,
			"message":    "Abri a conversa em uma nova aba",
			"new_tab_id": newTabID,
		}
		jsonResult, _ := json.Marshal(result)
		return string(jsonResult), nil
	} else {
		err := a.manager.OpenConversationInCurrentTab(uint(convID))
		if err != nil {
			return fmt.Sprintf(`{"error": "Erro ao abrir conversa: %v"}`, err), nil
		}
		return `{"success": true, "message": "Abri a conversa na aba atual"}`, nil
	}
}

func (a *ChatManagerAgent) executeCloseTab(args map[string]interface{}) (string, error) {
	var tabID uint
	if val, ok := args["tab_id"].(float64); ok {
		tabID = uint(val)
	} else {
		// Pega a aba atual
		currentID, err := a.manager.GetCurrentTabID()
		if err != nil {
			return fmt.Sprintf(`{"error": "Erro ao obter aba atual: %v"}`, err), nil
		}
		tabID = currentID
	}

	err := a.manager.CloseTab(tabID)
	if err != nil {
		return fmt.Sprintf(`{"error": "Erro ao fechar aba: %v"}`, err), nil
	}

	return `{"success": true, "message": "Fechei a aba"}`, nil
}

func (a *ChatManagerAgent) executeRenameTab(args map[string]interface{}) (string, error) {
	newTitle, ok := args["new_title"].(string)
	if !ok || newTitle == "" {
		return `{"error": "new_title é obrigatório"}`, nil
	}

	var tabID uint
	if val, ok := args["tab_id"].(float64); ok {
		tabID = uint(val)
	} else {
		currentID, err := a.manager.GetCurrentTabID()
		if err != nil {
			return fmt.Sprintf(`{"error": "Erro ao obter aba atual: %v"}`, err), nil
		}
		tabID = currentID
	}

	err := a.manager.RenameTab(tabID, newTitle)
	if err != nil {
		return fmt.Sprintf(`{"error": "Erro ao renomear aba: %v"}`, err), nil
	}

	return fmt.Sprintf(`{"success": true, "message": "Renomeei a aba para '%s'"}`, newTitle), nil
}

func (a *ChatManagerAgent) executeNewConversation(args map[string]interface{}) (string, error) {
	title := "Nova Conversa"
	if val, ok := args["title"].(string); ok && val != "" {
		title = val
	}

	convID, err := a.manager.CreateNewConversation(title)
	if err != nil {
		return fmt.Sprintf(`{"error": "Erro ao criar conversa: %v"}`, err), nil
	}

	result := map[string]interface{}{
		"success":         true,
		"message":         fmt.Sprintf("Criei uma nova conversa: %s", title),
		"conversation_id": convID,
	}
	jsonResult, _ := json.Marshal(result)
	return string(jsonResult), nil
}

func (a *ChatManagerAgent) executeRenameConversation(args map[string]interface{}) (string, error) {
	newTitle, ok := args["new_title"].(string)
	if !ok || newTitle == "" {
		return `{"error": "new_title é obrigatório"}`, nil
	}

	var convID uint
	if val, ok := args["conversation_id"].(float64); ok {
		convID = uint(val)
	} else if a.ConversationID > 0 {
		convID = a.ConversationID
	} else {
		currentID, err := a.manager.GetCurrentConversationID()
		if err != nil {
			return fmt.Sprintf(`{"error": "Não consegui identificar a conversa atual: %v"}`, err), nil
		}
		convID = currentID
	}

	err := a.manager.RenameConversation(convID, newTitle)
	if err != nil {
		return fmt.Sprintf(`{"error": "Erro ao renomear conversa: %v"}`, err), nil
	}

	return fmt.Sprintf(`{"success": true, "message": "Renomeei a conversa para '%s'"}`, newTitle), nil
}

func (a *ChatManagerAgent) executeDeleteConversation(args map[string]interface{}) (string, error) {
	fmt.Println("🗑️ [Chat Manager] executeDeleteConversation CHAMADO!")
	fmt.Printf("🗑️ [Chat Manager] Args: %+v\n", args)
	fmt.Printf("🗑️ [Chat Manager] ConversationID do contexto: %d\n", a.ConversationID)

	var convID uint
	if val, ok := args["conversation_id"].(float64); ok {
		convID = uint(val)
		fmt.Printf("🗑️ [Chat Manager] Usando conversation_id do argumento: %d\n", convID)
	} else if a.ConversationID > 0 {
		// Usa o ConversationID do contexto setado pelo orquestrador
		convID = a.ConversationID
		fmt.Printf("🗑️ [Chat Manager] Usando ConversationID do contexto: %d\n", convID)
	} else {
		// Fallback para buscar do banco
		fmt.Println("🗑️ [Chat Manager] Buscando conversa da aba atual (fallback)...")
		currentID, err := a.manager.GetCurrentConversationID()
		if err != nil {
			fmt.Printf("🗑️ [Chat Manager] ERRO ao obter conversa atual: %v\n", err)
			return fmt.Sprintf(`{"error": "Não consegui identificar a conversa atual: %v"}`, err), nil
		}
		convID = currentID
		fmt.Printf("🗑️ [Chat Manager] Conversa da aba atual: %d\n", convID)
	}

	fmt.Printf("🗑️ [Chat Manager] Deletando conversa %d...\n", convID)
	err := a.manager.DeleteConversation(convID)
	if err != nil {
		fmt.Printf("🗑️ [Chat Manager] ERRO ao deletar: %v\n", err)
		return fmt.Sprintf(`{"error": "Erro ao deletar conversa: %v"}`, err), nil
	}

	fmt.Printf("🗑️ [Chat Manager] Conversa %d deletada com SUCESSO!\n", convID)
	return `{"success": true, "message": "Conversa apagada permanentemente"}`, nil
}

func (a *ChatManagerAgent) executeClearConversation(args map[string]interface{}) (string, error) {
	var convID uint
	if val, ok := args["conversation_id"].(float64); ok {
		convID = uint(val)
	} else if a.ConversationID > 0 {
		// Usa o ConversationID do contexto setado pelo orquestrador
		convID = a.ConversationID
		fmt.Printf("🧹 [Chat Manager] Usando ConversationID do contexto: %d\n", convID)
	} else {
		// Fallback para buscar do banco
		currentID, err := a.manager.GetCurrentConversationID()
		if err != nil {
			return fmt.Sprintf(`{"error": "Não consegui identificar a conversa atual: %v"}`, err), nil
		}
		convID = currentID
	}

	err := a.manager.ClearConversation(convID)
	if err != nil {
		return fmt.Sprintf(`{"error": "Erro ao limpar conversa: %v"}`, err), nil
	}

	return `{"success": true, "message": "Limpei todas as mensagens da conversa"}`, nil
}

func (a *ChatManagerAgent) executeDeleteMessages(args map[string]interface{}) (string, error) {
	var convID uint
	if val, ok := args["conversation_id"].(float64); ok {
		convID = uint(val)
	} else if a.ConversationID > 0 {
		convID = a.ConversationID
	} else {
		currentID, err := a.manager.GetCurrentConversationID()
		if err != nil {
			return fmt.Sprintf(`{"error": "Não consegui identificar a conversa atual: %v"}`, err), nil
		}
		convID = currentID
	}

	messageIDsRaw, ok := args["message_ids"].([]interface{})
	if !ok || len(messageIDsRaw) == 0 {
		return `{"error": "message_ids é obrigatório e deve conter pelo menos um ID"}`, nil
	}

	messageIDs := make([]uint, len(messageIDsRaw))
	for i, id := range messageIDsRaw {
		if idFloat, ok := id.(float64); ok {
			messageIDs[i] = uint(idFloat)
		}
	}

	err := a.manager.DeleteMessages(uint(convID), messageIDs)
	if err != nil {
		return fmt.Sprintf(`{"error": "Erro ao deletar mensagens: %v"}`, err), nil
	}

	return fmt.Sprintf(`{"success": true, "message": "Deletei %d mensagens"}`, len(messageIDs)), nil
}

func (a *ChatManagerAgent) executeSummarizeConversation(args map[string]interface{}) (string, error) {
	var convID uint
	if val, ok := args["conversation_id"].(float64); ok {
		convID = uint(val)
	} else if a.ConversationID > 0 {
		convID = a.ConversationID
	} else {
		currentID, err := a.manager.GetCurrentConversationID()
		if err != nil {
			return fmt.Sprintf(`{"error": "Não consegui identificar a conversa atual: %v"}`, err), nil
		}
		convID = currentID
	}

	summary, err := a.manager.GetConversationSummary(convID)
	if err != nil {
		return fmt.Sprintf(`{"error": "Erro ao obter resumo: %v"}`, err), nil
	}

	if summary == "" {
		return `{"message": "Esta conversa ainda não tem um resumo gerado. Tente novamente após mais mensagens."}`, nil
	}

	result := map[string]interface{}{
		"success": true,
		"summary": summary,
	}
	jsonResult, _ := json.Marshal(result)
	return string(jsonResult), nil
}

// OpenTabResult representa uma aba encontrada (usado pelo ContextSearcher)
type OpenTabResult struct {
	TabID          uint    `json:"tab_id"`
	ConversationID uint    `json:"conversation_id"`
	Title          string  `json:"title"`
	Summary        string  `json:"summary,omitempty"`
	Similarity     float32 `json:"similarity,omitempty"`
	IsActive       bool    `json:"is_active"`
}

// ConversationResult representa uma conversa encontrada no histórico
type ConversationResult struct {
	ConversationID uint    `json:"conversation_id"`
	Title          string  `json:"title"`
	Summary        string  `json:"summary"`
	Similarity     float32 `json:"similarity"`
	CreatedAt      string  `json:"created_at"`
	MessageCount   int     `json:"message_count"`
}
