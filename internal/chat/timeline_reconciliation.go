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
	target, release, err := s.lockRecoveryTarget(ctx, reference, reader)
	if err != nil {
		return nil, err
	}
	defer release()
	result := &apidto.TimelineRecoveryStatus{ConversationID: s.key.conversationID, Reference: reference, State: "unknown"}
	t := target.turn
	if t == nil {
		if _, ok := s.conversation.windows[s.windowID].recoveryReservations[reference]; ok {
			result.State = "retained"
		}
		return result, nil
	}
	snapshot := target.snapshot
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

type recoveryTarget struct {
	turn     *liveTurn
	snapshot *TimelineSelectionSnapshot
}

// A mesma autorização e identidade exata servem tanto ao estado como aos bytes.
// O release só é devolvido no sucesso; erros liberam a barreira aqui.
func (s *TimelineSubscription) lockRecoveryTarget(ctx context.Context, reference TimelineRecoveryReference, reader TimelineLifecycleReader) (_ *recoveryTarget, _ func(), err error) {
	authorized, c, release, err := s.lockAuthorized(ctx)
	if err != nil {
		return nil, nil, err
	}
	defer func() {
		if err != nil {
			release()
		}
	}()
	if err = ctx.Err(); err != nil {
		return nil, nil, err
	}
	if reader == nil || reference.ProjectionID == "" || reference.Incarnation == "" || reference.TurnID == "" || reference.ExecutionID == "" {
		return nil, nil, ErrTimelineSubscription
	}
	if err = database.CheckTimelineIdentityBudget([]string{reference.ProjectionID, reference.Incarnation, reference.TurnID, reference.ExecutionID}); err != nil {
		return nil, nil, err
	}
	snapshot, err := reader.ReadTurnLifecycles(authorized, s.key.conversationID, []string{reference.TurnID})
	if err != nil {
		return nil, nil, err
	}
	if err = ctx.Err(); err != nil {
		return nil, nil, err
	}
	if _, err = s.authorize(ctx); err != nil {
		return nil, nil, err
	}
	result := &recoveryTarget{snapshot: snapshot}
	if c.projectionID != reference.ProjectionID {
		return result, release, nil
	}
	t := c.turns[reference.TurnID]
	if t == nil || t.executionID != reference.ExecutionID {
		t = c.retained[reference.ExecutionID]
	}
	if t != nil && t.turnID == reference.TurnID && t.executionID == reference.ExecutionID && t.incarnation == reference.Incarnation {
		result.turn = t
	}
	return result, release, nil
}
