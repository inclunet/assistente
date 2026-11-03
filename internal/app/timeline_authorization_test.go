package app

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestTimelineAuthorizationDeadlineUsesQueryStartAndSessionExpiry(t *testing.T) {
	start := time.Now()
	if got := timelineAuthorizationDeadline(start, start.Add(time.Hour)); !got.Equal(start.Add(time.Second)) {
		t.Fatal("prazo não parte do início da consulta", got)
	}
	expiry := start.Add(100 * time.Millisecond)
	if got := timelineAuthorizationDeadline(start, expiry); !got.Equal(expiry) {
		t.Fatal("prazo ultrapassa sessão", got)
	}
}

func TestTimelineAuthorizationAdmissionDoesNotQuery(t *testing.T) {
	var queries atomic.Int32
	a := newTimelineAuthorization(context.Background(), time.Now().Add(time.Hour), func(context.Context) (time.Time, error) {
		queries.Add(1)
		return time.Now().Add(time.Hour), nil
	})
	defer a.close()
	for range 100 {
		if err := a.await(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	if queries.Load() != 0 {
		t.Fatal("admissão disparou SQL")
	}
}

func TestTimelineAuthorizationExpiredGrantWaitsForRenewal(t *testing.T) {
	started, release := make(chan struct{}), make(chan struct{})
	var startedOnce sync.Once
	a := newTimelineAuthorization(context.Background(), time.Now(), func(ctx context.Context) (time.Time, error) {
		startedOnce.Do(func() { close(started) })
		select {
		case <-ctx.Done():
			return time.Time{}, ctx.Err()
		case <-release:
			return time.Now().Add(time.Hour), nil
		}
	})
	defer a.close()
	<-started
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := a.await(ctx); !errors.Is(err, context.Canceled) {
		t.Fatal("emissão cancelada admitida", err)
	}
	blocked, stop := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer stop()
	if err := a.await(blocked); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal("prazo vencido admitiu emissão durante consulta", err)
	}
	close(release)
	ready, cancelReady := context.WithTimeout(context.Background(), time.Second)
	defer cancelReady()
	if err := a.await(ready); err != nil {
		t.Fatal("renovação não liberou emissão", err)
	}
}

func TestTimelineAuthorizationFailureClosesAdmission(t *testing.T) {
	a := newTimelineAuthorization(context.Background(), time.Now(), func(context.Context) (time.Time, error) {
		return time.Time{}, errors.New("persistência indisponível")
	})
	defer a.close()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := a.await(ctx); err == nil || errors.Is(err, context.DeadlineExceeded) {
		t.Fatal("falha não encerrou autorização", err)
	}
}

func TestTimelineAuthorizationLateQueryCannotExtendGrant(t *testing.T) {
	a := newTimelineAuthorization(context.Background(), time.Now(), func(ctx context.Context) (time.Time, error) {
		<-ctx.Done()
		return time.Now().Add(time.Hour), nil // simula driver retornando sucesso tardio
	})
	defer a.close()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := a.await(ctx); err == nil || errors.Is(err, context.DeadlineExceeded) {
		t.Fatal("resultado tardio renovou autorização", err)
	}
}

func TestTimelineAuthorizationCloseRejectsLateRenewal(t *testing.T) {
	started := make(chan struct{})
	a := newTimelineAuthorization(context.Background(), time.Now(), func(ctx context.Context) (time.Time, error) {
		close(started)
		<-ctx.Done()
		return time.Now().Add(time.Hour), nil
	})
	defer a.close()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("renovação não iniciou")
	}
	a.close()
	if err := a.await(context.Background()); err == nil {
		t.Fatal("resultado tardio ressuscitou autorização encerrada")
	}
}
