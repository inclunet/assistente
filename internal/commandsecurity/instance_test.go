package commandsecurity

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"assistente/internal/commandinstance"
	"github.com/glebarez/sqlite"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

func instanceTestDB(t *testing.T, path string) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(path), &gorm.Config{})
	if err != nil {
		t.Fatalf("abrir SQLite: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("obter conexão SQLite: %v", err)
	}
	sqlDB.SetMaxOpenConns(4)
	t.Cleanup(func() { _ = sqlDB.Close() })
	return db
}

func migratedInstanceDB(t *testing.T, name string) *gorm.DB {
	t.Helper()
	db := instanceTestDB(t, filepath.Join(t.TempDir(), name))
	if err := commandinstance.Migrate(context.Background(), db); err != nil {
		t.Fatalf("migrar commandinstance: %v", err)
	}
	return db
}

func bindInstance(t *testing.T, core *EpochService, db *gorm.DB) {
	t.Helper()
	if err := core.BindInstance(context.Background(), db); err != nil {
		t.Fatalf("bind: %v", err)
	}
}

func newInstanceCore(t *testing.T) *EpochService {
	t.Helper()
	core := newEpochServiceForTest(t)
	t.Cleanup(func() {
		if core.instance != nil {
			_ = core.instance.Close()
		}
	})
	return core
}

func instanceStartupID(t *testing.T) string {
	t.Helper()
	id, err := uuid.NewV7()
	if err != nil {
		t.Fatalf("gerar startup UUIDv7: %v", err)
	}
	return id.String()
}

func TestBindInstanceIsIdempotentOnlyForTheSameDatabase(t *testing.T) {
	firstDB := migratedInstanceDB(t, "first.db")
	secondDB := migratedInstanceDB(t, "second.db")
	core := newInstanceCore(t)

	bindInstance(t, core, firstDB)
	if err := core.BindInstance(context.Background(), firstDB); err != nil {
		t.Fatalf("bind repetido no mesmo banco: %v", err)
	}
	if err := core.BindInstance(context.Background(), secondDB); !errors.Is(err, ErrInvalidEpochInput) {
		t.Fatalf("outro banco: %v", err)
	}
}

func TestBindInstanceRejectsAfterExecutorRegistrationAndSecondCoreContends(t *testing.T) {
	db := migratedInstanceDB(t, "commands.db")
	core := newInstanceCore(t)
	if err := core.RegisterExecutorDrain(context.Background(), func(context.Context) error { return nil }); err != nil {
		t.Fatalf("registrar drain: %v", err)
	}
	if err := core.BindInstance(context.Background(), db); !errors.Is(err, ErrInvalidEpochInput) {
		t.Fatalf("bind depois do executor: %v", err)
	}

	core = newInstanceCore(t)
	bindInstance(t, core, db)
	other := newInstanceCore(t)
	if err := other.BindInstance(context.Background(), db); !errors.Is(err, commandinstance.ErrBusy) {
		t.Fatalf("segundo core não competiu pelo lock: %v", err)
	}
}

func TestReleaseInstanceRequiresSuccessfulDrain(t *testing.T) {
	db := migratedInstanceDB(t, "commands.db")
	core := newInstanceCore(t)
	bindInstance(t, core, db)
	if err := core.ReleaseInstance(context.Background()); !errors.Is(err, ErrDrainInProgress) {
		t.Fatalf("release antes do drain: %v", err)
	}

	failure := errors.New("executor ainda ativo")
	if err := core.RegisterExecutorDrain(context.Background(), func(context.Context) error { return failure }); err != nil {
		t.Fatal(err)
	}
	if _, err := core.CloseAndDrain(context.Background()); !errors.Is(err, failure) {
		t.Fatalf("falha do drain: %v", err)
	}
	if err := core.ReleaseInstance(context.Background()); !errors.Is(err, ErrDrainInProgress) {
		t.Fatalf("release após drain falho: %v", err)
	}
	other := newInstanceCore(t)
	if err := other.BindInstance(context.Background(), db); !errors.Is(err, commandinstance.ErrBusy) {
		t.Fatalf("falha do drain liberou lock: %v", err)
	}
}

func TestCancelledDrainPreservesInstanceExclusivity(t *testing.T) {
	db := migratedInstanceDB(t, "commands.db")
	core := newInstanceCore(t)
	bindInstance(t, core, db)
	if err := core.RegisterExecutorDrain(context.Background(), func(ctx context.Context) error { return ctx.Err() }); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := core.CloseAndDrain(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("drain cancelado: %v", err)
	}
	if err := core.ReleaseInstance(context.Background()); !errors.Is(err, ErrDrainInProgress) {
		t.Fatalf("cancelamento permitiu release: %v", err)
	}
	other := newInstanceCore(t)
	if err := other.BindInstance(context.Background(), db); !errors.Is(err, commandinstance.ErrBusy) {
		t.Fatalf("cancelamento liberou lock: %v", err)
	}
}

func TestSuccessfulDrainReleaseAllowsNextCoreAndRecoversExactOldGeneration(t *testing.T) {
	db := migratedInstanceDB(t, "commands.db")
	oldStartup := instanceStartupID(t)
	oldLease, err := commandinstance.Open(context.Background(), db, oldStartup)
	if err != nil {
		t.Fatal(err)
	}
	if err := oldLease.Close(); err != nil {
		t.Fatal(err)
	}

	core := newInstanceCore(t)
	bindInstance(t, core, db)
	proof, err := core.RestartProof(context.Background(), db)
	if err != nil || !proof.Valid() || proof.Includes(core.startup+":0") || !proof.Includes(oldStartup+":0") {
		// A live lease's proof is intentionally valid but never includes its own
		// startup; the old process generation must be recovered later.
		t.Fatalf("proof durante core vivo: valid=%v err=%v current=%v old=%v", proof.Valid(), err, proof.Includes(core.startup+":0"), proof.Includes(oldStartup+":0"))
	}
	if err := core.RegisterExecutorDrain(context.Background(), func(context.Context) error { return nil }); err != nil {
		t.Fatal(err)
	}
	if _, err := core.CloseAndDrain(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := core.ReleaseInstance(context.Background()); err != nil {
		t.Fatal(err)
	}

	next := newInstanceCore(t)
	bindInstance(t, next, db)
	nextProof, err := next.RestartProof(context.Background(), db)
	if err != nil || !nextProof.Valid() || !nextProof.Includes(oldStartup+":0") || !nextProof.Includes(core.startup+":0") || nextProof.Includes(next.startup+":0") {
		t.Fatalf("proof após release: valid=%v err=%v old=%v released=%v current=%v", nextProof.Valid(), err, nextProof.Includes(oldStartup+":0"), nextProof.Includes(core.startup+":0"), nextProof.Includes(next.startup+":0"))
	}
}

func TestBindDuringOpenDoesNotBlockCaptureOrDrainAndNeverPublishesLease(t *testing.T) {
	db := migratedInstanceDB(t, "commands.db")
	core := newInstanceCore(t)
	started := make(chan struct{})
	release := make(chan struct{})
	var once sync.Once
	var releaseOnce sync.Once
	t.Cleanup(func() {
		releaseOnce.Do(func() { close(release) })
		_ = db.Callback().Row().Remove("test:block-instance-open-row")
	})
	blockOpen := func(tx *gorm.DB) {
		if strings.Contains(strings.ToUpper(tx.Statement.SQL.String()), "PRAGMA DATABASE_LIST") {
			once.Do(func() { close(started) })
			<-release
		}
	}
	if err := db.Callback().Row().Before("gorm:row").Register("test:block-instance-open-row", blockOpen); err != nil {
		t.Fatal(err)
	}

	bindDone := make(chan error, 1)
	go func() { bindDone <- core.BindInstance(context.Background(), db) }()
	select {
	case <-started:
	case err := <-bindDone:
		t.Fatalf("bind terminou antes do ponto de I/O: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("bind não alcançou a barreira de I/O")
	}

	captureDone := make(chan error, 1)
	go func() {
		_, err := core.Capture(context.Background(), testEpochID(t), testEpochID(t))
		captureDone <- err
	}()
	select {
	case err := <-captureDone:
		if err != nil {
			t.Fatalf("capture bloqueado durante bind I/O: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("capture não progrediu durante bind I/O")
	}
	if err := core.RegisterExecutorDrain(context.Background(), func(context.Context) error { return nil }); !errors.Is(err, ErrStaleEpoch) {
		t.Fatalf("registro durante bind: %v", err)
	}
	drainDone := make(chan error, 1)
	go func() { _, err := core.CloseAndDrain(context.Background()); drainDone <- err }()
	select {
	case err := <-drainDone:
		if err != nil {
			t.Fatalf("close and drain concorrente: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("close and drain ficou bloqueado pelo bind I/O")
	}
	releaseCtx, cancelRelease := context.WithCancel(context.Background())
	releaseDone := make(chan error, 1)
	go func() { releaseDone <- core.ReleaseInstance(releaseCtx) }()
	cancelRelease()
	select {
	case err := <-releaseDone:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("release não respeitou cancelamento durante bind: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("release cancelado ficou bloqueado pelo bind I/O")
	}
	releaseOnce.Do(func() { close(release) })
	select {
	case err := <-bindDone:
		if !errors.Is(err, ErrStaleEpoch) {
			t.Fatalf("bind publicou lease após shutdown: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("bind não terminou após liberar a barreira")
	}
	if _, err := core.RestartProof(context.Background(), db); !errors.Is(err, ErrStaleEpoch) {
		t.Fatalf("core encerrado publicou proof: %v", err)
	}
}

func TestFromRestartProofRejectsZeroUnregisteredAndCopiedGenerations(t *testing.T) {
	if FromRestartProof(commandinstance.RecoveryProof{}).Valid() {
		t.Fatal("proof zero aceito")
	}
	dir := t.TempDir()
	originalPath := filepath.Join(dir, "original.db")
	db := instanceTestDB(t, originalPath)
	if err := commandinstance.Migrate(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	oldStartup := instanceStartupID(t)
	oldLease, err := commandinstance.Open(context.Background(), db, oldStartup)
	if err != nil {
		t.Fatal(err)
	}
	if err := oldLease.Close(); err != nil {
		t.Fatal(err)
	}
	core := newInstanceCore(t)
	bindInstance(t, core, db)
	proof, err := core.RestartProof(context.Background(), db)
	if err != nil {
		t.Fatal(err)
	}
	drained := FromRestartProof(proof)
	if !drained.Valid() || !drained.Includes(oldStartup+":0") || drained.Includes(instanceStartupID(t)+":0") {
		t.Fatalf("proof não filtrou gerações: valid=%v old=%v", drained.Valid(), drained.Includes(oldStartup+":0"))
	}

	// A cópia tem outra identidade física e não pode carregar a geração do
	// arquivo original, embora consiga abrir seu próprio schema.
	copyPath := filepath.Join(dir, "copy.db")
	originalBytes, err := os.ReadFile(originalPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(copyPath, originalBytes, 0o600); err != nil {
		t.Fatal(err)
	}
	copyDB := instanceTestDB(t, copyPath)
	copyLease, err := commandinstance.Open(context.Background(), copyDB, instanceStartupID(t))
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := copyLease.Close(); err != nil {
			t.Error(err)
		}
	}()
	copied := FromRestartProof(copyLease.RecoveryProof())
	if !copied.Valid() || copied.Includes(oldStartup+":0") {
		t.Fatalf("proof da cópia recuperou geração estrangeira: valid=%v inclui=%v", copied.Valid(), copied.Includes(oldStartup+":0"))
	}
}
