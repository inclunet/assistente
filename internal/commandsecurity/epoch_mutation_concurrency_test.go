package commandsecurity

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestEpochAdmissionWaitsForCoordinatedMutationAndRejectsOldSnapshot(t *testing.T) {
	service := newEpochServiceForTest(t)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	snapshot, err := service.Capture(ctx, testEpochID(t), testEpochID(t))
	if err != nil {
		t.Fatal(err)
	}
	entered, release := make(chan struct{}), make(chan struct{})
	defer func() {
		select {
		case <-release:
		default:
			close(release)
		}
	}()
	mutation := make(chan error, 1)
	go func() {
		mutation <- service.MutatePrincipal(ctx, snapshot.UserID, snapshot.SessionID, func() error {
			close(entered)
			select {
			case <-release:
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		})
	}()
	select {
	case <-entered:
	case <-ctx.Done():
		t.Fatal("mutação não iniciou")
	}
	if service.gate.mu.TryRLock() {
		service.gate.mu.RUnlock()
		t.Fatal("mutação sem exclusão")
	}
	admission := make(chan error, 1)
	go func() {
		admission <- service.Admit(ctx, snapshot, func(context.Context) error { return errors.New("revalidação inesperada de snapshot antigo") }, func() error { return errors.New("handoff indevido") })
	}()
	close(release)
	select {
	case err := <-mutation:
		if err != nil {
			t.Fatal(err)
		}
	case <-ctx.Done():
		t.Fatal("mutação não terminou")
	}
	select {
	case err := <-admission:
		if !errors.Is(err, ErrStaleEpoch) {
			t.Fatal(err)
		}
	case <-ctx.Done():
		t.Fatal("admissão não terminou")
	}
}
