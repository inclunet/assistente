package llm

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// SummaryService gera resumos de conversas usando LLM
type SummaryService struct {
	client *SyncClient
	model  string
}

// SummaryConfig contém configurações para o serviço de resumo
type SummaryConfig struct {
	APIKey  string
	BaseURL string
	Model   string // Modelo a usar (ex: gpt-4o-mini, gpt-3.5-turbo)
}

// NewSummaryService cria um novo serviço de resumo
func NewSummaryService(cfg SummaryConfig) *SummaryService {
	client := NewSyncClient(cfg.BaseURL, cfg.APIKey)

	model := cfg.Model
	if model == "" {
		model = "gpt-4o-mini" // Modelo padrão (rápido e barato)
	}

	return &SummaryService{
		client: client,
		model:  model,
	}
}

// ChatMessage representa uma mensagem para resumir
// Compatível com database.ChatMessage
type ChatMessage struct {
	Role    string
	Content string
}

// GenerateSummary gera um resumo da conversa usando LLM
func (s *SummaryService) GenerateSummary(messages []ChatMessage) (string, error) {
	if len(messages) == 0 {
		return "", fmt.Errorf("nenhuma mensagem para resumir")
	}

	// Monta o contexto da conversa
	var conversationText strings.Builder
	var title string

	for _, msg := range messages {
		if msg.Content == "" {
			continue
		}

		role := msg.Role
		switch role {
		case "user":
			role = "Usuário"
		case "assistant":
			role = "Assistente"
		case "system":
			// Captura o título se presente
			if strings.HasPrefix(msg.Content, "Título da conversa:") {
				title = strings.TrimPrefix(msg.Content, "Título da conversa:")
				title = strings.TrimSpace(title)
			}
			continue // Não inclui mensagens de sistema no texto
		case "tool":
			role = "Ferramenta"
		}

		// Limita o tamanho de cada mensagem para não estourar contexto
		content := msg.Content
		if len(content) > 1000 {
			content = content[:1000] + "..."
		}

		conversationText.WriteString(fmt.Sprintf("%s: %s\n\n", role, content))
	}

	// Adiciona título no início se existir
	var fullText strings.Builder
	if title != "" {
		fullText.WriteString(fmt.Sprintf("Título: %s\n\n", title))
	}
	fullText.WriteString("Conversa:\n\n")
	fullText.WriteString(conversationText.String())

	// Limita o tamanho total da conversa
	conversationStr := fullText.String()
	if len(conversationStr) > 8000 {
		conversationStr = conversationStr[:8000] + "\n\n[... conversa truncada ...]"
	}

	systemPrompt := `Você é um assistente especializado em criar resumos concisos de conversas.
Seu objetivo é capturar os pontos principais, decisões tomadas, e o contexto geral da conversa.

Regras:
- O resumo deve ter no máximo 3 parágrafos
- Foque nos temas principais discutidos
- Inclua decisões importantes ou conclusões
- Mencione tecnologias, projetos ou conceitos específicos discutidos
- Use linguagem clara e objetiva
- Escreva em português`

	userPrompt := fmt.Sprintf(`Por favor, crie um resumo conciso da seguinte conversa:

%s

Resumo:`, conversationStr)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	summary, err := s.client.SimpleChat(ctx, s.model, systemPrompt, userPrompt)
	if err != nil {
		return "", fmt.Errorf("erro ao gerar resumo: %w", err)
	}

	return strings.TrimSpace(summary), nil
}
