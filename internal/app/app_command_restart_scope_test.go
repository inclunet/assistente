package app

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"assistente/internal/commandcontract"
	"assistente/internal/commanddecision"
	"assistente/internal/commandinstance"
	"assistente/internal/commandledger"
	"assistente/internal/commandruntime"
	"assistente/internal/commandsecurity"
	"assistente/internal/database"
	"github.com/google/uuid"
)

func TestCommandRestartReconcilesOnlyRegisteredGenerationsAcrossOwnersAndSystem(t *testing.T) {
	ctx := context.Background()
	old := readyCommandProduct(t)
	store, err := commandledger.New(database.DB(), time.Now)
	if err != nil {
		t.Fatal(err)
	}
	oldEpoch, err := old.commandEpochs.Capture(ctx, old.currentUserID, old.currentAuthUser.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	otherUser := database.User{Username: "restart-other-owner", PasswordHash: "unused", IsActive: true, Role: database.UserRoleUser}
	if err := database.DB().Create(&otherUser).Error; err != nil {
		t.Fatal(err)
	}
	otherSession, err := old.sessionSvc.IssueSession(ctx, &otherUser, "restart-history")
	if err != nil {
		t.Fatal(err)
	}
	otherEpoch, err := old.commandEpochs.Capture(ctx, otherUser.ID, otherSession.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Millisecond)
	pendingFirst := seedRestartReceipt(t, oldEpoch, now.Add(time.Hour), commanddecision.Pending)
	acceptedFirst := seedRestartReceipt(t, oldEpoch, now.Add(-time.Hour), commanddecision.Accepted)
	pendingOther := seedRestartReceipt(t, otherEpoch, now.Add(time.Hour), commanddecision.Pending)
	acceptedOther := seedRestartReceipt(t, otherEpoch, now.Add(-time.Hour), commanddecision.Accepted)

	systemID := uuid.Must(uuid.NewV7()).String()
	requestVersion, requestFingerprint, argumentsFingerprint := "v1", repeatedHex("a"), repeatedHex("b")
	commandID := commandProductWorkspaceListID
	source := commandcontract.SourceSystem
	args := json.RawMessage(`{}`)
	systemEnvelope := commandcontract.Envelope{
		Version: commandcontract.EnvelopeVersion, InvocationID: systemID, CommandID: &commandID,
		Arguments: &args, AuthContextType: commandcontract.AuthSystem, AuthContextID: "instance",
		AuthGeneration: "system-auth", SecurityGeneration: oldEpoch.SecurityGeneration,
		ActorType: commandcontract.ActorAutomation, ActorID: "instance", SourceType: &source,
		BindingIDs: []string{}, RegistryVersion: commandProductRegistryVersion, CorrelationID: uuid.Must(uuid.NewV7()).String(),
		RequestFingerprintVersion: &requestVersion, RequestFingerprint: &requestFingerprint,
		ReceivedAt: now, ClientRequestedAt: &now,
	}
	if _, err := store.ReserveEnvelope(ctx, commandledger.EnvelopeRequest{Envelope: systemEnvelope, Mode: commandledger.ModeExecute, ArgumentsFingerprint: argumentsFingerprint, Risk: "low", ExpiresAt: now.Add(time.Hour)}); err != nil {
		t.Fatal(err)
	}

	fresh := restartCommandApp(t, old)
	if err := fresh.mountCommandProduct(ctx); !errors.Is(err, commandinstance.ErrBusy) {
		t.Fatalf("segunda instância deveria aguardar release: %v", err)
	}
	if _, err := old.commandEpochs.CloseAndDrain(ctx); err != nil {
		t.Fatal(err)
	}
	if err := old.commandEpochs.ReleaseInstance(ctx); err != nil {
		t.Fatal(err)
	}
	if err := fresh.mountCommandProduct(ctx); err != nil {
		t.Fatal(err)
	}
	if err := fresh.rebuildCommandLifecyclePersistedConfiguration(ctx); err != nil {
		t.Fatal(err)
	}
	if err := BootstrapCommandLifecycle(ctx, fresh); err != nil {
		t.Fatal(err)
	}

	assertReceiptState(t, pendingFirst, commanddecision.Cancelled)
	assertReceiptState(t, acceptedFirst, commanddecision.Expired)
	assertReceiptState(t, pendingOther, commanddecision.Cancelled)
	assertReceiptState(t, acceptedOther, commanddecision.Expired)
	var ledgerStatus, auditStatus string
	if err := database.DB().Table("command_idempotency_keys").Where("invocation_id = ?", systemID).Pluck("status", &ledgerStatus).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.DB().Table("command_invocations").Where("invocation_id = ?", systemID).Pluck("status", &auditStatus).Error; err != nil {
		t.Fatal(err)
	}
	if ledgerStatus != string(commandledger.OutcomeUnknown) || auditStatus != string(commandledger.OutcomeUnknown) {
		t.Fatalf("system não reconciliado atomicamente: ledger=%q audit=%q", ledgerStatus, auditStatus)
	}

	unknownID := uuid.Must(uuid.NewV7()).String()
	unknownEpoch := systemEnvelope
	unknownEpoch.InvocationID = unknownID
	unknownSecurity := "unknown-security-generation"
	unknownEpoch.SecurityGeneration = unknownSecurity
	if _, err := store.ReserveEnvelope(ctx, commandledger.EnvelopeRequest{Envelope: unknownEpoch, Mode: commandledger.ModeExecute, ArgumentsFingerprint: argumentsFingerprint, Risk: "low", ExpiresAt: now.Add(time.Hour)}); err != nil {
		t.Fatal(err)
	}
	if err := BootstrapCommandLifecycle(ctx, fresh); !errors.Is(err, commandruntime.ErrNotReady) {
		t.Fatalf("bootstrap deveria falhar fechado com geração desconhecida: %v", err)
	}
	var unknownStatus string
	if err := database.DB().Table("command_idempotency_keys").Where("invocation_id = ?", unknownID).Pluck("status", &unknownStatus).Error; err != nil {
		t.Fatal(err)
	}
	if unknownStatus != string(commandledger.Evaluating) {
		t.Fatalf("geração desconhecida foi alterada: %q", unknownStatus)
	}
	snapshot, err := CommandLifecycleSnapshot(fresh)
	if err != nil || snapshot.Published || snapshot.State == commandruntime.StateReady {
		t.Fatalf("bootstrap falho manteve mapa publicado: %+v %v", snapshot, err)
	}
}

func seedRestartReceipt(t *testing.T, epoch commandsecurity.EpochSnapshot, expires time.Time, state string) string {
	t.Helper()
	now := time.Now().UTC().UnixMilli()
	decisionID := uuid.Must(uuid.NewV7()).String()
	if err := database.DB().Table("command_decision_receipts").Create(map[string]any{
		"decision_id": decisionID, "subject_id": uuid.Must(uuid.NewV7()).String(), "user_id": epoch.UserID,
		"auth_context_id": epoch.SessionID, "request_fingerprint": repeatedHex("c"), "auth_generation": epoch.AuthGeneration,
		"security_generation": epoch.SecurityGeneration, "expires_at": expires.UnixMilli(), "status": state,
		"auth_context_type": "local_session", "subject_type": "invocation", "allowed_action_ids": `["apply","deny"]`,
		"accepted_action_id": func() *string {
			if state == commanddecision.Accepted {
				v := commanddecision.ApplyAction
				return &v
			}
			return nil
		}(),
		"responded_at": func() *int64 {
			if state == commanddecision.Accepted {
				return &now
			}
			return nil
		}(),
	}).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.DB().Table("command_decision_receipt_events").Create(map[string]any{"id": uuid.Must(uuid.NewV7()).String(), "decision_id": decisionID, "state": state, "occurred_ms": now}).Error; err != nil {
		t.Fatal(err)
	}
	return decisionID
}

func assertReceiptState(t *testing.T, decisionID, want string) {
	t.Helper()
	var got string
	if err := database.DB().Table("command_decision_receipts").Where("decision_id = ?", decisionID).Pluck("status", &got).Error; err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("receipt %s=%q, want %q", decisionID, got, want)
	}
	var events int64
	if err := database.DB().Table("command_decision_receipt_events").Where("decision_id = ? AND state = ?", decisionID, want).Count(&events).Error; err != nil || events != 1 {
		t.Fatalf("evento terminal ausente/duplicado: %d %v", events, err)
	}
}

func repeatedHex(char string) string {
	return strings.Repeat(char, 64)
}
