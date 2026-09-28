package desktopinstance

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"assistente/internal/commandinstance"
)

var _ io.Closer = (*Guard)(nil)

func fixture(t *testing.T) (string, string) {
	t.Helper()
	root := t.TempDir()
	path := filepath.Join(root, "db.sqlite")
	if err := os.WriteFile(path, []byte("database bytes must remain untouched"), 0o600); err != nil {
		t.Fatal(err)
	}
	return path, filepath.Join(root, "identities")
}

func mustAcquire(t *testing.T, path, identities string, callback func()) *Guard {
	t.Helper()
	g, err := acquire(path, identities, callback)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := g.Close(); err != nil {
			t.Error(err)
		}
	})
	return g
}

func TestContentionActivationReopenAndUnchangedDatabase(t *testing.T) {
	path, identities := fixture(t)
	want, _ := os.ReadFile(path)
	activated := make(chan struct{}, 1)
	g := mustAcquire(t, path, identities, func() {
		select {
		case activated <- struct{}{}:
		default:
		}
	})
	second, err := acquire(path, identities, func() { t.Error("secondary callback invoked") })
	if second != nil || !errors.Is(err, ErrAlreadyRunning) || errors.Is(err, ErrNotification) {
		t.Fatalf("second=%v err=%v", second, err)
	}
	select {
	case <-activated:
	case <-time.After(time.Second):
		t.Fatal("activation not delivered")
	}
	// The new desktop guard must not consume the existing commandinstance lock.
	commands, err := commandinstance.Acquire(context.Background(), path)
	if err != nil {
		t.Fatalf("command lock conflicts: %v", err)
	}
	if err := commands.Close(); err != nil {
		t.Fatal(err)
	}
	if err := g.Close(); err != nil {
		t.Fatal(err)
	}
	if err := g.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path + sidecarSuffix); err != nil {
		t.Fatalf("sidecar removed: %v", err)
	}
	mustAcquire(t, path, identities, func() {})
	got, err := os.ReadFile(path)
	if err != nil || string(got) != string(want) {
		t.Fatalf("database altered: %q %v", got, err)
	}
}

func TestDifferentDatabasesAreIndependent(t *testing.T) {
	path, identities := fixture(t)
	other := filepath.Join(filepath.Dir(path), "other.sqlite")
	if err := os.WriteFile(other, []byte("other"), 0o600); err != nil {
		t.Fatal(err)
	}
	mustAcquire(t, path, identities, func() {})
	mustAcquire(t, other, identities, func() {})
}

func TestAbsentDatabaseIsNotCreatedAndPathStaysReservedAfterCreation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "new.sqlite")
	activate := make(chan struct{}, 1)
	g, err := Acquire(path, func() {
		select {
		case activate <- struct{}{}:
		default:
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = g.Close() })
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("database was created: %v", err)
	}
	if err := os.WriteFile(path, []byte("created later"), 0o600); err != nil {
		t.Fatal(err)
	}
	second, err := Acquire(path, func() {})
	if second != nil || !errors.Is(err, ErrAlreadyRunning) {
		t.Fatalf("second=%v err=%v", second, err)
	}
}

func TestPathReservationSurvivesDatabaseReplacement(t *testing.T) {
	path, identities := fixture(t)
	mustAcquire(t, path, identities, func() {})
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("reset database"), 0o600); err != nil {
		t.Fatal(err)
	}
	second, err := acquire(path, identities, func() {})
	if second != nil || !errors.Is(err, ErrAlreadyRunning) {
		t.Fatalf("reset bypassed reservation: %v", err)
	}
}

func TestExistingHardlinkActivatesFirstAndReleasesPartialReservation(t *testing.T) {
	path, identities := fixture(t)
	alias := filepath.Join(t.TempDir(), "alias.sqlite")
	if err := os.Link(path, alias); err != nil {
		t.Skipf("hardlinks unavailable: %v", err)
	}
	activated := make(chan struct{}, 1)
	g := mustAcquire(t, path, identities, func() {
		select {
		case activated <- struct{}{}:
		default:
		}
	})
	second, err := acquire(alias, identities, func() {})
	if second != nil || !errors.Is(err, ErrAlreadyRunning) {
		t.Fatalf("hardlink bypassed reservation: %v", err)
	}
	select {
	case <-activated:
	case <-time.After(time.Second):
		t.Fatal("alias did not activate owner")
	}
	if err := g.Close(); err != nil {
		t.Fatal(err)
	}
	mustAcquire(t, alias, identities, func() {})
}

func TestSymlinkAndCleanPathAliases(t *testing.T) {
	path, identities := fixture(t)
	alias := filepath.Join(filepath.Dir(path), "alias.sqlite")
	if err := os.Symlink(path, alias); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	mustAcquire(t, alias, identities, func() {})
	for _, candidate := range []string{path, filepath.Dir(path) + string(os.PathSeparator) + "." + string(os.PathSeparator) + filepath.Base(path)} {
		second, err := acquire(candidate, identities, func() {})
		if second != nil || !errors.Is(err, ErrAlreadyRunning) {
			t.Fatalf("alias bypassed guard: %v", err)
		}
	}
}

func TestOccupiedUnpublishedLockFailsClosedWithinDeadline(t *testing.T) {
	path, identities := fixture(t)
	partial := &Guard{}
	if err := partial.take(path + sidecarSuffix); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		for _, held := range partial.locks {
			_ = held.close()
		}
	})
	start := time.Now()
	second, err := acquire(path, identities, func() {})
	if second != nil || !errors.Is(err, ErrNotification) || errors.Is(err, ErrAlreadyRunning) {
		t.Fatalf("unpublished owner: guard=%v err=%v", second, err)
	}
	if time.Since(start) > 3*notificationTimeout {
		t.Fatal("unbounded startup wait")
	}
}

func TestSilentEndpointFailsClosedWithinDeadline(t *testing.T) {
	path, identities := fixture(t)
	partial := &Guard{}
	if err := partial.take(path + sidecarSuffix); err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	data, _ := json.Marshal(endpoint{Address: listener.Addr().String(), Token: strings.Repeat("a", 64)})
	if _, err := partial.locks[0].file.WriteAt(data, 0); err != nil {
		t.Fatal(err)
	}
	// A listening but non-responsive owner is not permission to start services.
	t.Cleanup(func() {
		_ = listener.Close()
		for _, held := range partial.locks {
			_ = held.close()
		}
	})
	start := time.Now()
	second, err := acquire(path, identities, func() {})
	if second != nil || !errors.Is(err, ErrNotification) {
		t.Fatalf("silent owner: guard=%v err=%v", second, err)
	}
	if time.Since(start) > 3*notificationTimeout {
		t.Fatal("unbounded notification wait")
	}
}

func TestConcurrentAcquisitionHasOneOwnerAndCoalescesBeforeUI(t *testing.T) {
	path, identities := fixture(t)
	pending := make(chan struct{}, 1)
	callback := func() {
		select {
		case pending <- struct{}{}:
		default:
		}
	}
	const count = 12
	start := make(chan struct{})
	var wg sync.WaitGroup
	owners := make(chan *Guard, count)
	results := make(chan error, count)
	for range count {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			g, err := acquire(path, identities, callback)
			if g != nil {
				owners <- g
			}
			results <- err
		}()
	}
	close(start)
	wg.Wait()
	close(owners)
	close(results)
	if len(owners) != 1 {
		t.Errorf("owners=%d", len(owners))
	}
	for g := range owners {
		t.Cleanup(func() { _ = g.Close() })
	}
	for err := range results {
		if err != nil && !errors.Is(err, ErrAlreadyRunning) {
			t.Error(err)
		}
	}
	select {
	case <-pending:
	case <-time.After(time.Second):
		t.Fatal("startup activation lost")
	}
}

func TestActivationRejectsWrongTokenAndNonLoopbackMetadata(t *testing.T) {
	path, identities := fixture(t)
	activated := make(chan struct{}, 1)
	g := mustAcquire(t, path, identities, func() { activated <- struct{}{} })
	if err := sendActivation(endpoint{Address: g.listener.Addr().String(), Token: strings.Repeat("z", 64)}, time.Now().Add(time.Second)); err == nil {
		t.Fatal("wrong token accepted")
	}
	select {
	case <-activated:
		t.Fatal("unauthenticated activation")
	default:
	}
	for _, address := range []string{"example.com:1234", "192.0.2.1:1234", "127.0.0.1:0"} {
		if err := sendActivation(endpoint{Address: address, Token: strings.Repeat("a", 64)}, time.Now().Add(time.Second)); err == nil {
			t.Fatalf("invalid endpoint accepted: %s", address)
		}
	}
}

func TestInvalidInputDoesNotAcquire(t *testing.T) {
	path, identities := fixture(t)
	for _, candidate := range []string{"", filepath.Dir(path), filepath.Join(path, "db.sqlite")} {
		g, err := acquire(candidate, identities, func() {})
		if g != nil || err == nil {
			t.Fatalf("invalid path %q: guard=%v err=%v", candidate, g, err)
		}
	}
	if g, err := acquire(path, identities, nil); g != nil || err == nil {
		t.Fatal("nil callback accepted")
	}
}

func TestFirstLaunchCreatesOnlyParentDirectories(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "new", "subdir", "conversations.db")
	identities := filepath.Join(root, "identities")
	mustAcquire(t, path, identities, func() {})
	if info, err := os.Stat(filepath.Dir(path)); err != nil || !info.IsDir() {
		t.Fatalf("missing parent: %v", err)
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("database created: %v", err)
	}
	second, err := acquire(path, identities, func() {})
	if second != nil || !errors.Is(err, ErrAlreadyRunning) {
		t.Fatalf("first-launch reservation bypassed: %v", err)
	}
}

func TestFailedIdentityReservationReleasesPathLock(t *testing.T) {
	path, identities := fixture(t)
	if err := os.WriteFile(identities, []byte("not a directory"), 0o600); err != nil {
		t.Fatal(err)
	}
	if g, err := acquire(path, identities, func() {}); g != nil || err == nil {
		t.Fatal("invalid identity directory accepted")
	}
	mustAcquire(t, path, identities+"-valid", func() {})
}

func TestSlowCallbackDoesNotBlockNotificationOrClose(t *testing.T) {
	path, identities := fixture(t)
	started, release, finished := make(chan struct{}), make(chan struct{}), make(chan struct{})
	g := mustAcquire(t, path, identities, func() { close(started); <-release; close(finished) })
	defer close(release)
	if second, err := acquire(path, identities, func() {}); second != nil || !errors.Is(err, ErrAlreadyRunning) {
		t.Fatalf("first activation: %v", err)
	}
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("callback did not start")
	}
	for range 4 {
		if second, err := acquire(path, identities, func() {}); second != nil || !errors.Is(err, ErrAlreadyRunning) {
			t.Fatalf("blocked callback broke IPC: %v", err)
		}
	}
	if len(g.activate) != 1 {
		t.Fatalf("pending requests not coalesced: %d", len(g.activate))
	}
	closed := make(chan error, 1)
	go func() { closed <- g.Close() }()
	select {
	case err := <-closed:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("Close waited for callback")
	}
	select {
	case <-finished:
		t.Fatal("callback unexpectedly finished")
	default:
	}
}
func TestGuardExposesReservedCanonicalPath(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "db", "..", "conversations.db")
	g, err := acquire(path, filepath.Join(root, "identities"), func() {})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = g.Close() }()
	want, err := canonicalPath(path)
	if err != nil || g.DatabasePath() != want {
		t.Fatalf("reserved=%q want=%q err=%v", g.DatabasePath(), want, err)
	}
}

func TestGuardCanonicalPathSurvivesSymlinkRetarget(t *testing.T) {
	path, identities := fixture(t)
	alias := filepath.Join(filepath.Dir(path), "alias.sqlite")
	if err := os.Symlink(path, alias); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	g := mustAcquire(t, alias, identities, func() {})
	want, err := canonicalPath(path)
	if err != nil {
		t.Fatal(err)
	}
	// Apenas o alias descartável da fixture é substituído, nunca o banco.
	if err := os.Remove(alias); err != nil {
		t.Fatal(err)
	}
	other := filepath.Join(filepath.Dir(path), "other.sqlite")
	if err := os.WriteFile(other, []byte("another database"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(other, alias); err != nil {
		t.Fatal(err)
	}
	if g.DatabasePath() != want {
		t.Fatalf("reserved path changed: got=%q want=%q", g.DatabasePath(), want)
	}
	if second, err := acquire(g.DatabasePath(), identities, func() {}); second != nil || !errors.Is(err, ErrAlreadyRunning) {
		t.Fatalf("original database lost reservation: %v", err)
	}
}
