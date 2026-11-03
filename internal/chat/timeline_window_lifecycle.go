package chat

import (
	"assistente/internal/database"
	"context"
	"maps"
	"strings"
)

// Comprova os turnos da janela anterior mesmo quando a nova seleção os corta.
// Ausência na página não é ausência no storage; a leitura não carrega payloads.
func (s *TimelineSubscription) readPreviousWindowLifecycle(ctx context.Context, w *subscribedWindow, version database.ChatTimelineVersion) (*TimelineSelectionSnapshot, error) {
	ids := []string{}
	for _, itemID := range w.orderedIDs {
		_, id, _ := strings.Cut(itemID, ":")
		if s.conversation.turns[id] != nil {
			ids = append(ids, id)
		}
	}
	if len(ids) == 0 {
		return nil, nil
	}
	if s.owner.reader == nil {
		return nil, ErrTimelineSubscription
	}
	readCtx, cancel := context.WithTimeout(ctx, canonicalReadTimeout)
	defer cancel()
	lifecycle, err := s.owner.reader.ReadTurnLifecycles(readCtx, s.key.conversationID, uniquePublicationIDs(ids))
	if err != nil {
		return nil, err
	}
	if lifecycle.Version != version {
		return nil, ErrProjectionRecoveryRequired
	}
	return lifecycle, nil
}

// A prova é incorporada ao snapshot antes de qualquer efeito do projetor.
// Os guards do leitor interno (incluindo encarnação) continuam precedendo-a.
type subscriptionLifecycleReader struct {
	subscription *TimelineSubscription
	previous     *subscribedWindow
	reader       WindowSnapshotReader
}

func (r subscriptionLifecycleReader) ReadWindow(ctx context.Context, q database.MessageWindowQuery) (*TimelineWindowSnapshot, error) {
	snapshot, err := r.reader.ReadWindow(ctx, q)
	if err != nil {
		return nil, err
	}
	lifecycle, err := r.subscription.readPreviousWindowLifecycle(ctx, r.previous, snapshot.Version)
	if err != nil {
		return nil, err
	}
	if lifecycle == nil {
		return snapshot, nil
	}
	result := *snapshot
	result.TurnIDs = uniquePublicationIDs(append(append([]string(nil), snapshot.TurnIDs...), lifecycle.TurnIDs...))
	result.Turns = maps.Clone(snapshot.Turns)
	if result.Turns == nil {
		result.Turns = make(map[string]*database.ChatTurn)
	}
	for _, id := range lifecycle.TurnIDs {
		if turn := lifecycle.Turns[id]; turn != nil {
			result.Turns[id] = turn
		} else {
			delete(result.Turns, id)
		}
	}
	return &result, nil
}
