package controllers

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"assistente/internal/questionnaire"
	"assistente/internal/updater"
)

type fakeUpdaterService struct {
	mu    sync.Mutex
	calls int
	info  *updater.UpdateInfo
	err   error
	block bool
}

func (f *fakeUpdaterService) CheckForUpdates(ctx context.Context) (*updater.UpdateInfo, error) {
	f.mu.Lock()
	f.calls++
	info, err, block := f.info, f.err, f.block
	f.mu.Unlock()
	if block {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	return info, err
}

func (f *fakeUpdaterService) ApplyUpdate(context.Context) error { return nil }

func (f *fakeUpdaterService) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

type recordingEmitter struct {
	mu     sync.Mutex
	events []string
}

func (e *recordingEmitter) Emit(event string, _ any) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.events = append(e.events, event)
}

func (e *recordingEmitter) count(event string) int {
	e.mu.Lock()
	defer e.mu.Unlock()
	n := 0
	for _, got := range e.events {
		if got == event {
			n++
		}
	}
	return n
}

func newTestUpdaterController(service updaterService, emitter *recordingEmitter, mgr *questionnaire.Manager) *UpdaterController {
	ctrl := NewUpdaterController(UpdaterControllerConfig{
		Updater:          service,
		Emitter:          emitter,
		QuestionnaireMgr: mgr,
		AppVersion:       "1.0.0",
	})
	ctrl.startupDelay = 0
	ctrl.checkInterval = 10 * time.Millisecond
	ctrl.checkTimeout = time.Second
	return ctrl
}

func waitForCalls(t *testing.T, service *fakeUpdaterService, want int) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if service.callCount() >= want {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("checks = %d, quer pelo menos %d", service.callCount(), want)
}

func TestRunUpdateChecksFuncionaSemProvidersEPeriodicamente(t *testing.T) {
	service := &fakeUpdaterService{info: &updater.UpdateInfo{CurrentVersion: "1.0.0", LatestVersion: "1.0.0"}}
	ctrl := newTestUpdaterController(service, &recordingEmitter{}, nil)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		ctrl.RunUpdateChecks(ctx)
		close(done)
	}()

	waitForCalls(t, service, 3)
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("scheduler não encerrou após cancelamento")
	}
}

func TestRunUpdateChecksMantemGuardDeDesenvolvimento(t *testing.T) {
	service := &fakeUpdaterService{info: &updater.UpdateInfo{}}
	ctrl := newTestUpdaterController(service, &recordingEmitter{}, nil)
	ctrl.appVersion = "dev"

	ctrl.RunUpdateChecks(context.Background())

	if got := service.callCount(); got != 0 {
		t.Fatalf("checks em dev = %d, quer 0", got)
	}
}

func TestRunUpdateChecksCancelaFetchEmVoo(t *testing.T) {
	service := &fakeUpdaterService{block: true}
	ctrl := newTestUpdaterController(service, &recordingEmitter{}, nil)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		ctrl.RunUpdateChecks(ctx)
		close(done)
	}()

	waitForCalls(t, service, 1)
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("scheduler não fez join do fetch cancelado")
	}
}

func TestRequestUpdateCheckDeduplicaSinaisConcorrentes(t *testing.T) {
	service := &fakeUpdaterService{info: &updater.UpdateInfo{CurrentVersion: "1.0.0", LatestVersion: "1.0.0"}}
	ctrl := newTestUpdaterController(service, &recordingEmitter{}, nil)
	ctrl.startupDelay = time.Hour
	ctrl.checkInterval = time.Hour
	for range 20 {
		ctrl.RequestUpdateCheck()
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		ctrl.RunUpdateChecks(ctx)
		close(done)
	}()
	waitForCalls(t, service, 1)
	time.Sleep(20 * time.Millisecond)
	if got := service.callCount(); got != 1 {
		t.Fatalf("checks = %d, quer 1 para sinais agregados", got)
	}
	cancel()
	<-done
}

func TestRunUpdateChecksNaoRepetePromptDaMesmaVersao(t *testing.T) {
	service := &fakeUpdaterService{info: &updater.UpdateInfo{
		Available: true, CurrentVersion: "1.0.0", LatestVersion: "1.1.0",
	}}
	var mgr *questionnaire.Manager
	var mu sync.Mutex
	prompts := 0
	mgr = questionnaire.NewManager(func(event string, data any) {
		if event != questionnaire.EventQuestionnaire {
			return
		}
		payload := data.(map[string]any)
		mu.Lock()
		prompts++
		mu.Unlock()
		_ = mgr.Respond(payload["id"].(string), map[string]any{
			questionnaire.AnswerActionID: "later",
		}, false)
	})
	ctrl := newTestUpdaterController(service, &recordingEmitter{}, mgr)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		ctrl.RunUpdateChecks(ctx)
		close(done)
	}()

	waitForCalls(t, service, 3)
	cancel()
	<-done
	mu.Lock()
	defer mu.Unlock()
	if prompts != 1 {
		t.Fatalf("prompts = %d, quer 1 para a mesma versão", prompts)
	}
}

func TestRunUpdateChecksNaoDeduplicaPromptQueNaoPodeSerExibido(t *testing.T) {
	service := &fakeUpdaterService{info: &updater.UpdateInfo{
		Available: true, CurrentVersion: "1.0.0", LatestVersion: "1.1.0",
	}}
	ctrl := newTestUpdaterController(service, &recordingEmitter{}, nil)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		ctrl.RunUpdateChecks(ctx)
		close(done)
	}()

	waitForCalls(t, service, 3)
	cancel()
	<-done
	ctrl.stateMu.Lock()
	defer ctrl.stateMu.Unlock()
	if ctrl.promptedVersion != "" {
		t.Fatalf("versão marcada como exibida sem questionnaire: %q", ctrl.promptedVersion)
	}
}

func TestErroDeCheckEmiteFeedbackGenericoUmaVez(t *testing.T) {
	service := &fakeUpdaterService{err: errors.New("token e URL internos")}
	emitter := &recordingEmitter{}
	ctrl := newTestUpdaterController(service, emitter, nil)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		ctrl.RunUpdateChecks(ctx)
		close(done)
	}()

	waitForCalls(t, service, 3)
	cancel()
	<-done
	if got := emitter.count(updateCheckErrorEvent); got != 1 {
		t.Fatalf("%s = %d, quer 1 sem spam periódico", updateCheckErrorEvent, got)
	}
}

func TestPromptForUpdateEmiteQuestionnaireKindDecision(t *testing.T) {
	emitted := make(chan map[string]any, 1)
	mgr := questionnaire.NewManager(func(event string, data any) {
		if event == questionnaire.EventQuestionnaire {
			emitted <- data.(map[string]any)
		}
	})
	ctrl := newTestUpdaterController(&fakeUpdaterService{}, &recordingEmitter{}, mgr)
	done := make(chan struct{})
	go func() {
		ctrl.PromptForUpdate(context.Background(), &updater.UpdateInfo{
			CurrentVersion: "1.0.0",
			LatestVersion:  "1.1.0",
		})
		close(done)
	}()

	payload := <-emitted
	if got := payload["kind"]; got != questionnaire.KindDecision {
		t.Fatalf("kind = %v, quer %q", got, questionnaire.KindDecision)
	}
	actions, ok := payload["actions"].([]questionnaire.DecisionAction)
	if !ok || len(actions) != 2 || actions[0].ID != "update" || actions[1].ID != "later" {
		t.Fatalf("actions = %#v, quer ações diretas update/later do AEP-0091", payload["actions"])
	}
	if err := mgr.Respond(payload["id"].(string), map[string]any{
		questionnaire.AnswerActionID: "later",
	}, false); err != nil {
		t.Fatal(err)
	}
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("PromptForUpdate não encerrou após resposta")
	}
}
