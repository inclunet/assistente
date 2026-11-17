package workspace

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestWithCommandSnapshotKeepsSnapshotAndBlocksWriter(t *testing.T) {
	manager := commandSnapshotManager(t)
	want, err := manager.CommandSnapshot()
	if err != nil {
		t.Fatal(err)
	}

	entered := make(chan struct{})
	release := make(chan struct{})
	mismatch := make(chan bool, 1)
	writerDone := make(chan bool, 1)
	callbackErr := errors.New("callback failed")
	callbackDone := make(chan error, 1)
	go func() {
		callbackDone <- manager.WithCommandSnapshot(context.Background(), func(got CommandSnapshot) error {
			mismatch <- got != want
			close(entered)
			<-release
			mismatch <- got != want
			return callbackErr
		})
	}()
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("callback não iniciou")
	}
	if <-mismatch {
		t.Fatal("snapshot mudou durante callback")
	}

	go func() { writerDone <- manager.SetDefaultProfile("profile-b") }()
	select {
	case changed := <-writerDone:
		t.Fatalf("SetDefaultProfile atravessou read lock: %v", changed)
	case <-time.After(50 * time.Millisecond):
	}
	close(release)
	if err := <-callbackDone; !errors.Is(err, callbackErr) {
		t.Fatalf("erro do callback=%v, want %v", err, callbackErr)
	}
	if <-mismatch {
		t.Fatal("snapshot deixou de ser estável")
	}
	select {
	case changed := <-writerDone:
		if !changed {
			t.Fatal("SetDefaultProfile não aplicou alteração após release")
		}
	case <-time.After(time.Second):
		t.Fatal("writer não foi liberado")
	}
}

func TestWithCommandSnapshotCancellationAndNilInputsRelease(t *testing.T) {
	manager := commandSnapshotManager(t)
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if err := manager.WithCommandSnapshot(canceled, func(CommandSnapshot) error {
		t.Fatal("callback executado com contexto cancelado")
		return nil
	}); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelamento=%v, want context.Canceled", err)
	}
	if err := manager.WithCommandSnapshot(context.Background(), func(CommandSnapshot) error {
		return errors.New("callback error")
	}); err == nil {
		t.Fatal("erro do callback foi perdido")
	}
	manager.SetDefaultProfile("profile-b")

	if err := (*Manager)(nil).WithCommandSnapshot(context.Background(), func(CommandSnapshot) error { return nil }); !errors.Is(err, ErrCommandSnapshotNilManager) {
		t.Fatalf("manager nil=%v", err)
	}
	if err := manager.WithCommandSnapshot(nil, func(CommandSnapshot) error { return nil }); !errors.Is(err, ErrCommandSnapshotNilContext) { //nolint:staticcheck // Verifica a rejeição explícita de contexto nil.
		t.Fatalf("context nil=%v", err)
	}
	if err := manager.WithCommandSnapshot(context.Background(), nil); !errors.Is(err, ErrCommandSnapshotNilCallback) {
		t.Fatalf("callback nil=%v", err)
	}
}
