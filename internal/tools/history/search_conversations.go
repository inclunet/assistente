package history

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"assistente/internal/database"
	"assistente/internal/tools"
)

type searchConversationsArgs struct {
	Query string `json:"query"`
	Limit int    `json:"limit,omitempty"`
}

// SearchRepo abstrai a busca full-text no histórico de mensagens.
type SearchRepo interface {
	SearchMessages(query string, limit int) ([]database.MessageSearchResult, error)
}

// SearchConversationsTool busca no histórico de mensagens de todas as conversas.
// Usa FTS5 (full-text search) com ranking BM25 para encontrar discussões anteriores.
type SearchConversationsTool struct {
	repo SearchRepo
}

func NewSearchConversations(repo SearchRepo) *SearchConversationsTool {
	return &SearchConversationsTool{repo: repo}
}

func (t *SearchConversationsTool) Name() string {
	return "search_conversations"
}

func (t *SearchConversationsTool) Description() string {
	return "Searches the full message history across ALL conversations using full-text search. Use this to find if a topic was already discussed, what was concluded, or to recall past context. Supports words, \"exact phrases\", prefix* matching, and OR/AND/NOT operators. Returns results ranked by relevance (BM25)."
}

func (t *SearchConversationsTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"query": {
				"type": "string",
				"description": "Termo de busca. Exemplos: 'autenticação JWT', '\"rolling context\"', 'signal OR telegram', 'implement*'"
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

	var results []database.MessageSearchResult
	var err error
	if t.repo != nil {
		results, err = t.repo.SearchMessages(query, limit)
	} else {
		results, err = database.SearchMessageContent(query, limit)
	}
	if err != nil {
		return tools.ToolResult{Content: fmt.Sprintf("Erro na busca: %v", err), IsError: true}, nil
	}

	if len(results) == 0 {
		return tools.ToolResult{
			Content: fmt.Sprintf("Nenhum resultado encontrado para: %s", query),
			Metadata: map[string]any{
				"query":   query,
				"results": 0,
			},
		}, nil
	}

	// Agrupa resultados por conversa
	type convGroup struct {
		Title    string
		Messages []database.MessageSearchResult
	}
	grouped := make(map[uint]*convGroup)
	var order []uint
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
	sb.WriteString(fmt.Sprintf("Encontrados %d resultados em %d conversas para: %s\n\n", len(results), len(grouped), query))

	for _, convID := range order {
		g := grouped[convID]
		sb.WriteString(fmt.Sprintf("── Conversa #%d: %s ──\n", convID, g.Title))
		for _, msg := range g.Messages {
			sb.WriteString(fmt.Sprintf("  [%s] %s (msg #%d, %s)\n",
				msg.Role,
				msg.Snippet,
				msg.MessageID,
				msg.CreatedAt.Format("2006-01-02 15:04"),
			))
		}
		sb.WriteString("\n")
	}

	return tools.ToolResult{
		Content: sb.String(),
		Metadata: map[string]any{
			"query":         query,
			"results":       len(results),
			"conversations": len(grouped),
		},
	}, nil
}
