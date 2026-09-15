package commandmaintenance_test

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"assistente/internal/commanddecision"
	"assistente/internal/commandledger"
	"assistente/internal/commandmaintenance"
	"assistente/internal/commandsecurity"
	"github.com/glebarez/sqlite"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type acceptDecision struct{}

func (acceptDecision) Present(_ context.Context, r commanddecision.Request) (commanddecision.Response, error) {
	return commanddecision.Response{DecisionID: r.DecisionID, ActionID: commanddecision.ApplyAction}, nil
}

// Os outros domínios são spies: só decisões/invocações usam bancos reais aqui.
type otherDomains struct{ compactions int }

func (*otherDomains) RequeueExpiredLeases(context.Context, int) (int, bool, error) {
	return 0, false, nil
}
func (*otherDomains) Drain(context.Context, int) (commandmaintenance.BatchResult, error) {
	return commandmaintenance.BatchResult{}, nil
}
func (*otherDomains) Recover(context.Context, int) (commandmaintenance.BatchResult, error) {
	return commandmaintenance.BatchResult{}, nil
}
func (*otherDomains) Retain(context.Context, commandmaintenance.Policy) (int64, error) { return 0, nil }
func (*otherDomains) CleanOldDryRuns(context.Context, commandmaintenance.Policy) (int64, error) {
	return 0, nil
}
func (*otherDomains) CleanOrphanChat(context.Context, commandmaintenance.Policy) (int64, error) {
	return 0, nil
}
func (*otherDomains) CleanOldChat(context.Context, commandmaintenance.Policy) (int64, error) {
	return 0, nil
}
func (p *otherDomains) Compact(context.Context, int64) error { p.compactions++; return nil }

func TestCoordinatorComposesRealReceiptAndInvocationRecovery(t *testing.T) {
	ctx := context.Background()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "recovery.db")), &gorm.Config{})
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
	ledger, err := commandledger.New(db, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	decisions, err := commanddecision.New(db, acceptDecision{}, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	core, err := commandsecurity.NewEpochService(&commandsecurity.DispatchGate{})
	if err != nil {
		t.Fatal(err)
	}
	id := func() string { return uuid.Must(uuid.NewV7()).String() }
	var requests []commandledger.LocalReadRequest
	for range 2 {
		user, session := id(), id()
		epoch, err := core.CaptureAuthenticated(ctx, func(context.Context) (string, string, error) { return user, session, nil })
		if err != nil {
			t.Fatal(err)
		}
		now := time.Now().UTC()
		r := commandledger.LocalReadRequest{InvocationID: id(), Owner: commandledger.Owner{UserID: user, AuthContextID: session}, AuthGeneration: epoch.AuthGeneration, SecurityGeneration: epoch.SecurityGeneration, RegistryVersion: "v1", GlobalConfigGeneration: "v1", ActiveLayersGeneration: "v1", CommandID: "maintenance.read", SourceType: "palette", ArgumentsFingerprint: strings.Repeat("a", 64), RequestFingerprintVersion: "v1", RequestFingerprint: strings.Repeat("b", 64), CorrelationID: id(), ReceivedAt: now, ExpiresAt: now.Add(time.Hour)}
		if _, err := ledger.Reserve(ctx, r); err != nil {
			t.Fatal(err)
		}
		requests = append(requests, r)
		status, err := decisions.Decide(ctx, commanddecision.Request{SubjectType: "invocation", DecisionID: id(), MutationID: r.InvocationID, UserID: user, SessionID: session, Fingerprint: r.RequestFingerprint, AuthGeneration: epoch.AuthGeneration, SecurityGeneration: epoch.SecurityGeneration, ExpiresAt: now.Add(time.Minute), Body: "teste"})
		if err != nil || status != commanddecision.Accepted {
			t.Fatalf("decisão=%s %v", status, err)
		}
	}
	proof, err := core.CloseAndDrain(ctx)
	if err != nil {
		t.Fatal(err)
	}
	dr, err := commanddecision.NewCoordinatorRecovery(decisions, proof)
	if err != nil {
		t.Fatal(err)
	}
	lr, err := commandledger.NewCoordinatorRecovery(ledger, proof)
	if err != nil {
		t.Fatal(err)
	}
	other := &otherDomains{}
	c, err := commandmaintenance.New(commandmaintenance.Ports{Outbox: other, Decisions: dr, Invocations: lr, Claims: other, Jobs: other, Tools: other, InvocationDB: other, Activations: other, Compaction: other})
	if err != nil {
		t.Fatal(err)
	}
	p := commandmaintenance.Policy{InvocationRetention: time.Hour, InvocationsPerUser: 1, InvocationsSystemKeep: 1, ActivationRetention: time.Hour, ActivationsPerUser: 1, LeaseDuration: time.Minute, BatchSize: 1}
	first, err := c.Run(ctx, p)
	if err != nil || first.Recovered != 2 || !first.MoreRecovery || other.compactions != 0 {
		t.Fatalf("primeira=%+v compact=%d err=%v", first, other.compactions, err)
	}
	second, err := c.Run(ctx, p)
	if err != nil || second.Recovered != 2 || second.MoreRecovery || other.compactions != 1 {
		t.Fatalf("segunda=%+v compact=%d err=%v", second, other.compactions, err)
	}
	for _, r := range requests {
		got, err := ledger.Get(ctx, r.Owner, r.InvocationID)
		if err != nil || got.Status != commandledger.OutcomeUnknown {
			t.Fatalf("invocação=%+v %v", got, err)
		}
	}
	var cancelled int64
	if err := db.Table("command_decision_receipts").Where("status = ?", commanddecision.Cancelled).Count(&cancelled).Error; err != nil || cancelled != 2 {
		t.Fatalf("receipts=%d %v", cancelled, err)
	}
}
