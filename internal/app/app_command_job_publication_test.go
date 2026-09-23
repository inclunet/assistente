package app

import (
	"context"
	"testing"
	"time"

	"assistente/internal/commandactivation"
	"assistente/internal/commandjobactivation"
	"assistente/internal/config"
	"assistente/internal/database"
	"assistente/internal/jobs"
)

func commandJobPublicationApp(t *testing.T) *App {
	t.Helper()
	a := commandMaintenanceAppFixture(t)
	settings := config.DefaultMaintenanceSettings()
	settings.CommandJobActivationLeaseSeconds = 3
	if err := config.SaveMaintenance(settings); err != nil {
		t.Fatal(err)
	}
	return restartCommandMaintenanceApp(t, a)
}

func TestCommandJobSourceSurvivesRealConfigurationRebuild(t *testing.T) {
	a := commandJobPublicationApp(t)
	control := startCommandMaintenanceLiveJob(t, a)
	runID := <-control.RunID
	claim, first := waitLiveClaimAndLease(t, runID)
	epoch, err := a.commandEpochs.Capture(context.Background(), a.currentUserID, a.currentAuthUser.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	command, release, err := a.commandEpochs.WatchEpoch(context.Background(), epoch)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	if err := a.rebuildCommandLifecyclePersistedConfiguration(context.Background()); err != nil {
		t.Fatal(err)
	}
	if command.Err() == nil {
		t.Fatal("rebuild deixou preparação de comando obsoleta utilizável")
	}
	authority := a.commandJobAuthority.Load()
	if authority == nil || authority.watch.Err() != nil {
		t.Fatal("rebuild cancelou a autoridade da fonte de jobs")
	}
	deadline := time.Now().Add(8 * time.Second)
	for {
		var lease commandjobactivation.Lease
		if err := database.DB().Where("activation_id = ?", claim.ActivationID).Take(&lease).Error; err != nil {
			t.Fatal(err)
		}
		if lease.ExpiresAt.After(first.ExpiresAt) && time.Now().After(first.ExpiresAt) {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("fonte não renovou após rebuild real")
		}
		time.Sleep(25 * time.Millisecond)
	}
	control.Release()
	result, err := control.Join()
	if err != nil || result == nil || result.Status != jobs.RunStatusCompleted {
		t.Fatalf("job: %+v %v", result, err)
	}
	if err := waitForNoLiveLease(claim.ActivationID); err != nil {
		t.Fatal(err)
	}
}

func TestCommandJobLiveClaimEndsWhenAuthoritativeProfileChanges(t *testing.T) {
	a := commandJobPublicationApp(t)
	if err := a.workspaceMgr.SetProfile("job-required-profile"); err != nil {
		t.Fatal(err)
	}
	control := startCommandJobWithCondition(t, a, `{"version":1,"clauses":[{"field":"profile","op":"eq","value":"job-required-profile"}]}`)
	runID := <-control.RunID
	claim, _ := waitLiveClaimAndLease(t, runID)
	if err := a.workspaceMgr.SetProfile("different-profile"); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(8 * time.Second)
	for {
		var current commandactivation.Claim
		var leases int64
		if err := database.DB().Where("activation_id = ?", claim.ActivationID).Take(&current).Error; err != nil {
			t.Fatal(err)
		}
		if err := database.DB().Model(&commandjobactivation.Lease{}).Where("activation_id = ?", claim.ActivationID).Count(&leases).Error; err != nil {
			t.Fatal(err)
		}
		if current.State == commandactivation.StateInactive && leases == 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("condição deixou claim indevida ativa: %+v leases=%d", current, leases)
		}
		time.Sleep(25 * time.Millisecond)
	}
	var running int64
	if err := database.DB().Model(&database.JobRun{}).Where("id = ? AND status = ?", runID, jobs.RunStatusRunning).Count(&running).Error; err != nil {
		t.Fatal(err)
	}
	if running != 1 {
		t.Fatal("retirar camada não deveria encerrar o job")
	}
	control.Release()
	result, err := control.Join()
	if err != nil || result == nil || result.Status != jobs.RunStatusCompleted {
		t.Fatalf("job: %+v %v", result, err)
	}
}
