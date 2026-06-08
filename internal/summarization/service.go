package summarization

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"unicode/utf8"

	"assistente/internal/chat"
	"assistente/internal/core/ports"
	"assistente/internal/credentials"
	"assistente/internal/database"
	"assistente/internal/events"
	"assistente/internal/llm"
	"assistente/internal/profiles"
	"assistente/internal/toolinvocations"
)

// SummarizationRepository abstrai as operações de persistência necessárias para sumarização.
// Implementado por DBSummarizationStore; pode ser mockado em testes.
type SummarizationRepository interface {
	GetMessages(ctx context.Context, conversationID string) ([]chat.Message, error)
	GetConversationSummary(ctx context.Context, conversationID string) (summary string, upToMessageID string, err error)
	IsSummarizingInProgress(ctx context.Context, conversationID string) (bool, error)
	SetSummarizingInProgress(ctx context.Context, conversationID string, inProgress bool) error
	UpdateConversationSummary(ctx context.Context, conversationID string, summary string, upToMessageID string) error
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
// invocationResults: resultados hidratados de tool_invocations (best-effort).
// fallbackResults: resultados persistidos como mensagens role=tool (best-effort); quando presente e não-vazio, é autoritativo.
func BuildSummarizationUserPrompt(existingSummary string, messages []chat.Message, invocationResults map[string]map[string]string, fallbackResults map[string]map[string]string) string {
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
		_, _ = fmt.Fprintf(&sb, "**[%s]**: ", m.Role)
		content := m.Content
		if len(content) > 2000 {
			content = truncateUTF8Safe(content, 2000) + "... [truncated]"
		}
		sb.WriteString(content)
		if m.Role == "assistant" && strings.TrimSpace(m.ToolCalls) != "" {
			for _, c := range parseSummarizationToolCalls(m.ToolCalls) {
				turnID := ""
				if m.TurnID != nil {
					turnID = strings.TrimSpace(*m.TurnID)
				}
				callID := strings.TrimSpace(c.ID)

				// Se existe fallback role=tool não-vazio para este turn/call, ele já estará
				// presente na lista de mensagens e não deve ser duplicado nem sobrescrito.
				if turnID != "" && callID != "" {
					if byCall := fallbackResults[turnID]; byCall != nil {
						if strings.TrimSpace(byCall[callID]) != "" {
							continue
						}
					}
				}

				res := strings.TrimSpace(c.Result)
				if res == "" && turnID != "" && callID != "" {
					if byCall := invocationResults[turnID]; byCall != nil {
						res = strings.TrimSpace(byCall[callID])
					}
				}
				if res == "" {
					continue
				}
				name := strings.TrimSpace(c.Function.Name)
				if name == "" {
					name = c.ID
				}
				if len(res) > 2000 {
					res = truncateUTF8Safe(res, 2000) + "... [truncated]"
				}
				sb.WriteString("\n\n")
				sb.WriteString("Tool result (")
				sb.WriteString(name)
				sb.WriteString("): ")
				sb.WriteString(res)
			}
		}
		sb.WriteString("\n\n")
	}

	if existingSummary != "" {
		sb.WriteString("---\n\nPlease produce an updated summary that integrates the previous summary with the new messages above.")
	} else {
		sb.WriteString("---\n\nPlease produce a concise summary of the conversation above.")
	}

	return sb.String()
}

type summarizationToolCall struct {
	ID       string `json:"id"`
	Result   string `json:"result,omitempty"`
	Function struct {
		Name string `json:"name"`
	} `json:"function"`
}

func parseSummarizationToolCalls(raw string) []summarizationToolCall {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	var calls []summarizationToolCall
	if err := json.Unmarshal([]byte(raw), &calls); err == nil {
		return calls
	}
	var single summarizationToolCall
	if err := json.Unmarshal([]byte(raw), &single); err == nil {
		if strings.TrimSpace(single.ID) == "" {
			return nil
		}
		return []summarizationToolCall{single}
	}
	return nil
}

func truncateUTF8Safe(s string, maxBytes int) string {
	if len(s) <= maxBytes {
		return s
	}
	for maxBytes > 0 && maxBytes < len(s) && !utf8.RuneStart(s[maxBytes]) {
		maxBytes--
	}
	if maxBytes <= 0 {
		return ""
	}
	return s[:maxBytes]
}

// ServiceConfig agrupa as dependências do Service.
type ServiceConfig struct {
	Repo            SummarizationRepository
	Emitter         events.Emitter
	LLMRegistry     *llm.ProviderRegistry
	CredMgr         *credentials.Manager
	ProfileManager  *profiles.Manager
	ProfileResolver func(context.Context, *profiles.Profile) *profiles.Profile
	// RateLimiter aplica o mesmo rate limiting por usuário das chamadas de chat
	// (Issue #27 / AEP-0065) também à chamada LLM de sumarização, que é um vetor
	// de custo. Opcional: nil = sem limite.
	RateLimiter *llm.RateLimiter
	// RateLimitKeyFunc extrai a chave de limite (userID) do contexto. Opcional.
	RateLimitKeyFunc func(context.Context) string
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
//
// profileSlug é o slug do perfil DA CONVERSA (o mesmo resolvido no envio de
// mensagem, via tab/workspace — `params.ProfileSlug`). O resumo deve usar o
// provider/modelo desse perfil, não o do perfil ativo global (Issue #203). Só
// recai sobre o perfil ativo global quando o slug está vazio ou não pode ser
// resolvido — mesmo padrão de fallback de `chat.Interactor.PrepareContext`.
func (s *Service) CheckAndTriggerSummarization(ctx context.Context, conversationID string, profileSlug string) {
	if conversationID == "" {
		return
	}

	profile := s.resolveConversationProfile(profileSlug)
	if profile == nil {
		return
	}
	if profile.Chat.ContextWindow <= 0 {
		return
	}

	allRootMessages, err := s.cfg.Repo.GetMessages(ctx, conversationID)
	if err != nil {
		log.Printf("[Summary] Erro ao carregar mensagens para check: %v", err)
		return
	}

	existingSummary, summaryUpToID, _ := s.cfg.Repo.GetConversationSummary(ctx, conversationID)

	// Use index-based slicing instead of lexicographic ID comparison.
	// UUIDv7 ordering within the same millisecond is not guaranteed.
	var contextMessages []chat.Message
	if summaryUpToID == "" {
		contextMessages = allRootMessages
	} else {
		startIdx := -1
		for i, m := range allRootMessages {
			if m.ID == summaryUpToID {
				startIdx = i + 1
				break
			}
		}
		if startIdx >= 0 && startIdx < len(allRootMessages) {
			contextMessages = allRootMessages[startIdx:]
		} else {
			// summaryUpToID not found (deleted?) — treat all as context
			contextMessages = allRootMessages
		}
	}

	fallbackResults := collectSummarizationFallbackToolResults(contextMessages)
	invocationResults := loadSummarizationToolInvocationResults(ctx, contextMessages)
	if shouldTriggerSummarizationWithHydratedToolResults(profile, contextMessages, existingSummary, invocationResults, fallbackResults) {
		s.TriggerSummarizationInBackground(ctx, conversationID, profile, allRootMessages)
	}
}

// resolveConversationProfile resolve o perfil da conversa a partir do slug
// propagado pelo pipeline de envio. Replica o fallback de
// `chat.Interactor.PrepareContext`: usa `ProfileManager.Get(slug)` quando o slug
// está presente e só recai sobre `GetActive()` (perfil ativo global) quando o
// slug está vazio ou a leitura falha. Isso garante que o resumo use o mesmo
// provider/modelo do perfil em que a conversa efetivamente roda (Issue #203).
func (s *Service) resolveConversationProfile(profileSlug string) *profiles.Profile {
	if s.cfg.ProfileManager == nil {
		return nil
	}

	slug := strings.TrimSpace(profileSlug)
	if slug != "" {
		profile, err := s.cfg.ProfileManager.Get(slug)
		if err == nil && profile != nil {
			return profile
		}
		log.Printf("[Summary] Não foi possível obter perfil da conversa %q (%v) — usando perfil ativo global", slug, err)
	}

	profile, err := s.cfg.ProfileManager.GetActive()
	if err != nil || profile == nil {
		return nil
	}
	return profile
}

// TriggerSummarizationInBackground lança uma goroutine para sumarizar mensagens antigas.
// Respeita MinContextMessages: só mensagens além do threshold mínimo são sumarizadas.
func (s *Service) TriggerSummarizationInBackground(
	ctx context.Context,
	conversationID string,
	profile *profiles.Profile,
	allRootMessages []chat.Message,
) {
	inProgress, err := s.cfg.Repo.IsSummarizingInProgress(ctx, conversationID)
	if err != nil {
		log.Printf("[Summary] Erro ao verificar status: %v", err)
		return
	}
	if inProgress {
		log.Printf("[Summary] Sumarização já em andamento para conversa %s", conversationID)
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

	existingSummary, currentUpToID, err := s.cfg.Repo.GetConversationSummary(ctx, conversationID)
	if err != nil {
		log.Printf("[Summary] Erro ao buscar resumo existente: %v", err)
		return
	}

	var newMessages []chat.Message
	if currentUpToID == "" {
		newMessages = messagesToSummarize
	} else {
		startIdx := -1
		for i, m := range messagesToSummarize {
			if m.ID == currentUpToID {
				startIdx = i + 1
				break
			}
		}
		if startIdx >= 0 && startIdx < len(messagesToSummarize) {
			newMessages = messagesToSummarize[startIdx:]
		}
		// If currentUpToID not found, newMessages stays nil → treated as "nothing new"
	}
	if len(newMessages) == 0 {
		log.Printf("[Summary] Nenhuma mensagem nova para resumir (já resumido até ID %s)", currentUpToID)
		return
	}

	if err := s.cfg.Repo.SetSummarizingInProgress(ctx, conversationID, true); err != nil {
		log.Printf("[Summary] Erro ao marcar summarizing_in_progress: %v", err)
		return
	}

	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("🔴 [PANIC RECOVERED] executeSummarization (conversa %s): %v", conversationID, r)
				_ = s.cfg.Repo.SetSummarizingInProgress(ctx, conversationID, false)
			}
		}()
		s.executeSummarization(ctx, conversationID, profile, existingSummary, newMessages, lastSummarizedMsgID)
	}()
}

// executeSummarization chama o LLM para gerar o resumo das mensagens fornecidas.
func (s *Service) executeSummarization(
	ctx context.Context,
	conversationID string,
	profile *profiles.Profile,
	existingSummary string,
	newMessages []chat.Message,
	upToMessageID string,
) {
	if s.cfg.ProfileResolver != nil {
		profile = s.cfg.ProfileResolver(ctx, profile)
	}

	s.cfg.Emitter.Emit("chat:summary_started", ports.SummaryStartedEvent{
		ConversationID: conversationID,
		MessageCount:   len(newMessages),
	})

	defer func() {
		if err := s.cfg.Repo.SetSummarizingInProgress(ctx, conversationID, false); err != nil {
			log.Printf("[Summary] Erro ao desmarcar summarizing_in_progress: %v", err)
		}
	}()

	model := profile.Chat.Model

	fallbackResults := collectSummarizationFallbackToolResults(newMessages)
	invocationResults := loadSummarizationToolInvocationResults(ctx, newMessages)
	userPrompt := BuildSummarizationUserPrompt(existingSummary, newMessages, invocationResults, fallbackResults)

	log.Printf("[Summary] Iniciando sumarização: conversa=%s, modelo=%s, %d mensagens novas, resumo anterior=%d chars",
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

	// Aplica o mesmo rate limiting por usuário das chamadas de chat — a
	// sumarização também consome cota/custo do provedor (Issue #27 / AEP-0065).
	// Quando RateLimiter é nil, NewRateLimitedProvider devolve o provider inalterado.
	cp := llm.NewRateLimitedProvider(
		llm.NewChatProvider(provider, s.cfg.CredMgr),
		s.cfg.RateLimiter,
		s.cfg.RateLimitKeyFunc,
	)
	summary, err := cp.SimpleChat(ctx, model, SummaryPrompt, userPrompt)
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

	if err := s.cfg.Repo.UpdateConversationSummary(ctx, conversationID, summary, upToMessageID); err != nil {
		log.Printf("[Summary] Erro ao salvar resumo: %v", err)
		s.cfg.Emitter.Emit("chat:summary_error", ports.SummaryErrorEvent{
			ConversationID: conversationID,
			Error:          "Erro ao salvar resumo",
		})
		return
	}

	log.Printf("[Summary] Resumo salvo: conversa=%s, até msgID=%s, %d chars",
		conversationID, upToMessageID, len(summary))

	s.cfg.Emitter.Emit("chat:summary_completed", ports.SummaryCompletedEvent{
		ConversationID:       conversationID,
		SummaryUpToMessageID: upToMessageID,
		SummaryLength:        len(summary),
		MessageCount:         len(newMessages),
	})
}

func shouldTriggerSummarizationWithHydratedToolResults(
	profile *profiles.Profile,
	contextMessages []chat.Message,
	existingSummary string,
	invocationResults map[string]map[string]string,
	fallbackResults map[string]map[string]string,
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
	// Soma somente resultados que serão adicionados como "Tool result (...)".
	estimated += estimateHydratedToolResultTokens(contextMessages, invocationResults, fallbackResults)

	if estimated > budget {
		log.Printf("[Summary] Trigger: estimated %d tokens > budget %d (window=%d, maxTokens=%d, margin=%d)",
			estimated, budget, contextWindow, maxTokens, safetyMargin)
		return true
	}
	return false
}

func collectSummarizationFallbackToolResults(messages []chat.Message) map[string]map[string]string {
	results := map[string]map[string]string{}
	for i := range messages {
		msg := &messages[i]
		if msg.Role != "tool" {
			continue
		}
		if msg.TurnID == nil {
			continue
		}
		turnID := strings.TrimSpace(*msg.TurnID)
		if turnID == "" {
			continue
		}
		callID := strings.TrimSpace(msg.ToolCallID)
		if callID == "" {
			continue
		}
		content := strings.TrimSpace(msg.Content)
		if content == "" {
			continue
		}
		byCall := results[turnID]
		if byCall == nil {
			byCall = map[string]string{}
			results[turnID] = byCall
		}
		byCall[callID] = msg.Content
	}
	return results
}

func estimateHydratedToolResultTokens(messages []chat.Message, invocationResults map[string]map[string]string, fallbackResults map[string]map[string]string) int {
	total := 0
	for _, m := range messages {
		if m.Role != "assistant" {
			continue
		}
		if strings.TrimSpace(m.ToolCalls) == "" {
			continue
		}
		if m.TurnID == nil {
			continue
		}
		turnID := strings.TrimSpace(*m.TurnID)
		if turnID == "" {
			continue
		}
		for _, c := range parseSummarizationToolCalls(m.ToolCalls) {
			callID := strings.TrimSpace(c.ID)
			if callID == "" {
				continue
			}
			// Se já há result embutido no tool_calls, já foi contado por EstimateMessagesTokens.
			if strings.TrimSpace(c.Result) != "" {
				continue
			}
			// Se há fallback role=tool não-vazio, o conteúdo já foi contado por EstimateMessagesTokens.
			if byCall := fallbackResults[turnID]; byCall != nil {
				if strings.TrimSpace(byCall[callID]) != "" {
					continue
				}
			}
			res := ""
			if byCall := invocationResults[turnID]; byCall != nil {
				res = strings.TrimSpace(byCall[callID])
			}
			if res == "" {
				continue
			}
			if len(res) > 2000 {
				res = truncateUTF8Safe(res, 2000)
			}
			total += EstimateTokens(res)
		}
	}
	return total
}

func loadSummarizationToolInvocationResults(ctx context.Context, messages []chat.Message) map[string]map[string]string {
	if len(messages) == 0 {
		return map[string]map[string]string{}
	}
	userID, err := database.RequireUserID(ctx)
	if err != nil {
		return map[string]map[string]string{}
	}

	seen := map[string]struct{}{}
	turnIDs := make([]string, 0)
	for _, msg := range messages {
		if msg.Role != "assistant" {
			continue
		}
		if strings.TrimSpace(msg.ToolCalls) == "" {
			continue
		}
		if msg.TurnID == nil {
			continue
		}
		turnID := strings.TrimSpace(*msg.TurnID)
		if turnID == "" {
			continue
		}
		if _, ok := seen[turnID]; ok {
			continue
		}
		seen[turnID] = struct{}{}
		turnIDs = append(turnIDs, turnID)
	}
	if len(turnIDs) == 0 {
		return map[string]map[string]string{}
	}

	results, err := toolinvocations.LoadChatToolInvocationResultsForTurnIDsWithUser(ctx, userID, turnIDs)
	if err != nil {
		log.Printf("[Summary] Erro ao hidratar tool invocations para sumarização: %v", err)
		return map[string]map[string]string{}
	}
	if len(results) == 0 {
		return map[string]map[string]string{}
	}
	return results
}
