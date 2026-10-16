package app

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"assistente/internal/commandcontract"
	"assistente/internal/commanddecision"
	"assistente/internal/commandledger"
	"assistente/internal/commandsecurity"
	"assistente/internal/database"
	"github.com/google/uuid"
)

// commandMaintenancePendingSeed é a saída do fixture consumida pelo teste de
// restart. Os IDs são persistidos por APIs reais; os helpers de asserção leem
// as duas autoridades sem depender de modelos internos dos pacotes.
type commandMaintenancePendingSeed struct {
	CurrentUserID      string
	OtherUserID        string
	ReceiptIDs         []string
	InvocationIDs      []string
	SystemInvocationID string
	OldTerminalID      string
	CurrentEpoch       commandsecurity.EpochSnapshot
	OtherEpoch         commandsecurity.EpochSnapshot
}

type commandMaintenanceTestPresenter struct{}

func (commandMaintenanceTestPresenter) Present(_ context.Context, request commanddecision.Request) (commanddecision.Response, error) {
	return commanddecision.Response{DecisionID: request.DecisionID, ActionID: commanddecision.ApplyAction}, nil
}

// seedCommandMaintenancePending cria somente estado persistido pendente e o
// ledger terminal usado pelo restart; não executa handlers nem efeitos de
// produto. Gerações desconhecidas devem ser adicionadas somente depois do boot,
// pois uma pendência desconhecida corretamente bloqueia a publicação.
func seedCommandMaintenancePending(t *testing.T, a *App) commandMaintenancePendingSeed {
	t.Helper()
	ctx := context.Background()
	if a == nil || a.commandEpochs == nil || a.currentAuthUser == nil || a.currentUserID == "" {
		t.Fatal("fixture de manutenção sem core/usuário atual")
	}
	db := database.DB()
	store, err := commandledger.New(db, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	decisions, err := commanddecision.New(db, commandMaintenanceTestPresenter{}, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	currentEpoch, err := a.commandEpochs.Capture(ctx, a.currentUserID, a.currentAuthUser.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	otherUser := database.User{Username: "maintenance-pending-other", PasswordHash: "fixture", IsActive: true, Role: database.UserRoleUser}
	if err := db.Create(&otherUser).Error; err != nil {
		t.Fatal(err)
	}
	otherSession, err := a.sessionSvc.IssueSession(ctx, &otherUser, "maintenance-pending")
	if err != nil {
		t.Fatal(err)
	}
	otherEpoch, err := a.commandEpochs.Capture(ctx, otherUser.ID, otherSession.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	seed := commandMaintenancePendingSeed{CurrentUserID: a.currentUserID, OtherUserID: otherUser.ID, CurrentEpoch: currentEpoch, OtherEpoch: otherEpoch}
	seedCurrent := seedCommandMaintenanceUser(t, store, decisions, currentEpoch, "maintenance-pending-current")
	seedOther := seedCommandMaintenanceUser(t, store, decisions, otherEpoch, "maintenance-pending-other")
	seed.ReceiptIDs = []string{seedCurrent.receiptID, seedOther.receiptID}
	seed.InvocationIDs = []string{seedCurrent.invocationID, seedOther.invocationID}

	seed.SystemInvocationID = seedCommandMaintenanceSystem(t, store, currentEpoch.SecurityGeneration)
	seed.OldTerminalID = seedCommandMaintenanceOldTerminal(t, store, currentEpoch)

	seed.assertBeforeRestart(t)
	return seed
}

type commandMaintenanceUserSeed struct{ receiptID, invocationID string }

func seedCommandMaintenanceUser(t *testing.T, store *commandledger.Store, decisions *commanddecision.Store, epoch commandsecurity.EpochSnapshot, label string) commandMaintenanceUserSeed {
	t.Helper()
	invocationID := uuid.Must(uuid.NewV7()).String()
	now := time.Now().UTC()
	request := commandledger.LocalReadRequest{
		InvocationID: invocationID, Owner: commandledger.Owner{UserID: epoch.UserID, AuthContextID: epoch.SessionID},
		AuthGeneration: epoch.AuthGeneration, SecurityGeneration: epoch.SecurityGeneration,
		RegistryVersion: commandProductRegistryVersion, GlobalConfigGeneration: "global", ActiveLayersGeneration: "layers",
		CommandID: commandProductWorkspaceListID, SourceType: "palette", ArgumentsFingerprint: repeatedHex("a"),
		RequestFingerprintVersion: "v1", RequestFingerprint: repeatedHex("b"), CorrelationID: uuid.Must(uuid.NewV7()).String(),
		ReceivedAt: now, ExpiresAt: now.Add(time.Hour),
	}
	if _, err := store.Reserve(context.Background(), request); err != nil {
		t.Fatalf("reserve %s: %v", label, err)
	}
	receiptID := uuid.Must(uuid.NewV7()).String()
	status, err := decisions.Decide(context.Background(), commanddecision.Request{
		SubjectType: "invocation", DecisionID: receiptID, MutationID: invocationID,
		UserID: epoch.UserID, SessionID: epoch.SessionID, Fingerprint: request.RequestFingerprint,
		AuthGeneration: epoch.AuthGeneration, SecurityGeneration: epoch.SecurityGeneration,
		ExpiresAt: now.Add(time.Hour), Body: label,
	})
	if err != nil || status != commanddecision.Accepted {
		t.Fatalf("receipt %s: status=%s err=%v", label, status, err)
	}
	return commandMaintenanceUserSeed{receiptID: receiptID, invocationID: invocationID}
}

func seedCommandMaintenanceInvocation(t *testing.T, store *commandledger.Store, epoch commandsecurity.EpochSnapshot, label string) string {
	t.Helper()
	id := uuid.Must(uuid.NewV7()).String()
	now := time.Now().UTC()
	request := commandledger.LocalReadRequest{
		InvocationID: id, Owner: commandledger.Owner{UserID: epoch.UserID, AuthContextID: epoch.SessionID},
		AuthGeneration: epoch.AuthGeneration, SecurityGeneration: epoch.SecurityGeneration,
		RegistryVersion: commandProductRegistryVersion, GlobalConfigGeneration: "global", ActiveLayersGeneration: "layers",
		CommandID: commandProductWorkspaceListID, SourceType: "palette", ArgumentsFingerprint: repeatedHex("e"),
		RequestFingerprintVersion: "v1", RequestFingerprint: repeatedHex("f"), CorrelationID: uuid.Must(uuid.NewV7()).String(),
		ReceivedAt: now, ExpiresAt: now.Add(time.Hour),
	}
	if _, err := store.Reserve(context.Background(), request); err != nil {
		t.Fatalf("reserve %s: %v", label, err)
	}
	return id
}

func seedCommandMaintenanceSystem(t *testing.T, store *commandledger.Store, securityGeneration string) string {
	t.Helper()
	commandID := commandProductWorkspaceListID
	source := commandcontract.SourceSystem
	requestVersion, requestFingerprint := "v1", repeatedHex("c")
	args := json.RawMessage(`{}`)
	id := uuid.Must(uuid.NewV7()).String()
	now := time.Now().UTC()
	_, err := store.ReserveEnvelope(context.Background(), commandledger.EnvelopeRequest{
		Envelope: commandcontract.Envelope{
			Version: commandcontract.EnvelopeVersion, InvocationID: id, CommandID: &commandID, Arguments: &args,
			AuthContextType: commandcontract.AuthSystem, AuthContextID: "instance", AuthGeneration: "system-auth",
			SecurityGeneration: securityGeneration, ActorType: commandcontract.ActorAutomation, ActorID: "instance", SourceType: &source,
			BindingIDs: []string{}, RegistryVersion: commandProductRegistryVersion, CorrelationID: uuid.Must(uuid.NewV7()).String(),
			RequestFingerprintVersion: &requestVersion, RequestFingerprint: &requestFingerprint, ReceivedAt: now,
		},
		Mode: commandledger.ModeExecute, ArgumentsFingerprint: repeatedHex("d"), Risk: "low", ExpiresAt: now.Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("reserve system: %v", err)
	}
	return id
}

func seedCommandMaintenanceOldTerminal(t *testing.T, store *commandledger.Store, epoch commandsecurity.EpochSnapshot) string {
	t.Helper()
	now := time.Now().UTC()
	id := seedCommandMaintenanceInvocation(t, store, epoch, "maintenance-pending-old-terminal")
	request := commandledger.LocalReadRequest{
		InvocationID: id, Owner: commandledger.Owner{UserID: epoch.UserID, AuthContextID: epoch.SessionID},
		AuthGeneration: epoch.AuthGeneration, SecurityGeneration: epoch.SecurityGeneration,
		RegistryVersion: commandProductRegistryVersion, GlobalConfigGeneration: "global", ActiveLayersGeneration: "layers",
		CommandID: commandProductWorkspaceListID, SourceType: "palette", ArgumentsFingerprint: repeatedHex("e"),
		RequestFingerprintVersion: "v1", RequestFingerprint: repeatedHex("f"), CorrelationID: uuid.Must(uuid.NewV7()).String(),
		ReceivedAt: now, ExpiresAt: now.Add(time.Hour),
	}
	for _, transition := range [][2]commandledger.Status{{commandledger.Evaluating, commandledger.Queued}, {commandledger.Queued, commandledger.Running}, {commandledger.Running, commandledger.Succeeded}} {
		if _, err := store.CompareAndSwap(context.Background(), request.Owner, id, transition[0], transition[1]); err != nil {
			t.Fatalf("terminal %s: %v", transition[1], err)
		}
	}
	old := now.Add(-48 * time.Hour)
	if err := database.DB().Table("command_idempotency_keys").Where("invocation_id = ?", id).Updates(map[string]any{"received_at": old, "expires_at": old, "status": commandledger.Succeeded}).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.DB().Table("command_invocations").Where("invocation_id = ?", id).Updates(map[string]any{"received_at": old, "completed_at": old, "status": commandledger.Succeeded}).Error; err != nil {
		t.Fatal(err)
	}
	return id
}

func (s commandMaintenancePendingSeed) assertBeforeRestart(t *testing.T) {
	t.Helper()
	for _, id := range s.ReceiptIDs {
		assertReceiptState(t, id, commanddecision.Accepted)
	}
	for _, id := range append(append([]string{}, s.InvocationIDs...), s.SystemInvocationID) {
		var status string
		if err := database.DB().Table("command_invocations").Where("invocation_id = ?", id).Pluck("status", &status).Error; err != nil {
			t.Fatal(err)
		}
		if status != string(commandledger.Evaluating) {
			t.Fatalf("invocação %s antes do restart=%q", id, status)
		}
	}
	var oldStatus string
	if err := database.DB().Table("command_invocations").Where("invocation_id = ?", s.OldTerminalID).Pluck("status", &oldStatus).Error; err != nil {
		t.Fatal(err)
	}
	if oldStatus != string(commandledger.Succeeded) {
		t.Fatalf("terminal antigo=%q", oldStatus)
	}
}

// assertAfterRestart valida a fronteira de recovery do Gate R04.3. Receipts
// usam os estados terminais próprios do domínio; outcome_unknown é o estado
// pareado obrigatório do ledger e da auditoria de cada invocation, inclusive
// a invocation system.
func (s commandMaintenancePendingSeed) assertAfterRestart(t *testing.T) {
	t.Helper()
	for _, id := range s.ReceiptIDs {
		assertReceiptState(t, id, commanddecision.Cancelled)
	}
	for _, id := range append(append([]string{}, s.InvocationIDs...), s.SystemInvocationID) {
		var ledgerStatus, auditStatus string
		if err := database.DB().Table("command_idempotency_keys").Where("invocation_id = ?", id).Pluck("status", &ledgerStatus).Error; err != nil {
			t.Fatal(err)
		}
		if err := database.DB().Table("command_invocations").Where("invocation_id = ?", id).Pluck("status", &auditStatus).Error; err != nil {
			t.Fatal(err)
		}
		if ledgerStatus != string(commandledger.OutcomeUnknown) || auditStatus != string(commandledger.OutcomeUnknown) {
			t.Fatalf("invocation %s não reconciliada atomicamente: ledger=%q audit=%q", id, ledgerStatus, auditStatus)
		}
	}
}

func TestSeedCommandMaintenancePendingUsesRealStores(t *testing.T) {
	a := commandMaintenanceAppFixture(t)
	seedCommandMaintenancePending(t, a)
}
