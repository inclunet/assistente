package chat

import (
	"context"
	"errors"
	"slices"
	"strconv"

	"assistente/internal/database"
)

var ErrTimelineRetention = errors.New("retenção de leitura inválida; libere os itens antes de navegar")

type TimelineRetentionRequest struct {
	Epoch          string
	WindowRevision string
	ItemIDs        []string
}

// SetRetention substitui as referências visuais desta inscrição. Não altera
// pinned persistido. A confirmação é publicada com os splices no mesmo lote.
func (s *TimelineSubscription) SetRetention(ctx context.Context, request TimelineRetentionRequest) (resultErr error) {
	authorized, c, release, err := s.lockAuthorized(ctx)
	if err != nil {
		return err
	}
	defer release()
	w := c.windows[s.windowID]
	if request.Epoch != w.epoch || (len(request.ItemIDs) > 0 && w.epoch != c.epoch) || request.WindowRevision != strconv.FormatUint(w.revision, 10) {
		return ErrTimelineWindowStale
	}
	if s.owner.reader == nil || len(request.ItemIDs) > database.MaxMessageWindowRows {
		return ErrTimelineRetention
	}
	if err := database.CheckTimelineIdentityBudget(request.ItemIDs); err != nil {
		return err
	}
	requested := uniquePublicationIDs(request.ItemIDs)
	for _, id := range requested {
		if !w.itemIDs[id] {
			return ErrTimelineRetention
		}
	}
	if w.syncRequired && len(requested) > 0 {
		return ErrTimelineWindowStale
	}
	version, err := s.owner.reader.ReadVersion(authorized, s.key.conversationID)
	if err != nil {
		return err
	}
	if version.Incarnation != w.version.Incarnation || (len(requested) > 0 && !w.syncRequired && version != w.version) {
		return ErrTimelineWindowStale
	}
	if slices.Equal(requested, w.metadata.RetainedItemIDs) {
		return nil
	}
	if len(requested) == 0 && (w.syncRequired || w.epoch != c.epoch || c.publicationErrorLocked() != nil) {
		// Uma base já invalidada não pode voltar à cauda sem snapshot. A liberação
		// explícita retira apenas referências, mantendo o último visual até recovery.
		updated := *w
		metadata := w.metadata
		metadata.RetainedItemIDs, metadata.RetentionBlocked = []string{}, false
		previous := strconv.FormatUint(w.revision, 10)
		updated.revision++
		updated.metadata, updated.syncRequired = metadata, true
		c.windows[s.windowID] = &updated
		c.publishCanonicalWindowChanges(s.key.conversationID, nil, nil, []CanonicalWindowPatch{{WindowID: s.windowID, PreviousRevision: previous, Revision: strconv.FormatUint(updated.revision, 10), Metadata: &metadata, SyncRequired: true}})
		return nil
	}
	finish := c.beginCanonicalPublication()
	defer func() { finish(s.key.conversationID, resultErr == nil) }()
	c.windows[s.windowID].pendingRetention = &requested
	return s.owner.reconcileStorageStageLocked(authorized, s.key.conversationID, c, nil, &version)
}

func membershipIDs(rows *database.MessageWindowResult) []string {
	ids := make([]string, 0, len(rows.Items))
	for _, row := range rows.Items {
		prefix := "message:"
		if row.Kind == database.MessageWindowItemKindTurn {
			prefix = "turn:"
		}
		ids = append(ids, prefix+row.ID)
	}
	return ids
}

func retainedInSnapshot(retained []string, snapshot *TimelineWindowSnapshot) []string {
	present := map[string]bool{}
	for _, item := range snapshot.Items {
		present[item.ItemID] = true
	}
	result := []string{}
	for _, id := range retained {
		if present[id] {
			result = append(result, id)
		}
	}
	return result
}

// Prova existência e escopo na mesma versão. Ausência na cauda não é exclusão.
func (p *LiveProjection) survivingRetention(ctx context.Context, conversationID string, w *subscribedWindow, version database.ChatTimelineVersion) ([]string, error) {
	retained := w.metadata.RetainedItemIDs
	if w.pendingRetention != nil {
		retained = *w.pendingRetention
	}
	if len(retained) == 0 {
		return []string{}, nil
	}
	selected, err := p.reader.ReadSelection(ctx, conversationID, retained)
	if err != nil {
		return nil, err
	}
	if selected.Version != version {
		return nil, ErrProjectionRecoveryRequired
	}
	present := map[string]bool{}
	for _, item := range selected.Items {
		sameScope := (item.ScopeParentID == nil && w.query.ParentID == nil) || (item.ScopeParentID != nil && w.query.ParentID != nil && *item.ScopeParentID == *w.query.ParentID)
		if sameScope {
			present[item.ItemID] = true
		}
	}
	result := []string{}
	for _, id := range retained {
		if present[id] {
			result = append(result, id)
		}
	}
	return result, nil
}

// Refresh de uma janela retida materializa suas fronteiras atuais, nunca a
// consulta end/before original. Versões diferentes impedem misturar as leituras.
type retainedWindowReader struct {
	subscription *TimelineSubscription
	window       *subscribedWindow
}

func (r retainedWindowReader) ReadWindow(ctx context.Context, query database.MessageWindowQuery) (*TimelineWindowSnapshot, error) {
	w := r.window
	version, err := r.subscription.owner.reader.ReadVersion(ctx, query.ConversationID)
	if err != nil {
		return nil, err
	}
	if version.Incarnation != w.version.Incarnation {
		return r.readCanonicalAtVersion(ctx, query, version)
	}
	retained, err := r.subscription.owner.survivingRetention(ctx, query.ConversationID, w, version)
	if err != nil {
		return nil, err
	}
	if w.mode == TimelineWindowFollowTail && len(retained) == 0 {
		return r.readCanonicalAtVersion(ctx, query, version)
	}
	membership, err := w.membership.ReadHistoricalMembership(ctx, query, w.metadata.BeforeCursor, w.metadata.AfterCursor)
	if err != nil {
		return nil, err
	}
	if membership.Version != version {
		return nil, ErrProjectionRecoveryRequired
	}
	ids := membershipIDs(membership.Selection)
	if !containsRetained(ids, retained) {
		return nil, ErrTimelineWindowStale
	}
	selected, err := r.subscription.owner.reader.ReadSelection(ctx, query.ConversationID, ids)
	if err != nil {
		return nil, err
	}
	if selected.Version != membership.Version {
		return nil, ErrProjectionRecoveryRequired
	}
	byID := map[string]*TimelineItem{}
	for _, item := range selected.Items {
		byID[item.ItemID] = item
	}
	result := &TimelineWindowSnapshot{Version: selected.Version, Turns: selected.Turns, Responses: map[string]*TimelineItem{}, BeforeCursor: w.metadata.BeforeCursor, AfterCursor: w.metadata.AfterCursor}
	for _, id := range ids {
		item := byID[id]
		if item == nil {
			return nil, ErrProjectionRecoveryRequired
		}
		result.Items = append(result.Items, item)
		if item.Kind == "response" {
			result.Responses[item.TurnID] = item
		}
	}
	result.TurnIDs = append([]string(nil), selected.TurnIDs...)
	rows := membership.Selection
	result.Window = MessageWindow{ConversationID: query.ConversationID, TotalCount: rows.TotalCount, StartIndex: rows.StartIndex, EndIndex: rows.EndIndex, HasBefore: rows.HasBefore, HasAfter: rows.HasAfter}
	return result, nil
}

func containsRetained(ids, retained []string) bool {
	for _, id := range retained {
		if !slices.Contains(ids, id) {
			return false
		}
	}
	return true
}

// Recusa uma prova que mudou entre a autorização de recuperação e o snapshot,
// antes de o projetor reconciliar lifecycle ou publicar qualquer conteúdo.
type incarnationWindowReader struct {
	reader      WindowSnapshotReader
	incarnation string
}

func (r incarnationWindowReader) ReadWindow(ctx context.Context, query database.MessageWindowQuery) (*TimelineWindowSnapshot, error) {
	snapshot, err := r.reader.ReadWindow(ctx, query)
	if err != nil {
		return nil, err
	}
	if snapshot.Version.Incarnation != r.incarnation {
		return nil, ErrProjectionRecoveryRequired
	}
	return snapshot, nil
}

func (r retainedWindowReader) readCanonicalAtVersion(ctx context.Context, query database.MessageWindowQuery, version database.ChatTimelineVersion) (*TimelineWindowSnapshot, error) {
	snapshot, err := r.subscription.reader.ReadCanonicalWindow(ctx, query)
	if err != nil {
		return nil, err
	}
	if snapshot.Version != version {
		return nil, ErrProjectionRecoveryRequired
	}
	return snapshot, nil
}
