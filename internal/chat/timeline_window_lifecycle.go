package chat

import (
	"assistente/internal/database"
	"context"
	"strings"
)

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

// Todos os consumidores usam o mesmo percurso limitado, com versão por parcela.
func (p *LiveProjection) visitWindowLifecycles(ctx context.Context, conversationID string, ids []string, version database.ChatTimelineVersion, visit func([]string, *TimelineSelectionSnapshot) error) error {
	if len(ids) == 0 {
		return nil
	}
	if p.reader == nil {
		return ErrTimelineSubscription
	}
	_, err := visitVersionedSelections(ctx, conversationID, ids, &version, p.reader.ReadTurnLifecycles, visit)
	return err
}

// Todo produtor comprova os turnos antes de qualquer janela perder participantes.
func (p *LiveProjection) reconcileParticipatingLifecycles(ctx context.Context, conversationID string, c *projectionConversation, version database.ChatTimelineVersion) error {
	// Janelas aguardando sync ainda referenciam buffers live. Sua exclusão da
	// publicação não dispensa comprovar que a execução continua sendo a atual.
	var referencedIDs []string
	for _, window := range c.windows {
		referencedIDs = append(referencedIDs, window.orderedIDs...)
	}
	return p.visitWindowLifecycles(ctx, conversationID, liveWindowTurnIDs(c, referencedIDs), version, func(_ []string, snapshot *TimelineSelectionSnapshot) error {
		reconcileStorageLifecycleLocked(c, nil, snapshot)
		return nil
	})
}

// Guarda somente a decisão de validade, sob a mesma barreira. Não acumula
// registros por parcela nem sobrescreve os registros completos do snapshot.
// Todos os guards precedem os efeitos de readWindowLocked.
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
	c := r.subscription.conversation
	ids := liveWindowTurnIDs(c, r.previous.orderedIDs)
	if len(ids) == 0 {
		return snapshot, nil
	}
	validity := make(map[string]bool, len(ids))
	err = r.subscription.owner.visitWindowLifecycles(ctx, q.ConversationID, ids, snapshot.Version, func(_ []string, proof *TimelineSelectionSnapshot) error {
		for _, id := range proof.TurnIDs {
			validity[id] = projectedTurnCompatible(c.turns[id], proof.Version, proof.Turns[id])
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	result := *snapshot
	result.priorTurnValidity = validity
	return &result, nil
}
