package commandledger

import (
	"context"
	"errors"
	"testing"
	"time"

	"assistente/internal/commandcatalog"
	"assistente/internal/commandsecurity"
)

// Harness deliberadamente só de teste. Não é CommandExecutionService: check,
// identidade e fingerprints são fixtures, não autenticação/HMAC de produto.
// Não registra comandos reais, não cobre UI, receipts, eventos ou argumentos.
type readPipelineFixture struct {
	store       *Store
	registry    *commandcatalog.Registry
	gate        *commandsecurity.DispatchGate
	check       func(context.Context) error
	afterQueued func()
	start       func(context.Context) <-chan Status
}

func newReadPipelineFixture(t *testing.T) (*readPipelineFixture, LocalReadRequest) {
	t.Helper()
	req := validRequest()
	now := req.ReceivedAt
	store, _ := testStore(t, &now)
	metadata := map[string]commandcatalog.LocalizedMetadata{}
	for _, locale := range []string{"pt-BR", "en", "es"} {
		metadata[locale] = commandcatalog.LocalizedMetadata{Name: "Fixture", Description: "Leitura de teste", Category: "Teste"}
	}
	registry, err := commandcatalog.New([]commandcatalog.Registration{{Definition: commandcatalog.Definition{ID: req.CommandID, Effect: commandcatalog.Read, Decision: commandcatalog.NoDecision, Context: commandcatalog.ContextPolicy{None: true}, AllowedSources: []commandcatalog.Source{commandcatalog.Palette}, Presentation: &commandcatalog.Presentation{Version: "1", Locales: metadata}}, Handler: commandcatalog.HandlerContract{Effect: commandcatalog.Read}}})
	if err != nil {
		t.Fatal(err)
	}
	return &readPipelineFixture{store: store, registry: registry, gate: &commandsecurity.DispatchGate{}, check: func(context.Context) error { return nil }, start: func(context.Context) <-chan Status { done := make(chan Status, 1); done <- Succeeded; return done }}, req
}

func (f *readPipelineFixture) run(ctx context.Context, req LocalReadRequest) (Record, error) {
	if err := f.check(ctx); err != nil {
		return Record{}, err
	}
	reservation, err := f.store.Reserve(ctx, req)
	if err != nil {
		return Record{}, err
	}
	if !reservation.Created {
		return reservation.Record, nil
	}
	finish := func(from, to Status) (Record, error) {
		ok, err := f.store.CompareAndSwap(ctx, req.Owner, req.InvocationID, from, to)
		if err != nil {
			return Record{}, err
		}
		if !ok {
			return Record{}, ErrConflict
		}
		return f.store.Get(ctx, req.Owner, req.InvocationID)
	}
	definition, err := f.registry.CheckReadiness(req.CommandID, commandcatalog.Source(req.SourceType))
	if err != nil || !definition.Context.None {
		return finish(Evaluating, Denied)
	}
	stale := false
	err = f.gate.WithAdmission(ctx, func() error {
		if err := f.check(ctx); err != nil {
			stale = true
			return nil
		}
		ok, err := f.store.CompareAndSwap(ctx, req.Owner, req.InvocationID, Evaluating, Queued)
		if err != nil {
			return err
		}
		if !ok {
			return ErrConflict
		}
		return nil
	})
	if err != nil {
		return Record{}, err
	}
	if stale {
		return finish(Evaluating, CancelledStale)
	}
	if f.afterQueued != nil {
		f.afterQueued()
	}
	var done <-chan Status
	err = f.gate.WithAdmission(ctx, func() error {
		if err := f.check(ctx); err != nil {
			stale = true
			return nil
		}
		ok, err := f.store.CompareAndSwap(ctx, req.Owner, req.InvocationID, Queued, Running)
		if err != nil {
			return err
		}
		if !ok {
			return ErrConflict
		}
		done = f.start(ctx)
		return nil
	})
	if err != nil {
		return Record{}, err
	}
	if stale {
		return finish(Queued, CancelledStale)
	}
	select {
	case outcome, ok := <-done:
		if !ok || (outcome != Succeeded && outcome != Failed) {
			return finish(Running, OutcomeUnknown)
		}
		return finish(Running, outcome)
	case <-ctx.Done():
		return Record{}, ctx.Err() // Persistência independente de cancelamento ainda é requisito do executor futuro.
	}
}

func TestReadPipelineRunsOnceAndReplaysDurableOutcome(t *testing.T) {
	f, req := newReadPipelineFixture(t)
	ctx := context.Background()
	calls := 0
	f.start = func(context.Context) <-chan Status {
		calls++
		done := make(chan Status, 1)
		done <- Succeeded
		return done
	}
	first, err := f.run(ctx, req)
	if err != nil || first.Status != Succeeded {
		t.Fatalf("primeira: %+v %v", first, err)
	}
	second, err := f.run(ctx, req)
	if err != nil || second.ID != first.ID || second.Status != Succeeded || calls != 1 {
		t.Fatalf("replay: %+v %v calls=%d", second, err, calls)
	}
}

func TestReadPipelineLostOutcomeRequiresReconciliationNotRetry(t *testing.T) {
	f, req := newReadPipelineFixture(t)
	ctx := context.Background()
	calls := 0
	f.start = func(context.Context) <-chan Status {
		calls++
		done := make(chan Status)
		close(done)
		return done
	}
	record, err := f.run(ctx, req)
	if err != nil || record.Status != OutcomeUnknown {
		t.Fatalf("outcome perdido: %+v %v", record, err)
	}
	if _, err := f.run(ctx, req); err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatal("replay repetiu handoff incerto")
	}
	verified := 0
	ok, err := f.store.Reconcile(ctx, req.Owner, req.InvocationID, func(context.Context, Record) (Status, error) {
		verified++
		return Succeeded, nil // Fonte conclusiva fictícia, não handler real.
	})
	if err != nil || !ok {
		t.Fatalf("reconciliação: %v %v", ok, err)
	}
	record, err = f.run(ctx, req)
	if err != nil || record.Status != Succeeded || calls != 1 || verified != 1 {
		t.Fatalf("replay reconciliado: %+v %v", record, err)
	}
}

func TestReadPipelineReleasesGateWhileAwaitingOutcome(t *testing.T) {
	f, req := newReadPipelineFixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	started := make(chan struct{})
	done := make(chan Status, 1)
	result := make(chan error, 1)
	f.start = func(context.Context) <-chan Status { close(started); return done }
	go func() {
		record, err := f.run(ctx, req)
		if err == nil && record.Status != Succeeded {
			err = errors.New("estado final inesperado")
		}
		result <- err
	}()
	select {
	case <-started:
	case <-ctx.Done():
		t.Fatal("handoff não ocorreu")
	}
	mutation := make(chan error, 1)
	go func() { mutation <- f.gate.WithMutation(ctx, func() error { return nil }) }()
	select {
	case err := <-mutation:
		if err != nil {
			t.Fatal(err)
		}
	case <-ctx.Done():
		t.Fatal("gate permaneceu preso durante espera")
	}
	done <- Succeeded
	select {
	case err := <-result:
		if err != nil {
			t.Fatal(err)
		}
	case <-ctx.Done():
		t.Fatal("leitura não concluiu")
	}
}
