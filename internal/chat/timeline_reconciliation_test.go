package chat

import (
	"context"
	"errors"
	"strings"
	"testing"

	"assistente/internal/database"
	"gorm.io/gorm"
)

type reconciliationReader func(context.Context, string, []string) (*TimelineSelectionSnapshot, error)

func (r reconciliationReader) ReadTurnLifecycles(ctx context.Context, conversation string, ids []string) (*TimelineSelectionSnapshot, error) {
	return r(ctx, conversation, ids)
}

func reconciliationFixture(t *testing.T) (*TimelineSubscription, *fixtureTimelineSession, *ExecutionProjection, TimelineRecoveryReference, *TimelineSelectionSnapshot) {
	t.Helper()
	p, ctx, execution := liveFixture(t)
	live, err := p.Snapshot(ctx, "conversation", "question")
	if err != nil {
		t.Fatal(err)
	}
	session := &fixtureTimelineSession{ctx: ctx, id: "session"}
	c, release := p.lockConversation(projectionKey{"user", "conversation"}, false)
	sub := &TimelineSubscription{owner: p, key: projectionKey{"user", "conversation"}, conversation: c, session: session, sessionID: "session", windowID: "window", changed: make(chan struct{}, 1)}
	c.windows = map[string]*subscribedWindow{"window": {id: "window", handle: sub}}
	release()
	t.Cleanup(sub.Close)
	stored := recoveryStored("execution", 1)
	snapshot := &TimelineSelectionSnapshot{Version: stored.Version, Turns: map[string]*database.ChatTurn{"question": stored.Turn}}
	return sub, session, execution, referenceFromLive(live), snapshot
}

func TestTimelineReconciliationStatesAndExactIdentity(t *testing.T) {
	for _, scenario := range []string{"active", "retained", "persisted", "partial", "retry", "import", "removed", "finished-removed", "revision", "projection", "execution", "turn", "incarnation"} {
		t.Run(scenario, func(t *testing.T) {
			sub, session, execution, reference, snapshot := reconciliationFixture(t)
			expected := "active"
			switch scenario {
			case "retained":
				execution.StartStream(0).Append("volátil")
				finishTestProjection(execution, database.TurnCompleted, "", false, false)
				expected = "retained"
			case "persisted", "partial", "revision":
				if scenario == "partial" {
					execution.StartStream(0).Append("não confirmado")
				}
				finishTestProjection(execution, database.TurnCompleted, "", true, false)
				snapshot.Version.Revision = 2
				snapshot.Turns["question"].ActiveExecutionID = nil
				id := "execution"
				snapshot.Turns["question"].CurrentExecutionID = &id
				expected = "persisted"
				if scenario == "partial" {
					expected = "retained"
				}
				if scenario == "revision" {
					snapshot.Version.Revision++
					expected = "unknown"
				}
			case "retry":
				execution.StartStream(0).Append("tentativa anterior")
				if _, err := beginTestProjection(sub.owner, session.ctx, "conversation", "question", "retry"); err != nil {
					t.Fatal(err)
				}
				id := "retry"
				snapshot.Turns["question"].ActiveExecutionID = &id
				expected = "retained"
			case "import":
				snapshot.Version.Incarnation = "imported"
				expected = "retained"
			case "removed":
				delete(snapshot.Turns, "question")
				expected = "retained"
			case "finished-removed":
				finishTestProjection(execution, database.TurnCompleted, "", true, false)
				snapshot.Version.Revision = 2
				delete(snapshot.Turns, "question")
				expected = "unknown"
			case "projection":
				reference.ProjectionID = "recreated"
				expected = "unknown"
			case "execution":
				reference.ExecutionID = "other"
				expected = "unknown"
			case "turn":
				reference.TurnID = "other"
				expected = "unknown"
			case "incarnation":
				reference.Incarnation = "other"
				expected = "unknown"
			}
			epoch, revision, sequence := sub.conversation.epoch, sub.conversation.revision, sub.conversation.publication.sequence
			calls := 0
			reader := reconciliationReader(func(_ context.Context, conversation string, ids []string) (*TimelineSelectionSnapshot, error) {
				calls++
				if conversation != "conversation" || len(ids) != 1 || ids[0] != reference.TurnID {
					t.Fatal("consulta fora do alvo", conversation, ids)
				}
				return snapshot, nil
			})
			result, err := sub.ReconcileRecovery(session.ctx, reference, reader)
			if err != nil || result.State != expected || result.Reference != reference || result.ConversationID != "conversation" || calls != 1 {
				t.Fatalf("resultado: %+v, erro: %v, consultas: %d", result, err, calls)
			}
			if sub.conversation.epoch != epoch || sub.conversation.revision != revision || sub.conversation.publication.sequence != sequence {
				t.Fatal("consulta alterou protocolo")
			}
		})
	}
}

func TestTimelineReconciliationRejectsFailuresWithoutStatus(t *testing.T) {
	for _, scenario := range []string{"closed", "revoked", "session", "user", "revoked-during-read", "cancelled-during-read", "storage", "empty", "oversized"} {
		t.Run(scenario, func(t *testing.T) {
			sub, session, _, reference, snapshot := reconciliationFixture(t)
			ctx, cancel := context.WithCancel(session.ctx)
			defer cancel()
			switch scenario {
			case "closed":
				sub.Close()
			case "revoked":
				session.denied = true
			case "session":
				session.id = "new-session"
			case "user":
				session.ctx = database.WithUserID(context.Background(), "other")
			case "empty":
				reference.ExecutionID = ""
			case "oversized":
				reference.ExecutionID = strings.Repeat("x", database.MaxTimelineSourceBytes)
			}
			reader := reconciliationReader(func(context.Context, string, []string) (*TimelineSelectionSnapshot, error) {
				switch scenario {
				case "revoked-during-read":
					session.denied = true
				case "cancelled-during-read":
					cancel()
				case "storage":
					return nil, errors.New("storage unavailable")
				case "closed", "revoked", "session", "user", "empty", "oversized":
					t.Fatal("leitura não deveria ocorrer")
				}
				return snapshot, nil
			})
			result, err := sub.ReconcileRecovery(ctx, reference, reader)
			if err == nil || result != nil {
				t.Fatalf("falha virou status: %+v, %v", result, err)
			}
		})
	}
}

func TestTimelineReconciliationUsesAuthorizedLifecycleWithoutPayload(t *testing.T) {
	f, p, _, _ := automaticFixture(t)
	session := &fixtureTimelineSession{ctx: f.ctx, id: "session"}
	sub, _, err := p.SubscribeTimeline(f.ctx, session, "surface", database.MessageWindowQuery{ConversationID: f.conversation, Anchor: "end", Direction: "before", Limit: 2}, f.reader)
	if err != nil {
		t.Fatal(err)
	}
	defer sub.Close()
	live, err := p.Snapshot(f.ctx, f.conversation, f.question)
	if err != nil {
		t.Fatal(err)
	}
	reference := referenceFromLive(live)
	calls := 0
	if err = f.db.Callback().Query().Before("gorm:query").Register("reconciliation_no_payload", func(tx *gorm.DB) {
		if tx.Statement.Table == "chat_messages" || tx.Statement.Table == "tool_invocations" {
			_ = tx.AddError(errors.New("consulta de payload proibida neste teste"))
		}
		calls++
	}); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.db.Callback().Query().Remove("reconciliation_no_payload") }()
	result, err := sub.ReconcileRecovery(f.ctx, reference, f.reader)
	if err != nil || result.State != "active" || calls != 2 {
		t.Fatalf("consulta leve: %+v, %v, consultas %d", result, err, calls)
	}
	if err = f.db.Callback().Query().Remove("reconciliation_no_payload"); err != nil {
		t.Fatal(err)
	}
	if err = database.NewConversationRepository(f.db).DeleteConversationWithContext(f.ctx, f.conversation); err != nil {
		t.Fatal(err)
	}
	result, err = sub.ReconcileRecovery(f.ctx, reference, f.reader)
	if err == nil || result != nil {
		t.Fatal("conversa excluída revelou estado em memória")
	}
}
