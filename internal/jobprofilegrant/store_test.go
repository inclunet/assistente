package jobprofilegrant

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"assistente/internal/database"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func grantTestStore(t *testing.T) (*Store, *gorm.DB, context.Context, context.Context, database.Job, database.Job) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:grants-%s?mode=memory&cache=shared&_busy_timeout=5000", t.Name())), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&database.Job{}, &database.JobProfileGrant{}); err != nil {
		t.Fatal(err)
	}
	inputs := `{"profile":"especialista","prompt":"texto editorial"}`
	jobA := database.Job{UserID: "user-a", Slug: "job-a", Name: "Job A", Enabled: true, ToolCatalogID: "subagent", ToolName: ToolSubagent, Inputs: inputs}
	jobB := database.Job{UserID: "user-b", Slug: "job-a", Name: "Job B", Enabled: true, ToolCatalogID: "subagent", ToolName: ToolSubagent, Inputs: inputs}
	if err := db.Create(&jobA).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&jobB).Error; err != nil {
		t.Fatal(err)
	}
	return NewStore(db), db,
		database.WithUserID(context.Background(), "user-a"),
		database.WithUserID(context.Background(), "user-b"),
		jobA, jobB
}

func TestFingerprintDeterministicAndSecurityScoped(t *testing.T) {
	inputsA := map[string]any{"profile": "{{ .event.profile }}", "prompt": "A", "background": true}
	inputsB := map[string]any{"background": false, "prompt": "B", "profile": "{{ .event.profile }}"}
	a, okA := FingerprintForInputs(ToolSubagent, inputsA)
	b, okB := FingerprintForInputs(ToolSubagent, inputsB)
	if !okA || !okB || a != b {
		t.Fatalf("mudança editorial alterou fingerprint: %q != %q", a, b)
	}
	c, _ := FingerprintForInputs(ToolSubagent, map[string]any{"profile": "{{ .event.outro }}"})
	if a == c {
		t.Fatal("mudança da expressão de profile deveria alterar fingerprint")
	}
	if _, ok := FingerprintForInputs("outra_tool", inputsA); ok {
		t.Fatal("tool diferente não pode produzir fingerprint delegável")
	}
}

func TestStoreExactIsolationIdempotencyAndRevocation(t *testing.T) {
	store, db, userA, userB, jobA, jobB := grantTestStore(t)
	configA, err := store.CurrentDelegation(userA, jobA.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Grant(userA, jobA.ID, "especialista", configA.Fingerprint, "desktop"); err != nil {
		t.Fatal(err)
	}
	if err := store.Grant(userA, jobA.ID, "especialista", configA.Fingerprint, "desktop"); err != nil {
		t.Fatal(err)
	}
	var count int64
	if err := db.Model(&database.JobProfileGrant{}).Where("user_id = ?", "user-a").Count(&count).Error; err != nil || count != 1 {
		t.Fatalf("grant idempotente: count=%d err=%v", count, err)
	}
	valid, err := store.HasValid(userA, jobA.ID, "especialista", configA.Fingerprint)
	if err != nil || !valid {
		t.Fatalf("grant exato deveria valer: valid=%v err=%v", valid, err)
	}
	if valid, _ := store.HasValid(userA, jobA.ID, "outro", configA.Fingerprint); valid {
		t.Fatal("target diferente herdou grant")
	}
	configB, err := store.CurrentDelegation(userB, jobB.ID)
	if err != nil {
		t.Fatal(err)
	}
	if valid, _ := store.HasValid(userB, jobB.ID, "especialista", configB.Fingerprint); valid {
		t.Fatal("outro usuário herdou grant")
	}
	clone := database.Job{
		UserID: "user-a", Slug: "job-a-copia", Name: "Cópia", ToolCatalogID: "subagent",
		ToolName: ToolSubagent, Inputs: jobA.Inputs,
	}
	if err := db.Create(&clone).Error; err != nil {
		t.Fatal(err)
	}
	cloneConfig, err := store.CurrentDelegation(userA, clone.ID)
	if err != nil {
		t.Fatal(err)
	}
	if valid, _ := store.HasValid(userA, clone.ID, "especialista", cloneConfig.Fingerprint); valid {
		t.Fatal("clone herdou grant do job original")
	}
	if err := store.Revoke(userA, jobA.ID, "especialista", "usuário"); err != nil {
		t.Fatal(err)
	}
	if valid, _ := store.HasValid(userA, jobA.ID, "especialista", configA.Fingerprint); valid {
		t.Fatal("grant revogado permaneceu válido")
	}
	var revokedJob database.Job
	if err := db.First(&revokedJob, "id = ?", jobA.ID).Error; err != nil || revokedJob.Enabled {
		t.Fatalf("job sem grant deveria ser desabilitado: enabled=%v err=%v", revokedJob.Enabled, err)
	}
}

func TestStoreRevokesStaleFingerprint(t *testing.T) {
	store, db, userA, _, jobA, _ := grantTestStore(t)
	config, _ := store.CurrentDelegation(userA, jobA.ID)
	if err := store.Grant(userA, jobA.ID, "especialista", config.Fingerprint, "desktop"); err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&database.Job{}).Where("id = ?", jobA.ID).Update("inputs", `{"profile":"{{ .event.profile }}"}`).Error; err != nil {
		t.Fatal(err)
	}
	next, err := store.CurrentDelegation(userA, jobA.ID)
	if err != nil {
		t.Fatal(err)
	}
	valid, err := store.HasValid(userA, jobA.ID, "especialista", next.Fingerprint)
	if err != nil || valid {
		t.Fatalf("grant stale não pode valer: valid=%v err=%v", valid, err)
	}
	var revoked int64
	if err := db.Model(&database.JobProfileGrant{}).Where("job_id = ? AND revoked_at IS NOT NULL", jobA.ID).Count(&revoked).Error; err != nil || revoked != 1 {
		t.Fatalf("stale não foi auditado como revogado: count=%d err=%v", revoked, err)
	}
}

func TestStoreProfileRemovalRevokesEveryUser(t *testing.T) {
	store, _, userA, userB, jobA, jobB := grantTestStore(t)
	configA, _ := store.CurrentDelegation(userA, jobA.ID)
	configB, _ := store.CurrentDelegation(userB, jobB.ID)
	if err := store.Grant(userA, jobA.ID, "especialista", configA.Fingerprint, "desktop"); err != nil {
		t.Fatal(err)
	}
	if err := store.Grant(userB, jobB.ID, "especialista", configB.Fingerprint, "desktop"); err != nil {
		t.Fatal(err)
	}
	if err := store.RevokeProfileGlobal(userA, "especialista", "profile excluído"); err != nil {
		t.Fatal(err)
	}
	if valid, _ := store.HasValid(userA, jobA.ID, "especialista", configA.Fingerprint); valid {
		t.Fatal("grant do usuário A permaneceu após remoção global do profile")
	}
	if valid, _ := store.HasValid(userB, jobB.ID, "especialista", configB.Fingerprint); valid {
		t.Fatal("grant do usuário B permaneceu após remoção global do profile")
	}
	var enabled int64
	if err := store.db.Model(&database.Job{}).Where("enabled = ?", true).Count(&enabled).Error; err != nil || enabled != 0 {
		t.Fatalf("jobs sem grant após remoção deveriam ser desabilitados: enabled=%d err=%v", enabled, err)
	}
}

func TestStoreConcurrentGrantIsIdempotent(t *testing.T) {
	store, db, userA, _, jobA, _ := grantTestStore(t)
	config, _ := store.CurrentDelegation(userA, jobA.ID)
	var wg sync.WaitGroup
	errs := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs <- store.Grant(userA, jobA.ID, "especialista", config.Fingerprint, "desktop")
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	var count int64
	if err := db.Model(&database.JobProfileGrant{}).Where("job_id = ? AND revoked_at IS NULL", jobA.ID).Count(&count).Error; err != nil || count != 1 {
		t.Fatalf("concorrência criou duplicatas: count=%d err=%v", count, err)
	}
}
