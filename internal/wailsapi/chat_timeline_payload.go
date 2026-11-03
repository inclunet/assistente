package wailsapi

import (
	"assistente/internal/apidto"
	"assistente/internal/chat"
)

// Replay e eventos compartilham a única conversão da publicação pública.
// O core já copiou/roteou o conteúdo; índices do journal nunca cruzam a borda.
func timelineUpdates(batches []chat.CanonicalBatch) []apidto.TimelineUpdate {
	updates := make([]apidto.TimelineUpdate, 0, len(batches))
	for _, batch := range batches {
		update := apidto.TimelineUpdate{ConversationID: batch.ConversationID, Epoch: batch.Epoch, Sequence: batch.Sequence, Operations: append([]apidto.CanonicalOperation{}, batch.Operations...), WindowPatches: make([]apidto.TimelineWindowPatch, 0, len(batch.WindowPatches))}
		for _, patch := range batch.WindowPatches {
			update.WindowPatches = append(update.WindowPatches, apidto.TimelineWindowPatch{WindowID: patch.WindowID, PreviousRevision: patch.PreviousRevision, Revision: patch.Revision, Splices: patch.Splices, Items: patch.Items, Metadata: patch.Metadata, SyncRequired: patch.SyncRequired})
		}
		updates = append(updates, update)
	}
	return updates
}
