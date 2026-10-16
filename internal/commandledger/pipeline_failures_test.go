package commandledger

import (
	"context"
	"errors"
	"testing"
)

func TestReadPipelineInitialCheckErrorDoesNotReserveOrStart(t *testing.T) {
	f, req := newReadPipelineFixture(t)
	checkErr := errors.New("sessão não autenticada")
	checks := 0
	f.check = func(context.Context) error {
		checks++
		return checkErr
	}
	f.start = func(context.Context) <-chan Status {
		t.Fatal("start não deveria ser chamado")
		return nil
	}

	record, err := f.run(context.Background(), req)
	if !errors.Is(err, checkErr) {
		t.Fatalf("erro inicial: record=%+v err=%v", record, err)
	}
	if checks != 1 {
		t.Fatalf("check inicial chamado %d vezes, esperado 1", checks)
	}
	if _, err := f.store.Get(context.Background(), req.Owner, req.InvocationID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("reserva inesperada: err=%v", err)
	}
}

func TestReadPipelinePermissionRevokedAfterQueueCancelsStaleWithoutStart(t *testing.T) {
	f, req := newReadPipelineFixture(t)
	revoked := false
	f.check = func(context.Context) error {
		if revoked {
			return errors.New("permissão revogada")
		}
		return nil
	}
	f.afterQueued = func() { revoked = true }
	f.start = func(context.Context) <-chan Status {
		t.Fatal("start não deveria ser chamado após revogação")
		return nil
	}

	record, err := f.run(context.Background(), req)
	if err != nil || record.Status != CancelledStale {
		t.Fatalf("revogação na fila: record=%+v err=%v", record, err)
	}
}

func TestReadPipelineUnknownCommandIsDeniedWithoutStart(t *testing.T) {
	f, req := newReadPipelineFixture(t)
	req.CommandID = "missing.read"
	f.start = func(context.Context) <-chan Status {
		t.Fatal("start não deveria ser chamado para comando desconhecido")
		return nil
	}

	record, err := f.run(context.Background(), req)
	if err != nil || record.Status != Denied {
		t.Fatalf("comando desconhecido: record=%+v err=%v", record, err)
	}
}

func TestReadPipelineCheckErrorBeforeQueueCancelsStale(t *testing.T) {
	f, req := newReadPipelineFixture(t)
	checkErr := errors.New("sessão tornou-se inválida")
	checks := 0
	f.check = func(context.Context) error {
		checks++
		if checks == 2 {
			return checkErr
		}
		return nil
	}
	f.start = func(context.Context) <-chan Status {
		t.Fatal("start não deveria ser chamado após falha no check")
		return nil
	}

	record, err := f.run(context.Background(), req)
	if err != nil || record.Status != CancelledStale {
		t.Fatalf("falha antes da fila: record=%+v err=%v", record, err)
	}
	if checks != 2 {
		t.Fatalf("check chamado %d vezes, esperado 2", checks)
	}
}
