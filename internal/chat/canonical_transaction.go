package chat

import (
	"encoding/json"
	"strconv"
)

// Uma escrita persistida gera um único lote lógico, mesmo quando suas leituras
// precisam de parcelas. O estágio possui a mesma cota do journal.
type canonicalPublicationStage struct {
	started    bool
	overflow   bool
	bytes      int
	operations []CanonicalOperation
	patches    []CanonicalWindowPatch
	itemSizes  map[string]int
}

func (s *canonicalPublicationStage) append(operations []CanonicalOperation, patches []CanonicalWindowPatch) {
	if s.overflow {
		return
	}
	encoded, _ := json.Marshal(CanonicalBatch{Operations: operations, WindowPatches: patches})
	s.bytes += len(encoded)
	if s.bytes > canonicalReplayBytes {
		s.overflow = true
		s.operations, s.patches = nil, nil
		return
	}
	s.operations = append(s.operations, operations...)
	s.patches = append(s.patches, patches...)
}

// Não captura buffers nem desfaz storage: somente estado visual e publicação.
func (c *projectionConversation) beginCanonicalPublication() func(string, bool) {
	published, windows := c.publication, c.windows
	c.publication, c.windows = published.fork(), cloneSubscribedWindows(windows)
	stage := &canonicalPublicationStage{itemSizes: make(map[string]int)}
	c.publicationStage = stage
	return func(conversationID string, commit bool) {
		c.publicationStage = nil
		if !commit {
			c.publication, c.windows = published, windows
			return
		}
		if !stage.started {
			return
		}
		if stage.overflow {
			c.publication.batches, c.publication.sizes, c.publication.bytes = nil, nil, 0
			return
		}
		c.publication.recordBatch(CanonicalBatch{ConversationID: conversationID, Epoch: c.epoch, Sequence: strconv.FormatUint(c.publication.sequence, 10), Operations: stage.operations, WindowPatches: c.routeWindowOperations(stage.operations, stage.patches)})
	}
}

// Todos os IDs participantes são reconciliados na versão da escrita antes
// deste ponto. Tamanhos do estágio sobrevivem à evicção do cache compartilhado.
func (c *projectionConversation) validateWindowPublicationLocked() error {
	for _, w := range c.windows {
		if w.syncRequired || w.epoch != c.epoch {
			continue
		}
		total := 0
		for _, id := range w.orderedIDs {
			size, ok := c.publicationStage.itemSizes[id]
			if !ok {
				return ErrProjectionRecoveryRequired
			}
			total += size
			if total > canonicalSourceBytes {
				return ErrTimelineSourceLimit
			}
		}
	}
	return nil
}

func (c *projectionConversation) participatingIDs() []string {
	ids := []string{}
	for _, w := range c.windows {
		if !w.syncRequired && w.epoch == c.epoch {
			ids = append(ids, w.orderedIDs...)
		}
	}
	return uniquePublicationIDs(ids)
}
