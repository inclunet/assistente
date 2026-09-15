package commandledger

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"assistente/internal/commandcontract"
	"assistente/internal/commanddecision"
	"github.com/google/uuid"
)

type acceptingEnvelopeDecisionPresenter struct{}

func (acceptingEnvelopeDecisionPresenter) Present(_ context.Context, request commanddecision.Request) (commanddecision.Response, error) {
	return commanddecision.Response{DecisionID: request.DecisionID, ActionID: commanddecision.ApplyAction}, nil
}

func envelopeFixture(t *testing.T) (commandcontract.Envelope, time.Time) {
	t.Helper()
	user, _ := uuid.NewV7()
	session, _ := uuid.NewV7()
	invocation, _ := uuid.NewV7()
	now := time.Date(2026, 9, 14, 11, 0, 0, 0, time.UTC)
	return commandcontract.Envelope{
		Version: 1, InvocationID: invocation.String(), CommandID: stringPtr("workspace.read"),
		Arguments: rawPtr(`{"path":"secret.txt"}`), UserID: stringPtr(user.String()),
		AuthContextType: commandcontract.AuthLocalSession, AuthContextID: session.String(), AuthGeneration: "auth-1",
		SessionID: stringPtr(session.String()), SecurityGeneration: "security-1", ActorType: commandcontract.ActorUser,
		ActorID: user.String(), SourceType: sourcePtr(commandcontract.SourcePalette), BindingIDs: []string{"binding-1"},
		RegistryVersion: "registry-1", GlobalConfigGeneration: stringPtr("config-1"), ActiveLayersGeneration: stringPtr("layers-1"),
		CorrelationID: "correlation-1", RequestFingerprintVersion: stringPtr("v1"), RequestFingerprint: stringPtr(strings.Repeat("a", 64)),
		ReceivedAt: now,
	}, now
}

func envelopeRequest(t *testing.T) (EnvelopeRequest, time.Time) {
	t.Helper()
	envelope, now := envelopeFixture(t)
	envelope.RequestFingerprint = stringPtr(strings.Repeat("a", 64))
	return EnvelopeRequest{Envelope: envelope, Mode: ModeExecute, ArgumentsFingerprint: strings.Repeat("b", 64), ExpiresAt: now.Add(5 * time.Minute), Risk: "low"}, now
}

func TestReserveEnvelopePersistsFullOwnershipAndRedactsDocuments(t *testing.T) {
	req, now := envelopeRequest(t)
	store, db := testStore(t, &now)
	reservation, err := store.ReserveEnvelope(context.Background(), req)
	if err != nil || !reservation.Created {
		t.Fatalf("reserva: %+v, %v", reservation, err)
	}
	if reservation.Record.Ownership.ActorType != commandcontract.ActorUser || reservation.Record.Ownership.ActorID != req.Envelope.ActorID {
		t.Fatalf("ownership do ator perdido: %+v", reservation.Record.Ownership)
	}
	if reservation.Record.ArgumentsSummary != redactedDocument || reservation.Record.Envelope.Arguments == nil || string(*reservation.Record.Envelope.Arguments) != redactedDocument {
		t.Fatalf("argumentos não redigidos: %+v", reservation.Record)
	}
	if reservation.Record.ResultSummary != nil || reservation.Record.Status != Evaluating {
		t.Fatalf("reserva não deveria ser terminal: %+v", reservation.Record)
	}
	var ledger ledgerRow
	var audit invocationRow
	if err := db.First(&ledger).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.First(&audit).Error; err != nil {
		t.Fatal(err)
	}
	if ledger.ActorType == nil || ledger.ActorID == nil || *ledger.ActorType != string(req.Envelope.ActorType) || *ledger.ActorID != req.Envelope.ActorID {
		t.Fatalf("ator do ledger não persistido: %+v", ledger)
	}
	if audit.ArgumentsSummary != redactedDocument || audit.TriggerSpecSnapshot != nil || audit.ForegroundSummary != nil || audit.Provenance != nil {
		t.Fatalf("auditoria expôs documentos: %+v", audit)
	}
	if audit.RequestFingerprintVersion != "v1" || audit.RequestFingerprint != *req.Envelope.RequestFingerprint || audit.Risk != "low" {
		t.Fatalf("campos derivados não preservados: %+v", audit)
	}
	if ledger.InputFingerprint != nil || reservation.Record.InputFingerprint != "" {
		t.Fatalf("input fingerprint legado deveria permanecer ausente: ledger=%v record=%q", ledger.InputFingerprint, reservation.Record.InputFingerprint)
	}
}

func TestReserveEnvelopePersistsAndDeduplicatesInputFingerprint(t *testing.T) {
	req, now := envelopeRequest(t)
	req.InputFingerprint = strings.Repeat("f", 64)
	store, db := testStore(t, &now)
	first, err := store.ReserveEnvelope(context.Background(), req)
	if err != nil || !first.Created || first.Record.InputFingerprint != req.InputFingerprint {
		t.Fatalf("input fingerprint não persistido: %+v, %v", first, err)
	}
	var ledger ledgerRow
	if err := db.First(&ledger).Error; err != nil {
		t.Fatal(err)
	}
	if ledger.InputFingerprint == nil || *ledger.InputFingerprint != req.InputFingerprint {
		t.Fatalf("input fingerprint no ledger: %+v", ledger)
	}
	second := req
	second.InputFingerprint = strings.Repeat("e", 64)
	if _, err := store.ReserveEnvelope(context.Background(), second); !errors.Is(err, ErrConflict) {
		t.Fatalf("input fingerprint divergente deveria conflitar: %v", err)
	}
	if _, err := store.ReserveEnvelope(context.Background(), req); err != nil {
		t.Fatalf("reentrega com input fingerprint igual: %v", err)
	}
}

func TestReserveEnvelopeSuppressAndStaleHaveNoAudit(t *testing.T) {
	for _, test := range []struct {
		name   string
		stale  bool
		status Status
	}{
		{name: "suppress", status: Suppressed},
		{name: "stale", stale: true, status: RejectedStale},
	} {
		t.Run(test.name, func(t *testing.T) {
			req, now := envelopeRequest(t)
			req.Mode = ModeSuppress
			req.Envelope.CommandID = nil
			req.Envelope.Arguments = nil
			req.Envelope.TriggerType = stringPtr("palette")
			req.Envelope.TriggerSpec = rawPtr(`{"version":1,"selector":"fixture"}`)
			if test.stale {
				req.Envelope.SourceType = sourcePtr(commandcontract.SourceEvent)
				req.Envelope.TriggerType = stringPtr("event")
				sourceInstance, _ := uuid.NewV7()
				sourceEvent, _ := uuid.NewV7()
				req.Envelope.SourceInstanceID = stringPtr(sourceInstance.String())
				req.Envelope.SourceEventID = stringPtr(sourceEvent.String())
				req.Envelope.ObserverType = stringPtr("keyboard")
				occurred := now.Add(-2 * time.Minute)
				deadline := now.Add(-time.Minute)
				req.ExpiresAt = deadline
				req.Envelope.SourceOccurredAt = &occurred
				req.Envelope.SourceReplayPolicyGeneration = stringPtr("replay-1")
				req.Envelope.SourceReplayDeadline = &deadline
				req.Envelope.TriggerSpec = rawPtr(`{"version":1,"name":"fixture"}`)
				req.Envelope.Provenance = rawPtr(`{"version":1,"_source":"fixture"}`)
			}
			store, db := testStore(t, &now)
			reservation, err := store.ReserveEnvelope(context.Background(), req)
			if err != nil || !reservation.Created || reservation.Record.Status != test.status {
				t.Fatalf("reserva terminal: %+v, %v", reservation, err)
			}
			var audits int64
			if err := db.Model(&invocationRow{}).Count(&audits).Error; err != nil {
				t.Fatal(err)
			}
			if audits != 0 {
				t.Fatalf("status %s criou auditoria", test.status)
			}
			if reservation.Record.ResultSummary == nil || !strings.Contains(*reservation.Record.ResultSummary, `"status":"`+string(test.status)+`"`) {
				t.Fatalf("resumo terminal inválido: %v", reservation.Record.ResultSummary)
			}
			if test.stale && !reservation.Record.ExpiresAt.Equal(req.ExpiresAt) {
				t.Fatal("replay stale alongou deadline imutável")
			}
		})
	}
}

func TestReserveEnvelopePhysicalSourceEventIDIsNotReplayStale(t *testing.T) {
	req, now := envelopeRequest(t)
	sourceInstance, _ := uuid.NewV7()
	sourceEvent, _ := uuid.NewV7()
	req.Envelope.SourceType = sourcePtr(commandcontract.SourceKeyboardLocal)
	req.Envelope.TriggerType = stringPtr(string(commandcontract.SourceKeyboardLocal))
	req.Envelope.SourceInstanceID = stringPtr(sourceInstance.String())
	req.Envelope.SourceEventID = stringPtr(sourceEvent.String())
	req.Envelope.ObserverType = stringPtr("keyboard")
	req.Envelope.TriggerSpec = rawPtr(`{"version":1,"name":"fixture"}`)
	store, db := testStore(t, &now)
	reservation, err := store.ReserveEnvelope(context.Background(), req)
	if err != nil || !reservation.Created || reservation.Record.Status != Evaluating {
		t.Fatalf("origem física não deve ser stale: %+v, %v", reservation, err)
	}
	var audits int64
	if err := db.Model(&invocationRow{}).Count(&audits).Error; err != nil {
		t.Fatal(err)
	}
	if audits != 1 {
		t.Fatalf("origem física deveria auditar: %d", audits)
	}
	expired := req
	expiredID, _ := uuid.NewV7()
	expired.Envelope.InvocationID = expiredID.String()
	expired.Envelope.RequestFingerprint = stringPtr(strings.Repeat("e", 64))
	expired.Envelope.ReceivedAt = now.Add(-2 * time.Minute)
	expired.ExpiresAt = now.Add(-time.Minute)
	if _, err := store.ReserveEnvelope(context.Background(), expired); !errors.Is(err, ErrExpired) {
		t.Fatalf("expiração física deveria ser rejeitada: %v", err)
	}
}

func TestReserveEnvelopeTrustedContextStaleHasPriorityAndNoAudit(t *testing.T) {
	req, now := envelopeRequest(t)
	req.Mode = ModeSuppress
	req.RejectedStale = true
	req.Envelope.CommandID = nil
	req.Envelope.Arguments = nil
	req.Envelope.TriggerType = stringPtr("palette")
	req.Envelope.TriggerSpec = rawPtr(`{"version":1,"name":"context-stale"}`)
	req.Envelope.ReceivedAt = now.Add(-2 * time.Minute)
	req.ExpiresAt = now.Add(-time.Minute)
	store, db := testStore(t, &now)
	reservation, err := store.ReserveEnvelope(context.Background(), req)
	if err != nil || !reservation.Created || reservation.Record.Status != RejectedStale {
		t.Fatalf("stale contextual não teve prioridade: %+v, %v", reservation, err)
	}
	var audits int64
	if err := db.Model(&invocationRow{}).Count(&audits).Error; err != nil {
		t.Fatal(err)
	}
	if audits != 0 {
		t.Fatalf("stale contextual criou auditoria: %d", audits)
	}
}

func TestReserveEnvelopeDeduplicatesEventAndScopesActor(t *testing.T) {
	req, now := envelopeRequest(t)
	req.Envelope.SourceType = sourcePtr(commandcontract.SourceEvent)
	sourceInstance, _ := uuid.NewV7()
	sourceEvent, _ := uuid.NewV7()
	req.Envelope.SourceInstanceID = stringPtr(sourceInstance.String())
	req.Envelope.SourceEventID = stringPtr(sourceEvent.String())
	req.Envelope.ObserverType = stringPtr("event")
	req.Envelope.TriggerType = stringPtr("event")
	req.Envelope.TriggerSpec = rawPtr(`{"version":1,"name":"fixture"}`)
	occurred := now.Add(-time.Second)
	deadline := now.Add(time.Minute)
	req.Envelope.SourceOccurredAt = &occurred
	req.Envelope.SourceReplayPolicyGeneration = stringPtr("replay-1")
	req.Envelope.SourceReplayDeadline = &deadline
	req.Envelope.Provenance = rawPtr(`{"version":1,"_source":"fixture"}`)
	store, _ := testStore(t, &now)
	first, err := store.ReserveEnvelope(context.Background(), req)
	if err != nil || !first.Created {
		t.Fatalf("primeira ocorrência: %+v, %v", first, err)
	}
	second, err := store.ReserveEnvelope(context.Background(), req)
	if err != nil || second.Created || second.Record.ID != first.Record.ID {
		t.Fatalf("reentrega: %+v, %v", second, err)
	}
	req.Envelope.RequestFingerprint = stringPtr(strings.Repeat("c", 64))
	if _, err := store.ReserveEnvelope(context.Background(), req); !errors.Is(err, ErrConflict) {
		t.Fatalf("fingerprint divergente: %v", err)
	}
	other := FullOwnership{UserID: first.Record.Ownership.UserID, AuthContextType: first.Record.Ownership.AuthContextType, AuthContextID: first.Record.Ownership.AuthContextID, ActorType: commandcontract.ActorAgent, ActorID: "agent-1"}
	if _, err := store.GetEnvelopeByEvent(context.Background(), other, *req.Envelope.SourceEventID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("ator fora do escopo: %v", err)
	}
	got, err := store.GetEnvelopeByEvent(context.Background(), first.Record.Ownership, *req.Envelope.SourceEventID)
	if err != nil || got.Ownership.ActorID != req.Envelope.ActorID {
		t.Fatalf("consulta por evento: %+v, %v", got, err)
	}
}

func TestReserveEnvelopeRollsBackLedgerWhenAuditFails(t *testing.T) {
	req, now := envelopeRequest(t)
	store, db := testStore(t, &now)
	if err := db.Exec(`CREATE TRIGGER reject_envelope_audit BEFORE INSERT ON command_invocations BEGIN SELECT RAISE(ABORT, 'fixture'); END`).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := store.ReserveEnvelope(context.Background(), req); !errors.Is(err, ErrInconsistent) {
		t.Fatalf("erro sanitizado: %v", err)
	}
	var ledgers, audits int64
	db.Model(&ledgerRow{}).Count(&ledgers)
	db.Model(&invocationRow{}).Count(&audits)
	if ledgers != 0 || audits != 0 {
		t.Fatalf("rollback parcial: ledger=%d audit=%d", ledgers, audits)
	}
}

func TestCompareAndSwapEnvelopeStoresImmutableTerminalResult(t *testing.T) {
	req, now := envelopeRequest(t)
	store, db := testStore(t, &now)
	if _, err := store.ReserveEnvelope(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	owner := ownershipFromEnvelope(req.Envelope)
	for _, step := range []struct{ from, to Status }{{Evaluating, Queued}, {Queued, Running}} {
		if ok, err := store.CompareAndSwapEnvelope(context.Background(), owner, req.Envelope.InvocationID, step.from, step.to); err != nil || !ok {
			t.Fatalf("transição %s->%s: %v, %v", step.from, step.to, ok, err)
		}
	}
	ref, _ := uuid.NewV7()
	if ok, err := store.CompareAndSwapEnvelope(context.Background(), owner, req.Envelope.InvocationID, Running, Failed, EnvelopeResult{ResultRef: ref.String()}); err != nil || !ok {
		t.Fatalf("falha terminal: %v, %v", ok, err)
	}
	var ledger ledgerRow
	var audit invocationRow
	db.First(&ledger)
	db.First(&audit)
	if ledger.Status != Failed || audit.Status != Failed || ledger.ResultSummary == nil || audit.ResultSummary == nil || *ledger.ResultSummary != *audit.ResultSummary || audit.ErrorCode == nil || *audit.ErrorCode != "command_failed" || ledger.ResultRef == nil || *ledger.ResultRef != ref.String() {
		t.Fatalf("resultado terminal: ledger=%+v audit=%+v", ledger, audit)
	}
	if ok, err := store.CompareAndSwapEnvelope(context.Background(), owner, req.Envelope.InvocationID, Running, Succeeded); err != nil || ok {
		t.Fatalf("terminal permitiu sobrescrita: %v, %v", ok, err)
	}
	var after ledgerRow
	db.First(&after)
	if after.Status != Failed || after.ResultSummary == nil || *after.ResultSummary != *ledger.ResultSummary {
		t.Fatalf("resultado terminal foi alterado: %+v", after)
	}
}

func TestCompareAndSwapEnvelopeWithDecisionConsumesReceiptInSameTransaction(t *testing.T) {
	req, _ := envelopeRequest(t)
	now := time.Now().UTC()
	req.ExpiresAt = now.Add(24 * time.Hour)
	ledger, db := testStore(t, &now)
	if err := commanddecision.Migrate(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	decisions, err := commanddecision.New(db, acceptingEnvelopeDecisionPresenter{}, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ledger.ReserveEnvelope(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	decisionID, _ := uuid.NewV7()
	decisionRequest := commanddecision.Request{
		SubjectType: "invocation", DecisionID: decisionID.String(), MutationID: req.Envelope.InvocationID,
		UserID: *req.Envelope.UserID, SessionID: req.Envelope.AuthContextID, Fingerprint: *req.Envelope.RequestFingerprint,
		AuthGeneration: req.Envelope.AuthGeneration, SecurityGeneration: req.Envelope.SecurityGeneration,
		ExpiresAt: now.Add(5 * time.Minute), Body: "apply",
	}
	if _, err := decisions.Decide(context.Background(), decisionRequest); err != nil {
		t.Fatal(err)
	}
	ok, err := ledger.CompareAndSwapEnvelopeWithDecision(context.Background(), ownershipFromEnvelope(req.Envelope), req.Envelope.InvocationID, decisionRequest, decisions)
	if err != nil || !ok {
		t.Fatalf("CAS com receipt: %v, %v", ok, err)
	}
	record, err := ledger.GetEnvelopeByID(context.Background(), ownershipFromEnvelope(req.Envelope), req.Envelope.InvocationID)
	if err != nil || record.Status != Queued {
		t.Fatalf("estado após receipt: %+v, %v", record, err)
	}
	var receiptStatus string
	if err := db.Raw("SELECT status FROM command_decision_receipts WHERE decision_id = ?", decisionRequest.DecisionID).Scan(&receiptStatus).Error; err != nil {
		t.Fatal(err)
	}
	if receiptStatus != commanddecision.Consumed {
		t.Fatalf("receipt não consumido: %q", receiptStatus)
	}
	var persistedDecisionID string
	if err := db.Raw("SELECT authorization_decision_id FROM command_invocations WHERE invocation_id = ?", req.Envelope.InvocationID).Scan(&persistedDecisionID).Error; err != nil {
		t.Fatal(err)
	}
	if persistedDecisionID != decisionRequest.DecisionID {
		t.Fatalf("decision id não persistido na auditoria: %q", persistedDecisionID)
	}
}

func TestCompareAndSwapEnvelopeWithDecisionRollsBackConsumedReceiptOnCASConflict(t *testing.T) {
	req, _ := envelopeRequest(t)
	now := time.Now().UTC()
	req.ExpiresAt = now.Add(24 * time.Hour)
	ledger, db := testStore(t, &now)
	if err := commanddecision.Migrate(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	decisions, err := commanddecision.New(db, acceptingEnvelopeDecisionPresenter{}, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ledger.ReserveEnvelope(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	decisionID, _ := uuid.NewV7()
	decisionRequest := commanddecision.Request{
		SubjectType: "invocation", DecisionID: decisionID.String(), MutationID: req.Envelope.InvocationID,
		UserID: *req.Envelope.UserID, SessionID: req.Envelope.AuthContextID, Fingerprint: *req.Envelope.RequestFingerprint,
		AuthGeneration: req.Envelope.AuthGeneration, SecurityGeneration: req.Envelope.SecurityGeneration,
		ExpiresAt: now.Add(5 * time.Minute), Body: "apply",
	}
	if _, err := decisions.Decide(context.Background(), decisionRequest); err != nil {
		t.Fatal(err)
	}
	owner := ownershipFromEnvelope(req.Envelope)
	if ok, err := ledger.CompareAndSwapEnvelope(context.Background(), owner, req.Envelope.InvocationID, Evaluating, Queued); err != nil || !ok {
		t.Fatalf("fixture CAS: %v, %v", ok, err)
	}
	if ok, err := ledger.CompareAndSwapEnvelopeWithDecision(context.Background(), owner, req.Envelope.InvocationID, decisionRequest, decisions); ok || !errors.Is(err, ErrConflict) {
		t.Fatalf("conflito deveria reverter receipt: %v, %v", ok, err)
	}
	var receiptStatus string
	if err := db.Raw("SELECT status FROM command_decision_receipts WHERE decision_id = ?", decisionRequest.DecisionID).Scan(&receiptStatus).Error; err != nil {
		t.Fatal(err)
	}
	if receiptStatus != commanddecision.Accepted {
		t.Fatalf("receipt consumido apesar do rollback: %q", receiptStatus)
	}
}

func TestReconcileEnvelopeDoesNotCreateNewEffectAndRecoversPanic(t *testing.T) {
	req, now := envelopeRequest(t)
	store, _ := testStore(t, &now)
	if _, err := store.ReserveEnvelope(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	owner := ownershipFromEnvelope(req.Envelope)
	for _, step := range []struct{ from, to Status }{{Evaluating, Queued}, {Queued, Running}, {Running, OutcomeUnknown}} {
		if ok, err := store.CompareAndSwapEnvelope(context.Background(), owner, req.Envelope.InvocationID, step.from, step.to); err != nil || !ok {
			t.Fatalf("transição %s->%s: %v, %v", step.from, step.to, ok, err)
		}
	}
	called := 0
	ok, err := store.ReconcileEnvelope(context.Background(), owner, req.Envelope.InvocationID, func(_ context.Context, record FullRecord) (Status, error) {
		called++
		if record.Status != OutcomeUnknown || record.Envelope.Arguments == nil || string(*record.Envelope.Arguments) != redactedDocument {
			t.Fatalf("callback recebeu dado incorreto: %+v", record)
		}
		return Succeeded, nil
	})
	if err != nil || !ok || called != 1 {
		t.Fatalf("reconciliação: %v, %v, chamadas=%d", ok, err, called)
	}
	ok, err = store.ReconcileEnvelope(context.Background(), owner, req.Envelope.InvocationID, func(context.Context, FullRecord) (Status, error) {
		called++
		panic("untrusted")
	})
	if err != nil || ok || called != 1 {
		t.Fatalf("terminal não deveria callback: %v, %v, chamadas=%d", ok, err, called)
	}

	second := req
	secondID, _ := uuid.NewV7()
	second.Envelope.InvocationID = secondID.String()
	second.Envelope.RequestFingerprint = stringPtr(strings.Repeat("d", 64))
	if _, err := store.ReserveEnvelope(context.Background(), second); err != nil {
		t.Fatal(err)
	}
	secondOwner := ownershipFromEnvelope(second.Envelope)
	for _, step := range []struct{ from, to Status }{{Evaluating, Queued}, {Queued, Running}, {Running, OutcomeUnknown}} {
		if ok, err := store.CompareAndSwapEnvelope(context.Background(), secondOwner, second.Envelope.InvocationID, step.from, step.to); err != nil || !ok {
			t.Fatalf("segunda transição %s->%s: %v, %v", step.from, step.to, ok, err)
		}
	}
	if ok, err := store.ReconcileEnvelope(context.Background(), secondOwner, second.Envelope.InvocationID, func(context.Context, FullRecord) (Status, error) {
		panic("trusted callback fixture")
	}); ok || !errors.Is(err, ErrVerifierPanic) {
		t.Fatalf("panic do callback: %v, %v", ok, err)
	}
	unchanged, err := store.GetEnvelopeByID(context.Background(), secondOwner, second.Envelope.InvocationID)
	if err != nil || unchanged.Status != OutcomeUnknown {
		t.Fatalf("panic alterou outcome_unknown: %+v, %v", unchanged, err)
	}
}

func rawPtr(value string) *json.RawMessage {
	raw := json.RawMessage(value)
	return &raw
}

func sourcePtr(value commandcontract.SourceType) *commandcontract.SourceType { return &value }
