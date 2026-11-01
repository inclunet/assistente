package agent

import (
	"assistente/internal/chat"
	"assistente/internal/database"
	"assistente/internal/toolinvocations"
	"assistente/internal/turnlease"
	"context"
	"strings"
)

// Testes isolados de handlers usam o repositório de mensagens da própria
// fixture. A integração do lifecycle é exercitada separadamente com SQLite real.
type handlerTestTurnStore struct{ repo chat.MessageRepository }

func newTestAgentService(cfg ServiceConfig) *Service {
	cfg.Turns = &handlerTestTurnStore{repo: cfg.MsgRepo}
	cfg.Timeline = &handlerTestTimelineReader{repo: cfg.MsgRepo}
	return NewService(cfg)
}

func (s *handlerTestTurnStore) CheckReady(context.Context) error { return nil }
func (s *handlerTestTurnStore) CreateExecutionLease(context.Context, string) (*turnlease.Lease, error) {
	panic("fixture sem admissão")
}
func (s *handlerTestTurnStore) BeginExecution(context.Context, string, string, string, string, string, string) (*database.ChatTurnAttempt, error) {
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

// O fake de handlers isola transporte/lifecycle. Testes de snapshot e runtime
// usam DBTimelineReader real, sem fallback produtivo para leituras avulsas.
type handlerTestTimelineReader struct{ repo chat.MessageRepository }

func (r *handlerTestTimelineReader) ReadSelection(context.Context, string, []string) (*chat.TimelineSelectionSnapshot, error) {
	panic("fixture de handler sem mutação externa")
}

func (r *handlerTestTimelineReader) ReadPublicationTurn(ctx context.Context, conversationID, turnID string) (*chat.TimelineTurnSnapshot, error) {
	return r.ReadTurn(ctx, conversationID, turnID)
}

func (r *handlerTestTimelineReader) ReadTurn(ctx context.Context, conversationID, turnID string) (*chat.TimelineTurnSnapshot, error) {
	question, err := r.repo.GetMessage(ctx, turnID)
	if err != nil {
		return nil, err
	}
	var parent *string
	if question != nil {
		parent = question.ParentID
	}
	messages, err := r.repo.GetMessagesByTurnID(ctx, conversationID, parent, turnID, 0)
	if err != nil {
		return nil, err
	}
	userID, err := database.RequireUserID(ctx)
	if err != nil {
		return nil, err
	}
	summaries, err := toolinvocations.LoadSummariesForTurnIDsWithUser(ctx, userID, []string{turnID})
	if err != nil {
		return nil, err
	}
	responseMessages := make([]chat.Message, 0, len(messages))
	for _, message := range messages {
		if message.Role != "user" {
			responseMessages = append(responseMessages, message)
		}
	}
	response, err := chat.ProjectPersistedResponse(conversationID, turnID, parent, responseMessages, chat.ToolInvocationSummariesToTurnSegments(summaries)[turnID], nil)
	return &chat.TimelineTurnSnapshot{Response: response}, err
}

func (r *handlerTestTimelineReader) ReadVersion(context.Context, string) (database.ChatTimelineVersion, error) {
	panic("fixture de handler sem admissão")
}

func (s *handlerTestTurnStore) FinishResponseWithVersion(ctx context.Context, conversationID, turnID, executionID, state, reason, responseID string, response *database.MessageOptions) (database.ChatTimelineVersion, error) {
	return database.ChatTimelineVersion{}, s.FinishResponse(ctx, conversationID, turnID, executionID, state, reason, responseID, response)
}

func (r *handlerTestTimelineReader) ReadWindow(context.Context, database.MessageWindowQuery) (*chat.TimelineWindowSnapshot, error) {
	panic("fixture de handler sem leitura de janela")
}
