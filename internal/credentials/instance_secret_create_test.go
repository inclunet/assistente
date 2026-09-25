package credentials

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"assistente/internal/database"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

var ensureTestDEK = []byte("01234567890123456789012345678901")

func openEnsureSecretDB(t *testing.T) (*gorm.DB, string, func()) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "credentials.db")
	db, err := gorm.Open(sqlite.Open(path+"?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("sql db: %v", err)
	}
	sqlDB.SetMaxOpenConns(16)
	if err := db.AutoMigrate(&database.CredentialEntry{}, &database.CredentialKeyWrap{}); err != nil {
		t.Fatalf("migrate credentials: %v", err)
	}
	previous := database.DB()
	database.SetDB(db)
	cleanup := func() {
		database.SetDB(previous)
		_ = sqlDB.Close()
	}
	return db, path, cleanup
}

func loadedEnsureManager(t *testing.T, store *DBStore) *Manager {
	t.Helper()
	mgr := NewManagerWithStoreAndPersistence(ensureTestDEK, store, true)
	if err := mgr.LoadInstanceSecrets(context.Background()); err != nil {
		t.Fatalf("load instance secrets: %v", err)
	}
	if !mgr.IntegrityStatus().OK {
		t.Fatalf("vault should be validated: %+v", mgr.IntegrityStatus())
	}
	return mgr
}

func TestEnsureInstanceSecretPersistsAndReopens(t *testing.T) {
	db, path, cleanup := openEnsureSecretDB(t)
	defer cleanup()
	store := NewDBStore()
	mgr := loadedEnsureManager(t, store)

	var creates atomic.Int32
	got, err := mgr.EnsureInstanceSecret(context.Background(), "internal-auth:test-secret", func() (string, error) {
		creates.Add(1)
		return "durable-value", nil
	})
	if err != nil || got != "durable-value" {
		t.Fatalf("first ensure = %q, %v", got, err)
	}
	got, err = mgr.EnsureInstanceSecret(context.Background(), "internal-auth:test-secret", func() (string, error) {
		creates.Add(1)
		return "must-not-replace", nil
	})
	if err != nil || got != "durable-value" {
		t.Fatalf("second ensure = %q, %v", got, err)
	}
	if creates.Load() != 1 {
		t.Fatalf("create calls = %d, want 1", creates.Load())
	}

	var count int64
	if err := db.Model(&database.CredentialEntry{}).Where("user_id = '' AND pattern = ?", "internal-auth:test-secret").Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("persisted rows = %d, want 1", count)
	}

	// Um Manager novo também precisa usar o valor persistido, não o cache do
	// Manager anterior.
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	if err := sqlDB.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := gorm.Open(sqlite.Open(path+"?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)"), &gorm.Config{})
	if err != nil {
		t.Fatalf("reopen sqlite: %v", err)
	}
	reopenedSQL, err := reopened.DB()
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := reopenedSQL.Close(); err != nil {
			t.Errorf("close reopened sqlite: %v", err)
		}
	}()
	database.SetDB(reopened)
	reopenedMgr := loadedEnsureManager(t, NewDBStore())
	got, err = reopenedMgr.EnsureInstanceSecret(context.Background(), "internal-auth:test-secret", func() (string, error) {
		creates.Add(1)
		return "must-not-replace-after-reopen", nil
	})
	if err != nil || got != "durable-value" {
		t.Fatalf("reopened ensure = %q, %v", got, err)
	}
	if creates.Load() != 1 {
		t.Fatalf("create calls after reopen = %d, want 1", creates.Load())
	}
}

func TestEnsureInstanceSecretCallbackCanUseStorePool(t *testing.T) {
	db, _, cleanup := openEnsureSecretDB(t)
	defer cleanup()
	store := NewDBStore()
	mgr := loadedEnsureManager(t, store)
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	sqlDB.SetMaxOpenConns(1)

	got, err := mgr.EnsureInstanceSecret(context.Background(), "internal-auth:pool-callback", func() (string, error) {
		var one int
		if err := db.Raw("SELECT 1").Scan(&one).Error; err != nil {
			return "", err
		}
		if one != 1 {
			return "", errors.New("unexpected query result")
		}
		return "pool-safe", nil
	})
	if err != nil || got != "pool-safe" {
		t.Fatalf("ensure = %q, %v", got, err)
	}
}

func TestEnsureInstanceSecretConcurrentManagersUseOneWinner(t *testing.T) {
	_, _, cleanup := openEnsureSecretDB(t)
	defer cleanup()
	store := NewDBStore()
	const managers = 8
	all := make([]*Manager, managers)
	for i := range all {
		all[i] = loadedEnsureManager(t, store)
	}

	started := make(chan struct{}, managers)
	release := make(chan struct{})
	var releaseOnce sync.Once
	releaseAll := func() { releaseOnce.Do(func() { close(release) }) }
	defer releaseAll()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	values := make([]string, managers)
	errs := make([]error, managers)
	var wg sync.WaitGroup
	for i, mgr := range all {
		wg.Add(1)
		go func(i int, mgr *Manager) {
			defer wg.Done()
			values[i], errs[i] = mgr.EnsureInstanceSecret(ctx, "internal-auth:race", func() (string, error) {
				select {
				case started <- struct{}{}:
				case <-ctx.Done():
					return "", ctx.Err()
				}
				select {
				case <-release:
				case <-ctx.Done():
					return "", ctx.Err()
				}
				return fmt.Sprintf("candidate-%d", i), nil
			})
		}(i, mgr)
	}
	for i := 0; i < managers; i++ {
		select {
		case <-started:
		case <-ctx.Done():
			releaseAll()
			wg.Wait()
			t.Fatalf("waiting for manager %d to start: %v", i, ctx.Err())
		}
	}
	releaseAll()
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-ctx.Done():
		releaseAll()
		<-done
		t.Fatalf("waiting for concurrent managers: %v", ctx.Err())
	}

	winner := values[0]
	if winner == "" {
		t.Fatalf("winner vazio: values=%v errs=%v", values, errs)
	}
	for i := range values {
		if errs[i] != nil || values[i] != winner {
			t.Fatalf("manager %d = %q, %v; all=%v", i, values[i], errs[i], values)
		}
	}
	var count int64
	if err := database.DB().Model(&database.CredentialEntry{}).Where("user_id = '' AND pattern = ?", "internal-auth:race").Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("race rows = %d, want 1", count)
	}
}

func TestEnsureInstanceSecretNoPersistenceDoesNotCreate(t *testing.T) {
	_, _, cleanup := openEnsureSecretDB(t)
	defer cleanup()
	mgr := NewManagerWithStoreAndPersistence(ensureTestDEK, NewDBStore(), false)
	var creates atomic.Int32
	_, err := mgr.EnsureInstanceSecret(context.Background(), "internal-auth:no-persist", func() (string, error) {
		creates.Add(1)
		return "should-not-exist", nil
	})
	if !errors.Is(err, ErrInstanceSecretPersistenceRequired) {
		t.Fatalf("error = %v, want persistence error", err)
	}
	if creates.Load() != 0 {
		t.Fatal("create callback called without persistence")
	}
}

func TestEnsureInstanceSecretRequiresValidatedIntegrity(t *testing.T) {
	_, _, cleanup := openEnsureSecretDB(t)
	defer cleanup()
	mgr := NewManagerWithStoreAndPersistence(ensureTestDEK, NewDBStore(), true)
	var creates atomic.Int32
	_, err := mgr.EnsureInstanceSecret(context.Background(), "internal-auth:not-loaded", func() (string, error) {
		creates.Add(1)
		return "should-not-exist", nil
	})
	if !errors.Is(err, ErrInstanceSecretIntegrity) {
		t.Fatalf("error = %v, want integrity error", err)
	}
	if creates.Load() != 0 {
		t.Fatal("create callback called before vault validation")
	}
}

func TestEnsureInstanceSecretRejectsMasterWrapChangedAfterLoad(t *testing.T) {
	db, _, cleanup := openEnsureSecretDB(t)
	defer cleanup()
	mgr := loadedEnsureManager(t, NewDBStore())
	if err := db.Create(&database.CredentialKeyWrap{
		Kind:       KeyWrapKindMaster,
		DekID:      "different-dek",
		Salt:       "salt",
		WrappedDEK: "wrapped",
	}).Error; err != nil {
		t.Fatal(err)
	}
	var creates atomic.Int32
	_, err := mgr.EnsureInstanceSecret(context.Background(), "internal-auth:changed-wrap", func() (string, error) {
		creates.Add(1)
		return "should-not-exist", nil
	})
	if !errors.Is(err, ErrInstanceSecretIntegrity) {
		t.Fatalf("error = %v, want integrity error", err)
	}
	if creates.Load() != 0 {
		t.Fatal("create callback called after master wrap changed")
	}
}

func TestEnsureInstanceSecretContextCancellation(t *testing.T) {
	_, _, cleanup := openEnsureSecretDB(t)
	defer cleanup()
	mgr := loadedEnsureManager(t, NewDBStore())
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var creates atomic.Int32
	_, err := mgr.EnsureInstanceSecret(ctx, "internal-auth:canceled", func() (string, error) {
		creates.Add(1)
		return "should-not-exist", nil
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context.Canceled", err)
	}
	if creates.Load() != 0 {
		t.Fatal("create callback called after cancellation")
	}
}

func TestEnsureInstanceSecretExistingInvalidIsNeverReplaced(t *testing.T) {
	cases := []struct {
		name     string
		authType string
		tokenEnc string
	}{
		{name: "empty", authType: "secret", tokenEnc: ""},
		{name: "wrong-type", authType: "bearer", tokenEnc: "ciphertext"},
		{name: "corrupt", authType: "secret", tokenEnc: "not-a-ciphertext"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, _, cleanup := openEnsureSecretDB(t)
			defer cleanup()
			pattern := "internal-auth:invalid-" + tc.name
			if err := database.DB().Create(&database.CredentialEntry{
				UUIDModel: database.UUIDModel{ID: "existing-" + tc.name},
				UserID:    "",
				Pattern:   pattern,
				AuthType:  tc.authType,
				TokenEnc:  tc.tokenEnc,
			}).Error; err != nil {
				t.Fatal(err)
			}
			mgr := loadedEnsureManager(t, NewDBStore())
			var creates atomic.Int32
			_, err := mgr.EnsureInstanceSecret(context.Background(), pattern, func() (string, error) {
				creates.Add(1)
				return "replacement", nil
			})
			if !errors.Is(err, ErrInstanceSecretInvalid) {
				t.Fatalf("error = %v, want invalid secret", err)
			}
			if creates.Load() != 0 {
				t.Fatal("existing invalid secret was replaced")
			}
			var count int64
			if err := database.DB().Model(&database.CredentialEntry{}).Where("user_id = '' AND pattern = ?", pattern).Count(&count).Error; err != nil {
				t.Fatal(err)
			}
			if count != 1 {
				t.Fatalf("rows = %d, want 1", count)
			}
			var preserved database.CredentialEntry
			if err := database.DB().Where("user_id = '' AND pattern = ?", pattern).First(&preserved).Error; err != nil {
				t.Fatal(err)
			}
			if preserved.ID != "existing-"+tc.name || preserved.AuthType != tc.authType || preserved.TokenEnc != tc.tokenEnc {
				t.Fatalf("existing invalid credential changed: before=(%q,%q,%q) after=(%q,%q,%q)", "existing-"+tc.name, tc.authType, tc.tokenEnc, preserved.ID, preserved.AuthType, preserved.TokenEnc)
			}
		})
	}
}

func TestEnsureInstanceSecretNeverUsesUserFallback(t *testing.T) {
	_, _, cleanup := openEnsureSecretDB(t)
	defer cleanup()
	pattern := "internal-auth:user-only"
	encoder := NewManager(ensureTestDEK)
	auth, err := encoder.encryptAuth(&AuthConfig{Type: "secret", Token: "user-value"})
	if err != nil {
		t.Fatal(err)
	}
	if err := database.DB().Create(&database.CredentialEntry{
		UUIDModel: database.UUIDModel{ID: "user-secret"},
		UserID:    "user-a",
		Pattern:   pattern,
		AuthType:  auth.Type,
		TokenEnc:  auth.Token,
	}).Error; err != nil {
		t.Fatal(err)
	}

	mgr := loadedEnsureManager(t, NewDBStore())
	got, err := mgr.EnsureInstanceSecret(database.WithUserID(context.Background(), "user-a"), pattern, func() (string, error) {
		return "instance-value", nil
	})
	if err != nil || got != "instance-value" {
		t.Fatalf("ensure = %q, %v", got, err)
	}
	var count int64
	if err := database.DB().Model(&database.CredentialEntry{}).Where("pattern = ?", pattern).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatalf("rows = %d, want user + instance", count)
	}
}

func TestEnsureInstanceSecretProviderErrorIsRedacted(t *testing.T) {
	_, _, cleanup := openEnsureSecretDB(t)
	defer cleanup()
	mgr := loadedEnsureManager(t, NewDBStore())
	sentinel := errors.New("provider details containing secret-value")
	_, err := mgr.EnsureInstanceSecret(context.Background(), "internal-auth:provider-error", func() (string, error) {
		return "", sentinel
	})
	if !errors.Is(err, ErrInstanceSecretCreate) {
		t.Fatalf("error = %v, want generation sentinel", err)
	}
	if err.Error() == sentinel.Error() || strings.Contains(err.Error(), "secret-value") {
		t.Fatalf("provider error leaked: %v", err)
	}
}

type ensureFailingStore struct{ err error }

func (s ensureFailingStore) SaveCredential(context.Context, StoredCredential) error { return s.err }
func (s ensureFailingStore) ListCredentials(context.Context) ([]StoredCredential, error) {
	return nil, nil
}
func (s ensureFailingStore) DeleteCredential(context.Context, string) error { return s.err }
func (s ensureFailingStore) SaveKeyWrap(context.Context, KeyWrap) error     { return nil }
func (s ensureFailingStore) GetKeyWrap(context.Context, string) (*KeyWrap, error) {
	return nil, nil
}
func (s ensureFailingStore) HasKeyWrap(context.Context, string) (bool, error) { return false, nil }
func (s ensureFailingStore) GetInstanceCredential(context.Context, string) (StoredCredential, bool, error) {
	return StoredCredential{}, false, s.err
}
func (s ensureFailingStore) InsertInstanceCredentialIfAbsent(context.Context, StoredCredential) (bool, error) {
	return false, s.err
}

func TestEnsureInstanceSecretStoreErrorIsRedacted(t *testing.T) {
	secretErr := errors.New("sql detail containing secret-value and ciphertext")
	store := ensureFailingStore{err: secretErr}
	mgr := NewManagerWithStoreAndPersistence(ensureTestDEK, store, true)
	mgr.integrity.set(VaultIntegrityStatus{OK: true, KeychainDekID: DEKIdentity(ensureTestDEK)})
	_, err := mgr.EnsureInstanceSecret(context.Background(), "internal-auth:store-error", func() (string, error) {
		return "should-not-exist", nil
	})
	if !errors.Is(err, ErrInstanceSecretStore) {
		t.Fatalf("error = %v, want store sentinel", err)
	}
	if strings.Contains(err.Error(), "secret-value") || strings.Contains(err.Error(), "ciphertext") {
		t.Fatalf("store error leaked: %v", err)
	}
}

func TestEnsureInstanceSecretSanitizesWrappedContextErrors(t *testing.T) {
	if got := sanitizeInstanceSecretStoreError(fmt.Errorf("private: %w", context.Canceled)); got != context.Canceled {
		t.Fatalf("canceled error = %v, want canonical context.Canceled", got)
	}
	if got := sanitizeInstanceSecretStoreError(fmt.Errorf("private: %w", context.DeadlineExceeded)); got != context.DeadlineExceeded {
		t.Fatalf("deadline error = %v, want canonical context.DeadlineExceeded", got)
	}
}
