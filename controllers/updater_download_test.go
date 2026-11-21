package controllers

import (
	"context"
	"testing"
	"time"
)

type deadlineUpdater struct {
	fakeUpdaterService
	apply func(context.Context) error
}

func (u *deadlineUpdater) ApplyUpdate(ctx context.Context) error { return u.apply(ctx) }

func TestUpdateDeadlineAllowsDownloadAndPreservesCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	service := &deadlineUpdater{apply: func(applyCtx context.Context) error {
		deadline, ok := applyCtx.Deadline()
		if !ok || time.Until(deadline) < 19*time.Minute {
			t.Error("controller encurta prazo de download")
		}
		cancel()
		<-applyCtx.Done()
		return applyCtx.Err()
	}}
	emitter := &recordingEmitter{}
	ctrl := newTestUpdaterController(service, emitter, nil)
	ctrl.applyUpdateWithProgress(ctx)
	if emitter.count("update:error") != 1 || emitter.count("update:completed") != 0 {
		t.Fatal("cancelamento anunciado como sucesso")
	}
}
