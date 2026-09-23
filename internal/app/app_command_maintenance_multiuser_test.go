package app

import (
	"context"
	"errors"
	"testing"
	"time"

	"assistente/internal/commandactivation"
	"assistente/internal/commandjobactivation"
	"assistente/internal/database"
	"assistente/internal/workspace"
	"github.com/google/uuid"
)

func TestCommandMaintenanceRecoversGlobalClaimsAcrossUsersWithUnavailableWorkspace(t *testing.T) {
	a := commandMaintenanceAppFixture(t)
	ctx := context.Background()
	if err := a.configureCommandMaintenance(ctx); err != nil {
		t.Fatal(err)
	}
	mounted := a.commandMaintenance.Load()
	if mounted == nil || mounted.consumer == nil || mounted.ports.Claims == nil {
		t.Fatal("montagem real de claims ausente")
	}
	if a.currentUserID == "" || a.currentAuthUser == nil {
		t.Fatal("fixture não publicou usuário/sessão ativos")
	}
	activeUser := a.currentUserID
	activeSession := a.currentAuthUser.SessionID
	foreignUser := uuid.Must(uuid.NewV7()).String()
	seedClaimsAndLeases(t, 7, activeUser, foreignUser)
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := mounted.ports.Claims.Recover(cancelled, 2); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelamento: %v", err)
	}

	const batch = 2
	processed := 0
	passes := 0
	for {
		result, err := mounted.ports.Claims.Recover(ctx, batch)
		if err != nil {
			t.Fatalf("recuperação multiusuário na passagem %d: %v", passes+1, err)
		}
		processed += result.Processed
		passes++
		if passes == 1 {
			// Falha apenas no banco temporário: a página seguinte deve fazer
			// rollback sem perder a continuação dos dois itens já confirmados.
			db := database.DB()
			if err := db.Exec("CREATE TEMP TRIGGER fail_claim_recovery BEFORE UPDATE ON command_layer_activation_state BEGIN SELECT RAISE(ABORT, 'test recovery failure'); END").Error; err != nil {
				t.Fatal(err)
			}
			failed, recoveryErr := mounted.ports.Claims.Recover(ctx, batch)
			if err := db.Exec("DROP TRIGGER fail_claim_recovery").Error; err != nil {
				t.Fatal(err)
			}
			if recoveryErr == nil || failed.Processed != 0 {
				t.Fatalf("rollback: %+v %v", failed, recoveryErr)
			}
			var remaining int64
			if err := db.Model(&commandjobactivation.Lease{}).Count(&remaining).Error; err != nil || remaining != 5 {
				t.Fatalf("leases após rollback=%d err=%v", remaining, err)
			}
		}
		if !result.More {
			break
		}
		if passes > 8 {
			t.Fatal("cursor de claims não esgotou")
		}
	}
	if processed != 7 || passes != 4 {
		t.Fatalf("paginação processed=%d passes=%d, want 7/4", processed, passes)
	}

	result, err := mounted.ports.Claims.Recover(ctx, batch)
	if err != nil || result.Processed != 0 || result.More {
		t.Fatalf("repetição após esgotar=%+v err=%v", result, err)
	}
	assertClaimsReconciled(t, 7)
	if a.currentUserID != activeUser || a.currentAuthUser == nil || a.currentAuthUser.SessionID != activeSession {
		t.Fatal("recuperação alterou login ativo")
	}

	// A manutenção continua podendo limpar uma claim sem fonte mesmo quando a
	// fonte de snapshot e a sessão corrente desaparecem. Restaure os ponteiros
	// antes do cleanup da fixture para não contaminar outros testes.
	workspaceManager := a.workspaceMgr
	authUser := a.currentAuthUser
	a.workspaceMgr = nil
	a.currentAuthUser = nil
	defer func() {
		a.workspaceMgr = workspaceManager
		a.currentAuthUser = authUser
	}()
	seedClaimsAndLeases(t, 2, activeUser, foreignUser)
	result, err = mounted.ports.Claims.Recover(ctx, batch)
	if err != nil || result.Processed != 2 || !result.More {
		t.Fatalf("contexto indisponível=%+v err=%v", result, err)
	}
	result, err = mounted.ports.Claims.Recover(ctx, batch)
	if err != nil || result.Processed != 0 || result.More {
		t.Fatalf("fim com contexto indisponível=%+v err=%v", result, err)
	}
	assertClaimsReconciled(t, 9)
	a.workspaceMgr = workspace.NewManager(t.TempDir()) // existe, mas não há snapshot inicializado
	seedClaimsAndLeases(t, 1, activeUser, foreignUser)
	result, err = mounted.ports.Claims.Recover(ctx, batch)
	if err != nil || result.Processed != 1 || result.More {
		t.Fatalf("snapshot ausente=%+v err=%v", result, err)
	}
	assertClaimsReconciled(t, 10)
}

func seedClaimsAndLeases(t *testing.T, count int, activeUser, foreignUser string) {
	t.Helper()
	db := database.DB()
	now := time.Now().UTC()
	for i := 0; i < count; i++ {
		userID := foreignUser
		if i%2 == 0 {
			userID = activeUser
		}
		activationID := uuid.Must(uuid.NewV7()).String()
		runID := uuid.Must(uuid.NewV7()).String()
		if err := db.Create(&commandactivation.Claim{
			ActivationID: activationID,
			LayerRefKind: commandactivation.UserRef, LayerRef: "layer-" + activationID,
			RuleRefKind: commandactivation.UserRef, RuleRef: "rule-" + activationID,
			UserID: userID, AuthContextType: "local_session", AuthContextID: uuid.Must(uuid.NewV7()).String(),
			AuthGeneration: "auth-seed", SecurityGeneration: "security-seed", SourceType: "job",
			State: commandactivation.StateActive, ActivatedAt: now, UpdatedAt: now,
			ExpiresAt: timePtr(now.Add(time.Hour)),
		}).Error; err != nil {
			t.Fatalf("seed claim %s: %v", userID, err)
		}
		if err := db.Create(&commandjobactivation.Lease{
			ID: uuid.Must(uuid.NewV7()).String(), ActivationID: activationID, UserID: userID,
			RunID: runID, RuntimeGeneration: "runtime-seed", ExpiresAt: now.Add(time.Hour), UpdatedAt: now,
		}).Error; err != nil {
			t.Fatalf("seed lease %s: %v", userID, err)
		}
	}
}

func assertClaimsReconciled(t *testing.T, total int) {
	t.Helper()
	db := database.DB()
	var active, inactive, leases int64
	if err := db.Model(&commandactivation.Claim{}).Where("state = ?", commandactivation.StateActive).Count(&active).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&commandactivation.Claim{}).Where("state = ?", commandactivation.StateInactive).Count(&inactive).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&commandjobactivation.Lease{}).Count(&leases).Error; err != nil {
		t.Fatal(err)
	}
	if active != 0 || inactive != int64(total) || leases != 0 {
		t.Fatalf("claims após recovery: active=%d inactive=%d leases=%d total=%d", active, inactive, leases, total)
	}
}

func timePtr(v time.Time) *time.Time { return &v }
