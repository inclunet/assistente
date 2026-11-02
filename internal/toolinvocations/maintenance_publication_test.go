package toolinvocations

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestMaintenanceDiscoveryCancellationReleasesGate(t *testing.T) {
	repo, userCtx, _ := setupRepositoryTest(t)
	repo.maintenanceCoordinator = func(context.Context, []string, func() error) error { return errors.New("discovery should not finish") }
	sqlDB, err := repo.db.DB()
	if err != nil {
		t.Fatal(err)
	}
	sqlDB.SetMaxOpenConns(1)
	conn, err := sqlDB.Conn(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close() }()
	before := sqlDB.Stats().WaitCount
	ctx, cancel := context.WithCancel(userCtx)
	defer cancel()
	done := make(chan error, 1)
	go func() { _, err := repo.CleanOldChat(ctx, 24*time.Hour); done <- err }()
	deadline := time.NewTimer(3 * time.Second)
	defer deadline.Stop()
	tick := time.NewTicker(time.Millisecond)
	defer tick.Stop()
	for sqlDB.Stats().WaitCount == before {
		select {
		case <-tick.C:
		case <-deadline.C:
			cancel()
			_ = conn.Close()
			lateErr := <-done
			t.Fatal("descoberta não aguardou conexão", lateErr)
		}
	}
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatal("cancelamento perdido", err)
		}
	case <-time.After(3 * time.Second):
		_ = conn.Close()
		lateErr := <-done
		t.Fatal("descoberta ignorou cancelamento", lateErr)
	}
}
