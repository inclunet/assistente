package database

import (
	"assistente/internal/logging"
	"context"
	"encoding/json"
	"gorm.io/gorm"
)

// TokenRepository encapsula estatisticas de tokens e janela de contexto com um *gorm.DB injetado.
type TokenRepository struct {
	db *gorm.DB
}

func modelCallCountSelect(db *gorm.DB) string {
	modelCallConditions := "chat_messages.total_tokens > 0 OR (chat_messages.tool_calls IS NOT NULL AND TRIM(chat_messages.tool_calls) NOT IN ('', '[]', 'null'))"
	if db != nil && db.Migrator().HasTable(&ToolInvocation{}) {
		modelCallConditions += ` OR EXISTS (
			SELECT 1
			FROM tool_invocations ti
			WHERE ti.user_id = conversations.user_id
				AND ti.origin_type = 'chat'
				AND ti.origin_id = chat_messages.turn_id
				AND TRIM(ti.tool_call_id) <> ''
				AND CASE
					WHEN json_valid(ti.metadata) THEN json_extract(ti.metadata, '$.display.assistant_message_id')
					ELSE NULL
				END = chat_messages.id
		)`
	}
	return "COALESCE(SUM(CASE WHEN chat_messages.role = 'assistant' AND (" + modelCallConditions + ") THEN 1 ELSE 0 END), 0) as model_call_count"
}

// NewTokenRepository cria um TokenRepository com o *gorm.DB injetado.
func NewTokenRepository(database *gorm.DB) *TokenRepository {
	return &TokenRepository{db: database}
}

// GetConversationTokenStatsWithContext retorna estatísticas de tokens de uma
// conversa pertencente ao usuário do contexto.
//
// SECURITY: fail-closed (AEP-0052 / B11). Sem userID = ErrUserScopeRequired.
func GetConversationTokenStatsWithContext(ctx context.Context, conversationID string) (map[string]int, error) {
	return NewTokenRepository(db).GetConversationTokenStatsWithContext(ctx, conversationID)
}

func (r *TokenRepository) GetConversationTokenStatsWithContext(ctx context.Context, conversationID string) (map[string]int, error) {
	db := r.db
	if _, err := RequireUserID(ctx); err != nil {
		return nil, err
	}
	var result struct {
		TotalPromptTokens     int
		TotalCompletionTokens int
		TotalTokens           int
		TotalCacheReadTokens  int
		TotalCacheWriteTokens int
		TotalCacheMissTokens  int
	}
	err := scopedMessageQuery(ctx, db.Model(&ChatMessage{})).
		Where("chat_messages.conversation_id = ?", conversationID).
		Select("SUM(chat_messages.prompt_tokens) as total_prompt_tokens, SUM(chat_messages.completion_tokens) as total_completion_tokens, SUM(chat_messages.total_tokens) as total_tokens, COALESCE(SUM(chat_messages.cache_read_tokens), 0) as total_cache_read_tokens, COALESCE(SUM(chat_messages.cache_write_tokens), 0) as total_cache_write_tokens, COALESCE(SUM(chat_messages.cache_miss_tokens), 0) as total_cache_miss_tokens").
		Scan(&result).Error
	if err != nil {
		return nil, err
	}
	return map[string]int{
		"prompt_tokens":      result.TotalPromptTokens,
		"completion_tokens":  result.TotalCompletionTokens,
		"total_tokens":       result.TotalTokens,
		"cache_read_tokens":  result.TotalCacheReadTokens,
		"cache_write_tokens": result.TotalCacheWriteTokens,
		"cache_miss_tokens":  result.TotalCacheMissTokens,
	}, nil
}

// GetAllTokenStatsWithContext retorna estatísticas de tokens de todas as
// conversas do usuário do contexto.
//
// SECURITY: fail-closed (AEP-0052 / B11). Sem userID retornaria estatísticas
// agregadas globalmente — vetor de inferência sobre uso da instância.
func GetAllTokenStatsWithContext(ctx context.Context) (map[string]int, error) {
	return NewTokenRepository(db).GetAllTokenStatsWithContext(ctx)
}

func (r *TokenRepository) GetAllTokenStatsWithContext(ctx context.Context) (map[string]int, error) {
	db := r.db
	if _, err := RequireUserID(ctx); err != nil {
		return nil, err
	}
	var result struct {
		TotalPromptTokens     int
		TotalCompletionTokens int
		TotalTokens           int
		TotalCacheReadTokens  int
		TotalCacheWriteTokens int
		TotalCacheMissTokens  int
	}
	err := scopedMessageQuery(ctx, db.Model(&ChatMessage{})).
		Select("SUM(chat_messages.prompt_tokens) as total_prompt_tokens, SUM(chat_messages.completion_tokens) as total_completion_tokens, SUM(chat_messages.total_tokens) as total_tokens, COALESCE(SUM(chat_messages.cache_read_tokens), 0) as total_cache_read_tokens, COALESCE(SUM(chat_messages.cache_write_tokens), 0) as total_cache_write_tokens, COALESCE(SUM(chat_messages.cache_miss_tokens), 0) as total_cache_miss_tokens").
		Scan(&result).Error
	if err != nil {
		return nil, err
	}
	return map[string]int{
		"prompt_tokens":      result.TotalPromptTokens,
		"completion_tokens":  result.TotalCompletionTokens,
		"total_tokens":       result.TotalTokens,
		"cache_read_tokens":  result.TotalCacheReadTokens,
		"cache_write_tokens": result.TotalCacheWriteTokens,
		"cache_miss_tokens":  result.TotalCacheMissTokens,
	}, nil
}

// TokenStats representa estatísticas detalhadas de tokens
type TokenStats struct {
	PromptTokens     int    `json:"prompt_tokens"`
	CompletionTokens int    `json:"completion_tokens"`
	TotalTokens      int    `json:"total_tokens"`
	CacheReadTokens  int    `json:"cache_read_tokens"`
	CacheWriteTokens int    `json:"cache_write_tokens"`
	CacheMissTokens  int    `json:"cache_miss_tokens"`
	MessageCount     int    `json:"message_count"`
	ModelCallCount   int    `json:"model_call_count"`
	Model            string `json:"model,omitempty"`
}

// ToolUsageBreakdown detalha o uso de um tool específico
type ToolUsageBreakdown struct {
	ToolName              string `json:"tool_name"`
	CallCount             int    `json:"call_count"`
	TotalPromptTokens     int    `json:"total_prompt_tokens"`
	TotalCompletionTokens int    `json:"total_completion_tokens"`
	TotalTokens           int    `json:"total_tokens"`
}

// DetailedTokenStats fornece breakdown detalhado de tokens por categoria
type DetailedTokenStats struct {
	// Dados básicos da conversa
	PromptTokens     int    `json:"prompt_tokens"`
	CompletionTokens int    `json:"completion_tokens"`
	TotalTokens      int    `json:"total_tokens"`
	CacheReadTokens  int    `json:"cache_read_tokens"`
	CacheWriteTokens int    `json:"cache_write_tokens"`
	CacheMissTokens  int    `json:"cache_miss_tokens"`
	MessageCount     int    `json:"message_count"`
	ModelCallCount   int    `json:"model_call_count"`
	Model            string `json:"model,omitempty"`

	// ContextTokens é a ocupação ATUAL da janela de contexto, derivada do
	// usage reportado pelo provedor no último turno do assistente
	// (prompt_tokens + completion_tokens da resposta mais recente). Diferente
	// de TotalTokens (que é o acumulado de todas as requisições e serve para
	// estimar custo/billing), este valor reflete quanto da janela está em uso
	// agora — sem somar repetidamente o histórico reenviado a cada turno.
	ContextTokens int `json:"context_tokens"`

	// Breakdown de contexto (sistema + resumo + mensagens)
	SystemPromptEstimatedTokens int `json:"system_prompt_estimated_tokens"`
	SummaryTokens               int `json:"summary_tokens"`
	MessagesInContextCount      int `json:"messages_in_context_count"`
	MessagesInContextTokens     int `json:"messages_in_context_tokens"`
	MessagesOutOfContextCount   int `json:"messages_out_of_context_count"`
	MessagesOutOfContextTokens  int `json:"messages_out_of_context_tokens"`

	// Tool calling
	ToolsUsedCount int                  `json:"tools_used_count"`
	ToolBreakdown  []ToolUsageBreakdown `json:"tool_breakdown"`
}

// GetTurnTokenStatsWithContext retorna estatísticas de tokens para um turno
// específico do usuário do contexto.
//
// SECURITY: fail-closed (AEP-0052 / B11). Sem userID = ErrUserScopeRequired.
func GetTurnTokenStatsWithContext(ctx context.Context, conversationID string, turnID string) (*TokenStats, error) {
	return NewTokenRepository(db).GetTurnTokenStatsWithContext(ctx, conversationID, turnID)
}

func (r *TokenRepository) GetTurnTokenStatsWithContext(ctx context.Context, conversationID string, turnID string) (*TokenStats, error) {
	db := r.db
	if _, err := RequireUserID(ctx); err != nil {
		return nil, err
	}
	var result struct {
		TotalPromptTokens     int
		TotalCompletionTokens int
		TotalTokens           int
		TotalCacheReadTokens  int
		TotalCacheWriteTokens int
		TotalCacheMissTokens  int
		MessageCount          int
		ModelCallCount        int
	}
	err := scopedMessageQuery(ctx, db.Model(&ChatMessage{})).
		Where("chat_messages.conversation_id = ? AND chat_messages.turn_id = ?", conversationID, turnID).
		Select("COALESCE(SUM(chat_messages.prompt_tokens), 0) as total_prompt_tokens, COALESCE(SUM(chat_messages.completion_tokens), 0) as total_completion_tokens, COALESCE(SUM(chat_messages.total_tokens), 0) as total_tokens, COALESCE(SUM(chat_messages.cache_read_tokens), 0) as total_cache_read_tokens, COALESCE(SUM(chat_messages.cache_write_tokens), 0) as total_cache_write_tokens, COALESCE(SUM(chat_messages.cache_miss_tokens), 0) as total_cache_miss_tokens, COUNT(*) as message_count, " + modelCallCountSelect(db)).
		Scan(&result).Error
	if err != nil {
		return nil, err
	}
	return &TokenStats{
		PromptTokens:     result.TotalPromptTokens,
		CompletionTokens: result.TotalCompletionTokens,
		TotalTokens:      result.TotalTokens,
		CacheReadTokens:  result.TotalCacheReadTokens,
		CacheWriteTokens: result.TotalCacheWriteTokens,
		CacheMissTokens:  result.TotalCacheMissTokens,
		MessageCount:     result.MessageCount,
		ModelCallCount:   result.ModelCallCount,
	}, nil
}

// GetConversationDetailedTokenStatsWithContext retorna estatísticas detalhadas
// de tokens de uma conversa pertencente ao usuário do contexto.
//
// SECURITY: fail-closed (AEP-0052). Sem userID no ctx retorna
// ErrUserScopeRequired.
func GetConversationDetailedTokenStatsWithContext(ctx context.Context, conversationID string) (*TokenStats, error) {
	return NewTokenRepository(db).GetConversationDetailedTokenStatsWithContext(ctx, conversationID)
}

func (r *TokenRepository) GetConversationDetailedTokenStatsWithContext(ctx context.Context, conversationID string) (*TokenStats, error) {
	db := r.db
	if _, err := RequireUserID(ctx); err != nil {
		return nil, err
	}
	var result struct {
		TotalPromptTokens     int
		TotalCompletionTokens int
		TotalTokens           int
		TotalCacheReadTokens  int
		TotalCacheWriteTokens int
		TotalCacheMissTokens  int
		MessageCount          int
		ModelCallCount        int
	}
	err := scopedMessageQuery(ctx, db.Model(&ChatMessage{})).
		Where("chat_messages.conversation_id = ?", conversationID).
		Select("COALESCE(SUM(chat_messages.prompt_tokens), 0) as total_prompt_tokens, COALESCE(SUM(chat_messages.completion_tokens), 0) as total_completion_tokens, COALESCE(SUM(chat_messages.total_tokens), 0) as total_tokens, COALESCE(SUM(chat_messages.cache_read_tokens), 0) as total_cache_read_tokens, COALESCE(SUM(chat_messages.cache_write_tokens), 0) as total_cache_write_tokens, COALESCE(SUM(chat_messages.cache_miss_tokens), 0) as total_cache_miss_tokens, COUNT(*) as message_count, " + modelCallCountSelect(db)).
		Scan(&result).Error
	if err != nil {
		return nil, err
	}

	var mostUsedModel string
	scopedMessageQuery(ctx, db.Model(&ChatMessage{})).
		Where("chat_messages.conversation_id = ? AND chat_messages.model != ''", conversationID).
		Select("chat_messages.model").
		Group("chat_messages.model").
		Order("COUNT(*) DESC").
		Limit(1).
		Scan(&mostUsedModel)

	return &TokenStats{
		PromptTokens:     result.TotalPromptTokens,
		CompletionTokens: result.TotalCompletionTokens,
		TotalTokens:      result.TotalTokens,
		CacheReadTokens:  result.TotalCacheReadTokens,
		CacheWriteTokens: result.TotalCacheWriteTokens,
		CacheMissTokens:  result.TotalCacheMissTokens,
		MessageCount:     result.MessageCount,
		ModelCallCount:   result.ModelCallCount,
		Model:            mostUsedModel,
	}, nil
}

// GetDetailedTokenStatsWithContext retorna agregação completa de tokens com
// breakdown por categoria, restrita ao usuário do contexto.
//
// SECURITY: fail-closed (AEP-0052). A guarda explícita evita que callers
// distraídos descubram o número de tokens de qualquer conversa por ID — a
// rota natural via DBStore já enforça userID, mas chamadas diretas a esta
// função (em util/test/scripts) precisam falhar em vez de vazar.
func GetDetailedTokenStatsWithContext(ctx context.Context, conversationID string, summaryUpToMessageID string) (*DetailedTokenStats, error) {
	return NewTokenRepository(db).GetDetailedTokenStatsWithContext(ctx, conversationID, summaryUpToMessageID)
}

func (r *TokenRepository) GetDetailedTokenStatsWithContext(ctx context.Context, conversationID string, summaryUpToMessageID string) (*DetailedTokenStats, error) {
	db := r.db
	if _, err := RequireUserID(ctx); err != nil {
		return nil, err
	}
	basicStats, err := r.GetConversationDetailedTokenStatsWithContext(ctx, conversationID)
	if err != nil {
		return nil, err
	}

	// 2. Recuperar resumo (se houver)
	summaryTokens := 0
	summary, _, err := NewSummarizationRepository(r.db).GetConversationSummaryWithContext(ctx, conversationID)
	if err == nil && summary != "" {
		// Estima tokens do resumo: ~1 token a cada 4 caracteres
		summaryTokens = (len(summary) + 3) / 4
	}

	// 3. Contar mensagens in-context vs out-of-context
	// Usa índice na lista ordenada por created_at (como HistoryLoader.Load)
	// em vez de comparação lexicográfica de IDs, evitando problemas com
	// UUIDs gerados no mesmo milissegundo.
	var messagesInContextCount, messagesOutOfContextCount int
	var messagesInContextTokens, messagesOutOfContextTokens int

	if summaryUpToMessageID != "" {
		var allMessages []ChatMessage
		if err := scopedMessageQuery(ctx, db.Model(&ChatMessage{})).
			Where("chat_messages.conversation_id = ? AND chat_messages.parent_id IS NULL", conversationID).
			Order("chat_messages.created_at ASC").
			Select("chat_messages.id, chat_messages.total_tokens").
			Find(&allMessages).Error; err == nil {

			cutIdx := -1
			for i, m := range allMessages {
				if m.ID == summaryUpToMessageID {
					cutIdx = i
					break
				}
			}

			if cutIdx >= 0 {
				// Out of context: mensagens até cutIdx (inclusive)
				for _, m := range allMessages[:cutIdx+1] {
					messagesOutOfContextCount++
					messagesOutOfContextTokens += m.TotalTokens
				}
				// In context: mensagens após cutIdx
				for _, m := range allMessages[cutIdx+1:] {
					messagesInContextCount++
					messagesInContextTokens += m.TotalTokens
				}
			} else {
				// summaryUpToMessageID não encontrado: tratar tudo como in-context
				messagesInContextCount = basicStats.MessageCount
				messagesInContextTokens = basicStats.TotalTokens
			}
		}
	} else {
		// Se não há sumarização, todas são in-context
		messagesInContextCount = basicStats.MessageCount
		messagesInContextTokens = basicStats.TotalTokens
	}

	// 4. Breakdown de tool usage
	toolBreakdown, toolsUsedCount, err := r.getToolUsageBreakdownWithContext(ctx, conversationID)
	if err != nil {
		return nil, err
	}

	// Estima tokens do system prompt: ~1 token a cada 4 caracteres
	// O DefaultSystemPrompt tem ~500 caracteres, então ~125 tokens
	systemPromptEstimatedTokens := 125

	// Ocupação atual da janela de contexto a partir do usage oficial do
	// provedor (último turno do assistente). Não soma o histórico reenviado.
	// Um erro aqui (DB/escopo) não é fatal — mantém contextTokens=0 —, mas é
	// logado para não mascarar inconsistências no payload (vs. demais campos).
	contextTokens, ctxErr := r.getLatestReportedContextTokens(ctx, conversationID)
	if ctxErr != nil {
		logging.Warnf(ctx, "database.token-repository", "[DB] aviso: falha ao obter contextTokens da conversa %s: %v", conversationID, ctxErr)
	}

	return &DetailedTokenStats{
		PromptTokens:                basicStats.PromptTokens,
		CompletionTokens:            basicStats.CompletionTokens,
		TotalTokens:                 basicStats.TotalTokens,
		CacheReadTokens:             basicStats.CacheReadTokens,
		CacheWriteTokens:            basicStats.CacheWriteTokens,
		CacheMissTokens:             basicStats.CacheMissTokens,
		MessageCount:                basicStats.MessageCount,
		ModelCallCount:              basicStats.ModelCallCount,
		Model:                       basicStats.Model,
		ContextTokens:               contextTokens,
		SystemPromptEstimatedTokens: systemPromptEstimatedTokens,
		SummaryTokens:               summaryTokens,
		MessagesInContextCount:      messagesInContextCount,
		MessagesInContextTokens:     messagesInContextTokens,
		MessagesOutOfContextCount:   messagesOutOfContextCount,
		MessagesOutOfContextTokens:  messagesOutOfContextTokens,
		ToolsUsedCount:              toolsUsedCount,
		ToolBreakdown:               toolBreakdown,
	}, nil
}

// getToolUsageBreakdown extrai informações de uso de tools das mensagens
func (r *TokenRepository) getToolUsageBreakdownWithContext(ctx context.Context, conversationID string) ([]ToolUsageBreakdown, int, error) {
	db := r.db
	var messages []ChatMessage
	// Propaga falha de DB em vez de degradar silenciosamente para um
	// breakdown vazio — um erro de query mascarado distorceria as estatísticas
	// detalhadas sem qualquer sinal ao caller.
	if err := scopedMessageQuery(ctx, db.Model(&ChatMessage{})).
		Where("chat_messages.conversation_id = ? AND chat_messages.tool_calls != '' AND chat_messages.tool_calls IS NOT NULL", conversationID).
		Select("chat_messages.tool_calls, chat_messages.prompt_tokens, chat_messages.completion_tokens").
		Find(&messages).Error; err != nil {
		return nil, 0, err
	}

	// Map para agregar tool usage
	toolMap := make(map[string]*ToolUsageBreakdown)

	for _, msg := range messages {
		if msg.ToolCalls == "" {
			continue
		}

		// Parse JSON das tool calls, tolerando tanto array (`[{...}]`) quanto
		// objeto único (`{...}`), espelhando o cleanup em message_repository.go.
		toolCalls := parseToolCallObjects(msg.ToolCalls)

		for _, toolCall := range toolCalls {
			if funcData, ok := toolCall["function"].(map[string]interface{}); ok {
				if toolName, ok := funcData["name"].(string); ok {
					if _, exists := toolMap[toolName]; !exists {
						toolMap[toolName] = &ToolUsageBreakdown{
							ToolName: toolName,
						}
					}
					toolMap[toolName].CallCount++
					// Distribuir tokens igualmente entre tools usados nessa mensagem
					toolCount := len(toolCalls)
					if toolCount > 0 {
						toolMap[toolName].TotalPromptTokens += msg.PromptTokens / toolCount
						toolMap[toolName].TotalCompletionTokens += msg.CompletionTokens / toolCount
					}
				}
			}
		}
	}

	// Converter map para slice
	var result []ToolUsageBreakdown
	for _, breakdown := range toolMap {
		breakdown.TotalTokens = breakdown.TotalPromptTokens + breakdown.TotalCompletionTokens
		result = append(result, *breakdown)
	}

	return result, len(toolMap), nil
}

// parseToolCallObjects decodifica o payload JSON de tool_calls aceitando tanto
// um array de objetos (`[{...}]`) quanto um objeto único (`{...}`). Retorna nil
// quando o payload é vazio ou inválido. Centraliza a tolerância de formato já
// adotada no cleanup de tool invocations (message_repository.go).
func parseToolCallObjects(raw string) []map[string]interface{} {
	if raw == "" {
		return nil
	}
	var anyPayload any
	if err := json.Unmarshal([]byte(raw), &anyPayload); err != nil {
		return nil
	}
	switch v := anyPayload.(type) {
	case []interface{}:
		result := make([]map[string]interface{}, 0, len(v))
		for _, item := range v {
			if obj, ok := item.(map[string]interface{}); ok {
				result = append(result, obj)
			}
		}
		return result
	case map[string]interface{}:
		return []map[string]interface{}{v}
	default:
		return nil
	}
}

// GetContextWindowUsageWithContext calcula a porcentagem de uso da janela de
// contexto para o usuário do contexto.
//
// SECURITY: fail-closed (AEP-0052 / B11). Embora delegue para
// GetConversationDetailedTokenStatsWithContext (que já tem gate), valida
// no topo para defesa em camadas — se o gate interno for relaxado por
// engano em refactor futuro, este nível continua fail-closed.
func GetContextWindowUsageWithContext(ctx context.Context, conversationID string, contextLimit int) (float64, int, error) {
	return NewTokenRepository(db).GetContextWindowUsageWithContext(ctx, conversationID, contextLimit)
}

func (r *TokenRepository) GetContextWindowUsageWithContext(ctx context.Context, conversationID string, contextLimit int) (float64, int, error) {
	if _, err := RequireUserID(ctx); err != nil {
		return 0, 0, err
	}
	// A ocupação da janela de contexto deve refletir o tamanho ATUAL do
	// contexto — não a soma acumulada de todos os turnos. Somar
	// prompt_tokens de cada mensagem conta o histórico reenviado a cada
	// requisição repetidamente, inflando o percentual muito além de 100% e
	// disparando alertas críticos falsos (issue #197). Usamos o usage oficial
	// do provedor no último turno do assistente como base.
	contextTokens, err := r.getLatestReportedContextTokens(ctx, conversationID)
	if err != nil {
		return 0, 0, err
	}
	if contextLimit <= 0 {
		return 0, contextTokens, nil
	}
	percentage := (float64(contextTokens) / float64(contextLimit)) * 100
	return percentage, contextTokens, nil
}

// getLatestReportedContextTokens retorna a ocupação atual da janela de contexto
// derivada do usage reportado pelo provedor no turno mais recente do assistente
// (prompt_tokens + completion_tokens da última mensagem do assistente com
// total_tokens > 0). Esse valor representa o que o provedor efetivamente contou
// como prompt da última requisição mais a resposta — i.e. quanto da janela está
// ocupado agora —, ao contrário da soma acumulada usada para custo/billing.
//
// Retorna 0 quando ainda não há usage reportado (ex.: conversa nova ou provedor
// que não devolve usage). Respeita o escopo de usuário do contexto (AEP-0052).
func getLatestReportedContextTokens(ctx context.Context, conversationID string) (int, error) {
	return NewTokenRepository(db).getLatestReportedContextTokens(ctx, conversationID)
}

func (r *TokenRepository) getLatestReportedContextTokens(ctx context.Context, conversationID string) (int, error) {
	db := r.db
	if _, err := RequireUserID(ctx); err != nil {
		return 0, err
	}
	var latest struct {
		PromptTokens     int
		CompletionTokens int
	}
	err := scopedMessageQuery(ctx, db.Model(&ChatMessage{})).
		Where("chat_messages.conversation_id = ? AND chat_messages.role = ? AND chat_messages.total_tokens > 0", conversationID, "assistant").
		Order("chat_messages.created_at DESC, chat_messages.id DESC").
		Limit(1).
		Select("chat_messages.prompt_tokens, chat_messages.completion_tokens").
		Scan(&latest).Error
	if err != nil {
		return 0, err
	}
	return latest.PromptTokens + latest.CompletionTokens, nil
}

// GetRecentMessagesTokenCountWithContext retorna o total de tokens das N
// mensagens mais recentes do usuário do contexto.
//
// SECURITY: fail-closed (AEP-0052 / B11). Sem userID = ErrUserScopeRequired.
func GetRecentMessagesTokenCountWithContext(ctx context.Context, conversationID string, messageLimit int) (int, error) {
	return NewTokenRepository(db).GetRecentMessagesTokenCountWithContext(ctx, conversationID, messageLimit)
}

func (r *TokenRepository) GetRecentMessagesTokenCountWithContext(ctx context.Context, conversationID string, messageLimit int) (int, error) {
	db := r.db
	if _, err := RequireUserID(ctx); err != nil {
		return 0, err
	}
	var totalTokens int
	err := scopedMessageQuery(ctx, db.Model(&ChatMessage{})).
		Where("chat_messages.conversation_id = ?", conversationID).
		Order("chat_messages.created_at DESC").
		Limit(messageLimit).
		Select("SUM(chat_messages.total_tokens)").
		Scan(&totalTokens).Error
	return totalTokens, err
}
