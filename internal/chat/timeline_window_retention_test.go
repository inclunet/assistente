package chat

import (
	"context"
	"errors"
	"github.com/google/uuid"
	"reflect"
	"strconv"
	"testing"
	"time"

	"assistente/internal/database"
)

func retainWindow(t *testing.T, f snapshotFixture, sub *TimelineSubscription, ids ...string) {
	t.Helper()
	w := sub.conversation.windows[sub.windowID]
	if err := sub.SetRetention(f.ctx, TimelineRetentionRequest{Epoch: sub.conversation.epoch, WindowRevision: strconv.FormatUint(w.revision, 10), ItemIDs: ids}); err != nil {
		t.Fatal(err)
	}
}

func TestWindowRetentionBlocksCutAndReleasesAtomically(t *testing.T) {
	f, p := membershipFixture(t)
	sub, initial := membershipSubscribe(t, f, p, "reading", 2)
	initial = followTail(t, f, sub, initial, 2)
	other, otherInitial := membershipSubscribe(t, f, p, "other", 2)
	otherInitial = followTail(t, f, other, otherInitial, 2)
	retainWindow(t, f, sub, "message:first", "message:last")
	membershipWrite(t, f, p, "new", 30)
	w := sub.conversation.windows[sub.windowID]
	if !w.metadata.RetentionBlocked || !w.metadata.HasAfter || !reflect.DeepEqual(w.orderedIDs, initial.OrderedItemIDs) {
		t.Fatal("retenção não protegeu leitura")
	}
	if !reflect.DeepEqual(other.conversation.windows[other.windowID].orderedIDs, []string{"message:last", "message:new"}) {
		t.Fatal("retenção vazou para outra superfície")
	}
	// Liberação parcial permite somente os cortes que preservam a referência restante.
	retainWindow(t, f, sub, "message:last")
	w = sub.conversation.windows[sub.windowID]
	if w.metadata.RetentionBlocked || !reflect.DeepEqual(w.orderedIDs, []string{"message:last", "message:new"}) {
		t.Fatal("liberação parcial não reconciliou")
	}
	membershipWrite(t, f, p, "newest", 40)
	if !sub.conversation.windows[sub.windowID].metadata.RetentionBlocked {
		t.Fatal("referência restante foi cortada")
	}
	retainWindow(t, f, sub)
	delivered := append([]string(nil), initial.OrderedItemIDs...)
	retained := []string{}
	blocked := false
	for _, batch := range membershipReplay(t, f, sub, initial) {
		for _, patch := range batch.WindowPatches {
			delivered = applyModeTestSplices(delivered, patch.Splices)
			if patch.Metadata != nil {
				retained = patch.Metadata.RetainedItemIDs
				blocked = patch.Metadata.RetentionBlocked
			}
		}
	}
	if blocked || len(retained) != 0 || !reflect.DeepEqual(delivered, []string{"message:new", "message:newest"}) {
		t.Fatal("replay não confirmou liberação/corte", delivered, retained, blocked)
	}
	sub.Close()
	membershipWrite(t, f, p, "outside", 50)
	membershipReplay(t, f, other, otherInitial)
}

func TestWindowRetentionRefreshKeepsReadingAndRejectsNavigation(t *testing.T) {
	f, p := membershipFixture(t)
	sub, initial := membershipSubscribe(t, f, p, "reading", 2)
	initial = followTail(t, f, sub, initial, 2)
	retainWindow(t, f, sub, "message:first")
	membershipWrite(t, f, p, "new", 30)
	w := sub.conversation.windows[sub.windowID]
	fresh, err := sub.Refresh(f.ctx, strconv.FormatUint(w.revision, 10), nil)
	if err != nil || !fresh.RetentionBlocked || !reflect.DeepEqual(fresh.OrderedItemIDs, initial.OrderedItemIDs) || !reflect.DeepEqual(fresh.RetainedItemIDs, []string{"message:first"}) {
		t.Fatal("Refresh cortou leitura", err)
	}
	if _, err := sub.Navigate(f.ctx, navigationFrom(fresh)); !errors.Is(err, ErrTimelineRetention) {
		t.Fatal("Navigate cortou leitura", err)
	}
	if _, err := sub.Refresh(f.ctx, fresh.WindowRevision, &TimelineWindowPosition{Anchor: "end", Limit: 2}); !errors.Is(err, ErrTimelineRetention) {
		t.Fatal("Refresh explícito cortou leitura", err)
	}
	// A resposta é cópia: o consumidor não altera o compromisso do backend.
	fresh.RetainedItemIDs[0] = "mutado"
	if sub.conversation.windows[sub.windowID].metadata.RetainedItemIDs[0] != "message:first" {
		t.Fatal("snapshot compartilha retenção")
	}
}

func TestWindowRetentionDeletionWins(t *testing.T) {
	f, p := membershipFixture(t)
	sub, initial := membershipSubscribe(t, f, p, "reading", 2)
	initial = followTail(t, f, sub, initial, 2)
	retainWindow(t, f, sub, "message:first")
	membershipWrite(t, f, p, "new", 30)
	if err := p.ApplyStorageMutation(f.ctx, f.conversation, nil, func() ([]string, error) { return nil, database.DeleteMessageWithContext(f.ctx, "first") }); err != nil {
		t.Fatal(err)
	}
	w := sub.conversation.windows[sub.windowID]
	if len(w.metadata.RetainedItemIDs) != 0 || w.metadata.RetentionBlocked || w.itemIDs["message:first"] {
		t.Fatal("exclusão ressuscitada")
	}
	delivered := append([]string(nil), initial.OrderedItemIDs...)
	for _, batch := range membershipReplay(t, f, sub, initial) {
		for _, patch := range batch.WindowPatches {
			delivered = applyModeTestSplices(delivered, patch.Splices)
		}
	}
	if !reflect.DeepEqual(delivered, []string{"message:last", "message:new"}) {
		t.Fatal("replay reteve excluído", delivered)
	}
}
func TestWindowRetentionLostUnretainedBoundaryPreservesOtherReference(t *testing.T) {
	f, p := membershipFixture(t)
	sub, initial := membershipSubscribe(t, f, p, "reading", 2)
	followTail(t, f, sub, initial, 2)
	retainWindow(t, f, sub, "message:first")
	membershipWrite(t, f, p, "new", 30)
	membershipWrite(t, f, p, "newest", 40)
	if err := p.ApplyStorageMutation(f.ctx, f.conversation, nil, func() ([]string, error) { return nil, database.DeleteMessageWithContext(f.ctx, "last") }); err != nil {
		t.Fatal(err)
	}
	w := sub.conversation.windows[sub.windowID]
	if !w.syncRequired || !reflect.DeepEqual(w.metadata.RetainedItemIDs, []string{"message:first"}) {
		t.Fatal("fronteira perdida descartou retenção")
	}
	if _, err := sub.Refresh(f.ctx, strconv.FormatUint(w.revision, 10), nil); err == nil {
		t.Fatal("recuperação saltou fronteira perdida")
	}
	retainWindow(t, f, sub)
	w = sub.conversation.windows[sub.windowID]
	fresh, err := sub.Refresh(f.ctx, strconv.FormatUint(w.revision, 10), nil)
	if err != nil || len(fresh.RetainedItemIDs) != 0 || !reflect.DeepEqual(fresh.OrderedItemIDs, []string{"message:new", "message:newest"}) {
		t.Fatal("liberação não permitiu recuperação", err)
	}
}

func TestWindowRetentionFailedReleaseRollsBackAndRejectsStaleRequests(t *testing.T) {
	f, p := membershipFixture(t)
	sub, initial := membershipSubscribe(t, f, p, "reading", 2)
	initial = followTail(t, f, sub, initial, 2)
	if err := sub.SetRetention(f.ctx, TimelineRetentionRequest{Epoch: initial.Epoch, WindowRevision: initial.WindowRevision, ItemIDs: []string{"message:outside"}}); !errors.Is(err, ErrTimelineRetention) {
		t.Fatal("ID fora da janela aceito", err)
	}
	retainWindow(t, f, sub, "message:first")
	membershipWrite(t, f, p, "new", 30)
	beforeWindows, beforePublication := cloneSubscribedWindows(sub.conversation.windows), sub.conversation.publication.fork()
	probe := &membershipProbe{DBTimelineReader: f.reader, drift: true}
	p.reader = probe
	w := sub.conversation.windows[sub.windowID]
	err := sub.SetRetention(f.ctx, TimelineRetentionRequest{Epoch: sub.conversation.epoch, WindowRevision: strconv.FormatUint(w.revision, 10)})
	if !errors.Is(err, ErrProjectionRecoveryRequired) || !reflect.DeepEqual(beforeWindows, sub.conversation.windows) || !reflect.DeepEqual(beforePublication, sub.conversation.publication) {
		t.Fatal("liberação parcial escapou", err)
	}
	p.reader = f.reader
	retainWindow(t, f, sub)
	if err := sub.SetRetention(f.ctx, TimelineRetentionRequest{Epoch: initial.Epoch, WindowRevision: initial.WindowRevision, ItemIDs: initial.OrderedItemIDs}); !errors.Is(err, ErrTimelineWindowStale) {
		t.Fatal("pedido atrasado restaurou retenção", err)
	}
}

func TestWindowRetentionMovedItemCannotLeaveHistoricalInterval(t *testing.T) {
	for _, automatic := range []bool{false, true} {
		t.Run(strconv.FormatBool(automatic), func(t *testing.T) {
			f, p := membershipFixture(t)
			historicalMessage(t, f, "middle", 15, nil)
			sub, initial := membershipSubscribe(t, f, p, "reading", 3)
			retainWindow(t, f, sub, "message:middle")
			move := func() ([]string, error) {
				return []string{"message:middle"}, f.db.Model(&database.ChatMessage{}).Where("id=?", "middle").Update("created_at", time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)).Error
			}
			if automatic {
				if err := p.ApplyStorageMutation(f.ctx, f.conversation, []string{"message:middle"}, move); err != nil {
					t.Fatal(err)
				}
			} else {
				if _, err := move(); err != nil {
					t.Fatal(err)
				}
			}
			w := sub.conversation.windows[sub.windowID]
			if automatic && !w.syncRequired {
				t.Fatal("movimento cortou item retido")
			}
			if _, err := sub.Refresh(f.ctx, strconv.FormatUint(w.revision, 10), nil); !errors.Is(err, ErrTimelineWindowStale) {
				t.Fatal("Refresh descartou item movido", err)
			}
			if !reflect.DeepEqual(w.orderedIDs, initial.OrderedItemIDs) || !reflect.DeepEqual(w.metadata.RetainedItemIDs, []string{"message:middle"}) {
				t.Fatal("retenção perdida")
			}
			retainWindow(t, f, sub)
		})
	}
}

func TestWindowRetentionQuestionOnlyRefreshReconcilesExternalRetry(t *testing.T) {
	f, p, e, _ := automaticFixture(t)
	sub, _, err := p.SubscribeTimeline(f.ctx, &fixtureTimelineSession{ctx: f.ctx, id: "s"}, "question", database.MessageWindowQuery{ConversationID: f.conversation, Anchor: "start", Direction: "after", Limit: 1}, f.reader)
	if err != nil {
		t.Fatal(err)
	}
	defer sub.Close()
	retainWindow(t, f, sub, "message:"+f.question)
	repo := database.NewChatTurnRepository(f.db)
	if err := repo.Finish(f.ctx, f.conversation, f.question, e.executionID, database.TurnInterrupted, "cancelled"); err != nil {
		t.Fatal(err)
	}
	next, err := uuid.NewV7()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.BeginExecution(f.ctx, f.conversation, f.question, "", database.TurnModeRetry, next.String(), ""); err != nil {
		t.Fatal(err)
	}
	w := sub.conversation.windows[sub.windowID]
	fresh, err := sub.Refresh(f.ctx, strconv.FormatUint(w.revision, 10), nil)
	if err != nil || !reflect.DeepEqual(fresh.OrderedItemIDs, []string{"message:" + f.question}) {
		t.Fatal("snapshot inválido", err)
	}
	if !e.conversation.turns[e.turnID].detached {
		t.Fatal("pergunta isolada manteve execução substituída")
	}
}

func TestWindowRetentionCapacityDoesNotCutReading(t *testing.T) {
	f, p := membershipFixture(t)
	sub, initial := membershipSubscribe(t, f, p, "reading", 2)
	initial = followTail(t, f, sub, initial, 2)
	retainWindow(t, f, sub, "message:first")
	membershipWrite(t, f, p, "outside", 30)
	membershipWrite(t, f, p, "middle", 15)
	w := sub.conversation.windows[sub.windowID]
	if !w.syncRequired || !reflect.DeepEqual(w.orderedIDs, initial.OrderedItemIDs) || len(w.metadata.RetainedItemIDs) != 1 {
		t.Fatal("cota cortou leitura")
	}
	retainWindow(t, f, sub)
}

func TestWindowRetentionPublicationFailurePreservesFocusUntilExplicitRelease(t *testing.T) {
	for _, releaseFirst := range []bool{false, true} {
		t.Run(strconv.FormatBool(releaseFirst), func(t *testing.T) {
			f, p := membershipFixture(t)
			sub, initial := membershipSubscribe(t, f, p, "reading", 2)
			initial = followTail(t, f, sub, initial, 2)
			retainWindow(t, f, sub, "message:first")
			p.reader = &membershipProbe{DBTimelineReader: f.reader, drift: true}
			membershipWrite(t, f, p, "new", 30)
			c := sub.conversation
			if c.epoch == initial.Epoch || c.publicationErrorLocked() == nil {
				t.Fatal("falha não invalidou publicação")
			}
			p.reader = f.reader
			w := c.windows[sub.windowID]
			if releaseFirst {
				err := sub.SetRetention(f.ctx, TimelineRetentionRequest{Epoch: w.epoch, WindowRevision: strconv.FormatUint(w.revision, 10)})
				if err != nil {
					t.Fatal("liberação bloqueada pela época de recuperação", err)
				}
			}
			w = c.windows[sub.windowID]
			fresh, err := sub.Refresh(f.ctx, strconv.FormatUint(w.revision, 10), nil)
			if err != nil {
				t.Fatal(err)
			}
			if releaseFirst {
				if len(fresh.RetainedItemIDs) != 0 || !reflect.DeepEqual(fresh.OrderedItemIDs, []string{"message:last", "message:new"}) {
					t.Fatal("liberação não recuperou cauda")
				}
			} else if !reflect.DeepEqual(fresh.RetainedItemIDs, []string{"message:first"}) || !reflect.DeepEqual(fresh.OrderedItemIDs, initial.OrderedItemIDs) {
				t.Fatal("época de falha apagou foco")
			}
		})
	}
}

func TestWindowRetentionNewIncarnationDoesNotCarryReferences(t *testing.T) {
	f, p := membershipFixture(t)
	sub, initial := membershipSubscribe(t, f, p, "reading", 2)
	initial = followTail(t, f, sub, initial, 2)
	retainWindow(t, f, sub, "message:first")
	if err := f.db.Exec("UPDATE chat_timeline_versions SET incarnation=lower(hex(randomblob(16))) WHERE conversation_id=?", f.conversation).Error; err != nil {
		t.Fatal(err)
	}
	w := sub.conversation.windows[sub.windowID]
	fresh, err := sub.Refresh(f.ctx, strconv.FormatUint(w.revision, 10), nil)
	if err != nil || fresh.Epoch == initial.Epoch || len(fresh.RetainedItemIDs) != 0 {
		t.Fatal("referências atravessaram encarnação", err)
	}
}

func TestWindowRetentionNewIncarnationCanReplaceLostCursorExplicitly(t *testing.T) {
	f, p := membershipFixture(t)
	sub, initial := membershipSubscribe(t, f, p, "reading", 2)
	request := navigationFrom(initial)
	request.Anchor, request.Cursor, request.Direction = "", initial.BeforeCursor, "after"
	fresh, err := sub.Navigate(f.ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	retainWindow(t, f, sub, "message:last")
	if err := database.DeleteMessageWithContext(f.ctx, "first"); err != nil {
		t.Fatal(err)
	}
	if err := f.db.Exec("UPDATE chat_timeline_versions SET incarnation=lower(hex(randomblob(16))) WHERE conversation_id=?", f.conversation).Error; err != nil {
		t.Fatal(err)
	}
	w := sub.conversation.windows[sub.windowID]
	revision := strconv.FormatUint(w.revision, 10)
	if _, err := sub.Refresh(f.ctx, revision, nil); err == nil {
		t.Fatal("cursor removido aceito")
	}
	oldIncarnation := w.version.Incarnation
	beforeWindows, beforePublication, beforeEpoch := cloneSubscribedWindows(sub.conversation.windows), sub.conversation.publication.fork(), sub.conversation.epoch
	sub.reader = navigationReaderFunc(func(ctx context.Context, query database.MessageWindowQuery) (*TimelineWindowSnapshot, error) {
		snapshot, err := f.reader.ReadCanonicalWindow(ctx, query)
		if err == nil {
			snapshot.Version.Incarnation = oldIncarnation
		}
		return snapshot, err
	})
	if _, err := sub.Refresh(f.ctx, revision, &TimelineWindowPosition{Anchor: "end", Direction: "before", Limit: 1}); !errors.Is(err, ErrProjectionRecoveryRequired) {
		t.Fatal("prova de encarnação divergente aceita", err)
	}
	if !reflect.DeepEqual(beforeWindows, sub.conversation.windows) || !reflect.DeepEqual(beforePublication, sub.conversation.publication) || beforeEpoch != sub.conversation.epoch {
		t.Fatal("snapshot divergente alterou publicação")
	}
	if len(sub.conversation.windows[sub.windowID].metadata.RetainedItemIDs) == 0 {
		t.Fatal("drift descartou referências")
	}
	sub.reader = f.reader
	recovered, err := sub.Refresh(f.ctx, revision, &TimelineWindowPosition{Anchor: "end", Direction: "before", Limit: 1})
	if err != nil || recovered.Epoch == fresh.Epoch || len(recovered.RetainedItemIDs) != 0 || !reflect.DeepEqual(recovered.OrderedItemIDs, []string{"message:last"}) {
		t.Fatal("encarnação nova bloqueou recuperação explícita", err)
	}
}

func TestWindowRetentionExternalDeletionReleasesTailButNotSurvivingReferences(t *testing.T) {
	for _, keepOther := range []bool{false, true} {
		t.Run(strconv.FormatBool(keepOther), func(t *testing.T) {
			f, p := membershipFixture(t)
			sub, initial := membershipSubscribe(t, f, p, "reading", 2)
			followTail(t, f, sub, initial, 2)
			ids := []string{"message:first"}
			if keepOther {
				ids = append(ids, "message:last")
			}
			retainWindow(t, f, sub, ids...)
			membershipWrite(t, f, p, "new", 30)
			if err := database.DeleteMessageWithContext(f.ctx, "first"); err != nil {
				t.Fatal(err)
			}
			w := sub.conversation.windows[sub.windowID]
			fresh, err := sub.Refresh(f.ctx, strconv.FormatUint(w.revision, 10), nil)
			if keepOther {
				if err == nil || !reflect.DeepEqual(w.metadata.RetainedItemIDs, ids) {
					t.Fatal("fronteira perdida soltou outro item", err)
				}
			} else {
				if err != nil || fresh.Mode != TimelineWindowFollowTail || fresh.RetentionBlocked || len(fresh.RetainedItemIDs) != 0 || !reflect.DeepEqual(fresh.OrderedItemIDs, []string{"message:last", "message:new"}) {
					t.Fatal("referência excluída bloqueou cauda", err)
				}
				if _, err := sub.Navigate(f.ctx, navigationFrom(fresh)); err != nil {
					t.Fatal("exclusão manteve navegação bloqueada", err)
				}
			}
		})
	}
}

func TestWindowRetentionReleaseRejectsOldStreamAfterExternalRetry(t *testing.T) {
	f, p, e, _ := automaticFixture(t)
	stream := e.StartStream(0)
	stream.Append("anterior")
	sub, _, err := p.SubscribeTimeline(f.ctx, &fixtureTimelineSession{ctx: f.ctx, id: "s"}, "question", database.MessageWindowQuery{ConversationID: f.conversation, Anchor: "start", Direction: "after", Limit: 1}, f.reader)
	if err != nil {
		t.Fatal(err)
	}
	defer sub.Close()
	retainWindow(t, f, sub, "message:"+f.question)
	repo := database.NewChatTurnRepository(f.db)
	if err := repo.Finish(f.ctx, f.conversation, f.question, e.executionID, database.TurnInterrupted, "cancelled"); err != nil {
		t.Fatal(err)
	}
	next, err := uuid.NewV7()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.BeginExecution(f.ctx, f.conversation, f.question, "", database.TurnModeRetry, next.String(), ""); err != nil {
		t.Fatal(err)
	}
	retainWindow(t, f, sub)
	if stream.Append("tardio") || !e.conversation.turns[e.turnID].detached {
		t.Fatal("liberação aceitou stream da tentativa substituída")
	}
}
