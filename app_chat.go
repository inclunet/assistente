package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"

	"assistente/internal/chat"
	"assistente/internal/config"
	"assistente/internal/llm"
	mcplib "assistente/internal/mcp"
	"assistente/internal/profiles"
	"assistente/internal/skills"
	"assistente/internal/tools"
)

// SendMessage é o binding Wails para envio de mensagens. Source padrão: "wails".
// Se a conversa pertence a um canal externo (Signal, Telegram), a resposta do assistente
// também será reenviada ao mensageiro de origem (bridge bidirecional).
func (a *App) SendMessage(conversationID uint, userContent string, userMedia string, params ChatParams) (uint, error) {
	// Bridge: se a conversa é de canal externo, registra callback para reenviar resposta
	if conversationID > 0 && a.msgGateway != nil && a.responseNotifier != nil {
		a.registerChannelBridge(conversationID)
	}
	return a.sendMessageInternal(conversationID, userContent, userMedia, params, "wails")
}

// SendMessageFromChannel é chamado pelo Gateway de mensageria.
// Funciona como SendMessage mas permite especificar a origem (source).
func (a *App) SendMessageFromChannel(conversationID uint, content, media string, params ChatParams, source string) (uint, error) {
	return a.sendMessageInternal(conversationID, content, media, params, source)
}

// sendMessageInternal contém a lógica de processamento de mensagens.
// Usado por SendMessage (Wails) e SendMessageFromChannel (mensageiros).
func (a *App) sendMessageInternal(conversationID uint, userContent string, userMedia string, params ChatParams, source string) (uint, error) {
	// Validação de tamanho do conteúdo
	if len(userContent) > MaxMessageContentSize {
		errMsg := fmt.Sprintf("Mensagem muito grande (%d bytes). Máximo permitido: %d bytes", len(userContent), MaxMessageContentSize)
		a.emitter.Emit("chat:error", errMsg)
		return 0, fmt.Errorf("%s", errMsg)
	}

	// Validação de tamanho da mídia
	if len(userMedia) > MaxMediaSize {
		errMsg := fmt.Sprintf("Mídia muito grande (%d bytes). Máximo permitido: %d bytes", len(userMedia), MaxMediaSize)
		a.emitter.Emit("chat:error", errMsg)
		return 0, fmt.Errorf("%s", errMsg)
	}

	cfg, err := config.Load()
	if err != nil {
		a.emitter.Emit("chat:error", "Erro ao carregar configuração: "+err.Error())
		return 0, err
	}

	// Verifica se há alguma forma de autenticação disponível.
	// A APIKey no config.json é legada; provedores modernos usam o credential manager.
	// Só bloqueia se não houver NENHUMA credencial configurada (nem config, nem provedores).
	if cfg.APIKey == "" {
		providerCount, _ := a.providerSvc.Count()
		if providerCount == 0 {
			a.emitter.Emit("chat:error", "Nenhum provedor LLM configurado. Configure um provedor nas configurações.")
			return 0, fmt.Errorf("nenhum provedor LLM configurado")
		}
	}

	if conversationID == 0 {
		const errMsg = "conversationID é obrigatório — conversas devem ser criadas ao criar/resetar a tab"
		a.emitter.Emit("chat:error", errMsg)
		return 0, errors.New(errMsg)
	}

	// Auto-rename: se a conversa tem título genérico, atualiza com o conteúdo da primeira mensagem.
	if conversationID > 0 && userContent != "" {
		conv, convErr := a.convSvc.GetConversationInfo(conversationID)
		if convErr == nil && conv != nil && conv.Title == "Nova Conversa" {
			title := userContent
			if len(title) > 50 {
				title = title[:50]
			}
			if err := a.convSvc.UpdateConversation(conversationID, title, ""); err == nil {
				a.emitter.Emit("conversation:renamed", map[string]interface{}{
					"conversation_id": conversationID,
					"new_title":       title,
				})
			}
		}
	}

	// Obtém o perfil: usa profileSlug se especificado (canais), senão o ativo global
	var activeProfile *profiles.Profile
	var profileErr error
	if a.profileManager == nil {
		log.Printf("[SendMessage] profileManager não inicializado — continuando sem perfil")
	} else if params.ProfileSlug != "" {
		activeProfile, profileErr = a.profileManager.Get(params.ProfileSlug)
		if profileErr != nil {
			log.Printf("[SendMessage] Erro ao obter perfil '%s' do canal: %v — usando perfil ativo global", params.ProfileSlug, profileErr)
			activeProfile, profileErr = a.profileManager.GetActive()
		}
	} else {
		activeProfile, profileErr = a.profileManager.GetActive()
	}
	if profileErr != nil {
		log.Printf("[SendMessage] Erro ao obter perfil: %v", profileErr)
	}

	// Resolve sentinelas $default para provedor/modelo real
	if activeProfile != nil {
		activeProfile = a.resolveProfileDefaults(activeProfile)
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
		// MaxTokensMode: "legacy" (max_tokens) ou "completion_tokens" (max_completion_tokens)
		if activeProfile.Chat.MaxTokensMode != "" {
			params.MaxTokensMode = activeProfile.Chat.MaxTokensMode
		}

		// 3. Aplica configuração de reasoning effort
		params.ReasoningEffort = activeProfile.Chat.ReasoningEffort

		// 4. Aplica limites do agentic loop
		if activeProfile.Chat.MaxAgenticIterations > 0 {
			params.MaxAgenticIterations = activeProfile.Chat.MaxAgenticIterations
		}
		if activeProfile.Chat.ResponseTimeout > 0 {
			params.ResponseTimeout = activeProfile.Chat.ResponseTimeout
		}
	}

	// Se ainda não tem modelo, usa o padrão do config
	if params.Model == "" {
		params.Model = cfg.DefaultModel
		log.Printf("[SendMessage] Usando modelo padrão: %s", params.Model)
	}

	// 0.5. Se userContent está vazio e há mídia de áudio, transcreve automaticamente.
	// Isso garante que a transcrição fique salva no DB e visível na UI.
	// Também extrai o áudio base64 para persistir junto com a mensagem.
	var userAudioBase64 string
	var userAudioMime string
	if userMedia != "" {
		userAudioBase64, userAudioMime = extractAudioFromMedia(userMedia)
	}
	if userContent == "" && userMedia != "" {
		// Verifica se o perfil tem STT configurado como whisper_api (necessário para canais)
		if source != "wails" && activeProfile != nil {
			sttProvider := activeProfile.Input.STTProvider
			if sttProvider == "webspeech" || sttProvider == "" {
				log.Printf("[SendMessage] Canal %s: STT '%s' não suporta transcrição server-side — ignorando áudio", source, sttProvider)
				userContent = "[Mensagem de áudio recebida, mas transcrição automática não está configurada. Configure Whisper no perfil deste canal para processar mensagens de voz.]"
			}
		}
		if userContent == "" {
			userContent = a.transcribeAudioFromMedia(userMedia)
		}
	}

	// 1. Salva mensagem do usuário no banco (com source para badge visual e áudio persistido)
	userMsg, err := a.msgRepo.CreateMessage(chat.MessageOptions{
		ConversationID: conversationID,
		Role:           "user",
		Content:        userContent,
		Media:          userMedia,
		Audio:          userAudioBase64,
		AudioMimeType:  userAudioMime,
		Source:         source,
	})
	if err != nil {
		a.emitter.Emit("chat:error", "Erro ao salvar mensagem: "+err.Error())
		return 0, err
	}
	fmt.Printf("✅ Mensagem do usuário salva: ID=%d (source=%s)\n", userMsg.ID, source)

	// 2. Emite evento informando que mensagem do usuário foi criada
	//    Inclui o conteúdo para que o frontend atualize mensagens de canais (ex: transcrição de áudio)
	a.emitter.Emit("chat:messages_ready", map[string]interface{}{
		"conversationId": conversationID,
		"userMessageId":  userMsg.ID,
		"userContent":    userMsg.Content,
	})

	// 3. Carrega histórico da conversa para contexto (com rolling context)
	messages, conversationSummary, err := a.loadConversationHistory(conversationID, activeProfile)
	if err != nil {
		a.emitter.Emit("chat:error", "Erro ao carregar histórico: "+err.Error())
		return 0, err
	}

	// 3.5. Detecta invocação de skill via /slash command
	var slashSkillContent string
	invokedSkillSlug := ""
	var invokedFilesystemScope *tools.FilesystemScope

	// Contexto de template para skills (disponível via {{ .Profile }} e flags derivadas)
	// Isso permite que skills ajustem instruções conforme o perfil ativo (ex.: toolcalling ligado/desligado).
	skillTplData := a.buildSkillTemplateData(activeProfile, params.ProfileSlug, conversationID)
	if slug, args, ok := parseSlashCommand(userContent); ok && a.skillMgr != nil {
		skill, err := a.skillMgr.Get(slug)
		if err == nil && skill.IsUserInvocable() {
			log.Printf("[Skills] Slash command detectado: /%s args=%q", slug, args)
			invokedSkillSlug = slug
			if skill.Filesystem != nil {
				invokedFilesystemScope = &tools.FilesystemScope{
					Read:  append([]string{}, skill.Filesystem.Read...),
					Write: append([]string{}, skill.Filesystem.Write...),
					Deny:  append([]string{}, skill.Filesystem.Deny...),
				}
			}

			// Substitui $ARGUMENTS, $N, e variáveis de sessão no conteúdo
			sessionVars := map[string]string{
				"CLAUDE_SESSION_ID": fmt.Sprintf("%d", conversationID),
			}
			processedContent := skills.SubstituteArguments(skill.Content, args, sessionVars)
			processedContent = skills.ProcessTemplate(processedContent, skillTplData)

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
	var enabledSkills []string
	var disableOnDemand bool
	if activeProfile != nil {
		enabledSkills = activeProfile.Chat.EnabledSkills
		disableOnDemand = activeProfile.Chat.DisableOnDemandSkills
		if activeProfile.Chat.DisableSkills {
			enabledSkills = []string{}
		}
	}
	messages = a.buildFullSystemPrompt(messages, enabledSkills, disableOnDemand, skillTplData, slashSkillContent, conversationSummary)

	// 5. Pré-processamento de mídia:
	//    a) Converte formatos de áudio não suportados (aac, ogg, etc.) para texto via Whisper
	//    b) Se MediaSupport indica que o modelo não suporta áudio/documento, aplica fallback
	messages = a.preprocessMediaMessages(messages, activeProfile)

	// 6. Processa com LLM
	// Determina quais ferramentas estão habilitadas pelo perfil ativo
	var llmToolDefs []llm.ToolDefinition
	disableTools := activeProfile != nil && activeProfile.Chat.DisableTools

	if !disableTools && a.toolRegistry != nil && a.toolRegistry.Count() > 0 {
		var toolDefs []tools.ToolDefinition

		// Filtra ferramentas pelo perfil: nil/não especificado = todas, lista = apenas as listadas
		if activeProfile != nil && activeProfile.Chat.EnabledTools != nil {
			toolDefs = a.toolRegistry.FilterByNames(activeProfile.Chat.EnabledTools)
		} else {
			toolDefs = a.toolRegistry.ToDefinitions()
		}

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

	// Resolve o ChatProvider para o provedor do perfil ativo.
	if activeProfile == nil || activeProfile.Chat.LLMProvider == "" {
		errMsg := "Nenhum provedor LLM configurado no perfil ativo."
		a.emitter.Emit("chat:error", errMsg)
		return 0, fmt.Errorf("%s", errMsg)
	}

	requestStreamer, err := a.getChatProviderForProvider(activeProfile.Chat.LLMProvider)
	if err != nil {
		errMsg := fmt.Sprintf("Provedor LLM não disponível: %v", err)
		log.Printf("[SendMessage] ERRO: %s", errMsg)
		a.emitter.Emit("chat:error", errMsg)
		return 0, fmt.Errorf("%s", errMsg)
	}
	log.Printf("[SendMessage] ChatProvider resolvido para provedor: %s", activeProfile.Chat.LLMProvider)

	// MCP nativo: se provider suporta e há servidores HTTP elegíveis, configura native path.
	// Servidores HTTP elegíveis vão para o LLM via native connector; suas bridge tools
	// são removidas do llmToolDefs para evitar duplicatas.
	// Servidores STDIO/locais continuam via bridge/adapter normalmente.
	// Se o perfil tem EnabledTools, apenas tools permitidas passam pelo caminho nativo.
	if !disableTools && requestStreamer.SupportsNativeMCP() && a.mcpMgr != nil {
		nativeServers := a.mcpMgr.GetEligibleNativeMCPServers()
		if len(nativeServers) > 0 {
			var enabledSet map[string]bool
			if activeProfile.Chat.EnabledTools != nil {
				enabledSet = make(map[string]bool, len(activeProfile.Chat.EnabledTools))
				for _, n := range activeProfile.Chat.EnabledTools {
					enabledSet[n] = true
				}
			}

			var mcpConfigs []llm.MCPServerConfig
			nativeToolNames := make(map[string]bool)

			for _, srv := range nativeServers {
				cfg := llm.MCPServerConfig{
					Name:      srv.Name,
					URL:       srv.URL,
					AuthToken: srv.AuthToken,
					ToolNames: srv.ToolNames,
				}

				if enabledSet != nil {
					var allowed []string
					var allowedFull []string
					for _, fullName := range srv.ToolNames {
						if enabledSet[fullName] {
							if _, originalName, ok := mcplib.ParseToolName(fullName); ok {
								allowed = append(allowed, originalName)
							}
							allowedFull = append(allowedFull, fullName)
						}
					}
					if len(allowed) == 0 {
						log.Printf("[SendMessage] MCP nativo: servidor %q excluído (nenhuma tool habilitada no perfil)", srv.Name)
						continue
					}
					cfg.AllowedTools = allowed
					cfg.ToolNames = allowedFull
				}

				mcpConfigs = append(mcpConfigs, cfg)
				for _, tn := range cfg.ToolNames {
					nativeToolNames[tn] = true
				}
			}

			if len(mcpConfigs) > 0 {
				requestStreamer = requestStreamer.WithMCPServers(mcpConfigs)
				log.Printf("[SendMessage] MCP nativo: %d servidores HTTP configurados", len(mcpConfigs))
			}

			// Remove bridge tools que agora vão por native (evita duplicata)
			if len(nativeToolNames) > 0 {
				filtered := make([]llm.ToolDefinition, 0, len(llmToolDefs))
				for _, td := range llmToolDefs {
					if !nativeToolNames[td.Function.Name] {
						filtered = append(filtered, td)
					}
				}
				removed := len(llmToolDefs) - len(filtered)
				if removed > 0 {
					log.Printf("[SendMessage] MCP nativo: %d bridge tools removidas (nativas agora)", removed)
				}
				llmToolDefs = filtered
			}
		}
	}

	// Se há ferramentas disponíveis, usa o agentic loop; caso contrário, streaming simples
	// Cria contexto cancelável por conversa — permite barge-in (SIP) cancelar LLM em andamento.
	convCtx, convCancel := context.WithCancel(a.ctx)
	a.registerStreamingContext(conversationID, convCancel)

	if len(llmToolDefs) > 0 {
		agentCtx := convCtx
		if invokedSkillSlug != "" {
			agentCtx = tools.WithExecutionContext(agentCtx, tools.ExecutionContext{
				InvokedSkillSlug: invokedSkillSlug,
				Filesystem:       invokedFilesystemScope,
			})
		}
		go func() {
			defer a.recoverFromPanic(conversationID, "runAgenticLoop")
			defer a.unregisterStreamingContext(conversationID)
			a.runAgenticLoop(agentCtx, cfg, messages, params, conversationID, userMsg.ID, llmToolDefs, requestStreamer)
		}()
	} else {
		// Sem ferramentas: streaming simples
		handler := &appStreamHandler{
			baseStreamHandler: baseStreamHandler{
				emitter:        a.emitter,
				conversationID: conversationID,
			},
			app:           a,
			userMessageID: userMsg.ID,
		}
		go func() {
			defer a.recoverFromPanic(conversationID, "StreamChat")
			defer a.unregisterStreamingContext(conversationID)
			requestStreamer.StreamChat(convCtx, messages, params, handler)
		}()
	}
	return conversationID, nil
}

// DefaultSystemPrompt é re-exportado de internal/chat para compatibilidade.
var DefaultSystemPrompt = chat.DefaultSystemPrompt

// buildFullSystemPrompt composes the complete system prompt with DefaultSystemPrompt, skills injection, invoked skill, and conversation summary.
// enabledSkills: nil = todos os skills, [] = nenhum, ["slug1","slug2"] = apenas esses.
// slashSkillContent: conteúdo processado de um skill invocado via /slash (pode ser vazio).
// conversationSummary: resumo de mensagens antigas da conversa (rolling context).
func (a *App) buildFullSystemPrompt(messages []Message, enabledSkills []string, disableOnDemand bool, skillTplData any, slashSkillContent string, conversationSummary string) []Message {
	var parts []string

	// 1. Base prompt (DefaultSystemPrompt)
	// Only add DefaultSystemPrompt if skills are present or slash skill is invoked
	// (avoids "Developer instruction not enabled" on simple models)
	if len(enabledSkills) > 0 || slashSkillContent != "" {
		parts = append(parts, chat.DefaultSystemPrompt)
	}

	// 2. Skills injection (auto + available)
	skillsSection := a.buildSkillsPromptSection(enabledSkills, disableOnDemand, skillTplData)
	if skillsSection != "" {
		parts = append(parts, "\n\n"+skillsSection)
	}

	// 2.5. Invoked skill via /slash command
	if slashSkillContent != "" {
		parts = append(parts, "\n\n"+slashSkillContent)
	}

	// 3. Conversation summary (rolling context)
	if conversationSummary != "" {
		parts = append(parts, "\n\n<conversation_summary>\nSummary of earlier messages in this conversation (these messages are no longer in the context window but their content is captured below):\n\n"+conversationSummary+"\n</conversation_summary>")
	}

	return chat.InjectSystemPrompt(messages, strings.Join(parts, ""))
}

type skillTemplateData struct {
	Profile            *profiles.Profile
	ProfileSlug        string
	ToolCallingEnabled bool
	EnabledTools       []string
	EnabledToolCount   int
	ConversationID     uint
	// Workspace context
	WorkspaceName    string
	WorkspaceProfile string
	ActiveTabTitle   string
	ActiveTabType    string
	Tabs             []skillTabInfo
	TabCount         int
}

type skillTabInfo struct {
	Title     string
	Type      string
	ContentID string
	IsActive  bool
}

func (a *App) buildSkillTemplateData(activeProfile *profiles.Profile, profileSlug string, conversationID uint) skillTemplateData {
	enabledToolNames := a.computeEnabledToolNames(activeProfile)
	data := skillTemplateData{
		Profile:            activeProfile,
		ProfileSlug:        profileSlug,
		ToolCallingEnabled: len(enabledToolNames) > 0,
		EnabledTools:       enabledToolNames,
		EnabledToolCount:   len(enabledToolNames),
		ConversationID:     conversationID,
	}

	if a.workspaceMgr != nil {
		if ws := a.workspaceMgr.Active(); ws != nil {
			data.WorkspaceName = ws.Name
			data.WorkspaceProfile = ws.Profile
			data.TabCount = len(ws.Tabs.Items)
			data.Tabs = make([]skillTabInfo, 0, len(ws.Tabs.Items))
			for _, tab := range ws.Tabs.Items {
				isActive := tab.ID == ws.Tabs.Active
				info := skillTabInfo{
					Title:     tab.Title,
					Type:      string(tab.Type),
					ContentID: tab.ContentID,
					IsActive:  isActive,
				}
				data.Tabs = append(data.Tabs, info)
				if isActive {
					data.ActiveTabTitle = tab.Title
					data.ActiveTabType = string(tab.Type)
				}
			}
		}
	}

	return data
}

func (a *App) computeEnabledToolNames(activeProfile *profiles.Profile) []string {
	if activeProfile != nil && activeProfile.Chat.DisableTools {
		return nil
	}
	if a.toolRegistry == nil || a.toolRegistry.Count() == 0 {
		return nil
	}

	var toolDefs []tools.ToolDefinition
	if activeProfile != nil && activeProfile.Chat.EnabledTools != nil {
		toolDefs = a.toolRegistry.FilterByNames(activeProfile.Chat.EnabledTools)
	} else {
		toolDefs = a.toolRegistry.ToDefinitions()
	}
	if len(toolDefs) == 0 {
		return nil
	}

	names := make([]string, 0, len(toolDefs))
	for _, td := range toolDefs {
		names = append(names, td.Function.Name)
	}
	return names
}

// buildSkillsPromptSection constrói a seção de skills para o system prompt.
// enabledSkills: nil = usa auto_load do skill, [] = nenhum (skills desabilitados), ["slug1","slug2"] = autoload ordenado.
// disableOnDemand: true = não incluir skills sob demanda.
func (a *App) buildSkillsPromptSection(enabledSkills []string, disableOnDemand bool, skillTemplateData any) string {
	if a.skillMgr == nil {
		return ""
	}

	// Se enabledSkills é um slice vazio (não nil), skills estão explicitamente desabilitados
	if enabledSkills != nil && len(enabledSkills) == 0 {
		return ""
	}

	var autoSkills []skills.Skill
	var availableSkills []skills.Skill

	if enabledSkills != nil {
		// Perfil define lista explícita de autoload (ordenada)
		allSkills, err := a.skillMgr.GetAllSkillsFull()
		if err != nil {
			log.Printf("[Skills] Erro ao carregar skills: %v", err)
			return ""
		}

		autoSkills = skills.FilterByNamesOrdered(allSkills, enabledSkills)
		if !disableOnDemand {
			availableSkills = skills.FilterExcludeNames(allSkills, enabledSkills)
		}
	} else {
		// Sem lista no perfil: usa auto_load do próprio skill (backward compat)
		var err error
		autoSkills, err = a.skillMgr.GetAutoSkills()
		if err != nil {
			log.Printf("[Skills] Erro ao carregar auto skills: %v", err)
		}

		if !disableOnDemand {
			availableSkills, err = a.skillMgr.GetAvailableSkills()
			if err != nil {
				log.Printf("[Skills] Erro ao carregar available skills: %v", err)
			}
		}
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

			autoContent := s.Content
			autoContent = skills.ProcessTemplate(autoContent, skillTemplateData)
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

// DefaultMaxContextMessages é o limite padrão de mensagens no contexto
const DefaultMaxContextMessages = 50

// loadConversationHistory carrega o histórico de mensagens de uma conversa.
// Respeita rolling context: se há resumo, exclui mensagens já resumidas do contexto.
// O perfil define MaxContextMessages (limite de msgs) e ContextWindow (trigger de sumarização).
func (a *App) loadConversationHistory(conversationID uint, profile *profiles.Profile) ([]Message, string, error) {
	maxCtxMsgs := DefaultMaxContextMessages
	if profile != nil {
		maxCtxMsgs = profile.GetMaxContextMessages()
	}

	loader := chat.HistoryLoader{Repo: a.msgRepo, MaxMsgs: maxCtxMsgs}
	dbMessages, existingSummary, err := loader.Load(conversationID)
	if err != nil {
		return nil, "", err
	}

	messages := make([]Message, 0, len(dbMessages))
	for _, m := range dbMessages {
		// Otimização de contexto: omitir mensagens intermediárias de tool calling
		// de turnos anteriores. O modelo já processou esses resultados e produziu
		// uma resposta final com a informação sintetizada — reenviar a cadeia
		// tool_call→tool_result desperdiça tokens sem valor.
		// Dados completos permanecem no banco e visíveis na UI.
		if m.Role == "tool" {
			continue
		}
		if m.Role == "assistant" && m.ToolCalls != "" && strings.TrimSpace(m.Content) == "" {
			continue
		}

		msg := Message{
			Role:       m.Role,
			ToolCallID: m.ToolCallID,
		}

		// Assistant com conteúdo textual + tool_calls: preserva texto, descarta tool_calls.
		// O texto intermediário ("Vou buscar...") pode ter valor de contexto.

		// Processa conteúdo (pode ser texto simples ou multimodal)
		if m.Media != "" {
			var mediaParts []map[string]interface{}
			if err := json.Unmarshal([]byte(m.Media), &mediaParts); err == nil {
				var content []interface{}
				// Verifica se a mensagem já tem conteúdo textual (ex: transcrição de áudio já feita)
				hasTextContent := m.Content != ""
				if hasTextContent {
					content = append(content, map[string]interface{}{
						"type": "text",
						"text": m.Content,
					})
				}
				// Converte formato do banco para formato OpenAI (multimodal)
				for _, mp := range mediaParts {
					mediaType, _ := mp["type"].(string)
					data, _ := mp["data"].(string)
					name, _ := mp["name"].(string)

					if strings.HasPrefix(mediaType, "image/") {
						// Imagens: formato image_url com data URI
						content = append(content, map[string]interface{}{
							"type": "image_url",
							"image_url": map[string]interface{}{
								"url": fmt.Sprintf("data:%s;base64,%s", mediaType, data),
							},
						})
					} else if strings.HasPrefix(mediaType, "audio/") {
						// Se já temos transcrição no Content, não re-transcreve o áudio
						// (evita duplicação: content já tem o texto transcrito)
						if hasTextContent {
							log.Printf("[Media] Áudio ignorado no histórico — já temos transcrição no content")
							continue
						}
						audioFmt := strings.TrimPrefix(mediaType, "audio/")
						if supportedAudioFormats[audioFmt] {
							// Formato suportado (wav, mp3): envia direto como input_audio
							content = append(content, map[string]interface{}{
								"type": "input_audio",
								"input_audio": map[string]interface{}{
									"data":   data,
									"format": audioFmt,
								},
							})
						} else {
							// Formato não suportado (aac, ogg, webm, etc.):
							// tenta transcrever com Whisper imediatamente
							transcribed := false
							if a.ensureSpeechManager() {
								filename := whisperFilename(audioFmt)
								log.Printf("[Media] Tentando transcrever áudio %s via Whisper (filename=%s)", audioFmt, filename)
								result, err := a.speechManager.Transcribe(data, filename)
								if err == nil && result.Text != "" {
									log.Printf("[Media] Áudio %s transcrito via Whisper ao carregar histórico: %s", audioFmt, truncateStr(result.Text, 100))
									content = append(content, map[string]interface{}{
										"type": "text",
										"text": result.Text,
									})
									transcribed = true
								} else if err != nil {
									log.Printf("[Media] Erro ao transcrever %s via Whisper: %v", audioFmt, err)
								}
							}
							if !transcribed {
								// NUNCA enviar formato não suportado como input_audio.
								// Placeholder textual para não quebrar a API.
								log.Printf("[Media] Áudio %s não transcrito — adicionando placeholder textual", audioFmt)
								content = append(content, map[string]interface{}{
									"type": "text",
									"text": fmt.Sprintf("[Mensagem de áudio recebida (%s) — não foi possível transcrever]", audioFmt),
								})
							}
						}
					} else if mediaType == "application/pdf" || strings.HasPrefix(mediaType, "text/") {
						// Documentos (PDF, texto): envia como file para modelos que suportam
						content = append(content, map[string]interface{}{
							"type": "file",
							"file": map[string]interface{}{
								"filename":  name,
								"data":      data,
								"mime_type": mediaType,
							},
						})
					} else if strings.HasPrefix(mediaType, "video/") {
						// Vídeo: tenta enviar direto (Gemini suporta)
						content = append(content, map[string]interface{}{
							"type": "video",
							"video": map[string]interface{}{
								"data":      data,
								"mime_type": mediaType,
							},
						})
					} else {
						// Outros formatos: informa ao modelo como texto
						content = append(content, map[string]interface{}{
							"type": "text",
							"text": fmt.Sprintf("[Arquivo anexado: %s (%s)]", name, mediaType),
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

	return messages, existingSummary, nil
}
