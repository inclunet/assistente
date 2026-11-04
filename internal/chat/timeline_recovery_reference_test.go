package chat

import (
	"context"
	"testing"

	"assistente/internal/database"
)

func referenceFromLive(live *LiveTurnSnapshot) TimelineRecoveryReference {
	return TimelineRecoveryReference{ProjectionID: live.ProjectionID, Incarnation: live.Incarnation, TurnID: live.TurnID, ExecutionID: live.ExecutionID}
}

func TestTimelineRecoveryReferenceStableAcrossChunksAndOwnedByWire(t *testing.T) {
	p, ctx, execution := liveFixture(t)
	base := compositionItem(t, []Message{{UUIDModel: database.UUIDModel{ID: "response"}}}, nil)
	before, err := p.Snapshot(ctx, "conversation", "question")
	if err != nil {
		t.Fatal(err)
	}
	first, err := composeResponse(base, before)
	if err != nil {
		t.Fatal(err)
	}
	wire := canonicalItem(first)
	if wire.RecoveryReference == nil || wire.RecoveryReference.ProjectionID == "" || *wire.RecoveryReference != referenceFromLive(before) {
		t.Fatalf("referência incompleta: %+v", wire.RecoveryReference)
	}
	execution.StartStream(0).Append("texto protegido")
	after, err := p.Snapshot(ctx, "conversation", "question")
	if err != nil {
		t.Fatal(err)
	}
	second, err := composeResponse(base, after)
	if err != nil {
		t.Fatal(err)
	}
	if *second.RecoveryReference != *wire.RecoveryReference {
		t.Fatal("chunk alterou identidade de recuperação")
	}
	wire.RecoveryReference.ExecutionID = "alterado pelo consumidor"
	if first.RecoveryReference.ExecutionID != "execution" || second.RecoveryReference.ExecutionID != "execution" {
		t.Fatal("DTO compartilha referência mutável com a projeção")
	}
	if base.RecoveryReference != nil {
		t.Fatal("fabricou referência na fonte persistida")
	}
}

func TestTimelineRecoveryReferenceSeparatesRetryAndRetainedAttempt(t *testing.T) {
	p, ctx, first := liveFixture(t)
	first.StartStream(0).Append("tentativa anterior")
	before, err := p.Snapshot(ctx, "conversation", "question")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := beginTestProjection(p, ctx, "conversation", "question", "retry"); err != nil {
		t.Fatal(err)
	}
	recovery, err := p.ReadRecovery(ctx, "conversation", "question", recoveryReader{func() (*TimelineTurnSnapshot, error) {
		return recoveryStored("retry", 2), nil
	}})
	if err != nil {
		t.Fatal(err)
	}
	if recovery.Active == nil || len(recovery.Retained) != 1 {
		t.Fatalf("execuções ausentes: %+v", recovery)
	}
	if referenceFromLive(recovery.Retained[0]) != referenceFromLive(before) {
		t.Fatal("retry mudou identidade da tentativa retida")
	}
	if referenceFromLive(recovery.Active) == referenceFromLive(before) || recovery.Active.ExecutionID != "retry" {
		t.Fatal("nova execução pode confirmar bytes da anterior")
	}
}

func TestTimelineRecoveryReferenceSurvivesEpochChangeButNotProjectionRecreation(t *testing.T) {
	p, ctx, execution := liveFixture(t)
	execution.StartStream(0).Append("antes da importação")
	before, err := p.Snapshot(ctx, "conversation", "question")
	if err != nil {
		t.Fatal(err)
	}
	recovery, err := p.ReadRecovery(ctx, "conversation", "question", recoveryReader{func() (*TimelineTurnSnapshot, error) {
		value := recoveryStored("", 2)
		value.Version.Incarnation = "imported"
		return value, nil
	}})
	if err != nil {
		t.Fatal(err)
	}
	if recovery.Epoch == before.Epoch || len(recovery.Retained) != 1 {
		t.Fatal("fixture não mudou a época")
	}
	if referenceFromLive(recovery.Retained[0]) != referenceFromLive(before) {
		t.Fatal("referência perdeu encarnação original")
	}
	restarted, restartedCtx, _ := liveFixture(t)
	other, err := restarted.Snapshot(restartedCtx, "conversation", "question")
	if err != nil {
		t.Fatal(err)
	}
	if other.ProjectionID == before.ProjectionID {
		t.Fatal("projeções diferentes reutilizaram identidade")
	}
}

func TestTimelineRecoveryReferenceClearedOnlyWhenNoLongerProtected(t *testing.T) {
	for _, persisted := range []bool{false, true} {
		t.Run(map[bool]string{false: "volatile", true: "persisted"}[persisted], func(t *testing.T) {
			p, ctx, execution := liveFixture(t)
			base := compositionItem(t, []Message{{UUIDModel: database.UUIDModel{ID: "response"}}}, nil)
			active, err := p.Snapshot(ctx, "conversation", "question")
			if err != nil {
				t.Fatal(err)
			}
			before, err := composeResponse(base, active)
			if err != nil {
				t.Fatal(err)
			}
			finishTestProjection(execution, database.TurnCompleted, "", persisted, false)
			finished, err := p.Snapshot(ctx, "conversation", "question")
			if err != nil {
				t.Fatal(err)
			}
			after, err := composeResponse(base, finished)
			if err != nil {
				t.Fatal(err)
			}
			if persisted != (after.RecoveryReference == nil) {
				t.Fatalf("referência terminal incorreta: %+v", after.RecoveryReference)
			}
			oldWire, newWire := canonicalItem(before), canonicalItem(after)
			if oldWire.ExecutionID != active.ExecutionID || newWire.ExecutionID != active.ExecutionID {
				t.Fatal("terminal perdeu identidade da execução")
			}
			operations := canonicalDiff(&oldWire, newWire)
			if len(operations) != 1 || operations[0].Kind != "item_upsert" || operations[0].Item == nil {
				t.Fatal("terminal não publica a referência completa", operations)
			}
		})
	}
}

func TestTimelineRecoveryReferenceFromBatchCreatedConversation(t *testing.T) {
	p := NewLiveProjection(nil)
	ctx := database.WithUserID(context.Background(), "user")
	conversations, release := p.lockConversationBatch("user", []string{"conversation"})
	projectionID := conversations["conversation"].projectionID
	release()
	execution, err := beginTestProjection(p, ctx, "conversation", "question", "execution")
	if err != nil {
		t.Fatal(err)
	}
	execution.StartStream(0).Append("conteúdo")
	live, err := p.Snapshot(ctx, "conversation", "question")
	if err != nil {
		t.Fatal(err)
	}
	item, err := composeResponse(compositionItem(t, []Message{{UUIDModel: database.UUIDModel{ID: "response"}}}, nil), live)
	if err != nil {
		t.Fatal(err)
	}
	wire := canonicalItem(item)
	if projectionID == "" || wire.RecoveryReference == nil || wire.RecoveryReference.ProjectionID == "" {
		t.Fatalf("referência inválida após operação em lote: %+v", wire.RecoveryReference)
	}
	t.Cleanup(func() {
		c, done := p.lockConversation(projectionKey{"user", "conversation"}, false)
		defer done()
		if c != nil && c.timer != nil {
			c.timer.Stop()
		}
	})
}
