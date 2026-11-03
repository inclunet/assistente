package chat

import (
	"context"
	"encoding/json"
	"errors"
	"gorm.io/gorm"
	"sort"
	"strconv"
	"time"

	"assistente/internal/database"
)

// Contêineres de janela são trocados, nunca alterados em publicações provisórias.
// O handle permanece um capability estável mesmo quando o estado é revertido.
func cloneSubscribedWindows(windows map[string]*subscribedWindow) map[string]*subscribedWindow {
	result := make(map[string]*subscribedWindow, len(windows))
	for id, w := range windows {
		copy := *w
		result[id] = &copy
	}
	return result
}

// Chamado apenas após transições persistidas, sob a barreira da conversa.
// O caller mantém um checkpoint de publicação/janelas até terminar todo o lote.
func (p *LiveProjection) refreshWindowMembershipLocked(ctx context.Context, conversationID string, c *projectionConversation, version database.ChatTimelineVersion) error {
	ids := make([]string, 0, len(c.windows))
	for id := range c.windows {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	patches := make([]CanonicalWindowPatch, 0)
	newIDs := map[string]bool{}
	budget := canonicalSourceBytes
	for _, id := range ids {
		w := c.windows[id]
		if w.epoch != c.epoch || w.syncRequired || w.version == version {
			continue
		}
		authorized, err := w.handle.authorize(ctx)
		if err != nil {
			delete(c.windows, id)
			continue
		}
		if w.membership == nil {
			return ErrTimelineSubscription
		}
		deadline, ok := ctx.Deadline()
		if !ok {
			deadline = time.Now().Add(canonicalReadTimeout)
		}
		readCtx, cancel := context.WithDeadline(authorized, deadline)
		snapshot, err := w.membership.ReadHistoricalMembership(readCtx, w.query, w.metadata.BeforeCursor, w.metadata.AfterCursor)
		if err != nil {
			if !errors.Is(err, database.ErrTimelineCursorStale) && !errors.Is(err, database.ErrTimelineCursorInvalid) && !errors.Is(err, ErrTimelineSourceLimit) && !errors.Is(err, gorm.ErrRecordNotFound) {
				cancel()
				return err
			}
			// A falha de fronteira só é local depois de provar owner e versão.
			count, readErr := w.membership.ReadWindowMembership(readCtx, database.MessageWindowQuery{ConversationID: conversationID})
			cancel()
			if readErr != nil {
				return readErr
			}
			if count.Version != version {
				return ErrProjectionRecoveryRequired
			}
			previous := strconv.FormatUint(w.revision, 10)
			w.revision++
			w.syncRequired = true
			patches = append(patches, CanonicalWindowPatch{WindowID: id, PreviousRevision: previous, Revision: strconv.FormatUint(w.revision, 10), SyncRequired: true})
			continue
		}
		cancel()
		if snapshot.Version != version {
			return ErrProjectionRecoveryRequired
		}
		encoded, _ := json.Marshal(snapshot.Selection)
		budget -= len(encoded)
		if budget < 0 {
			return ErrTimelineSourceLimit
		}
		rows := snapshot.Selection
		ordered := make([]string, 0, len(rows.Items))
		membership := make(map[string]bool, len(rows.Items))
		for _, row := range rows.Items {
			prefix := "message:"
			if row.Kind == database.MessageWindowItemKindTurn {
				prefix = "turn:"
			}
			itemID := prefix + row.ID
			ordered = append(ordered, itemID)
			membership[itemID] = true
			if !w.itemIDs[itemID] {
				newIDs[itemID] = true
			}
		}
		metadata := TimelineWindowMetadata{TotalCount: rows.TotalCount, StartIndex: rows.StartIndex, EndIndex: rows.EndIndex, HasBefore: rows.HasBefore, HasAfter: rows.HasAfter, BeforeCursor: w.metadata.BeforeCursor, AfterCursor: w.metadata.AfterCursor}
		splices := windowSplices(w.orderedIDs, ordered)
		if len(splices) > 0 || w.metadata != metadata {
			previous := strconv.FormatUint(w.revision, 10)
			w.revision++
			patches = append(patches, CanonicalWindowPatch{WindowID: id, PreviousRevision: previous, Revision: strconv.FormatUint(w.revision, 10), Splices: splices, Metadata: &metadata})
		}
		w.version = version
		w.itemIDs, w.orderedIDs, w.metadata = membership, ordered, metadata
	}
	// Só itens que entraram precisam de payload. A seleção é agregada e limitada.
	refs := make([]string, 0, len(newIDs))
	for id := range newIDs {
		refs = append(refs, id)
	}
	sort.Strings(refs)
	var items []*TimelineItem
	if len(refs) > 0 {
		snapshot, err := p.reader.ReadSelection(ctx, conversationID, refs)
		if err != nil {
			return err
		}
		if snapshot.Version != version || len(snapshot.Items) != len(refs) {
			return ErrProjectionRecoveryRequired
		}
		reconcileStorageLifecycleLocked(c, refs, snapshot)
		if !windowCompositionWithinBudget(c, snapshot.Items) {
			return ErrTimelineSourceLimit
		}
		for _, item := range snapshot.Items {
			composed, err := projectLiveItemLocked(c, item)
			if err != nil {
				return err
			}
			items = append(items, composed)
		}
	}
	byID := make(map[string]CanonicalItem, len(items))
	for _, item := range items {
		view := canonicalItem(item)
		if old, ok := c.publication.items[item.ItemID]; ok {
			view.Revision = old.Revision
		}
		byID[item.ItemID] = view
	}
	for i := range patches {
		for _, splice := range patches[i].Splices {
			for _, id := range splice.InsertIDs {
				// O trecho também pode conter IDs antigos entre duas alterações.
				if view, ok := byID[id]; ok {
					encoded, _ := json.Marshal(view)
					budget -= len(encoded)
					if budget < 0 {
						return ErrTimelineSourceLimit
					}
					patches[i].Items = append(patches[i].Items, view)
				}
			}
		}
	}
	c.publishCanonicalWindowChanges(conversationID, items, nil, patches)
	return nil
}
