package app

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestCommandLayerHandoffRefusesCancelledAdmission(t *testing.T) {
	parent, cancel := context.WithCancel(context.Background())
	cancel()
	handoff := newCommandLayerMutationHandoff(parent)
	defer handoff.Dispose()
	if err := handoff.Claim(); err == nil {
		t.Fatal("admissão cancelada transferiu o commit")
	}
	select {
	case <-handoff.Context().Done():
	case <-time.After(time.Second):
		t.Fatal("cancelamento anterior não encerrou a operação")
	}
}

func TestCommandLayerHandoffKeepsBoundedCommitAfterOwnInvalidation(t *testing.T) {
	parent, cancel := context.WithCancel(context.Background())
	defer cancel()
	handoff := newCommandLayerMutationHandoff(parent)
	defer handoff.Dispose()
	if err := handoff.Claim(); err != nil {
		t.Fatal(err)
	}
	cancel()
	handoff.Cancel()
	if err := handoff.Context().Err(); err != nil {
		t.Fatalf("invalidação posterior cancelou o commit adquirido: %v", err)
	}
	deadline, ok := handoff.Context().Deadline()
	if !ok || time.Until(deadline) <= 0 || time.Until(deadline) > commandLayerMutationHandoffTimeout {
		t.Fatalf("commit sem prazo limitado: %v %v", deadline, ok)
	}
}

func TestCommandLayerHandoffCancelAndClaimRaceHasSingleWinner(t *testing.T) {
	for i := 0; i < 100; i++ {
		parent, cancel := context.WithCancel(context.Background())
		handoff := newCommandLayerMutationHandoff(parent)
		start := make(chan struct{})
		var claimErr error
		var group sync.WaitGroup
		group.Add(2)
		go func() { defer group.Done(); <-start; handoff.Cancel() }()
		go func() { defer group.Done(); <-start; claimErr = handoff.Claim() }()
		close(start)
		group.Wait()
		if claimErr == nil && handoff.Context().Err() != nil {
			t.Error("claim confirmado perdeu seu contexto para cancelamento concorrente")
		}
		if claimErr != nil && handoff.Context().Err() == nil {
			t.Error("claim recusado manteve contexto executável após cancelamento")
		}
		cancel()
		handoff.Dispose()
	}
}
