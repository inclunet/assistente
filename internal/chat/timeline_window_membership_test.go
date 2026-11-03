package chat

import (
	"context"
	"errors"
	"gorm.io/gorm"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"testing"

	"assistente/internal/database"
)

func membershipSyncRevision(t *testing.T, sub *TimelineSubscription, snapshot *SubscribedTimelineSnapshot, f snapshotFixture) string {
	t.Helper()
	after, _ := strconv.ParseUint(snapshot.Sequence, 10, 64)
	batches, ok, err := sub.Replay(f.ctx, snapshot.Epoch, snapshot.WindowRevision, after, f.reader)
	if err != nil || !ok {
		t.Fatalf("sync replay: %v %v", ok, err)
	}
	for _, batch := range batches {
		for _, patch := range batch.WindowPatches {
			if patch.SyncRequired {
				return patch.Revision
			}
		}
	}
	t.Fatal("janela não recebeu syncRequired")
	return ""
}

func membershipFixture(t *testing.T) (snapshotFixture, *LiveProjection) {
	t.Helper()
	f := newSnapshotFixture(t)
	if err := database.DeleteMessageWithContext(f.ctx, f.question); err != nil {
		t.Fatal(err)
	}
	historicalMessage(t, f, "first", 10, nil)
	historicalMessage(t, f, "last", 20, nil)
	return f, NewLiveProjection(f.reader)
}

func membershipSubscribe(t *testing.T, f snapshotFixture, p *LiveProjection, surface string, limit int) (*TimelineSubscription, *SubscribedTimelineSnapshot) {
	t.Helper()
	s, v, err := p.SubscribeTimeline(f.ctx, &fixtureTimelineSession{ctx: f.ctx, id: "session"}, surface, database.MessageWindowQuery{ConversationID: f.conversation, Anchor: "end", Direction: "before", Limit: limit}, f.reader)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Close)
	return s, v
}

func membershipWrite(t *testing.T, f snapshotFixture, p *LiveProjection, id string, day int) {
	t.Helper()
	if err := p.ApplyStorageMutation(f.ctx, f.conversation, []string{}, func() ([]string, error) { historicalMessage(t, f, id, day, nil); return []string{"message:" + id}, nil }); err != nil {
		t.Fatal(err)
	}
}

func membershipReplay(t *testing.T, f snapshotFixture, sub *TimelineSubscription, s *SubscribedTimelineSnapshot) []CanonicalBatch {
	t.Helper()
	after, _ := strconv.ParseUint(s.Sequence, 10, 64)
	batches, ok, err := sub.Replay(f.ctx, s.Epoch, s.WindowRevision, after, f.reader)
	if err != nil || !ok {
		t.Fatalf("replay: %v %v", ok, err)
	}
	return batches
}

func TestWindowMembershipPublishesHistoricalSplicesAndRoutesPastOperations(t *testing.T) {
	f, p := membershipFixture(t)
	sub, initial := membershipSubscribe(t, f, p, "history", 10)
	other, otherInitial := membershipSubscribe(t, f, p, "last-only", 1)
	membershipWrite(t, f, p, "older", 1)
	membershipWrite(t, f, p, "newer", 30)
	membershipWrite(t, f, p, "middle", 15)
	if err := p.ApplyStorageMutation(f.ctx, f.conversation, []string{"message:middle"}, func() ([]string, error) {
		return nil, database.UpdateMessageTextWithContext(f.ctx, "middle", "texto atualizado")
	}); err != nil {
		t.Fatal(err)
	}
	if err := p.ApplyStorageMutation(f.ctx, f.conversation, nil, func() ([]string, error) { return nil, database.DeleteMessageWithContext(f.ctx, "middle") }); err != nil {
		t.Fatal(err)
	}
	batches := membershipReplay(t, f, sub, initial)
	ids := append([]string{}, initial.OrderedItemIDs...)
	revision := initial.WindowRevision
	inserted, removed, updated := false, false, false
	for _, batch := range batches {
		for _, patch := range batch.WindowPatches {
			if patch.WindowID != sub.windowID || patch.PreviousRevision != revision || patch.SyncRequired || len(patch.OperationIndexes) != 0 {
				t.Fatalf("patch inválido: %+v", patch)
			}
			revision = patch.Revision
			for _, splice := range patch.Splices {
				tail := append([]string{}, ids[splice.Index+splice.DeleteCount:]...)
				ids = append(append(ids[:splice.Index:splice.Index], splice.InsertIDs...), tail...)
				for _, id := range splice.InsertIDs {
					if id == "message:middle" {
						inserted = true
					}
				}
				if splice.DeleteCount > 0 {
					removed = true
				}
			}
			for _, item := range patch.Items {
				if item.ItemID != "message:middle" || canonicalText(item) == "" {
					t.Fatal("materialização de entrada inválida")
				}
			}
		}
		for _, op := range batch.Operations {
			if op.ItemID == "message:middle" {
				updated = true
			}
		}
	}
	if !inserted || !removed || !updated || !reflect.DeepEqual(ids, initial.OrderedItemIDs) {
		t.Fatalf("replay filtrou pela participação final: %v %v %v %v", inserted, removed, updated, ids)
	}
	w := sub.conversation.windows[sub.windowID]
	if w.metadata.TotalCount != 4 || w.metadata.StartIndex != 1 || w.metadata.EndIndex != 2 || !w.metadata.HasBefore || !w.metadata.HasAfter {
		t.Fatalf("metadados: %+v", w.metadata)
	}
	for _, batch := range membershipReplay(t, f, other, otherInitial) {
		if len(batch.Operations) > 0 {
			t.Fatal("conteúdo de outra janela vazou")
		}
		for _, patch := range batch.WindowPatches {
			if len(patch.Splices) > 0 || len(patch.Items) > 0 {
				t.Fatal("histórico seguiu cauda")
			}
		}
	}
	sub.Close()
	membershipWrite(t, f, p, "outside", 31)
	if _, err := other.Refresh(f.ctx, strconv.FormatUint(other.conversation.windows[other.windowID].revision, 10), nil); err != nil {
		t.Fatal(err)
	}
}

func TestWindowMembershipSnapshotBaselineNeverReplaysOldAppends(t *testing.T) {
	f, p, e, _ := automaticFixture(t)
	stream := e.StartStream(0)
	stream.Append("primeiro")
	e.conversation.mu.Lock()
	e.conversation.flushPublicationLocked(f.conversation)
	e.conversation.mu.Unlock()
	sub, initial := membershipSubscribe(t, f, p, "live", 10)
	batches, ok, err := sub.Replay(f.ctx, initial.Epoch, initial.WindowRevision, 0, f.reader)
	if err != nil || !ok {
		t.Fatal(err)
	}
	for _, batch := range batches {
		if len(batch.Operations) > 0 || len(batch.WindowPatches) > 0 {
			t.Fatal("snapshot repetiu conteúdo anterior")
		}
	}
	stream.Append(" depois")
	e.conversation.mu.Lock()
	e.conversation.flushPublicationLocked(f.conversation)
	e.conversation.mu.Unlock()
	batches = membershipReplay(t, f, sub, initial)
	text := ""
	for _, batch := range batches {
		for _, op := range batch.Operations {
			if op.Kind == "segment_append" {
				text += op.Text
			}
		}
	}
	if text != " depois" {
		t.Fatalf("append incorreto: %q", text)
	}
	fresh, err := sub.Navigate(f.ctx, navigationFrom(initial))
	if err != nil {
		t.Fatal(err)
	}
	if _, ok, err := sub.Replay(f.ctx, initial.Epoch, initial.WindowRevision, 0, f.reader); ok || !errors.Is(err, ErrTimelineWindowStale) {
		t.Fatal("revisão anterior aceita", err)
	}
	batches, ok, err = sub.Replay(f.ctx, fresh.Epoch, fresh.WindowRevision, 0, f.reader)
	if err != nil || !ok {
		t.Fatal(err)
	}
	for _, batch := range batches {
		if len(batch.Operations) > 0 {
			t.Fatal("Navigate repetiu append")
		}
	}
}

func TestWindowMembershipLostBoundaryIsLocalAndRefreshRestores(t *testing.T) {
	f, p := membershipFixture(t)
	sub, initial := membershipSubscribe(t, f, p, "wide", 10)
	other, otherInitial := membershipSubscribe(t, f, p, "last", 1)
	if err := p.ApplyStorageMutation(f.ctx, f.conversation, nil, func() ([]string, error) { return nil, database.DeleteMessageWithContext(f.ctx, "first") }); err != nil {
		t.Fatal(err)
	}
	revision := membershipSyncRevision(t, sub, initial, f)
	if !reflect.DeepEqual(sub.conversation.windows[sub.windowID].orderedIDs, initial.OrderedItemIDs) {
		t.Fatal("sync apagou visual")
	}
	membershipWrite(t, f, p, "outside", 30)
	for _, batch := range membershipReplay(t, f, other, otherInitial) {
		for _, patch := range batch.WindowPatches {
			if patch.SyncRequired {
				t.Fatal("outra janela invalidada")
			}
		}
	}
	fresh, err := sub.Refresh(f.ctx, revision, &TimelineWindowPosition{Anchor: "end", Direction: "before", Limit: 10})
	if err != nil || fresh.WindowRevision == revision || sub.conversation.windows[sub.windowID].syncRequired || len(fresh.Items) != 2 {
		t.Fatal("Refresh não recuperou", err)
	}
}

type membershipProbe struct {
	*DBTimelineReader
	fail  bool
	calls int
	drift bool
}

func (r *membershipProbe) ReadHistoricalMembership(ctx context.Context, q database.MessageWindowQuery, first, last string) (*TimelineMembershipSnapshot, error) {
	r.calls++
	if r.fail {
		return nil, errors.New("falha injetada")
	}
	return r.DBTimelineReader.ReadHistoricalMembership(ctx, q, first, last)
}
func (r *membershipProbe) ReadSelection(ctx context.Context, conversationID string, ids []string) (*TimelineSelectionSnapshot, error) {
	result, err := r.DBTimelineReader.ReadSelection(ctx, conversationID, ids)
	if err == nil && r.drift && len(ids) > 0 {
		result.Version.Revision++
	}
	return result, err
}

func TestWindowMembershipFailureRollsBackEveryWindowAndJournal(t *testing.T) {
	for _, drift := range []bool{false, true} {
		t.Run(strconv.FormatBool(drift), func(t *testing.T) {
			f, p := membershipFixture(t)
			a, initial := membershipSubscribe(t, f, p, "one", 10)
			b, _ := membershipSubscribe(t, f, p, "two", 10)
			c := a.conversation
			ids := []string{a.windowID, b.windowID}
			sort.Strings(ids)
			probe := &membershipProbe{DBTimelineReader: f.reader, fail: !drift, drift: drift}
			if drift {
				p.reader = probe
			} else {
				c.windows[ids[1]].membership = probe
			}
			beforeWindows, beforePublication := cloneSubscribedWindows(c.windows), c.publication.fork()
			membershipWrite(t, f, p, "middle", 15)
			if c.publicationErrorLocked() == nil || !reflect.DeepEqual(beforePublication, c.publication) || !reflect.DeepEqual(beforeWindows, c.windows) {
				t.Fatal("publicação parcial escapou")
			}
			if _, err := database.GetMessageContentWithContext(f.ctx, "middle"); err != nil {
				t.Fatal("falha auxiliar desfez escrita", err)
			}
			probe.fail, probe.drift = false, false
			fresh, err := a.Refresh(f.ctx, initial.WindowRevision, nil)
			if err != nil || len(fresh.Items) != 3 || c.publicationErrorLocked() != nil {
				t.Fatal("recuperação falhou", err)
			}
		})
	}
}

func TestWindowMembershipTokensNeverReadSelection(t *testing.T) {
	f, p, e, r := automaticFixture(t)
	probe := &membershipProbe{DBTimelineReader: f.reader}
	sub, _, err := p.SubscribeTimeline(f.ctx, &fixtureTimelineSession{ctx: f.ctx, id: "s"}, "live", database.MessageWindowQuery{ConversationID: f.conversation, Limit: 10}, probe)
	if err != nil {
		t.Fatal(err)
	}
	defer sub.Close()
	reads, calls := r.reads.Load(), probe.calls
	stream := e.StartStream(0)
	for range 1000 {
		stream.Append("x")
	}
	e.conversation.mu.Lock()
	e.conversation.flushPublicationLocked(f.conversation)
	e.conversation.mu.Unlock()
	if r.reads.Load() != reads || probe.calls != calls {
		t.Fatal("token consultou banco")
	}
}

func TestWindowMembershipEnteringCachedItemPreservesItsRevision(t *testing.T) {
	f, p := membershipFixture(t)
	sub, initial := membershipSubscribe(t, f, p, "history", 10)
	// Outra leitura materializa o item antes de a inscrição histórica reconciliar.
	historicalMessage(t, f, "middle", 15, nil)
	other, err := p.ReadCanonicalWindow(f.ctx, database.MessageWindowQuery{ConversationID: f.conversation, AnchorItemID: "message:middle", Direction: "around", Limit: 1}, f.reader)
	if err != nil || len(other.Items) != 1 {
		t.Fatal(err)
	}
	oldRevision := sub.conversation.publication.items["message:middle"].Revision
	if err := p.ApplyStorageMutation(f.ctx, f.conversation, []string{}, func() ([]string, error) { return nil, nil }); err != nil {
		t.Fatal(err)
	}
	entered := false
	for _, batch := range membershipReplay(t, f, sub, initial) {
		for _, patch := range batch.WindowPatches {
			for _, item := range patch.Items {
				if item.ItemID == "message:middle" {
					entered = true
					if item.Revision != oldRevision || canonicalText(item) == "" {
						t.Fatal("entrada mudou revisão ou omitiu conteúdo")
					}
				}
			}
		}
	}
	if !entered || sub.conversation.publication.items["message:middle"].Revision != oldRevision {
		t.Fatal("item cacheado não materializado corretamente")
	}
}

func TestWindowMembershipOverflowAndDeletedThreadRequireLocalSync(t *testing.T) {
	for _, thread := range []bool{false, true} {
		t.Run(strconv.FormatBool(thread), func(t *testing.T) {
			f := newSnapshotFixture(t)
			var parent *string
			if thread {
				parent = &f.question
			}
			historicalMessage(t, f, "first", 10, parent)
			historicalMessage(t, f, "last", 20, parent)
			p := NewLiveProjection(f.reader)
			sub, initial, err := p.SubscribeTimeline(f.ctx, &fixtureTimelineSession{ctx: f.ctx, id: "s"}, "history", database.MessageWindowQuery{ConversationID: f.conversation, ParentID: parent, Anchor: "start", Direction: "after", Limit: 2}, f.reader)
			if err != nil {
				t.Fatal(err)
			}
			defer sub.Close()
			if thread {
				err = p.ApplyStorageMutation(f.ctx, f.conversation, nil, func() ([]string, error) { return nil, database.DeleteMessageWithContext(f.ctx, f.question) })
			} else {
				membershipWrite(t, f, p, "middle", 15)
			}
			if err != nil {
				t.Fatal(err)
			}
			membershipSyncRevision(t, sub, initial, f)
			if sub.conversation.publicationErrorLocked() != nil {
				t.Fatal("falha local bloqueou conversa")
			}
		})
	}
}

func TestWindowMembershipRevokedSessionDoesNotReceivePublication(t *testing.T) {
	f, p := membershipFixture(t)
	session := &fixtureTimelineSession{ctx: f.ctx, id: "s"}
	sub, initial, err := p.SubscribeTimeline(f.ctx, session, "history", database.MessageWindowQuery{ConversationID: f.conversation, Limit: 10}, f.reader)
	if err != nil {
		t.Fatal(err)
	}
	defer sub.Close()
	session.denied = true
	membershipWrite(t, f, p, "middle", 15)
	if sub.conversation.windows[sub.windowID] != nil {
		t.Fatal("inscrição revogada permaneceu ativa")
	}
	if _, ok, err := sub.Replay(f.ctx, initial.Epoch, initial.WindowRevision, 0, f.reader); ok || err == nil {
		t.Fatal("revogação ignorada")
	}
	for _, batch := range sub.conversation.publication.batches {
		for _, patch := range batch.WindowPatches {
			if patch.WindowID == sub.windowID {
				t.Fatal("dados roteados após revogação")
			}
		}
	}
}

func TestWindowMembershipSnapshotCoversEvictedEarlierJournal(t *testing.T) {
	f, p := membershipFixture(t)
	sub, initial := membershipSubscribe(t, f, p, "history", 10)
	sub.conversation.publication.batches = nil
	sub.conversation.publication.sizes = nil
	sub.conversation.publication.bytes = 0
	batches, ok, err := sub.Replay(f.ctx, initial.Epoch, initial.WindowRevision, 0, f.reader)
	if err != nil || !ok || len(batches) != 0 {
		t.Fatal("lacuna anterior ao snapshot exigiu resync", err)
	}
	membershipWrite(t, f, p, "middle", 15)
	sub.conversation.publication.batches = nil
	sub.conversation.publication.sizes = nil
	sub.conversation.publication.bytes = 0
	if _, ok, err := sub.Replay(f.ctx, initial.Epoch, initial.WindowRevision, 0, f.reader); err != nil || ok {
		t.Fatal("lacuna posterior aceita", err)
	}
}

func TestWindowMembershipOneTransactionProducesOneAtomicBatch(t *testing.T) {
	f, p := membershipFixture(t)
	sub, initial := membershipSubscribe(t, f, p, "history", 10)
	err := p.ApplyStorageMutation(f.ctx, f.conversation, []string{"message:last"}, func() ([]string, error) {
		err := f.db.Transaction(func(tx *gorm.DB) error {
			if err := tx.Exec("UPDATE chat_messages SET content=? WHERE id=?", "editada", "last").Error; err != nil {
				return err
			}
			var model database.ChatMessage
			if err := tx.First(&model, "id=?", "last").Error; err != nil {
				return err
			}
			model.ID = "middle"
			model.CreatedAt = model.CreatedAt.AddDate(0, 0, -5)
			model.Content = "inserida"
			return tx.Create(&model).Error
		})
		return []string{"message:middle"}, err
	})
	if err != nil {
		t.Fatal(err)
	}
	batches := membershipReplay(t, f, sub, initial)
	if len(batches) != 1 {
		t.Fatalf("escrita dividida em %d lotes", len(batches))
	}
	edit, insert := false, false
	for _, op := range batches[0].Operations {
		if op.ItemID == "message:last" {
			edit = true
		}
	}
	for _, patch := range batches[0].WindowPatches {
		for _, item := range patch.Items {
			if item.ItemID == "message:middle" && canonicalText(item) == "inserida" {
				insert = true
			}
		}
	}
	if !edit || !insert {
		t.Fatal("estado intermediário exposto")
	}
}

func TestWindowMembershipBudgetIncludesOldEvictedAndGrowingItems(t *testing.T) {
	for _, grow := range []bool{false, true} {
		for _, evict := range []bool{false, true} {
			t.Run(strconv.FormatBool(grow)+"/"+strconv.FormatBool(evict), func(t *testing.T) {
				f, p := membershipFixture(t)
				if err := database.UpdateMessageTextWithContext(f.ctx, "first", strings.Repeat("a", 6*1024*1024)); err != nil {
					t.Fatal(err)
				}
				sub, initial := membershipSubscribe(t, f, p, "history", 10)
				if evict {
					sub.conversation.publication.removeItem("message:first")
				}
				before := cloneSubscribedWindows(sub.conversation.windows)
				target := "middle"
				if grow {
					target = "last"
				}
				err := p.ApplyStorageMutation(f.ctx, f.conversation, []string{"message:" + target}, func() ([]string, error) {
					if !grow {
						historicalMessage(t, f, target, 15, nil)
					}
					return []string{"message:" + target}, database.UpdateMessageTextWithContext(f.ctx, target, strings.Repeat("b", 3*1024*1024))
				})
				if err != nil {
					t.Fatal("writer foi desfeito", err)
				}
				for _, id := range []string{"first", target} {
					if _, err := f.reader.ReadSelection(f.ctx, f.conversation, []string{"message:" + id}); err != nil {
						t.Fatal("parcela individual não cabe", err)
					}
				}
				if !errors.Is(sub.conversation.publicationErrorLocked(), ErrTimelineSourceLimit) || !reflect.DeepEqual(before, sub.conversation.windows) {
					t.Fatal("janela acima de 8MiB avançou")
				}
				if _, ok, err := sub.Replay(f.ctx, initial.Epoch, initial.WindowRevision, 0, f.reader); ok || !errors.Is(err, ErrTimelineSourceLimit) {
					t.Fatal("replay parcial", err)
				}
			})
		}
	}
}

func TestWindowMembershipStageOverflowRequiresRecoverableSnapshot(t *testing.T) {
	f, p := membershipFixture(t)
	sub, initial := membershipSubscribe(t, f, p, "history", 10)
	err := p.ApplyStorageMutation(f.ctx, f.conversation, []string{}, func() ([]string, error) {
		historicalMessage(t, f, "middle", 15, nil)
		return []string{"message:middle"}, database.UpdateMessageTextWithContext(f.ctx, "middle", strings.Repeat("x", 3*1024*1024))
	})
	if err != nil || sub.conversation.publicationErrorLocked() != nil {
		t.Fatal(err)
	}
	if _, ok, err := sub.Replay(f.ctx, initial.Epoch, initial.WindowRevision, 0, f.reader); err != nil || ok {
		t.Fatal("overflow devolveu lote incompleto", err)
	}
	fresh, err := sub.Refresh(f.ctx, initial.WindowRevision, nil)
	if err != nil || len(fresh.Items) != 3 {
		t.Fatal("revisão perdida impediu recuperação", err)
	}
	if _, err := sub.Refresh(f.ctx, initial.WindowRevision, nil); !errors.Is(err, ErrTimelineWindowStale) {
		t.Fatal("recuperação anterior atravessou novo snapshot", err)
	}
}

func TestWindowMembershipReconcilesExternalContentBeforeAdvancingVersion(t *testing.T) {
	f, p := membershipFixture(t)
	sub, initial := membershipSubscribe(t, f, p, "history", 10)
	if err := database.UpdateMessageTextWithContext(f.ctx, "first", "mudança externa"); err != nil {
		t.Fatal(err)
	}
	membershipWrite(t, f, p, "middle", 15)
	changed := false
	for _, batch := range membershipReplay(t, f, sub, initial) {
		for _, op := range batch.Operations {
			if op.ItemID == "message:first" {
				changed = true
			}
		}
	}
	if !changed || canonicalText(sub.conversation.publication.items["message:first"]) != "mudança externa" {
		t.Fatal("cache obsoleto aceito como versão atual")
	}
}
