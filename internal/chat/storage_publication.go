package chat

import (
	"context"
	"sort"
	"strings"

	"assistente/internal/database"
	"github.com/google/uuid"
)

// ApplyStorageMutation envolve a operação externa inteira, antes de abrir sua
// transação. O callback não pode reentrar no projetor nem emitir eventos. IDs
// conhecidos são passados antes; criações podem devolver IDs após o commit.
// known == nil reconcilia todos os itens materializados (exclusões em cascata).
// Mesmo um callback com erro pode ter confirmado parte da operação: reconciliar
// nunca transforma sucesso de escrita em falha, nem esconde seu erro original.
func (p *LiveProjection) ApplyStorageMutation(ctx context.Context, conversationID string, known []string, write func() ([]string, error)) error {
	userID, err := database.RequireUserID(ctx)
	if err != nil {
		return err
	}
	// Reservar mesmo sem observadores: a primeira inscrição não pode capturar
	// estado anterior enquanto este writer está prestes a confirmar.
	c, release := p.lockConversation(projectionKey{userID, conversationID}, true)
	defer release()
	if p.reader == nil {
		_, err := write()
		return err
	}
	ids := known
	if known == nil {
		ids = c.materializedIDsLocked()
	}
	created, writeErr := write()
	if len(c.windows) == 0 && len(c.turns) == 0 && len(c.publication.items) == 0 {
		return writeErr // nenhuma base anterior; a primeira inscrição lerá após o commit
	}
	ids = uniquePublicationIDs(append(append([]string(nil), ids...), created...))
	if c.storagePublicationErr != nil {
		ids = uniquePublicationIDs(append(ids, c.materializedIDsLocked()...))
	}
	if err := p.reconcileStorageLocked(ctx, conversationID, c, ids); err != nil {
		c.failStoragePublicationLocked(ids, err)
	}
	return writeErr
}

func uniquePublicationIDs(ids []string) []string {
	seen := make(map[string]bool, len(ids))
	for _, id := range ids {
		seen[id] = true
	}
	result := make([]string, 0, len(seen))
	for id := range seen {
		result = append(result, id)
	}
	sort.Strings(result)
	return result
}

func (c *projectionConversation) materializedIDsLocked() []string {
	ids := make([]string, 0, len(c.publication.items)+len(c.turns))
	for id := range c.publication.items {
		ids = append(ids, id)
	}
	for id := range c.turns {
		ids = append(ids, "turn:"+id)
	}
	for _, window := range c.windows {
		for id := range window.itemIDs {
			ids = append(ids, id)
		}
	}
	for id := range c.pendingStorageIDs {
		ids = append(ids, id)
	}
	return uniquePublicationIDs(ids)
}

func (c *projectionConversation) failStoragePublicationLocked(ids []string, err error) {
	if c.publicationErrorLocked() == nil {
		c.epoch = uuid.NewString()
	}
	c.storagePublicationErr = err
	if c.pendingStorageIDs == nil {
		c.pendingStorageIDs = make(map[string]bool)
	}
	for _, id := range ids {
		// Suplemento para criações que ainda não entraram em nenhuma janela.
		// A época já obriga snapshot; itens fora desta cota serão descobertos
		// pela janela solicitada, sem acumular IDs indefinidamente durante falha.
		if len(c.pendingStorageIDs) < canonicalItemCacheCount {
			c.pendingStorageIDs[id] = true
		}
	}
	if c.publicationTimer != nil {
		c.publicationTimer.Stop()
		c.publicationTimer = nil
	}
	c.publicationGeneration++
}

func (p *LiveProjection) recoverStoragePublicationLocked(ctx context.Context, conversationID string, c *projectionConversation) error {
	if c.storagePublicationErr == nil {
		return nil
	}
	ids := c.materializedIDsLocked()
	if err := p.reconcileStorageLocked(ctx, conversationID, c, ids); err != nil {
		c.failStoragePublicationLocked(ids, err)
		return err
	}
	return nil
}

func (p *LiveProjection) reconcileStorageLocked(ctx context.Context, conversationID string, c *projectionConversation, ids []string) error {
	readCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), canonicalReadTimeout)
	defer cancel()
	snapshot, err := p.reader.ReadSelection(readCtx, conversationID, ids)
	if err != nil {
		return err
	}
	reconcileProjectionVersion(c, snapshot.Version)
	for _, id := range ids {
		if turnID, ok := strings.CutPrefix(id, "turn:"); ok {
			if t := c.turns[turnID]; t != nil {
				reconcileProjectedTurn(c, t, snapshot.Version, snapshot.Turns[turnID])
			}
		}
	}
	items := make([]*TimelineItem, 0, len(snapshot.Items))
	present := make(map[string]bool, len(snapshot.Items))
	for _, item := range snapshot.Items {
		present[item.ItemID] = true
		p.recoverPublicationLocked(c, item.TurnID, item)
		composed, err := projectLiveItemLocked(c, item)
		if err != nil {
			return err
		}
		items = append(items, composed)
	}
	removed := make([]string, 0)
	for _, id := range ids {
		if !present[id] {
			removed = append(removed, id)
		}
	}
	c.storagePublicationErr, c.pendingStorageIDs = nil, nil
	if err := c.publicationErrorLocked(); err != nil {
		return err
	}
	c.publishCanonicalChanges(conversationID, items, removed)
	c.flushPublicationLocked(conversationID)
	return nil
}
