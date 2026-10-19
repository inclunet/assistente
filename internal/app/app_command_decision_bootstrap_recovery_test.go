package app

import (
	"context"
	"errors"
	"testing"
	"time"

	"assistente/internal/commanddecision"
	"assistente/internal/commandruntime"
	"assistente/internal/database"
	"github.com/google/uuid"
)

func seedCommandBootstrapReceipt(t *testing.T, a *App, session, status string, expires time.Time) string {
	t.Helper()
	p := a.commandProduct.Load()
	epoch, err := a.commandEpochs.Capture(context.Background(), p.principal.UserID, p.principal.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	id := uuid.Must(uuid.NewV7()).String()
	row := map[string]any{
		"decision_id": id, "subject_id": uuid.Must(uuid.NewV7()).String(), "user_id": p.principal.UserID,
		"auth_context_id": session, "request_fingerprint": "bootstrap-recovery-test",
		"auth_generation": epoch.AuthGeneration, "security_generation": epoch.SecurityGeneration,
		"expires_at": expires.UnixMilli(), "status": status, "auth_context_type": "local_session",
		"subject_type": "config_mutation", "allowed_action_ids": `["apply","cancel"]`,
	}
	if status == commanddecision.Accepted {
		row["accepted_action_id"] = "apply"
		row["responded_at"] = time.Now().Add(-time.Minute).UnixMilli()
	}
	if err := database.DB().Table("command_decision_receipts").Create(row).Error; err != nil {
		t.Fatal(err)
	}
	return id
}

func commandBootstrapReceiptStatus(t *testing.T, id string) string {
	t.Helper()
	var status string
	if err := database.DB().Table("command_decision_receipts").Select("status").Where("decision_id = ?", id).Scan(&status).Error; err != nil {
		t.Fatal(err)
	}
	return status
}

func TestCommandCatalogRecoversExpiredConfigurationConfirmationsBeforeBootstrap(t *testing.T) {
	a := commandJobPublicationApp(t)
	old := a.commandProduct.Load()
	var ids []string
	for i := range 7 {
		status := commanddecision.Pending
		if i == 0 {
			status = commanddecision.Accepted
		}
		ids = append(ids, seedCommandBootstrapReceipt(t, a, old.principal.SessionID, status, time.Now().Add(-time.Hour)))
	}
	var before int64
	if err := database.DB().Table("command_config_mutations").Count(&before).Error; err != nil {
		t.Fatal(err)
	}
	if err := a.refreshCommandProductCatalog(context.Background()); err != nil {
		t.Fatalf("confirmações vencidas impediram hot-swap: %v", err)
	}
	assertCommandCatalogReadyAfterRecovery(t, a, old, "")
	for _, id := range ids {
		if status := commandBootstrapReceiptStatus(t, id); status != commanddecision.Expired {
			t.Fatalf("recibo não expirou: %s", status)
		}
	}
	// Repetir bootstrap não reaplica a configuração nem duplica evento de expiração.
	if err := a.refreshCommandProductCatalog(context.Background()); err != nil {
		t.Fatal(err)
	}
	var after, events int64
	if err := database.DB().Table("command_config_mutations").Count(&after).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.DB().Table("command_decision_receipt_events").Where("decision_id IN ?", ids).Count(&events).Error; err != nil {
		t.Fatal(err)
	}
	if after != before || events != 7 {
		t.Fatalf("recuperação repetiu efeito/evento: mutations=%d→%d events=%d", before, after, events)
	}
}

func TestCurrentCommandDecisionRecoveryPreservesLiveAndForeignConfirmations(t *testing.T) {
	a := commandJobPublicationApp(t)
	p := a.commandProduct.Load()
	expired := seedCommandBootstrapReceipt(t, a, p.principal.SessionID, commanddecision.Pending, time.Now().Add(-time.Hour))
	live := seedCommandBootstrapReceipt(t, a, p.principal.SessionID, commanddecision.Accepted, time.Now().Add(time.Hour))
	foreign := seedCommandBootstrapReceipt(t, a, uuid.Must(uuid.NewV7()).String(), commanddecision.Pending, time.Now().Add(-time.Hour))
	if err := a.recoverCurrentCommandDecisionSession(context.Background()); err != nil {
		t.Fatal(err)
	}
	for id, want := range map[string]string{expired: commanddecision.Expired, live: commanddecision.Accepted, foreign: commanddecision.Pending} {
		if status := commandBootstrapReceiptStatus(t, id); status != want {
			t.Fatalf("recibo alterado fora da elegibilidade: got=%s want=%s", status, want)
		}
	}
	if err := ResetCommandLifecycle(context.Background(), a, "test_recovery_preflight"); err != nil {
		t.Fatal(err)
	}
	if err := a.rebuildCommandLifecyclePersistedConfiguration(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := BootstrapCommandLifecycle(context.Background(), a); err == nil {
		t.Fatal("bootstrap ignorou confirmação válida/de outra sessão")
	}
	state, err := CommandLifecycleSnapshot(a)
	if err != nil || state.State == commandruntime.StateReady || state.Published {
		t.Fatalf("preflight não permaneceu fechado: %+v %v", state, err)
	}
}

func TestCurrentCommandDecisionRecoveryBatchesExpiredReceipts(t *testing.T) {
	a := commandJobPublicationApp(t)
	for range commanddecision.MaxRecoveryBatch + 1 {
		seedCommandBootstrapReceipt(t, a, a.commandProduct.Load().principal.SessionID, commanddecision.Pending, time.Now().Add(-time.Hour))
	}
	if err := a.recoverCurrentCommandDecisionSession(context.Background()); err != nil {
		t.Fatal(err)
	}
	var pending int64
	if err := database.DB().Table("command_decision_receipts").Where("status = ?", commanddecision.Pending).Count(&pending).Error; err != nil {
		t.Fatal(err)
	}
	if pending != 0 {
		t.Fatalf("lote incompleto: %d", pending)
	}
}

func TestCurrentCommandDecisionRecoveryExpiresInvocationReceiptWithoutCompletingInvocation(t *testing.T) {
	a := commandJobPublicationApp(t)
	p := a.commandProduct.Load()
	id := seedCommandBootstrapReceipt(t, a, p.principal.SessionID, commanddecision.Accepted, time.Now().Add(-time.Hour))
	db := database.DB()
	if err := db.Table("command_decision_receipts").Where("decision_id = ?", id).Updates(map[string]any{
		"subject_type": "invocation", "subject_id": "pending-invocation",
	}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`INSERT INTO command_invocations
		(invocation_id, schema_version, auth_context_type, auth_context_id, auth_generation, security_generation,
		 registry_version, binding_ids, actor_type, actor_id, arguments_summary, arguments_fingerprint,
		 correlation_id, request_fingerprint_version, request_fingerprint, risk, policy_decision, status, received_at)
		VALUES ('pending-invocation', 1, 'system', 'instance', 'auth', 'security', 'registry', '[]',
		 'system', 'instance', '{}', 'args', 'correlation', 'v1', 'fingerprint', 'low', 'allowed', 'running', CURRENT_TIMESTAMP)`).Error; err != nil {
		t.Fatal(err)
	}
	if err := ResetCommandLifecycle(context.Background(), a, "test_receipt_recovery"); err != nil {
		t.Fatal(err)
	}
	if err := a.recoverCurrentCommandDecisionSession(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := commandBootstrapReceiptStatus(t, id); got != commanddecision.Expired {
		t.Fatalf("confirmação vencida não expirou: %s", got)
	}
	var status string
	if err := db.Table("command_invocations").Select("status").Where("invocation_id = ?", "pending-invocation").Scan(&status).Error; err != nil {
		t.Fatal(err)
	}
	if status != "running" {
		t.Fatalf("recuperação de recibo alterou invocação: %s", status)
	}
	if err := (&appCommandLifecycleRuntime{}).Recover(context.Background(), commandruntime.Generation{Value: "test"}); !errors.Is(err, errCommandRecoveryPending) {
		t.Fatalf("invocação pendente deixou de bloquear preflight: %v", err)
	}
}

func TestCurrentCommandDecisionRecoveryRejectsLockedVaultAndLogout(t *testing.T) {
	for _, locked := range []bool{true, false} {
		t.Run(map[bool]string{true: "cofre_bloqueado", false: "logout"}[locked], func(t *testing.T) {
			a := commandJobPublicationApp(t)
			p := a.commandProduct.Load()
			id := seedCommandBootstrapReceipt(t, a, p.principal.SessionID, commanddecision.Pending, time.Now().Add(-time.Hour))
			if locked {
				if err := p.host.SetVaultUnlocked(context.Background(), false); err != nil {
					t.Fatal(err)
				}
			} else {
				a.setCurrentAuthUser(nil)
			}
			if err := a.recoverCurrentCommandDecisionSession(context.Background()); err == nil {
				t.Fatal("recuperação sem sessão/cofre válidos")
			}
			if status := commandBootstrapReceiptStatus(t, id); status != commanddecision.Pending {
				t.Fatalf("recibo alterado sem autoridade: %s", status)
			}
		})
	}
}
