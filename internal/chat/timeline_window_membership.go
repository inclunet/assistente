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
	if err := p.reconcileParticipatingLifecycles(ctx, conversationID, c, version); err != nil {
		return err
	}
	ids := make([]string, 0, len(c.windows))
	for id := range c.windows {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	patches := make([]CanonicalWindowPatch, 0)
	for _, id := range ids {
		w := c.windows[id]
		if w.epoch != c.epoch || w.syncRequired || (w.version == version && w.pendingRetention == nil) {
			continue
		}
		authorized, err := w.handle.authorize(ctx)
		if err != nil {
			delete(c.windows, id)
			continue
		}
		deadline, ok := ctx.Deadline()
		if !ok {
			deadline = time.Now().Add(canonicalReadTimeout)
		}
		readCtx, cancel := context.WithDeadline(authorized, deadline)
		retained, retainErr := p.survivingRetention(readCtx, conversationID, w, version)
		if retainErr != nil {
			cancel()
			return retainErr
		}
		snapshot, blocked, err := selectRetainedMembership(readCtx, w, retained, version)
		if err == nil && !containsRetained(membershipIDs(snapshot.Selection), retained) {
			err = database.ErrTimelineCursorStale
		}
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
			w.pendingRetention = nil
			w.metadata.RetainedItemIDs, w.metadata.RetentionBlocked = retained, blocked
			metadata := w.metadata
			patches = append(patches, CanonicalWindowPatch{WindowID: id, PreviousRevision: previous, Revision: strconv.FormatUint(w.revision, 10), SyncRequired: true, Metadata: &metadata})
			continue
		}
		cancel()
		encoded, _ := json.Marshal(snapshot.Selection)
		if len(encoded) > canonicalSourceBytes {
			return ErrTimelineSourceLimit
		}
		rows := snapshot.Selection
		ordered := make([]string, 0, len(rows.Items))
		entering := make([]CanonicalItem, 0)
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
				entering = append(entering, CanonicalItem{ItemID: itemID})
			}
		}
		metadata := TimelineWindowMetadata{RetainedItemIDs: retained, RetentionBlocked: blocked, TotalCount: rows.TotalCount, StartIndex: rows.StartIndex, EndIndex: rows.EndIndex, HasBefore: rows.HasBefore, HasAfter: rows.HasAfter, BeforeCursor: w.metadata.BeforeCursor, AfterCursor: w.metadata.AfterCursor}
		if w.mode == TimelineWindowFollowTail && !blocked {
			metadata.BeforeCursor, metadata.AfterCursor = "", ""
			if len(rows.Items) > 0 {
				metadata.BeforeCursor = database.EncodeTimelineCursor(conversationID, w.query.ParentID, rows.Items[0])
				metadata.AfterCursor = database.EncodeTimelineCursor(conversationID, w.query.ParentID, rows.Items[len(rows.Items)-1])
			}
		}
		splices := windowSplices(w.orderedIDs, ordered)
		if len(splices) > 0 || !sameWindowMetadata(w.metadata, metadata) {
			previous := strconv.FormatUint(w.revision, 10)
			w.revision++
			patches = append(patches, CanonicalWindowPatch{WindowID: id, PreviousRevision: previous, Revision: strconv.FormatUint(w.revision, 10), Splices: splices, Items: entering, Metadata: &metadata})
		}
		w.pendingRetention = nil
		w.version = version
		w.itemIDs, w.orderedIDs, w.metadata = membership, ordered, metadata
	}
	// O estágio recebe somente as identidades das entradas. Seus payloads vêm
	// do percurso parcelado único dos participantes, sem leitura agregada extra.
	c.publishCanonicalWindowChanges(conversationID, nil, nil, patches)
	return nil
}
