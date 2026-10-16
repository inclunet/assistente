package commandui

import (
	"errors"
	"testing"
	"time"
)

func TestBrokerReserveWithTTLUsesCustomDeadlineWithoutChangingDefault(t *testing.T) {
	const defaultTTL = 45 * time.Second
	b, err := New(4, defaultTTL)
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()

	owner := testOwner()
	defaultReservation, err := b.Reserve(owner, "command.default")
	if err != nil {
		t.Fatal(err)
	}
	customReservation, err := b.ReserveWithTTL(owner, "command.custom", 5*time.Minute)
	if err != nil {
		t.Fatal(err)
	}

	now := time.Now()
	defaultExpiry := b.entries[defaultReservation.Ticket].expiresAt
	customExpiry := b.entries[customReservation.Ticket].expiresAt
	if defaultExpiry.Before(now.Add(defaultTTL - time.Second)) {
		t.Fatalf("TTL padrão foi reduzido: expira em %s", defaultExpiry.Sub(now))
	}
	if customExpiry.Before(now.Add(5*time.Minute - time.Second)) {
		t.Fatalf("TTL customizado não aplicado: expira em %s", customExpiry.Sub(now))
	}
	if got := b.ttl; got != defaultTTL {
		t.Fatalf("TTL do broker foi alterado: %s", got)
	}
}

func TestBrokerReserveWithTTLRejectsInvalidValuesWithoutReservation(t *testing.T) {
	b, err := New(4, 45*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()

	owner := testOwner()
	for _, ttl := range []time.Duration{0, -time.Second, 5*time.Minute + time.Nanosecond} {
		reservation, err := b.ReserveWithTTL(owner, "command.invalid", ttl)
		if !errors.Is(err, ErrInvalidRequest) {
			t.Fatalf("TTL %s: erro = %v", ttl, err)
		}
		if reservation != (Reservation{}) {
			t.Fatalf("TTL %s criou reserva inválida: %+v", ttl, reservation)
		}
	}
	if len(b.entries) != 0 {
		t.Fatalf("reservas após entradas inválidas: %d", len(b.entries))
	}
}
