package app

import (
	"context"
	"testing"
	"time"

	"assistente/internal/commandjobevents"
	"assistente/internal/database"
	"assistente/internal/eventctx"
	"assistente/internal/jobs"
	"assistente/internal/tasklist"
	"github.com/google/uuid"
)

func tasklistOriginNegativeFixture(t *testing.T) (*App, context.Context, *jobs.Job, *liveCommandToolImpl) {
	t.Helper()
	a := commandJobPublicationApp(t)
	ctx := database.WithUserID(context.Background(), a.currentUserID)
	if err := database.DB().AutoMigrate(&database.TaskListWorkflow{}, &database.TaskList{}, &database.Task{}, &database.TaskNote{}); err != nil {
		t.Fatal(err)
	}
	a.taskSvc = tasklist.NewService(tasklist.ServiceConfig{Store: tasklist.NewDBStore(), Emitter: tasklistOriginNoopEmitter{}})
	tool := &liveCommandToolImpl{name: "tasklist-negative-" + uuid.Must(uuid.NewV7()).String(), started: make(chan struct{}), release: make(chan struct{})}
	a.toolRegistry.MustRegister(tool)
	t.Cleanup(func() { tool.releaseOnce.Do(func() { close(tool.release) }) })
	job := createTasklistOriginJob(t, a, ctx, tool.Name())
	a.wireTaskListDomainEvents()
	if err := a.jobMgr.Start(); err != nil {
		t.Fatal(err)
	}
	if !a.jobMgr.HasDomainListener("tasklist.list.created") {
		t.Fatal("listener produtivo ausente")
	}
	return a, ctx, job, tool
}

func TestTasklistServicePublicProvenanceDoesNotMintInternalAuthority(t *testing.T) {
	a, ctx, job, tool := tasklistOriginNegativeFixture(t)
	ctx = eventctx.With(ctx, eventctx.Provenance{Source: "user", ChainID: "untrusted-chain", ChainHistory: []string{}})
	if _, err := a.taskSvc.CreateTaskList(ctx, "Origem pública", "teste", nil, "public-origin"); err != nil {
		t.Fatal(err)
	}
	select {
	case <-tool.started:
	case <-time.After(5 * time.Second):
		t.Fatal("job legado não recebeu a mutação real")
	}
	run := waitTasklistOriginRun(t, job.DatabaseID)
	if run.RootOriginType != "unknown" || run.RootOriginID != "" {
		t.Fatalf("eventctx público ganhou autoridade interna: %+v", run)
	}
	tool.releaseOnce.Do(func() { close(tool.release) })
	waitTasklistOriginRunCompleted(t, job.DatabaseID, run.ID)
	var count int64
	if err := database.DB().Model(&commandjobevents.ActivationOutbox{}).Where("run_id = ?", run.ID).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("origem pública produziu %d fatos ativáveis", count)
	}
}

func TestTasklistServiceRevokedSessionDoesNotPublishAuthenticatedEvent(t *testing.T) {
	a, ctx, job, tool := tasklistOriginNegativeFixture(t)
	// Revogação durável no banco isolado do fixture, como nos testes de sessão
	// do dispatcher; não altera identidade nem banco pessoal do desenvolvedor.
	if err := database.DB().Exec("UPDATE sessions SET revoked_at = CURRENT_TIMESTAMP WHERE id = ?", a.currentAuthUser.SessionID).Error; err != nil {
		t.Fatal(err)
	}
	if _, _, release, err := a.captureCommandJobIdentity(ctx); err == nil {
		if release != nil {
			release()
		}
		t.Fatal("captura aceitou sessão revogada")
	}
	created, err := a.taskSvc.CreateTaskList(ctx, "Mutação best effort", "teste", nil, "revoked-origin")
	if err != nil || created == nil {
		t.Fatalf("falha de publicação desfez mutação: list=%+v err=%v", created, err)
	}
	select {
	case <-tool.started:
		t.Fatal("sessão revogada publicou evento autenticado")
	case <-time.After(100 * time.Millisecond):
	}
	var runs int64
	if err := database.DB().Model(&database.JobRun{}).Where("job_id = ?", job.DatabaseID).Count(&runs).Error; err != nil {
		t.Fatal(err)
	}
	if runs != 0 {
		t.Fatalf("sessão revogada iniciou %d runs", runs)
	}
}
