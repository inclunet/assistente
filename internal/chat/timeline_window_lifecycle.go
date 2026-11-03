package chat

import (
	"assistente/internal/database"
	"context"
	"maps"
	"strings"
)

// Comprova os turnos da janela anterior mesmo quando a nova seleção os corta.
// Ausência na página não é ausência no storage; a leitura não carrega payloads.
func liveWindowTurnIDs(c *projectionConversation, itemIDs []string) []string {
	ids := []string{}
	for _, itemID := range itemIDs {
		_, id, _ := strings.Cut(itemID, ":")
		if c.turns[id] != nil {
			ids = append(ids, id)
		}
	}
	return uniquePublicationIDs(ids)
}

func (p *LiveProjection) readWindowLifecycles(ctx context.Context, conversationID string, ids []string, version database.ChatTimelineVersion) (*TimelineSelectionSnapshot, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	if p.reader == nil {
		return nil, ErrTimelineSubscription
	}
	readCtx, cancel := context.WithTimeout(ctx, canonicalReadTimeout)
	defer cancel()
	lifecycle, err := p.reader.ReadTurnLifecycles(readCtx, conversationID, ids)
	if err != nil {
		return nil, err
	}
	if lifecycle.Version != version {
		return nil, ErrProjectionRecoveryRequired
	}
	return lifecycle, nil
}

// Todo produtor que troca participação comprova primeiro os turnos que podem
// sair de qualquer janela. Parcelas limitam a leitura sem carregar payloads.
func (p *LiveProjection) reconcileParticipatingLifecycles(ctx context.Context, conversationID string, c *projectionConversation, version database.ChatTimelineVersion) error {
	ids := liveWindowTurnIDs(c, c.participatingIDs())
	for start := 0; start < len(ids); start += 64 {
		snapshot, err := p.readWindowLifecycles(ctx, conversationID, ids[start:min(start+64, len(ids))], version)
		if err != nil {
			return err
		}
		reconcileStorageLifecycleLocked(c, nil, snapshot)
	}
	return nil
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
	lifecycle, err := r.subscription.owner.readWindowLifecycles(ctx, q.ConversationID, liveWindowTurnIDs(r.subscription.conversation, r.previous.orderedIDs), snapshot.Version)
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
