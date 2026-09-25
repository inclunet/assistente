package commandmaintenance_test

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"assistente/internal/commandcontract"
	"assistente/internal/commanddecision"
	"assistente/internal/commandledger"
	"assistente/internal/commandmaintenance"
	"assistente/internal/commandsecurity"
	"github.com/glebarez/sqlite"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type multiuserRecoveryFixture struct {
	db        *gorm.DB
	ledger    *commandledger.Store
	decisions *commanddecision.Store
	core      *commandsecurity.EpochService
	now       time.Time
}

func newMultiuserRecoveryFixture(t *testing.T) *multiuserRecoveryFixture {
	t.Helper()
	ctx := context.Background()
	// O Store usa este relógio também para decidir se a apresentação expirou,
	// enquanto context.WithDeadline usa o relógio real. Mantê-lo no passado
	// fazia a fixture nascer com ExpiresAt vencido quando a suíte rodava depois
	// das 10:01 UTC. Um instante futuro e fixo conserva o controle temporal sem
	// depender da carga ou da hora em que o processo foi iniciado.
	now := time.Now().UTC().Add(time.Hour).Truncate(time.Millisecond)
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "recovery-multiuser.db")), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	if err := commandledger.Migrate(ctx, db); err != nil {
		t.Fatal(err)
	}
	if err := commanddecision.Migrate(ctx, db); err != nil {
		t.Fatal(err)
	}
	ledger, err := commandledger.New(db, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	decisions, err := commanddecision.New(db, acceptDecision{}, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	core, err := commandsecurity.NewEpochService(&commandsecurity.DispatchGate{})
	if err != nil {
		t.Fatal(err)
	}
	return &multiuserRecoveryFixture{db: db, ledger: ledger, decisions: decisions, core: core, now: now}
}

type multiuserInvocation struct {
	user, session, invocation, decision string
	epoch                               commandsecurity.EpochSnapshot
}

func (f *multiuserRecoveryFixture) reserveUserInvocation(t *testing.T, core *commandsecurity.EpochService, user, session string) multiuserInvocation {
	t.Helper()
	ctx := context.Background()
	id := func() string { return uuid.Must(uuid.NewV7()).String() }
	epoch, err := core.Capture(ctx, user, session)
	if err != nil {
		t.Fatal(err)
	}
	now := f.now
	invocation := id()
	request := commandledger.LocalReadRequest{
		InvocationID:   invocation,
		Owner:          commandledger.Owner{UserID: user, AuthContextID: session},
		AuthGeneration: epoch.AuthGeneration, SecurityGeneration: epoch.SecurityGeneration,
		RegistryVersion: "registry-1", GlobalConfigGeneration: "config-1", ActiveLayersGeneration: "layers-1",
		CommandID: "maintenance.read", SourceType: "palette",
		ArgumentsFingerprint: strings.Repeat("a", 64), RequestFingerprintVersion: "v1", RequestFingerprint: strings.Repeat("b", 64),
		CorrelationID: id(), ReceivedAt: now, ExpiresAt: now.Add(time.Hour),
	}
	if _, err := f.ledger.Reserve(ctx, request); err != nil {
		t.Fatal(err)
	}
	decisionID := id()
	status, err := f.decisions.Decide(ctx, commanddecision.Request{
		SubjectType: "invocation", DecisionID: decisionID, MutationID: invocation,
		UserID: user, SessionID: session, Fingerprint: request.RequestFingerprint,
		AuthGeneration: epoch.AuthGeneration, SecurityGeneration: epoch.SecurityGeneration,
		ExpiresAt: now.Add(time.Minute), Body: "recovery test",
	})
	if err != nil || status != commanddecision.Accepted {
		t.Fatalf("decisão=%s err=%v", status, err)
	}
	return multiuserInvocation{user: user, session: session, invocation: invocation, decision: decisionID, epoch: epoch}
}

func (f *multiuserRecoveryFixture) reserveSystemInvocation(t *testing.T, epoch commandsecurity.EpochSnapshot) string {
	t.Helper()
	id := uuid.Must(uuid.NewV7()).String()
	commandID := "maintenance.system"
	source := commandcontract.SourceSystem
	auth := commandcontract.AuthSystem
	actor := commandcontract.ActorAutomation
	requestFingerprint := strings.Repeat("c", 64)
	args := json.RawMessage(`{"scope":"instance"}`)
	_, err := f.ledger.ReserveEnvelope(context.Background(), commandledger.EnvelopeRequest{
		Envelope: commandcontract.Envelope{
			Version: 1, InvocationID: id, CommandID: &commandID, Arguments: &args,
			AuthContextType: auth, AuthContextID: "instance", AuthGeneration: "system-auth",
			SecurityGeneration: epoch.SecurityGeneration, ActorType: actor, ActorID: "instance",
			SourceType: &source, BindingIDs: []string{}, RegistryVersion: "registry-1",
			CorrelationID: uuid.Must(uuid.NewV7()).String(), RequestFingerprintVersion: stringPtr("v1"),
			RequestFingerprint: &requestFingerprint, ReceivedAt: f.now,
		},
		Mode: commandledger.ModeExecute, ArgumentsFingerprint: strings.Repeat("d", 64),
		ExpiresAt: f.now.Add(time.Hour), Risk: "low",
	})
	if err != nil {
		t.Fatalf("reserva system: %v", err)
	}
	return id
}

func stringPtr(value string) *string { return &value }

func multiuserRecoveryCoordinator(t *testing.T, decisions *commanddecision.CoordinatorRecovery, invocations *commandledger.CoordinatorRecovery, compactions *atomic.Int32) *commandmaintenance.Coordinator {
	t.Helper()
	other := &otherDomains{}
	coordinator, err := commandmaintenance.New(commandmaintenance.Ports{
		Outbox: other, Decisions: decisions, Invocations: invocations, Claims: other,
		Jobs: other, Tools: other, InvocationDB: other, Activations: other,
		Compaction: compactionsAdapter{count: compactions},
	})
	if err != nil {
		t.Fatal(err)
	}
	return coordinator
}

type compactionsAdapter struct{ count *atomic.Int32 }

func (c compactionsAdapter) Compact(context.Context, int64) error { c.count.Add(1); return nil }

func multiuserStatus(t *testing.T, db *gorm.DB, table, column, id string) commandledger.Status {
	t.Helper()
	var row struct {
		Status commandledger.Status `gorm:"column:status"`
	}
	if err := db.Table(table).Select("status").Where(column+" = ?", id).Take(&row).Error; err != nil {
		t.Fatal(err)
	}
	return row.Status
}

func multiuserEventCount(t *testing.T, db *gorm.DB, table, column, id string) int64 {
	t.Helper()
	var count int64
	if err := db.Table(table).Where(column+" = ?", id).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	return count
}

func TestCoordinatorRecoveryMultiuserClosedUnknownAndSystem(t *testing.T) {
	f := newMultiuserRecoveryFixture(t)
	ctx := context.Background()
	user1, session1 := uuid.Must(uuid.NewV7()).String(), uuid.Must(uuid.NewV7()).String()
	user2, session2 := uuid.Must(uuid.NewV7()).String(), uuid.Must(uuid.NewV7()).String()
	first := f.reserveUserInvocation(t, f.core, user1, session1)
	second := f.reserveUserInvocation(t, f.core, user2, session2)
	systemID := f.reserveSystemInvocation(t, first.epoch)

	unknownCore, err := commandsecurity.NewEpochService(&commandsecurity.DispatchGate{})
	if err != nil {
		t.Fatal(err)
	}
	unknownUser, unknownSession := uuid.Must(uuid.NewV7()).String(), uuid.Must(uuid.NewV7()).String()
	unknown := f.reserveUserInvocation(t, unknownCore, unknownUser, unknownSession)
	proof, err := f.core.CloseAndDrain(ctx)
	if err != nil {
		t.Fatal(err)
	}
	decisions, err := commanddecision.NewCoordinatorRecovery(f.decisions, proof)
	if err != nil {
		t.Fatal(err)
	}
	invocations, err := commandledger.NewCoordinatorRecovery(f.ledger, proof)
	if err != nil {
		t.Fatal(err)
	}
	var compacted atomic.Int32
	coordinator := multiuserRecoveryCoordinator(t, decisions, invocations, &compacted)
	policy := commandmaintenance.Policy{InvocationRetention: time.Hour, InvocationsPerUser: 1, InvocationsSystemKeep: 1, ActivationRetention: time.Hour, ActivationsPerUser: 1, LeaseDuration: time.Minute, BatchSize: 1}
	runs := 0
	recovered := 0
	for ; runs < 5; runs++ {
		report, runErr := coordinator.Run(ctx, policy)
		if runErr != nil {
			t.Fatalf("passagem %d=%+v err=%v", runs+1, report, runErr)
		}
		recovered += report.Recovered
		if !report.MoreRecovery {
			break
		}
	}
	if runs != 3 || recovered != 5 {
		t.Fatalf("paginação multiusuário runs=%d recovered=%d", runs+1, recovered)
	}
	if got := compacted.Load(); got != 1 {
		t.Fatalf("compactação antes do fim da recuperação ou duplicada: %d", got)
	}
	for _, item := range []multiuserInvocation{first, second} {
		if got := multiuserStatus(t, f.db, "command_decision_receipts", "decision_id", item.decision); got != commandledger.Status(commanddecision.Cancelled) {
			t.Fatalf("receipt fechada=%s status=%s", item.decision, got)
		}
		if got := multiuserStatus(t, f.db, "command_invocations", "invocation_id", item.invocation); got != commandledger.OutcomeUnknown {
			t.Fatalf("invocação fechada=%s status=%s", item.invocation, got)
		}
	}
	if got := multiuserStatus(t, f.db, "command_idempotency_keys", "invocation_id", systemID); got != commandledger.OutcomeUnknown {
		t.Fatalf("system não recuperado: %s", got)
	}
	if got := multiuserStatus(t, f.db, "command_decision_receipts", "decision_id", unknown.decision); got != commandledger.Status(commanddecision.Accepted) {
		t.Fatalf("receipt de geração desconhecida alterada: %s", got)
	}
	if got := multiuserStatus(t, f.db, "command_invocations", "invocation_id", unknown.invocation); got != commandledger.Status(commandledger.Evaluating) {
		t.Fatalf("invocação de geração viva alterada: %s", got)
	}
}

func TestCoordinatorRecoveryMultiuserCancellationResumesWithoutRepeatingReceipts(t *testing.T) {
	f := newMultiuserRecoveryFixture(t)
	ctx := context.Background()
	items := []multiuserInvocation{
		f.reserveUserInvocation(t, f.core, uuid.Must(uuid.NewV7()).String(), uuid.Must(uuid.NewV7()).String()),
		f.reserveUserInvocation(t, f.core, uuid.Must(uuid.NewV7()).String(), uuid.Must(uuid.NewV7()).String()),
	}
	proof, err := f.core.CloseAndDrain(ctx)
	if err != nil {
		t.Fatal(err)
	}
	recovery, err := commanddecision.NewCoordinatorRecovery(f.decisions, proof)
	if err != nil {
		t.Fatal(err)
	}
	first, err := recovery.Recover(ctx, 1)
	if err != nil || first.Processed != 1 || !first.More {
		t.Fatalf("primeiro lote=%+v err=%v", first, err)
	}
	firstEvents := multiuserEventCount(t, f.db, "command_decision_receipt_events", "decision_id", items[0].decision)
	canceled, cancel := context.WithCancel(context.Background())
	const callback = "recovery_multiuser_cancel_second_receipt"
	var callbacks atomic.Int32
	if err := f.db.Callback().Create().After("gorm:create").Register(callback, func(tx *gorm.DB) {
		if tx.Statement.Table == "command_decision_receipt_events" {
			callbacks.Add(1)
			cancel()
		}
	}); err != nil {
		t.Fatal(err)
	}
	result, gotErr := recovery.Recover(canceled, 1)
	if err := f.db.Callback().Create().Remove(callback); err != nil {
		t.Fatal(err)
	}
	if !errors.Is(gotErr, context.Canceled) || result.Processed != 0 || !result.More {
		t.Fatalf("cancelamento=%+v err=%v callbacks=%d", result, gotErr, callbacks.Load())
	}
	if got := multiuserStatus(t, f.db, "command_decision_receipts", "decision_id", items[1].decision); got != commandledger.Status(commanddecision.Accepted) {
		t.Fatalf("receipt parcial após cancelamento: %s", got)
	}
	retry, err := recovery.Recover(ctx, 1)
	if err != nil || retry.Processed != 1 || retry.More {
		t.Fatalf("retomada=%+v err=%v", retry, err)
	}
	if got := multiuserEventCount(t, f.db, "command_decision_receipt_events", "decision_id", items[0].decision); got != firstEvents {
		t.Fatalf("primeira receipt teve efeito repetido: antes=%d depois=%d", firstEvents, got)
	}
	if got := multiuserEventCount(t, f.db, "command_decision_receipt_events", "decision_id", items[1].decision); got != 3 {
		t.Fatalf("eventos da segunda receipt=%d", got)
	}
}

func TestCoordinatorRecoveryMultiuserSQLErrorResumesInvocationPair(t *testing.T) {
	f := newMultiuserRecoveryFixture(t)
	ctx := context.Background()
	items := []multiuserInvocation{
		f.reserveUserInvocation(t, f.core, uuid.Must(uuid.NewV7()).String(), uuid.Must(uuid.NewV7()).String()),
		f.reserveUserInvocation(t, f.core, uuid.Must(uuid.NewV7()).String(), uuid.Must(uuid.NewV7()).String()),
	}
	proof, err := f.core.CloseAndDrain(ctx)
	if err != nil {
		t.Fatal(err)
	}
	recovery, err := commandledger.NewCoordinatorRecovery(f.ledger, proof)
	if err != nil {
		t.Fatal(err)
	}
	first, err := recovery.Recover(ctx, 1)
	if err != nil || first.Processed != 1 || !first.More {
		t.Fatalf("primeiro lote=%+v err=%v", first, err)
	}
	if got := multiuserStatus(t, f.db, "command_invocations", "invocation_id", items[0].invocation); got != commandledger.OutcomeUnknown {
		t.Fatalf("primeira invocação não recuperada: %s", got)
	}
	trigger := "recovery_multiuser_fail_second_invocation"
	statement := "CREATE TRIGGER " + trigger + " BEFORE UPDATE ON command_invocations WHEN NEW.invocation_id = '" + items[1].invocation + "' BEGIN SELECT RAISE(ABORT, 'recovery fixture'); END"
	if err := f.db.Exec(statement).Error; err != nil {
		t.Fatal(err)
	}
	failed, gotErr := recovery.Recover(ctx, 1)
	if gotErr == nil || failed.Processed != 0 || !failed.More {
		t.Fatalf("erro SQL=%+v err=%v", failed, gotErr)
	}
	if got := multiuserStatus(t, f.db, "command_invocations", "invocation_id", items[1].invocation); got != commandledger.Status(commandledger.Evaluating) {
		t.Fatalf("invocação foi alterada apesar do rollback: %s", got)
	}
	if err := f.db.Exec("DROP TRIGGER " + trigger).Error; err != nil {
		t.Fatal(err)
	}
	retry, err := recovery.Recover(ctx, 1)
	if err != nil || retry.Processed != 1 || retry.More {
		t.Fatalf("retomada SQL=%+v err=%v", retry, err)
	}
	repeat, err := recovery.Recover(ctx, 1)
	if err != nil || repeat.Processed != 0 || repeat.More {
		t.Fatalf("efeito repetido após ciclo=%+v err=%v", repeat, err)
	}
	if got := multiuserStatus(t, f.db, "command_idempotency_keys", "invocation_id", items[0].invocation); got != commandledger.OutcomeUnknown {
		t.Fatalf("ledger do primeiro perdeu atomicidade: %s", got)
	}
}
