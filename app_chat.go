package main

import (
	"context"
	"fmt"
	"log"

	"assistente/internal/chat"
	"assistente/internal/events"
	"assistente/internal/profiles"
	"assistente/internal/prompt"
	"assistente/internal/skills"
	"assistente/internal/toolprep"
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
	// Delega validação, renaming e resolução de perfil para o ChatInteractor
	pctx, err := a.chatInteractor.PrepareContext(context.Background(), chat.PrepareContextRequest{
		ConversationID: conversationID,
		UserContent:    userContent,
		UserMedia:      userMedia,
		Params:         params,
		Source:         source,
	})
	if err != nil {
		return 0, err
	}
	activeProfile := pctx.ActiveProfile
	params = pctx.Params
	userContent = pctx.UserContent

	// 0.5. Resolve conteúdo do usuário: extrai áudio do media e aplica STT fallback para canais.
	var sttProvider string
	if activeProfile != nil {
		sttProvider = activeProfile.Input.STTProvider
	}
	resolved := a.chatInteractor.ResolveUserContent(context.Background(), chat.ResolveUserContentRequest{
		Content:     userContent,
		Media:       userMedia,
		Source:      source,
		STTProvider: sttProvider,
		Transcribe:  a.whisperTranscribeFunc(),
	})
	userContent = resolved.Content

	// 1. Persiste mensagem do usuário, emite ready e carrega histórico via Interactor.
	rmsg, err := a.chatInteractor.RecordUserMessage(context.Background(), chat.RecordUserMessageRequest{
		ConversationID: conversationID,
		Content:        userContent,
		Media:          userMedia,
		AudioBase64:    resolved.AudioBase64,
		AudioMimeType:  resolved.AudioMimeType,
		Source:         source,
		ActiveProfile:  activeProfile,
		Transcribe:     a.whisperTranscribeFunc(),
	})
	if err != nil {
		return 0, err
	}
	userMsg := rmsg.UserMsg
	messages := rmsg.Messages
	conversationSummary := rmsg.ConversationSummary

	// 3.5–5. Detecta slash skill, compõe system prompt e pré-processa mídia.
	messages, invokedSkillSlug, invokedFilesystemScope := a.prepareMessages(
		messages, userContent, conversationSummary, conversationID, params, activeProfile,
	)

	// 6. Processa com LLM
	disableTools := activeProfile != nil && activeProfile.Chat.DisableTools
	var enabledTools []string
	if activeProfile != nil {
		enabledTools = activeProfile.Chat.EnabledTools
	}
	llmToolDefs := toolprep.BuildLLMToolDefs(a.toolRegistry, enabledTools, disableTools)

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
	requestStreamer, llmToolDefs = toolprep.ApplyNativeMCP(requestStreamer, llmToolDefs, a.mcpMgr, enabledTools, disableTools)

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
			a.runAgenticLoop(agentCtx, nil, messages, params, conversationID, userMsg.ID, llmToolDefs, requestStreamer)
		}()
	} else {
		// Sem ferramentas: streaming simples
		handler := &appStreamHandler{
			BaseStreamHandler: events.BaseStreamHandler{
				Emitter:        a.emitter,
				ConversationID: conversationID,
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
	b := a.promptBuilder
	if b == nil {
		b = a.newPromptBuilder()
	}
	return b.Build(messages, enabledSkills, disableOnDemand, skillTplData, slashSkillContent, conversationSummary)
}

// newPromptBuilder cria um Builder avulso com os deps atuais da App.
// Protege contra o trap de interface nil em Go (nil *Manager ≠ nil interface).
func (a *App) newPromptBuilder() *prompt.Builder {
	b := &prompt.Builder{Tools: a.toolRegistry}
	if a.skillMgr != nil {
		b.Skills = a.skillMgr
	}
	if a.workspaceMgr != nil {
		b.Workspace = a.workspaceMgr
	}
	return b
}

// prepareMessages detecta invocação de skill via /slash, injeta o system prompt completo
// e pré-processa mídias. Retorna as mensagens prontas + contexto de skill para o agentic loop.
func (a *App) prepareMessages(
	messages []Message,
	userContent, conversationSummary string,
	conversationID uint,
	params ChatParams,
	activeProfile *profiles.Profile,
) (out []Message, invokedSkillSlug string, invokedScope *tools.FilesystemScope) {
	skillTplData := a.promptBuilder.BuildTemplateData(activeProfile, params.ProfileSlug, conversationID)

	var slashSkillContent string
	if inv, found, _ := skills.Invoke(userContent, a.skillMgr, skillTplData, conversationID); found {
		slashSkillContent = inv.Content
		invokedSkillSlug = inv.SkillSlug
		if inv.Filesystem != nil {
			invokedScope = &tools.FilesystemScope{
				Read:  append([]string{}, inv.Filesystem.Read...),
				Write: append([]string{}, inv.Filesystem.Write...),
				Deny:  append([]string{}, inv.Filesystem.Deny...),
			}
		}
	}

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
	messages = a.preprocessMediaMessages(messages, activeProfile)
	return messages, invokedSkillSlug, invokedScope
}

// loadConversationHistory carrega o histórico de mensagens de uma conversa.
// Respeita rolling context: se há resumo, exclui mensagens já resumidas do contexto.
func (a *App) loadConversationHistory(conversationID uint, profile *profiles.Profile) ([]Message, string, error) {
	maxCtxMsgs := chat.DefaultMaxContextMessages
	if profile != nil {
		maxCtxMsgs = profile.GetMaxContextMessages()
	}
	loader := chat.MediaHistoryLoader{
		Repo:       a.msgRepo,
		Transcribe: a.whisperTranscribeFunc(),
		MaxMsgs:    maxCtxMsgs,
	}
	return loader.Load(conversationID)
}

// whisperTranscribeFunc cria o callback de transcrição para o MediaHistoryLoader e PreprocessMessages.
func (a *App) whisperTranscribeFunc() chat.TranscribeFunc {
	return func(audioBase64, filename string) (string, error) {
		if !a.ensureSpeechManager() {
			return "", nil
		}
		result, err := a.speechManager.Transcribe(audioBase64, filename)
		if err != nil {
			return "", err
		}
		if result == nil {
			return "", nil
		}
		return result.Text, nil
	}
}
