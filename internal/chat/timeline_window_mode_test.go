package chat

import (
	"context"
	"errors"
	"github.com/google/uuid"
	"reflect"
	"strconv"
	"testing"

	"assistente/internal/database"
)

func followTail(t *testing.T, f snapshotFixture, sub *TimelineSubscription, initial *SubscribedTimelineSnapshot, limit int) *SubscribedTimelineSnapshot {
	t.Helper()
	request := navigationFrom(initial)
	request.Mode, request.Limit = TimelineWindowFollowTail, limit
	snapshot, err := sub.Navigate(f.ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Mode != TimelineWindowFollowTail {
		t.Fatal("modo ausente no snapshot")
	}
	return snapshot
}

func TestFollowTailCutsAndGrowsWithoutMovingHistoricalSurface(t *testing.T) {
	f, p := membershipFixture(t)
	history, fixed := membershipSubscribe(t, f, p, "history", 2)
	tail, initial := membershipSubscribe(t, f, p, "tail", 2)
	initial = followTail(t, f, tail, initial, 2)
	membershipWrite(t, f, p, "new", 30)
	ids := append([]string(nil), initial.OrderedItemIDs...)
	for _, batch := range membershipReplay(t, f, tail, initial) {
		for _, patch := range batch.WindowPatches {
			if patch.SyncRequired {
				t.Fatal("crescimento exigiu reload")
			}
			for _, splice := range patch.Splices {
				suffix := append([]string(nil), ids[splice.Index+splice.DeleteCount:]...)
				ids = append(append(ids[:splice.Index:splice.Index], splice.InsertIDs...), suffix...)
			}
			if patch.Metadata == nil || patch.Metadata.TotalCount != 3 || patch.Metadata.StartIndex != 1 || patch.Metadata.HasAfter || patch.Metadata.AfterCursor == initial.AfterCursor {
				t.Fatal("metadados de cauda incorretos")
			}
			if len(patch.Items) != 1 || patch.Items[0].ItemID != "message:new" {
				t.Fatal("entrada sem conteúdo")
			}
		}
	}
	if !reflect.DeepEqual(ids, []string{"message:last", "message:new"}) {
		t.Fatal(ids)
	}
	for _, batch := range membershipReplay(t, f, history, fixed) {
		for _, patch := range batch.WindowPatches {
			if len(patch.Splices) > 0 {
				t.Fatal("histórico moveu")
			}
		}
	}
	if !reflect.DeepEqual(history.conversation.windows[history.windowID].orderedIDs, fixed.OrderedItemIDs) {
		t.Fatal("histórico alterado")
	}
	history.Close()
	membershipWrite(t, f, p, "newest", 40)
	w := tail.conversation.windows[tail.windowID]
	fresh, err := tail.Refresh(f.ctx, strconv.FormatUint(w.revision, 10), nil)
	if err != nil || fresh.Mode != TimelineWindowFollowTail || !reflect.DeepEqual(fresh.OrderedItemIDs, []string{"message:new", "message:newest"}) {
		t.Fatal("refresh perdeu modo", err)
	}
	// Paginar explicitamente interrompe acompanhamento; novas mensagens não cortam essa fatia.
	request := navigationFrom(fresh)
	request.Anchor, request.Direction = "start", "after"
	frozen, err := tail.Navigate(f.ctx, request)
	if err != nil || frozen.Mode != TimelineWindowHistorical {
		t.Fatal(err)
	}
	membershipWrite(t, f, p, "outside", 50)
	if !reflect.DeepEqual(tail.conversation.windows[tail.windowID].orderedIDs, frozen.OrderedItemIDs) {
		t.Fatal("navegação histórica continuou seguindo")
	}
}

func TestFollowTailSurvivesDeletedBoundaryAndEmptyWindow(t *testing.T) {
	f, p := membershipFixture(t)
	history, fixed := membershipSubscribe(t, f, p, "history", 2)
	tail, initial := membershipSubscribe(t, f, p, "tail", 2)
	initial = followTail(t, f, tail, initial, 2)
	for _, id := range []string{"last", "first"} {
		if err := p.ApplyStorageMutation(f.ctx, f.conversation, nil, func() ([]string, error) { return nil, database.DeleteMessageWithContext(f.ctx, id) }); err != nil {
			t.Fatal(err)
		}
	}
	membershipSyncRevision(t, history, fixed, f)
	w := tail.conversation.windows[tail.windowID]
	if w.syncRequired || len(w.orderedIDs) != 0 || w.metadata.BeforeCursor != "" || w.metadata.AfterCursor != "" {
		t.Fatal("cauda vazia inválida")
	}
	membershipWrite(t, f, p, "back", 30)
	if !reflect.DeepEqual(tail.conversation.windows[tail.windowID].orderedIDs, []string{"message:back"}) {
		t.Fatal("cauda vazia não recebeu mensagem")
	}
	for _, batch := range membershipReplay(t, f, tail, initial) {
		for _, patch := range batch.WindowPatches {
			if patch.SyncRequired {
				t.Fatal("cauda exigiu sync")
			}
		}
	}
}

func TestFollowTailRejectsInvalidAndStaleModeChanges(t *testing.T) {
	f, p := membershipFixture(t)
	sub, initial := membershipSubscribe(t, f, p, "tail", 2)
	for _, change := range []func(*TimelineWindowNavigation){
		func(r *TimelineWindowNavigation) { r.Mode = "invalid" },
		func(r *TimelineWindowNavigation) { r.Cursor = initial.BeforeCursor },
		func(r *TimelineWindowNavigation) { r.Anchor = "start" },
		func(r *TimelineWindowNavigation) { r.Direction = "after" },
		func(r *TimelineWindowNavigation) { r.AnchorItemID = "message:first" },
		func(r *TimelineWindowNavigation) { r.Limit = 0 },
		func(r *TimelineWindowNavigation) { r.Limit = 241 },
	} {
		request := navigationFrom(initial)
		request.Mode = TimelineWindowFollowTail
		change(&request)
		before := cloneSubscribedWindows(sub.conversation.windows)
		if _, err := sub.Navigate(f.ctx, request); !errors.Is(err, ErrTimelineSubscription) {
			t.Fatal("pedido inválido aceito", err)
		}
		if !reflect.DeepEqual(before, sub.conversation.windows) {
			t.Fatal("erro mudou inscrição")
		}
	}
	followTail(t, f, sub, initial, 2)
	if _, err := sub.Navigate(f.ctx, navigationFrom(initial)); !errors.Is(err, ErrTimelineWindowStale) {
		t.Fatal("pedido antigo mudou modo", err)
	}
}

func TestFollowTailKeepsQuestionBeforePreparingResponse(t *testing.T) {
	f, p, e, _ := automaticFixture(t)
	sub, initial := membershipSubscribe(t, f, p, "tail", 10)
	initial = followTail(t, f, sub, initial, 10)
	repo := database.NewChatTurnRepository(f.db)
	_, err := e.AssignResponse(func() (string, error) { return repo.EnsureResponse(f.ctx, f.conversation, f.question, e.executionID) })
	if err != nil {
		t.Fatal(err)
	}
	w := sub.conversation.windows[sub.windowID]
	if !reflect.DeepEqual(w.orderedIDs, []string{"message:" + f.question, "turn:" + f.question}) {
		t.Fatal("ordem/identidade", w.orderedIDs)
	}
	e.StartStream(0).Append("texto")
	e.conversation.mu.Lock()
	e.conversation.flushPublicationLocked(f.conversation)
	e.conversation.mu.Unlock()
	for _, batch := range membershipReplay(t, f, sub, initial) {
		for _, patch := range batch.WindowPatches {
			if len(patch.Splices) > 0 {
				t.Fatal("stream mudou identidade")
			}
		}
	}
}

func TestFollowTailRefreshPositionAndFailedNavigationPreserveContract(t *testing.T) {
	f, p := membershipFixture(t)
	sub, initial := membershipSubscribe(t, f, p, "tail", 2)
	initial = followTail(t, f, sub, initial, 2)
	sub.reader = navigationReaderFunc(func(context.Context, database.MessageWindowQuery) (*TimelineWindowSnapshot, error) {
		return nil, errors.New("recusado")
	})
	before := cloneSubscribedWindows(sub.conversation.windows)
	if _, err := sub.Navigate(f.ctx, navigationFrom(initial)); err == nil {
		t.Fatal("falha ignorada")
	}
	if !reflect.DeepEqual(before, sub.conversation.windows) {
		t.Fatal("falha mudou modo")
	}
	sub.reader = f.reader
	fresh, err := sub.Refresh(f.ctx, initial.WindowRevision, &TimelineWindowPosition{Anchor: "start", Direction: "after", Limit: 1})
	if err != nil || fresh.Mode != TimelineWindowHistorical {
		t.Fatal("refresh explícito não saiu da cauda", err)
	}
	membershipWrite(t, f, p, "outside", 30)
	if !reflect.DeepEqual(fresh.OrderedItemIDs, sub.conversation.windows[sub.windowID].orderedIDs) {
		t.Fatal("refresh não congelou janela")
	}
}

func TestFollowTailReceivesNewQuestionAndResponseInCanonicalOrder(t *testing.T) {
	f, p := membershipFixture(t)
	history, fixed := membershipSubscribe(t, f, p, "history", 10)
	tail, initial := membershipSubscribe(t, f, p, "tail", 10)
	initial = followTail(t, f, tail, initial, 10)
	var questionID string
	err := p.ApplyStorageMutation(f.ctx, f.conversation, []string{}, func() ([]string, error) {
		message, err := database.CreateMessageWithContext(f.ctx, database.MessageOptions{ConversationID: f.conversation, Role: "user", Content: "nova pergunta"})
		if err != nil {
			return nil, err
		}
		questionID = message.ID
		return []string{"message:" + questionID}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	execution, err := uuid.NewV7()
	if err != nil {
		t.Fatal(err)
	}
	executionID := execution.String()
	repo := database.NewChatTurnRepository(f.db)
	_, err = p.Admit(f.ctx, f.conversation, questionID, executionID, func() (database.ChatTimelineVersion, error) {
		version, err := f.reader.ReadVersion(f.ctx, f.conversation)
		if err != nil {
			return version, err
		}
		_, err = repo.BeginExecution(f.ctx, f.conversation, questionID, "", database.TurnModeNew, executionID, "")
		return version, err
	})
	if err != nil {
		t.Fatal(err)
	}
	ids := tail.conversation.windows[tail.windowID].orderedIDs
	if !reflect.DeepEqual(ids[len(ids)-2:], []string{"message:" + questionID, "turn:" + questionID}) {
		t.Fatal("ordem incorreta", ids)
	}
	if !reflect.DeepEqual(history.conversation.windows[history.windowID].orderedIDs, fixed.OrderedItemIDs) {
		t.Fatal("histórico seguiu cauda")
	}
	membershipReplay(t, f, tail, initial)
}
