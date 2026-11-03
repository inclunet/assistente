package chat

import (
	"slices"
	"sort"
	"strconv"
)

type CanonicalWindowPatch struct {
	WindowID         string                  `json:"windowId"`
	PreviousRevision string                  `json:"previousRevision"`
	Revision         string                  `json:"revision"`
	Splices          []TimelineWindowSplice  `json:"splices,omitempty"`
	Items            []CanonicalItem         `json:"items,omitempty"`
	Metadata         *TimelineWindowMetadata `json:"metadata,omitempty"`
	SyncRequired     bool                    `json:"syncRequired,omitempty"`
	// Roteamento histórico do journal, removido da resposta de Replay.
	OperationIndexes []int `json:"operationIndexes,omitempty"`
}

func metadataFromSubscription(s *SubscribedTimelineSnapshot) TimelineWindowMetadata {
	return TimelineWindowMetadata{TotalCount: s.TotalCount, StartIndex: s.StartIndex, EndIndex: s.EndIndex, HasBefore: s.HasBefore, HasAfter: s.HasAfter, BeforeCursor: s.BeforeCursor, AfterCursor: s.AfterCursor, RetainedItemIDs: append([]string{}, s.RetainedItemIDs...), RetentionBlocked: s.RetentionBlocked}
}

// Preserva o prefixo e o sufixo comuns, alterando somente o trecho necessário.
func windowSplices(before, after []string) []TimelineWindowSplice {
	start := 0
	for start < len(before) && start < len(after) && before[start] == after[start] {
		start++
	}
	oldEnd, newEnd := len(before), len(after)
	for oldEnd > start && newEnd > start && before[oldEnd-1] == after[newEnd-1] {
		oldEnd--
		newEnd--
	}
	if oldEnd == start && newEnd == start {
		return nil
	}
	return []TimelineWindowSplice{{Index: start, DeleteCount: oldEnd - start, InsertIDs: append([]string{}, after[start:newEnd]...)}}
}

func (c *projectionConversation) routeWindowOperations(operations []CanonicalOperation, changes []CanonicalWindowPatch) []CanonicalWindowPatch {
	byID := make(map[string]CanonicalWindowPatch, len(changes))
	for _, patch := range changes {
		byID[patch.WindowID] = patch
	}
	ids := make([]string, 0, len(c.windows))
	for id := range c.windows {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	result := make([]CanonicalWindowPatch, 0, len(ids))
	for _, id := range ids {
		w := c.windows[id]
		patch, ok := byID[id]
		if !ok {
			revision := strconv.FormatUint(w.revision, 10)
			patch = CanonicalWindowPatch{WindowID: id, PreviousRevision: revision, Revision: revision}
		}
		inserted := map[string]bool{}
		for _, item := range patch.Items {
			inserted[item.ItemID] = true
		}
		if !w.syncRequired && w.epoch == c.epoch {
			for i, op := range operations {
				if w.itemIDs[op.ItemID] && !inserted[op.ItemID] {
					patch.OperationIndexes = append(patch.OperationIndexes, i)
				}
			}
		}
		result = append(result, patch)
	}
	return result
}

func replayWindowBatches(w *subscribedWindow, revision string, batches []CanonicalBatch) ([]CanonicalBatch, bool, error) {
	current, ok := w.acceptsSnapshotRevision(revision)
	if !ok {
		return nil, false, ErrTimelineWindowStale
	}
	for i := range batches {
		batch := &batches[i]
		operations := make([]CanonicalOperation, 0)
		patches := make([]CanonicalWindowPatch, 0, 1)
		found := false
		for _, patch := range batch.WindowPatches {
			if patch.WindowID != w.id {
				continue
			}
			found = true
			if patch.PreviousRevision != strconv.FormatUint(current, 10) {
				return nil, false, ErrTimelineWindowStale
			}
			next, err := strconv.ParseUint(patch.Revision, 10, 64)
			if err != nil {
				return nil, false, ErrTimelineWindowStale
			}
			current = next
			for _, index := range patch.OperationIndexes {
				if index < 0 || index >= len(batch.Operations) {
					return nil, false, ErrTimelineWindowStale
				}
				operations = append(operations, batch.Operations[index])
			}
			patch.OperationIndexes = nil
			if patch.PreviousRevision != patch.Revision || patch.SyncRequired {
				patches = append(patches, patch)
			}
		}
		if !found {
			return nil, false, ErrTimelineWindowStale
		}
		batch.Operations = operations
		batch.WindowPatches = patches
	}
	if current != w.revision {
		return nil, false, ErrTimelineWindowStale
	}
	return batches, true, nil
}

// Uma lacuna pode esconder patches automáticos. Recuperação aceita a revisão
// conhecida desde o último snapshot, mas nunca atravessa Navigate/Refresh aceito.
func (w *subscribedWindow) acceptsSnapshotRevision(value string) (uint64, bool) {
	revision, err := strconv.ParseUint(value, 10, 64)
	return revision, err == nil && strconv.FormatUint(revision, 10) == value && revision >= w.snapshotRevision && revision <= w.revision
}

func sameWindowMetadata(a, b TimelineWindowMetadata) bool {
	return a.TotalCount == b.TotalCount && a.StartIndex == b.StartIndex && a.EndIndex == b.EndIndex && a.HasBefore == b.HasBefore && a.HasAfter == b.HasAfter && a.BeforeCursor == b.BeforeCursor && a.AfterCursor == b.AfterCursor && a.RetentionBlocked == b.RetentionBlocked && slices.Equal(a.RetainedItemIDs, b.RetainedItemIDs)
}
