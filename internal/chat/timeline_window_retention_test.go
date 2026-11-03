package chat

import (
	"context"
	"errors"
	"github.com/google/uuid"
	"reflect"
	"strconv"
	"strings"
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

func TestWindowRetentionTailCutReconcilesDepartingExecution(t *testing.T) {
	f, p, e, _ := automaticFixture(t)
	stream := e.StartStream(0)
	stream.Append("anterior")
	sub, initial, err := p.SubscribeTimeline(f.ctx, &fixtureTimelineSession{ctx: f.ctx, id: "s"}, "tail", database.MessageWindowQuery{ConversationID: f.conversation, Anchor: "end", Direction: "before", Limit: 2}, f.reader)
	if err != nil {
		t.Fatal(err)
	}
	defer sub.Close()
	followTail(t, f, sub, initial, 2)
	retainWindow(t, f, sub, "message:"+f.question, "turn:"+f.question)
	c := e.conversation
	c.mu.Lock()
	if c.publicationTimer != nil {
		c.publicationTimer.Stop()
		c.publicationTimer = nil
		c.publicationGeneration++
	}
	c.mu.Unlock()
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
	want := []string{}
	for _, text := range []string{"nova 1", "nova 2"} {
		m, err := database.CreateMessageWithContext(f.ctx, database.MessageOptions{ConversationID: f.conversation, Role: "user", Content: text})
		if err != nil {
			t.Fatal(err)
		}
		want = append(want, "message:"+m.ID)
	}
	if e.conversation.turns[e.turnID].detached {
		t.Fatal("fixture reconciliou antes da liberação")
	}
	retainWindow(t, f, sub)
	if !reflect.DeepEqual(c.windows[sub.windowID].orderedIDs, want) {
		t.Fatal("cauda não cortou os dois itens antigos")
	}
	if stream.Append("tardio") || !c.turns[e.turnID].detached {
		t.Fatal("corte aceitou stream substituído fora da janela")
	}
}

func TestTimelineTurnLifecycleReadDoesNotLoadPayloads(t *testing.T) {
	f := newSnapshotFixture(t)
	if err := f.db.Model(&database.ChatMessage{}).Where("id=?", f.question).Update("content", strings.Repeat("x", canonicalSourceBytes+1)).Error; err != nil {
		t.Fatal(err)
	}
	snapshot, err := f.reader.ReadTurnLifecycles(f.ctx, f.conversation, []string{f.question, "missing"})
	if err != nil || snapshot.Turns[f.question] == nil || len(snapshot.TurnIDs) != 2 || len(snapshot.Items) != 0 {
		t.Fatal("leitura leve falhou", err)
	}
	if _, err := f.reader.ReadTurnLifecycles(database.WithUserID(context.Background(), "other"), f.conversation, []string{f.question}); err == nil {
		t.Fatal("leitura não autorizada")
	}
	if _, err := f.reader.ReadTurnLifecycles(f.ctx, f.conversation, make([]string, database.MaxMessageWindowRows+1)); !errors.Is(err, ErrTimelineSourceLimit) {
		t.Fatal("limite ignorado", err)
	}
}

func TestWindowLifecycleSurvivesRefreshAndNavigationCuts(t *testing.T) {
	for _, mode := range []string{"delete-refresh", "retry-navigate", "active-navigate", "drift-refresh"} {
		t.Run(mode, func(t *testing.T) {
			f, p, e, _ := automaticFixture(t)
			stream := e.StartStream(0)
			stream.Append("antes")
			sub, initial, err := p.SubscribeTimeline(f.ctx, &fixtureTimelineSession{ctx: f.ctx, id: "s"}, "tail", database.MessageWindowQuery{ConversationID: f.conversation, Anchor: "end", Direction: "before", Limit: 2}, f.reader)
			if err != nil {
				t.Fatal(err)
			}
			defer sub.Close()
			followTail(t, f, sub, initial, 2)
			if mode == "delete-refresh" {
				retainWindow(t, f, sub, "message:"+f.question)
			}
			c := e.conversation
			c.mu.Lock()
			if c.publicationTimer != nil {
				c.publicationTimer.Stop()
				c.publicationTimer = nil
				c.publicationGeneration++
			}
			c.mu.Unlock()
			if mode == "retry-navigate" {
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
			}
			want := []string{}
			for _, text := range []string{"nova 1", "nova 2"} {
				m, err := database.CreateMessageWithContext(f.ctx, database.MessageOptions{ConversationID: f.conversation, Role: "user", Content: text})
				if err != nil {
					t.Fatal(err)
				}
				want = append(want, "message:"+m.ID)
			}
			if mode == "delete-refresh" {
				if err := database.DeleteMessageWithContext(f.ctx, f.question); err != nil {
					t.Fatal(err)
				}
			}
			w := c.windows[sub.windowID]
			revision := strconv.FormatUint(w.revision, 10)
			beforeWindows, beforePublication, beforeEpoch := cloneSubscribedWindows(c.windows), c.publication.fork(), c.epoch
			if mode == "drift-refresh" {
				p.reader = &driftingWindowLifecycleReader{f.reader}
			}
			var fresh *SubscribedTimelineSnapshot
			if mode == "delete-refresh" || mode == "drift-refresh" {
				fresh, err = sub.Refresh(f.ctx, revision, nil)
			} else {
				fresh, err = sub.Navigate(f.ctx, TimelineWindowNavigation{Epoch: c.epoch, WindowRevision: revision, TimelineWindowPosition: TimelineWindowPosition{Anchor: "end", Direction: "before", Limit: 2}})
			}
			if mode == "drift-refresh" {
				if !errors.Is(err, ErrProjectionRecoveryRequired) || !reflect.DeepEqual(beforeWindows, c.windows) || !reflect.DeepEqual(beforePublication, c.publication) || beforeEpoch != c.epoch {
					t.Fatal("drift alterou janela/publicação", err)
				}
				if !stream.Append("ainda válido") {
					t.Fatal("drift desvinculou execução sem prova")
				}
				return
			}
			if err != nil || !reflect.DeepEqual(fresh.OrderedItemIDs, want) {
				t.Fatal("corte não aplicado", err)
			}
			accepted := stream.Append("depois")
			if accepted != (mode == "active-navigate") {
				t.Fatalf("aceitação do stream = %v em %s", accepted, mode)
			}
		})
	}
}

type driftingWindowLifecycleReader struct{ *DBTimelineReader }

func (r *driftingWindowLifecycleReader) ReadTurnLifecycles(ctx context.Context, conversation string, ids []string) (*TimelineSelectionSnapshot, error) {
	snapshot, err := r.DBTimelineReader.ReadTurnLifecycles(ctx, conversation, ids)
	if err == nil {
		snapshot.Version.Revision++
	}
	return snapshot, err
}

func TestTimelineTurnLifecycleRejectsOversizedReferences(t *testing.T) {
	f := newSnapshotFixture(t)
	oversized := strings.Repeat("x", canonicalSourceBytes+1)
	if err := f.db.Table("chat_messages").Create(map[string]any{"id": oversized, "conversation_id": f.conversation, "turn_id": f.question, "role": "assistant", "content": "curto", "created_at": time.Now(), "updated_at": time.Now()}).Error; err != nil {
		t.Fatal(err)
	}
	if err := f.db.Model(&database.ChatTurn{}).Where("user_message_id=?", f.question).Update("response_message_id", oversized).Error; err != nil {
		t.Fatal(err)
	}
	snapshot, err := f.reader.ReadTurnLifecycles(f.ctx, f.conversation, []string{f.question})
	if !errors.Is(err, ErrTimelineSourceLimit) || len(snapshot.Turns) != 0 {
		t.Fatal("referência excessiva materializada", err)
	}
}

func TestWindowRetentionSiblingCutsReconcileLifecycle(t *testing.T) {
	for _, producer := range []string{"retention", "publication"} {
		for _, retry := range []bool{false, true} {
			t.Run(producer+"-"+strconv.FormatBool(retry), func(t *testing.T) {
				f, p, e, _ := automaticFixture(t)
				threadMessage, err := database.CreateMessageWithContext(f.ctx, database.MessageOptions{ConversationID: f.conversation, ParentID: &f.question, Role: "user", Content: "outra superfície"})
				if err != nil {
					t.Fatal(err)
				}
				stream := e.StartStream(0)
				stream.Append("antes")
				session := &fixtureTimelineSession{ctx: f.ctx, id: "s"}
				tail, initial, err := p.SubscribeTimeline(f.ctx, session, "tail", database.MessageWindowQuery{ConversationID: f.conversation, Anchor: "end", Direction: "before", Limit: 2}, f.reader)
				if err != nil {
					t.Fatal(err)
				}
				defer tail.Close()
				followTail(t, f, tail, initial, 2)
				other, _, err := p.SubscribeTimeline(f.ctx, session, "thread", database.MessageWindowQuery{ConversationID: f.conversation, ParentID: &f.question, Anchor: "start", Direction: "after", Limit: 1}, f.reader)
				if err != nil {
					t.Fatal(err)
				}
				defer other.Close()
				retainWindow(t, f, other, "message:"+threadMessage.ID)
				c := e.conversation
				c.mu.Lock()
				if c.publicationTimer != nil {
					c.publicationTimer.Stop()
					c.publicationTimer = nil
					c.publicationGeneration++
				}
				c.mu.Unlock()
				if retry {
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
				}
				want := []string{}
				for _, text := range []string{"nova 1", "nova 2"} {
					m, err := database.CreateMessageWithContext(f.ctx, database.MessageOptions{ConversationID: f.conversation, Role: "user", Content: text})
					if err != nil {
						t.Fatal(err)
					}
					want = append(want, "message:"+m.ID)
				}
				if producer == "retention" {
					retainWindow(t, f, other)
				} else {
					turnID := strings.TrimPrefix(want[1], "message:")
					execution, err := uuid.NewV7()
					if err != nil {
						t.Fatal(err)
					}
					_, err = p.Admit(f.ctx, f.conversation, turnID, execution.String(), func() (database.ChatTimelineVersion, error) {
						version, err := f.reader.ReadVersion(f.ctx, f.conversation)
						if err != nil {
							return version, err
						}
						_, err = database.NewChatTurnRepository(f.db).BeginExecution(f.ctx, f.conversation, turnID, "", database.TurnModeNew, execution.String(), "")
						return version, err
					})
					if err != nil || c.publicationErrorLocked() != nil {
						t.Fatal("publicação recusada", err, c.publicationErrorLocked())
					}
					want = []string{"message:" + turnID, "turn:" + turnID}
				}
				if !reflect.DeepEqual(c.windows[tail.windowID].orderedIDs, want) {
					t.Fatal("janela irmã não cortou o turno")
				}
				if accepted := stream.Append("depois"); accepted == retry {
					t.Fatalf("stream aceito=%v, retry=%v", accepted, retry)
				}
			})
		}
	}
}

func TestWindowLifecyclePartitionsAggregateSources(t *testing.T) {
	for _, failure := range []string{"", "drift", "read"} {
		t.Run("failure-"+failure, func(t *testing.T) {
			f := newSnapshotFixture(t)
			ids := []string{}
			for i := 0; i < 3; i++ {
				q, err := database.CreateMessageWithContext(f.ctx, database.MessageOptions{ConversationID: f.conversation, Role: "user", Content: "pergunta"})
				if err != nil {
					t.Fatal(err)
				}
				destination := strconv.Itoa(i) + strings.Repeat("x", 3*1024*1024)
				if err := f.db.Table("chat_messages").Create(map[string]any{"id": destination, "conversation_id": f.conversation, "turn_id": q.ID, "role": "assistant", "content": "curto", "created_at": time.Now(), "updated_at": time.Now()}).Error; err != nil {
					t.Fatal(err)
				}
				ids = append(ids, q.ID)
			}
			if err := database.NewChatTurnRepository(f.db).ReconcileImportedConversationInTransaction(f.ctx, f.conversation); err != nil {
				t.Fatal(err)
			}
			if _, err := f.reader.ReadTurnLifecycles(f.ctx, f.conversation, ids); !errors.Is(err, ErrTimelineSourceLimit) {
				t.Fatal("fixture não excede cota agregada", err)
			}
			probe := &partitionedLifecycleReader{DBTimelineReader: f.reader, failure: failure}
			p := NewLiveProjection(probe)
			var sub *TimelineSubscription
			for i, id := range ids {
				opened, _, err := p.SubscribeTimeline(f.ctx, &fixtureTimelineSession{ctx: f.ctx, id: "s"}, strconv.Itoa(i), database.MessageWindowQuery{ConversationID: f.conversation, AnchorItemID: "message:" + id, Direction: "around", Limit: 1}, f.reader)
				if err != nil {
					t.Fatal("janela individual inválida", err)
				}
				defer opened.Close()
				if sub == nil {
					sub = opened
				}
			}
			c := sub.conversation
			for _, id := range ids {
				c.turns[id] = &liveTurn{turnID: id, executionID: "previous", incarnation: c.incarnation}
			}
			w := c.windows[sub.windowID]
			windows, published := cloneSubscribedWindows(c.windows), c.publication.fork()
			err := sub.SetRetention(f.ctx, TimelineRetentionRequest{Epoch: c.epoch, WindowRevision: strconv.FormatUint(w.revision, 10), ItemIDs: []string{"message:" + ids[0]}})
			if failure != "" {
				if err == nil || !reflect.DeepEqual(windows, c.windows) || !reflect.DeepEqual(published, c.publication) {
					t.Fatal("segunda parcela falhou com publicação parcial", err)
				}
				if failure == "drift" && !errors.Is(err, ErrProjectionRecoveryRequired) {
					t.Fatal("drift não identificado", err)
				}
				return
			}
			if err != nil || probe.successes < 2 {
				t.Fatal("fontes individuais não foram parceladas", err, probe.successes)
			}
			for _, id := range ids {
				if !c.turns[id].detached {
					t.Fatal("lifecycle não reconciliado", id)
				}
			}
		})
	}
}

type partitionedLifecycleReader struct {
	*DBTimelineReader
	failure   string
	successes int
}

func (r *partitionedLifecycleReader) ReadTurnLifecycles(ctx context.Context, conversation string, ids []string) (*TimelineSelectionSnapshot, error) {
	snapshot, err := r.DBTimelineReader.ReadTurnLifecycles(ctx, conversation, ids)
	if err == nil {
		r.successes++
		if r.successes == 2 {
			if r.failure == "drift" {
				snapshot.Version.Revision++
			}
			if r.failure == "read" {
				return nil, errors.New("leitura indisponível")
			}
		}
	}
	return snapshot, err
}

func TestWindowLifecyclePartitionsMoreThan64Turns(t *testing.T) {
	f := newSnapshotFixture(t)
	probe := &partitionedLifecycleReader{DBTimelineReader: f.reader}
	p := NewLiveProjection(probe)
	version, err := f.reader.ReadVersion(f.ctx, f.conversation)
	if err != nil {
		t.Fatal(err)
	}
	c := &projectionConversation{epoch: "epoch", incarnation: version.Incarnation, turns: map[string]*liveTurn{}, windows: map[string]*subscribedWindow{}}
	for page := 0; page < 2; page++ {
		w := &subscribedWindow{epoch: c.epoch}
		for i := 0; i < 40; i++ {
			id := strconv.Itoa(page*40 + i)
			w.orderedIDs = append(w.orderedIDs, "message:"+id)
			c.turns[id] = &liveTurn{turnID: id, incarnation: c.incarnation, executionID: "old"}
		}
		c.windows[strconv.Itoa(page)] = w
	}
	if err := p.reconcileParticipatingLifecycles(f.ctx, f.conversation, c, version); err != nil {
		t.Fatal(err)
	}
	if probe.successes != 2 {
		t.Fatal("mais de64 turnos não foram parcelados", probe.successes)
	}
	for _, turn := range c.turns {
		if !turn.detached {
			t.Fatal("ausência não reconciliada")
		}
	}
}
