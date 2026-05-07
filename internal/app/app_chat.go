package app

import (
	"assistente/internal/chat"
	"assistente/internal/profiles"
	"assistente/internal/prompt"
)

// SendMessage é o binding Wails para envio de mensagens. Source padrão: "wails".
// A bridge canal↔Wails é gerenciada internamente pelo ChatController.
func (a *App) SendMessage(conversationID string, userContent string, userMedia string, params ChatParams) (string, error) {
	return a.chatCtrl.SendMessage(a.authenticatedContext(), conversationID, userContent, userMedia, params)
}

// RetryMessage reexecuta a resposta a partir de uma mensagem do usuário já persistida.
func (a *App) RetryMessage(conversationID string, messageID string, params ChatParams) (string, error) {
	return a.chatCtrl.RetryMessage(a.authenticatedContext(), conversationID, messageID, params)
}

// SendMessageFromChannel é chamado pelo Gateway de mensageria.
func (a *App) SendMessageFromChannel(conversationID string, content, media string, params ChatParams, source string) (string, error) {
	return a.chatCtrl.SendMessageFromChannel(a.authenticatedContext(), conversationID, content, media, params, source)
}

// DefaultSystemPrompt é re-exportado de internal/chat para compatibilidade.
var DefaultSystemPrompt = chat.DefaultSystemPrompt

// effectivePromptBuilder retorna a.promptBuilder se inicializado, ou constrói um Builder avulso.
// Protege contra o trap de interface nil em Go (nil *Manager ≠ nil interface).
// Em produção, a.promptBuilder é sempre não-nil após startup(). Usado por testes que criam &App{}.
func (a *App) effectivePromptBuilder() *prompt.Builder {
	if a.promptBuilder != nil {
		return a.promptBuilder
	}
	b := &prompt.Builder{Tools: a.toolRegistry}
	if a.skillMgr != nil {
		b.Skills = a.skillMgr
	}
	if a.workspaceMgr != nil {
		b.Workspace = a.workspaceMgr
	}
	return b
}

// buildFullSystemPrompt composes the complete system prompt with DefaultSystemPrompt, skills injection,
// invoked skill, and conversation summary. Used directly in unit tests via &App{}.
func (a *App) buildFullSystemPrompt(messages []Message, enabledSkills []string, disableOnDemand bool, skillTplData any, slashSkillContent string, conversationSummary string) []Message {
	return a.effectivePromptBuilder().Build(messages, enabledSkills, disableOnDemand, skillTplData, slashSkillContent, conversationSummary)
}

// loadConversationHistory carrega o histórico de mensagens de uma conversa.
// Respeita rolling context: se há resumo, exclui mensagens já resumidas do contexto.
func (a *App) loadConversationHistory(conversationID string, profile *profiles.Profile) ([]Message, string, error) {
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
		result, err := a.speechSvc.Transcribe(audioBase64, filename)
		if err != nil {
			return "", err
		}
		if result == nil {
			return "", nil
		}
		return result.Text, nil
	}
}
