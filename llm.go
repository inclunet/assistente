package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"assistente/internal/config"
	"assistente/internal/configdir"
	"assistente/internal/database"
	"assistente/internal/llm"
	"assistente/internal/skills"
	"assistente/internal/tools"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// Re-exporta tipos do pacote llm para compatibilidade
type Message = llm.Message
type ContentPart = llm.ContentPart
type ImageURL = llm.ImageURL
type StreamOptions = llm.StreamOptions
type ChatRequest = llm.ChatRequest
type ChatChoice = llm.ChatChoice
type Delta = llm.Delta
type Usage = llm.Usage
type ChatResponse = llm.ChatResponse
type StreamChunk = llm.StreamChunk
type Model = llm.Model
type ModelsResponse = llm.ModelsResponse
type ChatParams = llm.ChatParams
type SettingsInput = llm.SettingsInput

// Re-exporta funções utilitárias
var strPtr = llm.StrPtr

// ==================== StreamHandler Implementation ====================

// appStreamHandler implementa llm.StreamHandler usando *App
// NOVA ARQUITETURA v2: Hierarquia baseada na mensagem do usuário
// - n0: user/assistant (parentID=null)
// - n1: interações com agentes (parentID=userMessageID)
// - n2: interações do agente com tools (parentID=agentMessageID)
type appStreamHandler struct {
	app                *App
	conversationID     uint   // ID da conversa
	userMessageID      uint   // ID da mensagem do usuário (raiz da thread)
	accumulatedContent string // Conteúdo acumulado durante streaming

	// Reasoning/Thinking - chain of thought de modelos como DeepSeek, Claude, o1
	accumulatedReasoning string // Reasoning acumulado durante streaming
	isThinking           bool   // Flag indicando se está recebendo reasoning

	// Throttling para eventos de streaming
	mu            sync.Mutex  // Protege accumulatedContent durante throttling
	lastEmitTime  time.Time   // Última vez que um evento foi emitido
	throttleTimer *time.Timer // Timer para throttling
	pendingEmit   bool        // Flag indicando se há emissão pendente

	// Throttling para eventos de thinking
	lastThinkingEmitTime time.Time   // Última vez que um evento de thinking foi emitido
	thinkingTimer        *time.Timer // Timer para throttling de thinking
	pendingThinkingEmit  bool        // Flag indicando se há emissão de thinking pendente
}

func (h *appStreamHandler) OnChunk(content string) {
	h.mu.Lock()
	defer h.mu.Unlock()

	// Acumula o conteúdo
	h.accumulatedContent += content

	// Throttling: emite eventos apenas a cada 50ms
	const throttleInterval = 50 * time.Millisecond
	now := time.Now()

	// Se já passou tempo suficiente desde última emissão, emite imediatamente
	if now.Sub(h.lastEmitTime) >= throttleInterval {
		h.emitStreamEvent()
		h.lastEmitTime = now
		h.pendingEmit = false

		// Cancela timer pendente se houver
		if h.throttleTimer != nil {
			h.throttleTimer.Stop()
			h.throttleTimer = nil
		}
		return
	}

	// Caso contrário, agenda emissão se ainda não há uma pendente
	if !h.pendingEmit {
		h.pendingEmit = true

		// Calcula tempo restante até próxima emissão permitida
		remainingTime := throttleInterval - now.Sub(h.lastEmitTime)

		h.throttleTimer = time.AfterFunc(remainingTime, func() {
			h.mu.Lock()
			defer h.mu.Unlock()

			if h.pendingEmit {
				h.emitStreamEvent()
				h.lastEmitTime = time.Now()
				h.pendingEmit = false
			}
		})
	}
}

// emitStreamEvent emite o evento de streaming (deve ser chamado com mutex locked)
func (h *appStreamHandler) emitStreamEvent() {
	runtime.EventsEmit(h.app.ctx, "chat:stream", StreamEvent{
		Content:        h.accumulatedContent,
		Done:           false,
		ConversationId: h.conversationID,
	})
}

// OnThinking é chamado quando um chunk de reasoning/thinking é recebido
func (h *appStreamHandler) OnThinking(content string) {
	h.mu.Lock()
	defer h.mu.Unlock()

	// Marca que está pensando
	if !h.isThinking {
		h.isThinking = true
		// Emite evento de início de thinking imediatamente
		runtime.EventsEmit(h.app.ctx, "chat:thinking", map[string]interface{}{
			"content":        content,
			"done":           false,
			"conversationId": h.conversationID,
			"started":        true,
		})
	}

	// Acumula o reasoning
	h.accumulatedReasoning += content

	// Throttling: emite eventos apenas a cada 50ms
	const throttleInterval = 50 * time.Millisecond
	now := time.Now()

	// Se já passou tempo suficiente desde última emissão, emite imediatamente
	if now.Sub(h.lastThinkingEmitTime) >= throttleInterval {
		h.emitThinkingEvent()
		h.lastThinkingEmitTime = now
		h.pendingThinkingEmit = false

		// Cancela timer pendente se houver
		if h.thinkingTimer != nil {
			h.thinkingTimer.Stop()
			h.thinkingTimer = nil
		}
		return
	}

	// Caso contrário, agenda emissão se ainda não há uma pendente
	if !h.pendingThinkingEmit {
		h.pendingThinkingEmit = true

		// Calcula tempo restante até próxima emissão permitida
		remainingTime := throttleInterval - now.Sub(h.lastThinkingEmitTime)

		h.thinkingTimer = time.AfterFunc(remainingTime, func() {
			h.mu.Lock()
			defer h.mu.Unlock()

			if h.pendingThinkingEmit {
				h.emitThinkingEvent()
				h.lastThinkingEmitTime = time.Now()
				h.pendingThinkingEmit = false
			}
		})
	}
}

// emitThinkingEvent emite o evento de thinking (deve ser chamado com mutex locked)
func (h *appStreamHandler) emitThinkingEvent() {
	runtime.EventsEmit(h.app.ctx, "chat:thinking", map[string]interface{}{
		"content":        h.accumulatedReasoning,
		"done":           false,
		"conversationId": h.conversationID,
	})
}

// OnThinkingDone é chamado quando o reasoning termina
func (h *appStreamHandler) OnThinkingDone(fullReasoning string) {
	h.mu.Lock()
	// Cancela qualquer timer pendente
	if h.thinkingTimer != nil {
		h.thinkingTimer.Stop()
		h.thinkingTimer = nil
	}
	h.pendingThinkingEmit = false
	h.isThinking = false
	h.mu.Unlock()

	// Emite evento final de thinking
	runtime.EventsEmit(h.app.ctx, "chat:thinking", map[string]interface{}{
		"content":        fullReasoning,
		"done":           true,
		"conversationId": h.conversationID,
	})

	fmt.Printf("🧠 [THINKING] Reasoning completo: %d caracteres\n", len(fullReasoning))
}

func (h *appStreamHandler) OnError(err string) {
	h.mu.Lock()
	// Cancela qualquer timer pendente
	if h.throttleTimer != nil {
		h.throttleTimer.Stop()
		h.throttleTimer = nil
	}
	h.pendingEmit = false
	content := h.accumulatedContent
	h.mu.Unlock()

	// Emite evento final de erro (sempre emite, sem throttling)
	runtime.EventsEmit(h.app.ctx, "chat:stream", StreamEvent{
		Content:        content,
		Done:           true,
		Error:          err,
		ConversationId: h.conversationID,
	})
}

// OnToolCalls é chamado quando o LLM solicita execução de ferramentas.
// Por enquanto (antes do agentic loop), loga e emite como done.
// Será substituído pelo agentic loop na Fase 5.
func (h *appStreamHandler) OnToolCalls(calls []llm.ToolCall, fullResponse string, usage llm.Usage, model string) {
	fmt.Printf("🔧 [TOOL_CALLS] LLM solicitou %d ferramentas (ainda não implementado)\n", len(calls))
	for _, call := range calls {
		fmt.Printf("   - %s(%s)\n", call.Function.Name, call.Function.Arguments)
	}
	// Delega para OnDone até o agentic loop ser implementado
	h.OnDone(fullResponse, usage, model)
}

func (h *appStreamHandler) OnDone(fullResponse string, usage llm.Usage, model string) {
	// Cancela qualquer timer pendente e obtém conteúdo acumulado
	h.mu.Lock()
	if h.throttleTimer != nil {
		h.throttleTimer.Stop()
		h.throttleTimer = nil
	}
	h.pendingEmit = false
	accumulatedContent := h.accumulatedContent
	accumulatedReasoning := h.accumulatedReasoning
	h.mu.Unlock()

	// Usa o conteúdo acumulado ou o fullResponse
	finalContent := fullResponse
	if finalContent == "" {
		finalContent = accumulatedContent
	}

	// Salva resposta final do assistant no nível 0 (sem parentID)
	// Inclui reasoning se houver
	if h.conversationID > 0 && finalContent != "" {
		msg, err := database.CreateMessage(database.MessageOptions{
			ConversationID:   h.conversationID,
			Role:             "assistant",
			Content:          finalContent,
			Reasoning:        accumulatedReasoning,
			PromptTokens:     usage.PromptTokens,
			CompletionTokens: usage.CompletionTokens,
			TotalTokens:      usage.TotalTokens,
			Model:            model,
		})
		if err != nil {
			// Se a conversa foi deletada ou mensagem pai não existe, aborta silenciosamente
			if errors.Is(err, database.ErrConversationDeleted) || errors.Is(err, database.ErrParentMessageDeleted) {
				fmt.Printf("🛑 Conversa %d foi deletada/limpa - abortando processamento\n", h.conversationID)
				return
			}
			fmt.Printf("❌ Erro ao salvar resposta do assistant: %v\n", err)
		} else {
			if accumulatedReasoning != "" {
				fmt.Printf("✅ Resposta do assistant salva: ID=%d (nível 0) com %d chars de reasoning\n", msg.ID, len(accumulatedReasoning))
			} else {
				fmt.Printf("✅ Resposta do assistant salva: ID=%d (nível 0)\n", msg.ID)
			}
		}
	}

	// Emite evento final de streaming
	runtime.EventsEmit(h.app.ctx, "chat:stream", StreamEvent{
		Content:        finalContent,
		Done:           true,
		ConversationId: h.conversationID,
		FullResponse:   finalContent,
	})

	// Emite evento para frontend recarregar a conversa
	runtime.EventsEmit(h.app.ctx, "chat:done", map[string]interface{}{
		"conversationId": h.conversationID,
	})
}

// ==================== Wails Bindings ====================

// GetModels retorna a lista de modelos disponíveis na API
func (a *App) GetModels() ([]string, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, err
	}
	return llm.GetModels(cfg)
}

// Constantes de validação de input
const (
	// MaxMessageContentSize é o tamanho máximo permitido para o conteúdo de uma mensagem (500KB)
	MaxMessageContentSize = 500 * 1024
	// MaxMediaSize é o tamanho máximo permitido para mídia em base64 (10MB)
	MaxMediaSize = 10 * 1024 * 1024
)

// SendMessage envia uma mensagem para a API com streaming
// NOVA ARQUITETURA: Backend gerencia todo o estado
// 1. Cria mensagens no banco ANTES de streamar
// 2. Emite evento único com IDs das mensagens criadas
// 3. Streaming emite eventos com messageId
func (a *App) SendMessage(conversationID uint, userContent string, userMedia string, params ChatParams) (uint, error) {
	// Validação de tamanho do conteúdo
	if len(userContent) > MaxMessageContentSize {
		errMsg := fmt.Sprintf("Mensagem muito grande (%d bytes). Máximo permitido: %d bytes", len(userContent), MaxMessageContentSize)
		runtime.EventsEmit(a.ctx, "chat:error", errMsg)
		return 0, fmt.Errorf("%s", errMsg)
	}

	// Validação de tamanho da mídia
	if len(userMedia) > MaxMediaSize {
		errMsg := fmt.Sprintf("Mídia muito grande (%d bytes). Máximo permitido: %d bytes", len(userMedia), MaxMediaSize)
		runtime.EventsEmit(a.ctx, "chat:error", errMsg)
		return 0, fmt.Errorf("%s", errMsg)
	}

	cfg, err := config.Load()
	if err != nil {
		runtime.EventsEmit(a.ctx, "chat:error", "Erro ao carregar configuração: "+err.Error())
		return 0, err
	}

	if cfg.APIKey == "" {
		runtime.EventsEmit(a.ctx, "chat:error", "API Key não configurada. Por favor, configure sua API Key nas configurações.")
		return 0, fmt.Errorf("API Key não configurada")
	}

	// Se não tem conversationID, cria uma nova conversa
	createdNew := false
	if conversationID == 0 {
		title := userContent
		if len(title) > 50 {
			title = title[:50]
		}
		conv, err := database.CreateConversation(title, params.Model)
		if err != nil {
			runtime.EventsEmit(a.ctx, "chat:error", "Erro ao criar conversa: "+err.Error())
			return 0, err
		}
		conversationID = conv.ID
		createdNew = true
		fmt.Printf("✅ Nova conversa criada: ID=%d, título=%s\n", conversationID, title)

		// Atualiza a tab ativa com o novo conversation_id
		activeTab, err := database.GetActiveTab()
		if err == nil && activeTab != nil {
			err = database.LoadConversationInTab(activeTab.ID, conversationID)
			if err != nil {
				fmt.Printf("⚠️ Erro ao vincular conversa à tab: %v\n", err)
			} else {
				fmt.Printf("✅ Conversa %d vinculada à tab %d\n", conversationID, activeTab.ID)
			}
		}

		// Emite evento de criação para atualizar UI
		runtime.EventsEmit(a.ctx, "chat:conversation_created", map[string]interface{}{
			"id":    conversationID,
			"title": title,
		})
	}

	// Obtém o perfil ativo global
	activeProfile, profileErr := a.profileManager.GetActive()
	if profileErr != nil {
		log.Printf("[SendMessage] Erro ao obter perfil ativo: %v", profileErr)
		// Continua com valores padrão se houver erro
	}

	// Aplica configurações do perfil de chat ativo
	if activeProfile != nil {
		log.Printf("[SendMessage] Usando perfil: %s", activeProfile.Name)

		// 1. Aplica modelo do perfil (se não especificado nos params)
		if params.Model == "" && activeProfile.Chat.Model != "" {
			params.Model = activeProfile.Chat.Model
			log.Printf("[SendMessage] Modelo do perfil: %s", params.Model)
		}

		// 2. Aplica parâmetros do perfil
		if activeProfile.Chat.Temperature > 0 {
			params.Temperature = activeProfile.Chat.Temperature
		}
		if activeProfile.Chat.MaxTokens > 0 {
			params.MaxTokens = activeProfile.Chat.MaxTokens
		}
		if activeProfile.Chat.TopP > 0 {
			params.TopP = activeProfile.Chat.TopP
		}

		// 3. Aplica configuração de thinking/reasoning
		params.EnableThinking = activeProfile.Chat.EnableThinking
	}

	// Se ainda não tem modelo, usa o padrão do config
	if params.Model == "" {
		params.Model = cfg.DefaultModel
		log.Printf("[SendMessage] Usando modelo padrão: %s", params.Model)
	}

	// 1. Salva mensagem do usuário no banco
	userMsg, err := database.AddMessageWithMedia(conversationID, "user", userContent, userMedia)
	if err != nil {
		runtime.EventsEmit(a.ctx, "chat:error", "Erro ao salvar mensagem: "+err.Error())
		return 0, err
	}
	fmt.Printf("✅ Mensagem do usuário salva: ID=%d\n", userMsg.ID)

	// 2. Emite evento informando que mensagem do usuário foi criada
	runtime.EventsEmit(a.ctx, "chat:messages_ready", map[string]interface{}{
		"conversationId": conversationID,
		"userMessageId":  userMsg.ID,
		"createdNew":     createdNew,
	})

	// 3. Carrega histórico da conversa para contexto
	messages, err := a.loadConversationHistory(conversationID)
	if err != nil {
		runtime.EventsEmit(a.ctx, "chat:error", "Erro ao carregar histórico: "+err.Error())
		return 0, err
	}

	// 3.5. Detecta invocação de skill via /slash command
	var slashSkillContent string
	if slug, args, ok := parseSlashCommand(userContent); ok && a.skillMgr != nil {
		skill, err := a.skillMgr.Get(slug)
		if err == nil && skill.IsUserInvocable() {
			log.Printf("[Skills] Slash command detectado: /%s args=%q", slug, args)

			// Substitui $ARGUMENTS e $N no conteúdo
			processedContent := skills.SubstituteArguments(skill.Content, args)

			// Preprocessa !commands (respeita permissões de bash do skill)
			var allowedBashCmds []string
			if skill.Tools != nil && skill.Tools.BashCommands != nil {
				allowedBashCmds = skill.Tools.BashCommands.Allowed
			}
			processedContent = skills.PreprocessCommands(processedContent, allowedBashCmds)

			// Monta seção de contexto do skill invocado
			var sb strings.Builder
			sb.WriteString("<invoked_skill>\n")
			sb.WriteString("## ")
			sb.WriteString(skill.GetDisplayName())
			if skill.Type != "" {
				sb.WriteString(" [")
				sb.WriteString(skill.Type)
				sb.WriteString("]")
			}
			sb.WriteString("\n")
			sb.WriteString(processedContent)
			sb.WriteString("\n")

			// Progressive file loading: lista arquivos complementares do skill
			supplementary, _ := a.skillMgr.GetSkillFiles(slug)
			if len(supplementary) > 0 {
				sb.WriteString("\nSupporting files (use read_file to access when needed):\n")
				for _, f := range supplementary {
					sb.WriteString("- `")
					sb.WriteString(f)
					sb.WriteString("`\n")
				}
			}

			sb.WriteString("</invoked_skill>")
			slashSkillContent = sb.String()
		} else if err != nil {
			log.Printf("[Skills] Skill /%s não encontrado: %v", slug, err)
		}
	}

	// 4. Compõe system prompt completo (com skills do perfil)
	var profileSystemPrompt string
	var systemPromptPosition string
	var enabledSkills []string
	if activeProfile != nil {
		profileSystemPrompt = activeProfile.Chat.SystemPrompt
		systemPromptPosition = activeProfile.Chat.SystemPromptPosition
		if systemPromptPosition == "" {
			systemPromptPosition = "before"
		}
		enabledSkills = activeProfile.Chat.EnabledSkills
	}
	messages = a.buildFullSystemPrompt(messages, profileSystemPrompt, systemPromptPosition, enabledSkills, slashSkillContent)

	// 5. Processa com LLM
	// Determina quais ferramentas estão habilitadas pelo perfil ativo
	var llmToolDefs []llm.ToolDefinition
	if a.toolRegistry != nil && a.toolRegistry.Count() > 0 {
		var toolDefs []tools.ToolDefinition

		// Filtra ferramentas pelo perfil: nil = todas, lista = apenas as listadas
		if activeProfile != nil && activeProfile.Chat.EnabledTools != nil {
			toolDefs = a.toolRegistry.FilterByNames(activeProfile.Chat.EnabledTools)
			fmt.Printf("[Tools] Perfil '%s': %d ferramenta(s) habilitada(s) de %d\n",
				activeProfile.Name, len(toolDefs), a.toolRegistry.Count())
		} else {
			toolDefs = a.toolRegistry.ToDefinitions()
			fmt.Printf("[Tools] Perfil sem restrição: todas as %d ferramentas habilitadas\n", len(toolDefs))
		}

		// Converte para formato llm.ToolDefinition
		llmToolDefs = make([]llm.ToolDefinition, len(toolDefs))
		for i, td := range toolDefs {
			llmToolDefs[i] = llm.ToolDefinition{
				Type: td.Type,
				Function: llm.FunctionDefinition{
					Name:        td.Function.Name,
					Description: td.Function.Description,
					Parameters:  td.Function.Parameters,
				},
			}
		}
	}

	// Se há ferramentas disponíveis, usa o agentic loop; caso contrário, streaming simples
	if len(llmToolDefs) > 0 {
		go a.runAgenticLoop(a.ctx, cfg, messages, params, conversationID, userMsg.ID, llmToolDefs)
	} else {
		// Sem ferramentas: streaming simples (comportamento original)
		handler := &appStreamHandler{
			app:            a,
			conversationID: conversationID,
			userMessageID:  userMsg.ID,
		}
		go llm.StreamChat(a.ctx, cfg, messages, params, handler)
	}
	return conversationID, nil
}

// DefaultSystemPrompt is the base system prompt used when no custom prompt is provided
const DefaultSystemPrompt = `You are a helpful, intelligent assistant. You provide accurate, thoughtful responses and assist users with various tasks.

Key behaviors:
- Be concise but thorough
- When uncertain, acknowledge limitations
- Use markdown formatting for better readability
- Adapt your communication style to the user's needs`

// buildFullSystemPrompt composes the complete system prompt with base prompt, custom prompt, skills and invoked skill.
// enabledSkills: nil = todos os skills, [] = nenhum, ["slug1","slug2"] = apenas esses.
// slashSkillContent: conteúdo processado de um skill invocado via /slash (pode ser vazio).
func (a *App) buildFullSystemPrompt(messages []Message, customPrompt string, customPosition string, enabledSkills []string, slashSkillContent string) []Message {
	// Build the system prompt parts
	var parts []string

	// 1. Base prompt (custom or default)
	basePrompt := customPrompt
	if basePrompt == "" {
		basePrompt = DefaultSystemPrompt
	}
	parts = append(parts, basePrompt)

	// 2. Skills injection (auto + available)
	skillsSection := a.buildSkillsPromptSection(enabledSkills)
	if skillsSection != "" {
		parts = append(parts, "\n\n"+skillsSection)
	}

	// 2.5. Invoked skill via /slash command
	if slashSkillContent != "" {
		parts = append(parts, "\n\n"+slashSkillContent)
	}

	// 3. Memory injection (memory.md sempre no contexto)
	memorySection := a.buildMemoryContext()
	if memorySection != "" {
		parts = append(parts, "\n\n"+memorySection)
	}

	// Combine all parts
	fullSystemPrompt := strings.Join(parts, "")

	// Find existing system message or create new one
	systemIndex := -1
	for i, msg := range messages {
		if msg.Role == "system" {
			systemIndex = i
			break
		}
	}

	if systemIndex == -1 {
		// No existing system message, add at the beginning
		systemMsg := Message{
			Role:    "system",
			Content: fullSystemPrompt,
		}
		return append([]Message{systemMsg}, messages...)
	}

	// Existing system message found - combine based on position
	existingContent := ""
	switch content := messages[systemIndex].Content.(type) {
	case string:
		existingContent = content
	default:
		// If not a string, replace entirely
		newMessages := make([]Message, len(messages))
		copy(newMessages, messages)
		newMessages[systemIndex].Content = fullSystemPrompt
		return newMessages
	}

	var combinedContent string
	if customPosition == "after" {
		// Custom prompt goes after existing content
		combinedContent = existingContent + "\n\n" + fullSystemPrompt
	} else {
		// "before" is the default - custom prompt goes before existing content
		combinedContent = fullSystemPrompt + "\n\n" + existingContent
	}

	// Create new slice to avoid modifying the original
	newMessages := make([]Message, len(messages))
	copy(newMessages, messages)
	newMessages[systemIndex].Content = combinedContent

	return newMessages
}

// buildSkillsPromptSection constrói a seção de skills para o system prompt.
// Carrega skills auto (injeção direta) e skills disponíveis (referência para leitura sob demanda).
// enabledSkills: nil = todos, [] = nenhum, ["slug1"] = apenas esses.
func (a *App) buildSkillsPromptSection(enabledSkills []string) string {
	if a.skillMgr == nil {
		return ""
	}

	// Se enabledSkills é um slice vazio (não nil), nenhum skill habilitado
	if enabledSkills != nil && len(enabledSkills) == 0 {
		return ""
	}

	// Carrega skills auto (injetados no prompt)
	autoSkills, err := a.skillMgr.GetAutoSkills()
	if err != nil {
		log.Printf("[Skills] Erro ao carregar auto skills: %v", err)
		autoSkills = nil
	}

	// Carrega skills disponíveis (referenciados para leitura sob demanda)
	availableSkills, err := a.skillMgr.GetAvailableSkills()
	if err != nil {
		log.Printf("[Skills] Erro ao carregar available skills: %v", err)
		availableSkills = nil
	}

	// Filtra pelo perfil se enabledSkills não for nil
	if enabledSkills != nil {
		autoSkills = skills.FilterByNames(autoSkills, enabledSkills)
		availableSkills = skills.FilterByNames(availableSkills, enabledSkills)
	}

	if len(autoSkills) == 0 && len(availableSkills) == 0 {
		return ""
	}

	var sb strings.Builder

	// Seção de skills auto_load (conteúdo completo injetado)
	if len(autoSkills) > 0 {
		sb.WriteString("<auto_skills>\n")
		for i, s := range autoSkills {
			if i > 0 {
				sb.WriteString("\n")
			}
			sb.WriteString("## ")
			sb.WriteString(s.GetDisplayName())
			if s.Type != "" {
				sb.WriteString(" [")
				sb.WriteString(s.Type)
				sb.WriteString("]")
			}
			sb.WriteString("\n")

			// Preprocessa !commands no conteúdo auto_load
			autoContent := s.Content
			var allowedBashCmds []string
			if s.Tools != nil && s.Tools.BashCommands != nil {
				allowedBashCmds = s.Tools.BashCommands.Allowed
			}
			autoContent = skills.PreprocessCommands(autoContent, allowedBashCmds)

			sb.WriteString(autoContent)
			sb.WriteString("\n")

			// Progressive file loading: lista arquivos complementares do auto skill
			if a.skillMgr != nil {
				supplementary, _ := a.skillMgr.GetSkillFiles(s.Slug)
				if len(supplementary) > 0 {
					sb.WriteString("\nSupporting files (use read_file to access when needed):\n")
					for _, f := range supplementary {
						sb.WriteString("- `")
						sb.WriteString(f)
						sb.WriteString("`\n")
					}
				}
			}
		}
		sb.WriteString("</auto_skills>")
	}

	// Seção de skills disponíveis (referência para leitura via read_file)
	// Filtra skills com disable-model-invocation: true (modelo não pode invocá-los)
	var modelInvocableSkills []skills.Skill
	for _, s := range availableSkills {
		if s.IsModelInvocable() {
			modelInvocableSkills = append(modelInvocableSkills, s)
		}
	}

	if len(modelInvocableSkills) > 0 {
		if sb.Len() > 0 {
			sb.WriteString("\n\n")
		}
		sb.WriteString("<available_skills>\n")
		sb.WriteString("You have skills available that provide specialized instructions for specific tasks.\n")
		sb.WriteString("To use a skill, read its file using the read_file tool with the path indicated below.\n")
		sb.WriteString("Only read a skill when it's relevant to the current task.\n\n")
		for _, s := range modelInvocableSkills {
			sb.WriteString("- **")
			sb.WriteString(s.GetDisplayName())
			sb.WriteString("** (`")
			sb.WriteString(s.Slug)
			sb.WriteString("`)")
			if s.Type != "" {
				sb.WriteString(" [")
				sb.WriteString(s.Type)
				sb.WriteString("]")
			}
			sb.WriteString(": ")
			sb.WriteString(s.Description)
			sb.WriteString("\n  Path: `")
			sb.WriteString(s.Path)
			sb.WriteString("`\n")

			// Progressive file loading: lista arquivos complementares do skill
			if a.skillMgr != nil {
				supplementary, _ := a.skillMgr.GetSkillFiles(s.Slug)
				if len(supplementary) > 0 {
					sb.WriteString("  Supporting files:\n")
					for _, f := range supplementary {
						sb.WriteString("    - `")
						sb.WriteString(f)
						sb.WriteString("`\n")
					}
				}
			}
		}
		sb.WriteString("</available_skills>")
	}

	return sb.String()
}

// buildMemoryContext lê o arquivo memory.md do diretório de memória e retorna
// o conteúdo formatado para injeção no system prompt.
// Usa configdir.Resolver para resolução multi-diretório (exe < home < workdir).
// Apenas memory.md é carregado automaticamente; daily/weekly/monthly/yearly são sob demanda.
func (a *App) buildMemoryContext() string {
	resolver := configdir.NewResolver("memory")

	data, _, err := resolver.Read("memory.md")
	if err != nil {
		// memory.md não existe ainda — perfeitamente normal
		return ""
	}

	content := strings.TrimSpace(string(data))
	if content == "" {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("<user_memory>\n")
	sb.WriteString("The following are the user's core memories. Use this information to personalize your responses.\n")
	sb.WriteString("You can update this file at ~/.assistente/memory/memory.md when the user shares important personal information.\n\n")
	sb.WriteString(content)
	sb.WriteString("\n</user_memory>")

	return sb.String()
}

// MaxContextMessages define o limite de mensagens no contexto para evitar contextos muito grandes
const MaxContextMessages = 50

// loadConversationHistory carrega o histórico de mensagens de uma conversa
// OTIMIZADO: Busca apenas mensagens de nível 0 direto do SQL (sem carregar threads de agentes)
func (a *App) loadConversationHistory(conversationID uint) ([]Message, error) {
	// Busca apenas mensagens raiz (parentID IS NULL) direto do banco
	dbMessages, err := database.GetMessages(conversationID, nil)
	if err != nil {
		return nil, err
	}

	total := len(dbMessages)

	// Limita o contexto às últimas N mensagens para evitar contextos muito grandes
	if total > MaxContextMessages {
		// Mantém as primeiras 2 (system prompt + contexto inicial) + últimas (MaxContextMessages-2)
		kept := MaxContextMessages - 2
		dbMessages = append(dbMessages[:2], dbMessages[total-kept:]...)
		fmt.Printf("📋 [HISTORY] Conversa %d: %d msgs total, truncado para %d (limite: %d)\n",
			conversationID, total, len(dbMessages), MaxContextMessages)
	} else {
		fmt.Printf("📋 [HISTORY] Conversa %d: %d mensagens carregadas\n", conversationID, total)
	}

	messages := make([]Message, 0, len(dbMessages))
	for _, m := range dbMessages {
		msg := Message{
			Role:       m.Role,
			ToolCallID: m.ToolCallID,
		}

		// Reconstrói tool_calls do JSON armazenado no banco (para mensagens assistant com tool calls)
		if m.ToolCalls != "" {
			var toolCalls []llm.ToolCall
			if err := json.Unmarshal([]byte(m.ToolCalls), &toolCalls); err == nil {
				msg.ToolCalls = toolCalls
			}
		}

		// Processa conteúdo (pode ser texto simples ou multimodal)
		if m.Media != "" {
			var mediaParts []map[string]interface{}
			if err := json.Unmarshal([]byte(m.Media), &mediaParts); err == nil {
				var content []interface{}
				if m.Content != "" {
					content = append(content, map[string]interface{}{
						"type": "text",
						"text": m.Content,
					})
				}
				// Converte formato do banco para formato OpenAI
				for _, mp := range mediaParts {
					mediaType, _ := mp["type"].(string)
					data, _ := mp["data"].(string)

					// Determina o formato correto baseado no MIME type
					if strings.HasPrefix(mediaType, "image/") {
						content = append(content, map[string]interface{}{
							"type": "image_url",
							"image_url": map[string]interface{}{
								"url": fmt.Sprintf("data:%s;base64,%s", mediaType, data),
							},
						})
					} else if strings.HasPrefix(mediaType, "audio/") {
						content = append(content, map[string]interface{}{
							"type": "input_audio",
							"input_audio": map[string]interface{}{
								"data":   data,
								"format": strings.TrimPrefix(mediaType, "audio/"),
							},
						})
					} else {
						content = append(content, map[string]interface{}{
							"type": "text",
							"text": fmt.Sprintf("[Arquivo: %s (%s)]", mp["name"], mediaType),
						})
					}
				}
				msg.Content = content
			} else {
				msg.Content = m.Content
			}
		} else {
			msg.Content = m.Content
		}

		messages = append(messages, msg)
	}

	return messages, nil
}

// SendMessageSync envia uma mensagem sem streaming (para acessibilidade)
func (a *App) SendMessageSync(messages []Message, params ChatParams) (string, error) {
	cfg, err := config.Load()
	if err != nil {
		return "", err
	}

	if cfg.APIKey == "" {
		return "", fmt.Errorf("API Key não configurada")
	}

	return llm.SendMessageSync(cfg, messages, params)
}

// GetConfig retorna a configuração atual
func (a *App) GetConfig() (*config.Config, error) {
	return config.Load()
}

// SetChatModel atualiza apenas o modelo de chat na configuração
func (a *App) SetChatModel(model string) error {
	err := config.Update(func(existing *config.Config) *config.Config {
		existing.DefaultModel = model
		existing.ChatParams.Model = model
		return existing
	})
	if err != nil {
		return err
	}

	// Recarrega o cliente LLM para usar o novo modelo
	a.initLLMClient()

	log.Printf("[SetChatModel] Modelo atualizado para: %s", model)
	return nil
}

// SaveSettings salva as configurações
func (a *App) SaveSettings(input SettingsInput) error {
	// Aplica timeout padrão se não especificado
	responseTimeout := input.ResponseTimeout
	if responseTimeout <= 0 {
		responseTimeout = 180
	}

	err := config.Update(func(existing *config.Config) *config.Config {
		return &config.Config{
			APIKey:          input.APIKey,
			APIBaseURL:      input.APIBaseURL,
			DefaultModel:    input.ChatParams.Model,
			ResponseTimeout: responseTimeout,
			ChatParams: config.ModelParams{
				Model:       input.ChatParams.Model,
				Temperature: input.ChatParams.Temperature,
				MaxTokens:   input.ChatParams.MaxTokens,
				TopP:        input.ChatParams.TopP,
			},
			STTParams: config.STTParams{
				Provider:      input.STTParams.Provider,
				RecordingMode: input.STTParams.RecordingMode,
			},
		}
	})
	if err != nil {
		return err
	}

	// Atualiza o timeout do HTTP client
	llm.ConfigureResponseTimeout(responseTimeout)

	return nil
}

// SetDefaultModel salva o modelo padrão
func (a *App) SetDefaultModel(model string) error {
	return config.Update(func(cfg *config.Config) *config.Config {
		cfg.DefaultModel = model
		return cfg
	})
}

// TestConnection testa a conexão com a API
func (a *App) TestConnection() (bool, error) {
	models, err := a.GetModels()
	if err != nil {
		return false, err
	}
	if len(models) > 0 {
		return true, nil
	}
	return false, fmt.Errorf("nenhum modelo encontrado")
}

// TestConnectionWithModels testa a conexão e retorna os modelos disponíveis
func (a *App) TestConnectionWithModels() ([]string, error) {
	models, err := a.GetModels()
	if err != nil {
		return nil, fmt.Errorf("erro ao conectar com a API: %v", err)
	}
	if len(models) == 0 {
		return nil, fmt.Errorf("nenhum modelo encontrado na API")
	}
	return models, nil
}

// ResetConfig apaga o arquivo de configuração, resetando ao estado padrão
func (a *App) ResetConfig() error {
	configPath, err := config.GetConfigPath()
	if err != nil {
		return fmt.Errorf("erro ao obter caminho da configuração: %v", err)
	}

	// Verifica se o arquivo existe
	if _, err := os.Stat(configPath); err == nil {
		// Remove o arquivo
		if err := os.Remove(configPath); err != nil {
			return fmt.Errorf("erro ao remover arquivo de configuração: %v", err)
		}
	}

	return nil
}

// ResetDatabase apaga o banco de dados, resetando ao estado inicial
func (a *App) ResetDatabase() error {
	configPath, err := config.GetConfigPath()
	if err != nil {
		return fmt.Errorf("erro ao obter caminho do banco de dados: %v", err)
	}

	dbPath := filepath.Join(filepath.Dir(configPath), "conversations.db")

	// Fecha a conexão com o banco de dados antes de deletar
	if err := database.Close(); err != nil {
		return fmt.Errorf("erro ao fechar banco de dados: %v", err)
	}

	// Aguarda um momento para garantir que o arquivo foi liberado
	time.Sleep(100 * time.Millisecond)

	// Verifica se o arquivo existe
	if _, err := os.Stat(dbPath); err == nil {
		// Remove o banco de dados
		if err := os.Remove(dbPath); err != nil {
			return fmt.Errorf("erro ao remover banco de dados: %v", err)
		}

		// Remove arquivos auxiliares do SQLite (WAL e SHM)
		os.Remove(dbPath + "-wal")
		os.Remove(dbPath + "-shm")
	}

	// Reinicializa o banco de dados
	if err := database.Init(); err != nil {
		return fmt.Errorf("erro ao reinicializar banco: %v", err)
	}

	// Limpa as tabs (já que todas as conversas foram deletadas)
	if err := database.ClearAllTabs(); err != nil {
		log.Printf("[ResetDatabase] Erro ao limpar tabs: %v", err)
	}

	log.Println("[ResetDatabase] Banco resetado com sucesso")

	// Emite evento para o frontend limpar o estado
	runtime.EventsEmit(a.ctx, "database:reset")

	return nil
}

// parseSlashCommand detecta se uma mensagem é um slash command para invocar um skill.
// Formato: /skill-slug [argumentos...]
// Retorna (slug, args, true) se for um slash command válido, ("", "", false) caso contrário.
func parseSlashCommand(content string) (slug string, args string, ok bool) {
	content = strings.TrimSpace(content)

	// Deve começar com /
	if !strings.HasPrefix(content, "/") {
		return "", "", false
	}

	// Remove o /
	rest := content[1:]
	if rest == "" {
		return "", "", false
	}

	// Não deve começar com espaço (evita confusão com paths como "/ something")
	if rest[0] == ' ' {
		return "", "", false
	}

	// Separa slug dos argumentos (pelo primeiro espaço)
	parts := strings.SplitN(rest, " ", 2)
	slug = strings.ToLower(parts[0])

	// Valida que o slug parece um nome de skill (letras, números, hifens)
	for _, ch := range slug {
		if !((ch >= 'a' && ch <= 'z') || (ch >= '0' && ch <= '9') || ch == '-') {
			return "", "", false
		}
	}

	if len(parts) > 1 {
		args = strings.TrimSpace(parts[1])
	}

	return slug, args, true
}
