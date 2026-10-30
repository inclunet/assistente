package agent

import (
	"assistente/internal/chat"
	"assistente/internal/database"
	"context"
	"strings"
)

// Testes isolados de handlers usam o repositório de mensagens da própria
// fixture. A integração do lifecycle é exercitada separadamente com SQLite real.
type handlerTestTurnStore struct{ repo chat.MessageRepository }

func newTestAgentService(cfg ServiceConfig) *Service {
	cfg.Turns = &handlerTestTurnStore{repo: cfg.MsgRepo}
	return NewService(cfg)
}

func (s *handlerTestTurnStore) Begin(context.Context, string, string, string, string) (*database.ChatTurnAttempt, error) {
	panic("fixture de handler não implementa admissão de turnos")
}
func (s *handlerTestTurnStore) EnsureResponse(ctx context.Context, conversationID, turnID, _ string) (string, error) {
	return chat.EnsureAssistantPlaceholder(ctx, s.repo, conversationID, turnID)
}
func (s *handlerTestTurnStore) MarkStreaming(context.Context, string, string, string) error {
	return nil
}
func (s *handlerTestTurnStore) WritePartial(ctx context.Context, _, messageID, content, reasoning string, onlyWhenEmpty bool) error {
	if s.repo == nil {
		return nil
	}
	message, err := s.repo.GetMessage(ctx, messageID)
	if err != nil || message == nil {
		if onlyWhenEmpty {
			return err
		}
		message = &chat.Message{}
	}
	if onlyWhenEmpty && strings.TrimSpace(message.Content) != "" {
		return nil
	}
	if strings.TrimSpace(reasoning) == "" {
		reasoning = message.Reasoning
	}
	return s.repo.UpdateMessageContentAndReasoning(ctx, messageID, content, reasoning, message.PromptTokens, message.CompletionTokens, message.TotalTokens, message.Model)
}
func (s *handlerTestTurnStore) FinishResponse(ctx context.Context, _, _, _, _, _, responseID string, response *database.MessageOptions) error {
	if response == nil {
		return nil
	}
	_, err := chat.FinalizeAssistantMessage(ctx, s.repo, responseID, *response)
	return err
}
