package summarization

import (
	"context"
	"fmt"
	"log"
	"strings"

	"assistente/internal/chat"
	"assistente/internal/core/ports"
	"assistente/internal/credentials"
	"assistente/internal/events"
	"assistente/internal/llm"
	"assistente/internal/profiles"
)

// SummarizationRepository abstrai as operações de persistência necessárias para sumarização.
// Implementado por DBSummarizationStore; pode ser mockado em testes.
type SummarizationRepository interface {
	GetMessages(conversationID uint) ([]chat.Message, error)
	GetConversationSummary(conversationID uint) (summary string, upToMessageID uint, err error)
	IsSummarizingInProgress(conversationID uint) (bool, error)
	SetSummarizingInProgress(conversationID uint, inProgress bool) error
	UpdateConversationSummary(conversationID uint, summary string, upToMessageID uint) error
}

const (
	// charsPerToken é a heurística: ~4 caracteres por token.
	charsPerToken = 4

	// contextWindowSafetyMargin é a fração da janela reservada como margem de segurança.
	contextWindowSafetyMargin = 0.25
)

// SummaryPrompt é o system prompt usado para gerar resumos de conversas.
const SummaryPrompt = `You are a conversation summarizer. Your task is to create a concise but comprehensive summary of the conversation provided.

Rules:
- Preserve all key information: decisions, facts, user preferences, technical details, and action items
- If a previous summary is provided, integrate and extend it with the new messages (do not repeat what's already in the summary)
- Write in the same language as the conversation
- Be concise but don't lose important context
- Use bullet points for clarity
- Include any code snippets, file paths, or technical references that are relevant
- DO NOT add commentary or meta-text — output ONLY the summary`

// EstimateTokens estima a contagem de tokens de um texto usando heurística chars/token.
func EstimateTokens(text string) int {
	if len(text) == 0 {
		return 0
	}
	return (len(text) + charsPerToken - 1) / charsPerToken
}

// EstimateMessagesTokens estima o total de tokens para um slice de mensagens.
func EstimateMessagesTokens(messages []chat.Message) int {
	total := 0
	for _, m := range messages {
		total += EstimateTokens(m.Content)
		if m.ToolCalls != "" {
			total += EstimateTokens(m.ToolCalls)
		}
	}
	return total
}

// ShouldTriggerSummarization retorna true se o uso estimado do contexto exceder o budget seguro.
func ShouldTriggerSummarization(
	profile *profiles.Profile,
	contextMessages []chat.Message,
	existingSummary string,
) bool {
	if profile == nil || profile.Chat.ContextWindow <= 0 {
		return false
	}

	contextWindow := profile.Chat.ContextWindow
	maxTokens := profile.Chat.MaxTokens
	if maxTokens <= 0 {
		maxTokens = 4096
	}

	safetyMargin := int(float64(contextWindow) * contextWindowSafetyMargin)
	budget := contextWindow - maxTokens - safetyMargin
	if budget <= 0 {
		return false
	}

	estimated := EstimateMessagesTokens(contextMessages)
	if existingSummary != "" {
		estimated += EstimateTokens(existingSummary)
	}

	if estimated > budget {
		log.Printf("[Summary] Trigger: estimated %d tokens > budget %d (window=%d, maxTokens=%d, margin=%d)",
			estimated, budget, contextWindow, maxTokens, safetyMargin)
		return true
	}
	return false
}

// BuildSummarizationUserPrompt monta o user message para a chamada LLM de sumarização.
func BuildSummarizationUserPrompt(existingSummary string, messages []chat.Message) string {
	var sb strings.Builder

	if existingSummary != "" {
		sb.WriteString("## Previous Summary\n\n")
		sb.WriteString(existingSummary)
		sb.WriteString("\n\n---\n\n")
		sb.WriteString("## New Messages to Incorporate\n\n")
	} else {
		sb.WriteString("## Conversation to Summarize\n\n")
	}

	for _, m := range messages {
		sb.WriteString(fmt.Sprintf("**[%s]**: ", m.Role))
		content := m.Content
		if len(content) > 2000 {
			content = content[:2000] + "... [truncated]"
		}
		sb.WriteString(content)
		sb.WriteString("\n\n")
	}

	if existingSummary != "" {
		sb.WriteString("---\n\nPlease produce an updated summary that integrates the previous summary with the new messages above.")
	} else {
		sb.WriteString("---\n\nPlease produce a concise summary of the conversation above.")
	}

	return sb.String()
}

// ServiceConfig agrupa as dependências do Service.
type ServiceConfig struct {
	Repo            SummarizationRepository
	Emitter         events.Emitter
	LLMRegistry     *llm.ProviderRegistry
	CredMgr         *credentials.Manager
	ProfileManager  *profiles.Manager
	ProfileResolver func(*profiles.Profile) *profiles.Profile
}

// Service encapsula a lógica de sumarização de conversas, sem depender de Wails.
type Service struct {
	cfg ServiceConfig
}

// NewService cria um Service com as dependências fornecidas.
func NewService(cfg ServiceConfig) *Service {
	return &Service{cfg: cfg}
}

// CheckAndTriggerSummarization verifica se a conversa precisa de sumarização e dispara em background.
// Deve ser chamado APÓS a resposta do LLM ser salva.
func (s *Service) CheckAndTriggerSummarization(conversationID uint) {
	if conversationID == 0 {
		return
	}

	profile, err := s.cfg.ProfileManager.GetActive()
	if err != nil || profile == nil {
		return
	}
	if profile.Chat.ContextWindow <= 0 {
		return
	}

	allRootMessages, err := s.cfg.Repo.GetMessages(conversationID)
	if err != nil {
		log.Printf("[Summary] Erro ao carregar mensagens para check: %v", err)
		return
	}

	existingSummary, summaryUpToID, _ := s.cfg.Repo.GetConversationSummary(conversationID)

	var contextMessages []chat.Message
	for _, m := range allRootMessages {
		if m.ID > summaryUpToID {
			contextMessages = append(contextMessages, m)
		}
	}

	if ShouldTriggerSummarization(profile, contextMessages, existingSummary) {
		s.TriggerSummarizationInBackground(conversationID, profile, allRootMessages)
	}
}

// TriggerSummarizationInBackground lança uma goroutine para sumarizar mensagens antigas.
// Respeita MinContextMessages: só mensagens além do threshold mínimo são sumarizadas.
func (s *Service) TriggerSummarizationInBackground(
	conversationID uint,
	profile *profiles.Profile,
	allRootMessages []chat.Message,
) {
	inProgress, err := s.cfg.Repo.IsSummarizingInProgress(conversationID)
	if err != nil {
		log.Printf("[Summary] Erro ao verificar status: %v", err)
		return
	}
	if inProgress {
		log.Printf("[Summary] Sumarização já em andamento para conversa %d", conversationID)
		return
	}

	minKeep := profile.GetMinContextMessages()
	totalMessages := len(allRootMessages)

	if totalMessages <= minKeep {
		log.Printf("[Summary] Apenas %d mensagens, mínimo é %d — nada a sumarizar", totalMessages, minKeep)
		return
	}

	cutIndex := totalMessages - minKeep
	for cutIndex > 0 && allRootMessages[cutIndex].Role != "user" {
		cutIndex--
	}
	if cutIndex <= 0 {
		log.Printf("[Summary] Não encontrou ponto de corte válido (user message) — abortando")
		return
	}

	messagesToSummarize := allRootMessages[:cutIndex]
	lastSummarizedMsgID := messagesToSummarize[len(messagesToSummarize)-1].ID

	existingSummary, currentUpToID, err := s.cfg.Repo.GetConversationSummary(conversationID)
	if err != nil {
		log.Printf("[Summary] Erro ao buscar resumo existente: %v", err)
		return
	}

	var newMessages []chat.Message
	for _, m := range messagesToSummarize {
		if m.ID > currentUpToID {
			newMessages = append(newMessages, m)
		}
	}
	if len(newMessages) == 0 {
		log.Printf("[Summary] Nenhuma mensagem nova para resumir (já resumido até ID %d)", currentUpToID)
		return
	}

	if err := s.cfg.Repo.SetSummarizingInProgress(conversationID, true); err != nil {
		log.Printf("[Summary] Erro ao marcar summarizing_in_progress: %v", err)
		return
	}

	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("🔴 [PANIC RECOVERED] executeSummarization (conversa %d): %v", conversationID, r)
				_ = s.cfg.Repo.SetSummarizingInProgress(conversationID, false)
			}
		}()
		s.executeSummarization(conversationID, profile, existingSummary, newMessages, lastSummarizedMsgID)
	}()
}

// executeSummarization chama o LLM para gerar o resumo das mensagens fornecidas.
func (s *Service) executeSummarization(
	conversationID uint,
	profile *profiles.Profile,
	existingSummary string,
	newMessages []chat.Message,
	upToMessageID uint,
) {
	if s.cfg.ProfileResolver != nil {
		profile = s.cfg.ProfileResolver(profile)
	}

	s.cfg.Emitter.Emit("chat:summary_started", ports.SummaryStartedEvent{
		ConversationID: conversationID,
		MessageCount:   len(newMessages),
	})

	defer func() {
		if err := s.cfg.Repo.SetSummarizingInProgress(conversationID, false); err != nil {
			log.Printf("[Summary] Erro ao desmarcar summarizing_in_progress: %v", err)
		}
	}()

	model := profile.Chat.Model

	userPrompt := BuildSummarizationUserPrompt(existingSummary, newMessages)

	log.Printf("[Summary] Iniciando sumarização: conversa=%d, modelo=%s, %d mensagens novas, resumo anterior=%d chars",
		conversationID, model, len(newMessages), len(existingSummary))

	provider := s.cfg.LLMRegistry.Get(profile.Chat.LLMProvider)
	if provider == nil {
		log.Printf("[Summary] Provider não encontrado: %s", profile.Chat.LLMProvider)
		s.cfg.Emitter.Emit("chat:summary_error", ports.SummaryErrorEvent{
			ConversationID: conversationID,
			Error:          "Provider não encontrado",
		})
		return
	}

	cp := llm.NewChatProvider(provider, s.cfg.CredMgr)
	summary, err := cp.SimpleChat(context.Background(), model, SummaryPrompt, userPrompt)
	if err != nil {
		log.Printf("[Summary] Erro na chamada LLM: %v", err)
		s.cfg.Emitter.Emit("chat:summary_error", ports.SummaryErrorEvent{
			ConversationID: conversationID,
			Error:          fmt.Sprintf("Erro ao gerar resumo: %v", err),
		})
		return
	}

	summary = strings.TrimSpace(summary)
	if summary == "" {
		log.Printf("[Summary] LLM retornou resumo vazio — abortando")
		s.cfg.Emitter.Emit("chat:summary_error", ports.SummaryErrorEvent{
			ConversationID: conversationID,
			Error:          "Resumo gerado está vazio",
		})
		return
	}

	if err := s.cfg.Repo.UpdateConversationSummary(conversationID, summary, upToMessageID); err != nil {
		log.Printf("[Summary] Erro ao salvar resumo: %v", err)
		s.cfg.Emitter.Emit("chat:summary_error", ports.SummaryErrorEvent{
			ConversationID: conversationID,
			Error:          "Erro ao salvar resumo",
		})
		return
	}

	log.Printf("[Summary] Resumo salvo: conversa=%d, até msgID=%d, %d chars",
		conversationID, upToMessageID, len(summary))

	s.cfg.Emitter.Emit("chat:summary_completed", ports.SummaryCompletedEvent{
		ConversationID:       conversationID,
		SummaryUpToMessageID: upToMessageID,
		SummaryLength:        len(summary),
		MessageCount:         len(newMessages),
	})
}
