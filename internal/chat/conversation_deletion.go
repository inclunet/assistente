package chat

import (
	"context"

	"assistente/internal/database"
	"github.com/google/uuid"
)

// ApplyConversationDeletion coordena um delete transacional de IDs previamente
// normalizados/autorizados. O writer revalida a posse e devolve somente IDs do
// lote confirmado. Lifecycle e manutenção devem anteceder esta barreira.
// Falhas de escrita preservam inscrições, buffers e replay integralmente.
func (p *LiveProjection) ApplyConversationDeletion(ctx context.Context, ids []string, write func() ([]string, error)) ([]string, error) {
	userID, err := database.RequireUserID(ctx)
	if err != nil {
		return nil, err
	}
	ids = uniquePublicationIDs(ids)
	conversations, release := p.lockConversationBatch(userID, ids)
	defer release()
	deleted, err := write()
	if err != nil {
		return nil, err
	}
	for _, id := range deleted {
		if c := conversations[id]; c != nil {
			c.clearDeletedConversationLocked()
		}
	}
	return deleted, nil
}

// Reserva todas as referências antes de adquirir mutexes de conversa, em ordem
// estável. Nunca readquire p.mu mantendo c.mu: cleanup usa a ordem p.mu → c.mu.
func (p *LiveProjection) lockConversationBatch(userID string, ids []string) (map[string]*projectionConversation, func()) {
	conversations := make(map[string]*projectionConversation, len(ids))
	p.mu.Lock()
	for _, id := range ids {
		key := projectionKey{userID, id}
		c := p.conversations[key]
		if c == nil {
			c = &projectionConversation{epoch: uuid.NewString(), turns: make(map[string]*liveTurn), retained: make(map[string]*liveTurn)}
			p.conversations[key] = c
		}
		c.readers++
		conversations[id] = c
	}
	p.mu.Unlock()
	for _, id := range ids {
		conversations[id].mu.Lock()
	}
	return conversations, func() {
		for i := len(ids) - 1; i >= 0; i-- {
			conversations[ids[i]].mu.Unlock()
		}
		p.mu.Lock()
		for _, c := range conversations {
			c.readers--
		}
		p.mu.Unlock()
		for _, id := range ids {
			p.scheduleCleanup(projectionKey{userID, id}, conversations[id])
		}
	}
}

func (c *projectionConversation) clearDeletedConversationLocked() {
	if c.publicationTimer != nil {
		c.publicationTimer.Stop()
		c.publicationTimer = nil
	}
	c.publicationGeneration++
	c.epoch = uuid.NewString()
	c.incarnation = ""
	c.revision++
	c.lastRecoveryVersion = database.ChatTimelineVersion{}
	c.turns = make(map[string]*liveTurn)
	c.retained = make(map[string]*liveTurn)
	c.windows = nil
	c.publication = canonicalPublication{}
	c.storagePublicationErr, c.pendingStorageIDs = nil, nil
}
