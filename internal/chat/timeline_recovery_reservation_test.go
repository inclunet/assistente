package chat

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"assistente/internal/apidto"
	"assistente/internal/database"
)

func reserveFixture(t *testing.T) (*TimelineSubscription, *fixtureTimelineSession, *ExecutionProjection, apidto.TimelineRecoveryReservationRequest, *TimelineSelectionSnapshot) {
	t.Helper()
	sub, session, e, ref, snapshot := reconciliationFixture(t)
	e.StartStream(0).Append(strings.Repeat("x", recoveryPageTextBytes) + "fim")
	finishTestProjection(e, database.TurnCompleted, "", true, true)
	page, err := sub.ReadRecoveryPage(session.ctx, apidto.TimelineRecoveryPageRequest{Reference: ref}, pageReader(snapshot))
	if err != nil {
		t.Fatal(err)
	}
	return sub, session, e, apidto.TimelineRecoveryReservationRequest{Reference: ref, ContentVersion: page.ContentVersion}, snapshot
}

func TestRecoveryReservationSurvivesCollectionAndReleaseKeepsSource(t *testing.T) {
	sub, session, e, request, snapshot := reserveFixture(t)
	for range 2 {
		receipt, err := sub.ReserveRecovery(session.ctx, request, pageReader(snapshot))
		if err != nil || receipt.Reference != request.Reference || receipt.ContentVersion != request.ContentVersion {
			t.Fatal(receipt, err)
		}
	}
	w := sub.conversation.windows[sub.windowID]
	if e.StartStream(0).Append("tardio") {
		t.Fatal("reserva aceitou mutação terminal")
	}
	if len(w.recoveryReservations) != 1 {
		t.Fatal("reserva duplicada")
	}
	if err := sub.ReleaseRecovery(session.ctx, request); err != nil {
		t.Fatal(err)
	}
	if sub.conversation.turns[e.turnID] == nil || len(w.recoveryReservations) != 0 {
		t.Fatal("release descartou fonte")
	}
	if _, err := sub.ReserveRecovery(session.ctx, request, pageReader(snapshot)); err != nil {
		t.Fatal(err)
	}
	session.id = "revoked"
	if err := sub.ReleaseRecovery(session.ctx, request); !errors.Is(err, ErrTimelineSubscription) || len(w.recoveryReservations) != 1 {
		t.Fatal("liberação revogada alterou reserva", err)
	}
	session.id = sub.sessionID
	first, err := sub.ReadRecoveryPage(session.ctx, apidto.TimelineRecoveryPageRequest{Reference: request.Reference}, pageReader(snapshot))
	if err != nil {
		t.Fatal(err)
	}
	p, c := sub.owner, sub.conversation
	p.mu.Lock()
	c.mu.Lock()
	p.pruneLocked(sub.key, c, time.Now().Add(2*projectionRetention))
	c.mu.Unlock()
	p.mu.Unlock()
	if c.turns[e.turnID] != nil {
		t.Fatal("fixture não coletou turno confirmado")
	}
	last, err := sub.ReadRecoveryPage(session.ctx, apidto.TimelineRecoveryPageRequest{Reference: request.Reference, Cursor: first.NextCursor}, pageReader(snapshot))
	if err != nil || !last.Found || len(last.Chunks) != 1 || last.Chunks[0].Text != "fim" || last.ContentVersion != request.ContentVersion {
		t.Fatal(last, err)
	}
	status, err := sub.ReconcileRecovery(session.ctx, request.Reference, pageReader(snapshot))
	if err != nil || status.State != "retained" {
		t.Fatal(status, err)
	}
	if _, err := sub.ReserveRecovery(session.ctx, request, pageReader(snapshot)); err != nil {
		t.Fatal("idempotência após coleta", err)
	}
	bad := request
	bad.ContentVersion = "999"
	if err := sub.ReleaseRecovery(session.ctx, bad); !errors.Is(err, ErrRecoveryCursorStale) {
		t.Fatal(err)
	}
	for range 2 {
		if err := sub.ReleaseRecovery(session.ctx, request); err != nil {
			t.Fatal(err)
		}
	}
	missing, err := sub.ReadRecoveryPage(session.ctx, apidto.TimelineRecoveryPageRequest{Reference: request.Reference}, pageReader(snapshot))
	if err != nil || missing.Found {
		t.Fatal(missing, err)
	}
}

func TestRecoveryReservationRefusalsAreAtomic(t *testing.T) {
	for _, scenario := range []string{"active", "stale", "missing", "count", "budget", "bytes", "parts", "cancel", "storage", "revoked"} {
		t.Run(scenario, func(t *testing.T) {
			sub, session, e, request, snapshot := reserveFixture(t)
			reader := pageReader(snapshot)
			want := ErrRecoveryReservationLimit
			w := sub.conversation.windows[sub.windowID]
			switch scenario {
			case "active":
				e.currentLocked().finished = false
				want = ErrRecoveryMutable
			case "stale":
				request.ContentVersion = "999"
				want = ErrRecoveryCursorStale
			case "missing":
				request.Reference.ExecutionID = "missing"
				want = ErrRecoveryUnavailable
			case "count":
				w.recoveryReservations = map[TimelineRecoveryReference]recoveryReservation{}
				for i := range recoveryReservationLimit {
					ref := request.Reference
					ref.ExecutionID = fmt.Sprint(i)
					w.recoveryReservations[ref] = recoveryReservation{}
				}
			case "budget":
				ref := request.Reference
				ref.ExecutionID = "other"
				w.recoveryReservations = map[TimelineRecoveryReference]recoveryReservation{ref: {bytes: recoveryReservationBytes}}
			case "bytes":
				e.currentLocked().streams[0].parts[0].text.Grow(recoveryReservationBytes + 1)
			case "parts":
				for range recoveryReservationVisits {
					e.currentLocked().streams[0].parts = append(e.currentLocked().streams[0].parts, &livePart{})
				}
			case "cancel":
				ctx, cancel := context.WithCancel(session.ctx)
				session.ctx = ctx
				reader = func(context.Context, string, []string) (*TimelineSelectionSnapshot, error) {
					cancel()
					return snapshot, nil
				}
				want = context.Canceled
			case "storage":
				want = errors.New("storage")
				reader = func(context.Context, string, []string) (*TimelineSelectionSnapshot, error) { return nil, want }
			case "revoked":
				reader = func(context.Context, string, []string) (*TimelineSelectionSnapshot, error) {
					session.id = "new"
					return snapshot, nil
				}
				want = ErrTimelineSubscription
			}
			before := len(w.recoveryReservations)
			receipt, err := sub.ReserveRecovery(session.ctx, request, reader)
			if !errors.Is(err, want) || receipt != nil || len(w.recoveryReservations) != before {
				t.Fatal("recusa consumiu reserva", receipt, err)
			}
		})
	}
}

func TestRecoveryReservationWindowIsolationCloseAndDeletion(t *testing.T) {
	for _, closeAll := range []bool{false, true} {
		t.Run(fmt.Sprint(closeAll), func(t *testing.T) {
			sub, session, _, request, snapshot := reserveFixture(t)
			other := &TimelineSubscription{owner: sub.owner, key: sub.key, conversation: sub.conversation, session: session, sessionID: session.id, windowID: "other", changed: make(chan struct{}, 1)}
			sub.conversation.windows["other"] = &subscribedWindow{id: "other", handle: other}
			t.Cleanup(other.Close)
			for _, s := range []*TimelineSubscription{sub, other} {
				if _, err := s.ReserveRecovery(session.ctx, request, pageReader(snapshot)); err != nil {
					t.Fatal(err)
				}
			}
			delete(sub.conversation.turns, request.Reference.TurnID)
			if closeAll {
				sub.conversation.clearDeletedConversationLocked()
				// Recriar a conversa/execução não reativa capabilities removidas.
				sub.conversation.turns[request.Reference.TurnID] = &liveTurn{}
			} else {
				sub.Close()
			}
			if _, err := sub.ReadRecoveryPage(session.ctx, apidto.TimelineRecoveryPageRequest{Reference: request.Reference}, pageReader(snapshot)); err == nil {
				t.Fatal("capability encerrada leu")
			}
			page, err := other.ReadRecoveryPage(session.ctx, apidto.TimelineRecoveryPageRequest{Reference: request.Reference}, pageReader(snapshot))
			if closeAll {
				if err == nil || page != nil {
					t.Fatal("exclusão manteve reserva")
				}
			} else if err != nil || !page.Found {
				t.Fatal("close afetou outra janela", err)
			}
		})
	}
}

func TestRecoveryReservationDoesNotRetainStreamMetadata(t *testing.T) {
	sub, session, e, ref, snapshot := reconciliationFixture(t)
	e.StartStream(0).Append("preservado")
	if err := e.CommitContent(0, "preservado", "", func() (string, error) { return strings.Repeat("id", 1024*1024), nil }); err != nil {
		t.Fatal(err)
	}
	finishTestProjection(e, database.TurnCompleted, "", true, true)
	page, err := sub.ReadRecoveryPage(session.ctx, apidto.TimelineRecoveryPageRequest{Reference: ref}, pageReader(snapshot))
	if err != nil {
		t.Fatal(err)
	}
	request := apidto.TimelineRecoveryReservationRequest{Reference: ref, ContentVersion: page.ContentVersion}
	if _, err := sub.ReserveRecovery(session.ctx, request, pageReader(snapshot)); err != nil {
		t.Fatal(err)
	}
	held := sub.conversation.windows[sub.windowID].recoveryReservations[ref]
	if held.buffers.liveStreams != nil || len(held.buffers.reserved) != 1 {
		t.Fatal("reserva reteve streams completos")
	}
	source := e.currentLocked().streams[0]
	if held.buffers.reserved[0].parts[0] != source.parts[0] {
		t.Fatal("copiou texto")
	}
	delete(sub.conversation.turns, ref.TurnID)
	page, err = sub.ReadRecoveryPage(session.ctx, apidto.TimelineRecoveryPageRequest{Reference: ref}, pageReader(snapshot))
	if err != nil || !page.Found || len(page.Chunks) != 1 || page.Chunks[0].Text != "preservado" {
		t.Fatal(page, err)
	}
}
