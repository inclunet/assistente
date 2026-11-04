package chat

import (
	"context"

	"assistente/internal/apidto"
	"assistente/internal/database"
)

// ReconcileRecovery consulta somente lifecycle e identidade sob a barreira da
// conversa. Não altera época, publicação ou retenção e não confirma transferência
// dos bytes já vistos pelo cliente: uma execução pode receber novos chunks.
func (s *TimelineSubscription) ReconcileRecovery(ctx context.Context, reference TimelineRecoveryReference, reader TimelineLifecycleReader) (*apidto.TimelineRecoveryStatus, error) {
	authorized, c, release, err := s.lockAuthorized(ctx)
	if err != nil {
		return nil, err
	}
	defer release()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if reader == nil || reference.ProjectionID == "" || reference.Incarnation == "" || reference.TurnID == "" || reference.ExecutionID == "" {
		return nil, ErrTimelineSubscription
	}
	if err := database.CheckTimelineIdentityBudget([]string{reference.ProjectionID, reference.Incarnation, reference.TurnID, reference.ExecutionID}); err != nil {
		return nil, err
	}
	snapshot, err := reader.ReadTurnLifecycles(authorized, s.key.conversationID, []string{reference.TurnID})
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if _, err := s.authorize(ctx); err != nil {
		return nil, err
	}
	result := &apidto.TimelineRecoveryStatus{ConversationID: s.key.conversationID, Reference: reference, State: "unknown"}
	if c.projectionID != reference.ProjectionID {
		return result, nil
	}
	t := c.turns[reference.TurnID]
	if t == nil || t.executionID != reference.ExecutionID {
		t = c.retained[reference.ExecutionID]
	}
	if t == nil || t.turnID != reference.TurnID || t.executionID != reference.ExecutionID || t.incarnation != reference.Incarnation {
		return result, nil
	}
	compatible := projectedTurnCompatible(t, snapshot.Version, snapshot.Turns[reference.TurnID])
	switch {
	case !t.finished && !t.detached && compatible:
		result.State = "active"
	case !t.finished || t.volatile || hasUncommittedContent(t):
		result.State = "retained"
	case !t.detached && compatible && t.committedVersion.Incarnation != "" && t.committedVersion == snapshot.Version:
		result.State = "persisted"
	}
	return result, nil
}
