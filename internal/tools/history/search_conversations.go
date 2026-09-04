package history

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"assistente/internal/database"
	"assistente/internal/tools"
	"assistente/internal/tools/invocationctx"
)

type searchConversationsArgs struct {
	Query          string `json:"query"`
	ConversationID string `json:"conversation_id,omitempty"`
	Limit          int    `json:"limit,omitempty"`
}

// SearchRepo abstrai a busca full-text no histórico de mensagens.
type SearchRepo interface {
	SearchMessages(ctx context.Context, query string, limit int) ([]database.MessageSearchResult, error)
}

// ScopedSearchRepo estende SearchRepo com busca restrita a uma conversa.
// A interface separada preserva compatibilidade com implementações existentes
// que oferecem somente a busca global.
type ScopedSearchRepo interface {
	SearchMessagesInConversation(ctx context.Context, query, conversationID string, limit int) ([]database.MessageSearchResult, error)
}

// SearchConversationsTool busca no histórico de mensagens, globalmente ou em
// uma conversa específica.
// Usa FTS5 (full-text search) com ranking BM25 para encontrar discussões anteriores.
type SearchConversationsTool struct {
	repo SearchRepo
	// allowDirectFallback permite que o tool caia direto em
	// database.SearchMessageContentWithContext quando repo é nil. Só é
	// habilitado por NewSearchConversationsForTest — em produção o tool
	// SEMPRE precisa de um SearchRepo concreto (chat.DBStore) com gate
	// fail-closed por user. Ver B13 do review do AEP-0052.
	allowDirectFallback bool
}

// NewSearchConversations cria o tool exposto ao LLM. repo é obrigatório:
// passar nil aqui é bug de wiring que faria o agente cair no caminho legado
// fail-open antes do AEP-0052. Para testes que precisam do caminho direto,
// use NewSearchConversationsForTest.
//
// SECURITY: panic em vez de devolver tool quebrado garante que o agente
// nunca seja registrado com um SearchRepo nil em produção (B13 do review).
func NewSearchConversations(repo SearchRepo) *SearchConversationsTool {
	if repo == nil {
		panic("history.NewSearchConversations: repo é obrigatório em produção; use NewSearchConversationsForTest para fallback explícito")
	}
	return &SearchConversationsTool{repo: repo}
}

// NewSearchConversationsForTest cria o tool permitindo repo nil — nesse caso
// Execute cai direto em database.SearchMessageContentWithContext. Mantido só
// para testes que ainda exercitam o caminho de fallback; produção DEVE usar
// NewSearchConversations com um SearchRepo concreto.
func NewSearchConversationsForTest(repo SearchRepo) *SearchConversationsTool {
	return &SearchConversationsTool{repo: repo, allowDirectFallback: true}
}

func (t *SearchConversationsTool) Name() string {
	return "search_conversations"
}

// CatalogMetadata declara os metadados de catálogo da tool (AEP-0077, Fase 1).
func (t *SearchConversationsTool) CatalogMetadata() tools.CatalogMetadata {
	return tools.CatalogMetadata{Category: "history", Class: "read_context", Package: "history", Risk: "read"}
}

func (t *SearchConversationsTool) Description() string {
	return "Searches message history using full-text search. By default it searches ALL conversations. Pass a conversation_id to restrict results, or conversation_id=\"current\" to safely use the current conversation from the invocation context. Supports words, \"exact phrases\", prefix* matching, and OR/AND/NOT operators. Returns results ranked by relevance (BM25)."
}

func (t *SearchConversationsTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"query": {
				"type": "string",
				"description": "Termo de busca. Exemplos: 'autenticação JWT', '\"rolling context\"', 'signal OR telegram', 'implement*'"
			},
			"conversation_id": {
				"type": "string",
				"description": "Opcional. ID da conversa para restringir a busca. Use \"current\" para a conversa corrente. Omitir mantém a busca global em todas as conversas do usuário."
			},
			"limit": {
				"type": "integer",
				"description": "Número máximo de resultados (padrão: 20, máximo: 100)",
				"default": 20
			}
		},
		"required": ["query"],
		"additionalProperties": false
	}`)
}

func (t *SearchConversationsTool) Execute(ctx context.Context, args json.RawMessage) (tools.ToolResult, error) {
	var params searchConversationsArgs
	if err := json.Unmarshal(args, &params); err != nil {
		return tools.ToolResult{Content: "Erro ao parsear argumentos: " + err.Error(), IsError: true}, nil
	}

	query := strings.TrimSpace(params.Query)
	if query == "" {
		return tools.ToolResult{Content: "O parâmetro 'query' não pode ser vazio", IsError: true}, nil
	}

	limit := params.Limit
	if limit <= 0 {
		limit = 20
	}

	conversationID := strings.TrimSpace(params.ConversationID)
	if strings.EqualFold(conversationID, "current") {
		inv, ok := invocationctx.Get(ctx)
		if !ok || strings.TrimSpace(inv.ConversationID) == "" {
			return tools.ToolResult{
				Content: "Busca rejeitada: conversation_id=\"current\" requer uma conversa corrente no contexto de invocação",
				IsError: true,
			}, nil
		}
		conversationID = strings.TrimSpace(inv.ConversationID)
	}

	var results []database.MessageSearchResult
	var err error
	if t.repo != nil {
		if conversationID == "" {
			results, err = t.repo.SearchMessages(ctx, query, limit)
		} else if scopedRepo, ok := t.repo.(ScopedSearchRepo); ok {
			results, err = scopedRepo.SearchMessagesInConversation(ctx, query, conversationID, limit)
		} else {
			return tools.ToolResult{
				Content: "Erro de configuração: SearchRepo não suporta busca por conversation_id",
				IsError: true,
			}, nil
		}
	} else if t.allowDirectFallback {
		// SECURITY: SearchMessageContentWithContext já é fail-closed por
		// userID, mas mantemos a validação aqui para que o erro seja
		// emitido pelo tool (e não pela camada de banco) — o agente que
		// invocou o tool com ctx sem user precisa saber claramente.
		if _, gErr := database.RequireUserID(ctx); gErr != nil {
			return tools.ToolResult{Content: fmt.Sprintf("Busca rejeitada: %v", gErr), IsError: true}, nil
		}
		results, err = database.SearchMessageContentInConversationWithContext(ctx, query, conversationID, limit)
	} else {
		return tools.ToolResult{Content: "Erro de configuração: SearchConversationsTool sem repo", IsError: true}, nil
	}
	if err != nil {
		return tools.ToolResult{Content: fmt.Sprintf("Erro na busca: %v", err), IsError: true}, nil
	}

	if len(results) == 0 {
		metadata := map[string]any{
			"query":   query,
			"results": 0,
		}
		if conversationID != "" {
			metadata["conversation_id"] = conversationID
		}
		return tools.ToolResult{
			Content:  fmt.Sprintf("Nenhum resultado encontrado para: %s", query),
			Metadata: metadata,
		}, nil
	}

	// Agrupa resultados por conversa
	type convGroup struct {
		Title    string
		Messages []database.MessageSearchResult
	}
	grouped := make(map[string]*convGroup)
	var order []string
	for _, r := range results {
		g, ok := grouped[r.ConversationID]
		if !ok {
			g = &convGroup{Title: r.ConversationTitle}
			grouped[r.ConversationID] = g
			order = append(order, r.ConversationID)
		}
		g.Messages = append(g.Messages, r)
	}

	var sb strings.Builder
	_, _ = fmt.Fprintf(&sb, "Encontrados %d resultados em %d conversas para: %s\n\n", len(results), len(grouped), query)

	for _, convID := range order {
		g := grouped[convID]
		_, _ = fmt.Fprintf(&sb, "── Conversa #%s: %s ──\n", convID, g.Title)
		for _, msg := range g.Messages {
			_, _ = fmt.Fprintf(&sb, "  [%s] %s (msg #%s, %s)\n",
				msg.Role,
				msg.Snippet,
				msg.MessageID,
				msg.CreatedAt.Format("2006-01-02 15:04"),
			)
		}
		sb.WriteString("\n")
	}

	metadata := map[string]any{
		"query":         query,
		"results":       len(results),
		"conversations": len(grouped),
	}
	if conversationID != "" {
		metadata["conversation_id"] = conversationID
	}
	return tools.ToolResult{
		Content:  sb.String(),
		Metadata: metadata,
	}, nil
}
