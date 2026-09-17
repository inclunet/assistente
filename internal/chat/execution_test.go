package chat

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestStreamingManagerBeginCancelKeepsGateUntilWorkerFinishes(t *testing.T) {
	m := NewStreamingManager(nil)
	ctx, _, finish, err := m.Begin(context.Background(), "c")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(finish)
	m.Cancel("c")
	if !errors.Is(ctx.Err(), context.Canceled) {
		t.Fatal("preparação não cancelada")
	}
	if _, err := m.PrepareConversationDeletion([]string{"c"}); !errors.Is(err, ErrConversationActive) {
		t.Fatal("cancelamento liberou gate antes do worker terminar")
	}
	started := make(chan struct{})
	parent, stop := context.WithCancel(context.Background())
	t.Cleanup(stop)
	go func() {
		_, _, nextFinish, err := m.Begin(parent, "c")
		if err == nil {
			nextFinish()
			close(started)
		}
	}()
	select {
	case <-started:
		t.Fatal("novo turno iniciou durante finalização anterior")
	case <-time.After(20 * time.Millisecond):
	}
	finish()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("fila não prosseguiu após finalização")
	}
}

func TestStreamingManagerBeginWaitingContextCanBeCancelled(t *testing.T) {
	m := NewStreamingManager(nil)
	first, _, finish, err := m.Begin(context.Background(), "c")
	if err != nil {
		t.Fatal(err)
	}
	defer finish()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, _, _, err := m.Begin(ctx, "c"); !errors.Is(err, context.Canceled) {
		t.Fatalf("erro = %v", err)
	}
	if first.Err() != nil {
		t.Fatal("espera cancelou execução ativa")
	}
	_, _, otherFinish, err := m.Begin(context.Background(), "other")
	if err != nil {
		t.Fatal(err)
	}
	otherFinish()
}

func TestCancelExecutionCancelsWaitingTurnWithoutDroppingNext(t *testing.T) {
	m := NewStreamingManager(nil)
	_, _, finishA, err := m.Begin(context.Background(), "c", "A")
	if err != nil {
		t.Fatal(err)
	}
	defer finishA()
	m.CancelExecution("c", "A")
	finishedB := make(chan error, 1)
	go func() {
		_, _, finishB, err := m.Begin(context.Background(), "c", "B")
		if err == nil {
			finishB()
		}
		finishedB <- err
	}()
	deadline := time.After(time.Second)
	for {
		m.mu.Lock()
		registered := m.executions["c"]["B"] != nil
		m.mu.Unlock()
		if registered {
			break
		}
		select {
		case <-deadline:
			t.Fatal("B não registrou cancelamento durante espera")
		case <-time.After(time.Millisecond):
		}
	}
	m.CancelExecution("c", "B")
	select {
	case err := <-finishedB:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("B deve terminar sem adquirir execução: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("B continuou aguardando após cancelamento")
	}
	finishA()
	ctxC, _, finishC, err := m.Begin(context.Background(), "c", "C")
	if err != nil {
		t.Fatal(err)
	}
	defer finishC()
	m.CancelExecution("c", "B")
	if ctxC.Err() != nil {
		t.Fatal("cancelamento atrasado de B cancelou C")
	}
}
