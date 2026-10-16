package commandbootstrap

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"assistente/internal/commandledger"
	"assistente/internal/credentials"
	"assistente/internal/database"
	"github.com/glebarez/sqlite"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

const commandKeyPatternPrefix = "internal-auth:command-request-hmac:"

func commandKeyPattern(version string) string {
	return commandKeyPatternPrefix + version
}

func fakeDEK() []byte {
	return []byte("01234567890123456789012345678901")
}

func openKeysTestDB(t *testing.T, path string) (*gorm.DB, *sqlDBCloser) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(path), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("abrir SQLite temporário: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("obter conexão SQLite temporária: %v", err)
	}
	sqlDB.SetMaxOpenConns(1)
	return db, &sqlDBCloser{db: sqlDB}
}

type sqlDBCloser struct {
	db interface{ Close() error }
}

func (c *sqlDBCloser) Close(t *testing.T) {
	t.Helper()
	if err := c.db.Close(); err != nil {
		t.Errorf("fechar SQLite temporário: %v", err)
	}
}

func newKeysTestDB(t *testing.T) (*gorm.DB, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "keys.db")
	db, sqlDB := openKeysTestDB(t, path)
	previous := database.DB()
	database.SetDB(db)
	t.Cleanup(func() {
		database.SetDB(previous)
		sqlDB.Close(t)
	})

	ctx := context.Background()
	if err := Migrate(ctx, db); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	if err := db.AutoMigrate(&database.CredentialEntry{}, &database.CredentialKeyWrap{}); err != nil {
		t.Fatalf("AutoMigrate de credenciais: %v", err)
	}
	return db, path
}

func reopenKeysTestDB(t *testing.T, path string) *gorm.DB {
	t.Helper()
	db, sqlDB := openKeysTestDB(t, path)
	database.SetDB(db)
	t.Cleanup(func() { sqlDB.Close(t) })
	return db
}

func seedMasterWrap(t *testing.T, dek []byte) {
	t.Helper()
	store := credentials.NewDBStore()
	if err := store.SaveKeyWrap(context.Background(), credentials.KeyWrap{
		Kind:  credentials.KeyWrapKindMaster,
		DekID: credentials.DEKIdentity(dek),
	}); err != nil {
		t.Fatalf("persistir master wrap de fixture: %v", err)
	}
}

func loadedKeysManager(t *testing.T, dek []byte) *credentials.Manager {
	t.Helper()
	manager := credentials.NewManagerWithStoreAndPersistence(dek, credentials.NewDBStore(), true)
	if err := manager.LoadInstanceSecrets(context.Background()); err != nil {
		t.Fatalf("LoadInstanceSecrets: %v", err)
	}
	status := manager.IntegrityStatus()
	if !status.OK {
		t.Fatalf("fixture de integridade inválida: %+v", status)
	}
	return manager
}

func preparedKeysFixture(t *testing.T) (*gorm.DB, *credentials.Manager, []byte) {
	t.Helper()
	db, _ := newKeysTestDB(t)
	dek := fakeDEK()
	seedMasterWrap(t, dek)
	manager := loadedKeysManager(t, dek)
	version, err := PrepareKeys(context.Background(), db, manager)
	if err != nil {
		t.Fatalf("PrepareKeys fixture: %v", err)
	}
	if version != "v1" {
		t.Fatalf("PrepareKeys fixture = %q, want v1", version)
	}
	return db, manager, dek
}

func countRows(t *testing.T, db *gorm.DB, model any) int64 {
	t.Helper()
	var count int64
	if err := db.Model(model).Count(&count).Error; err != nil {
		t.Fatalf("contar %T: %v", model, err)
	}
	return count
}

func loadKeyVersion(t *testing.T, db *gorm.DB, version string) keyVersion {
	t.Helper()
	var row keyVersion
	if err := db.Where("version = ?", version).First(&row).Error; err != nil {
		t.Fatalf("ler key version %s: %v", version, err)
	}
	id, err := uuid.Parse(row.ID)
	if err != nil || id.Version() != 7 || id.Variant() != uuid.RFC4122 || id.String() != row.ID {
		t.Fatalf("ID da key version não é UUIDv7 canônico: %q", row.ID)
	}
	return row
}

func loadCredential(t *testing.T, db *gorm.DB, pattern string) database.CredentialEntry {
	t.Helper()
	var entry database.CredentialEntry
	if err := db.Where("user_id = '' AND pattern = ?", pattern).First(&entry).Error; err != nil {
		t.Fatalf("ler credential entry %s: %v", pattern, err)
	}
	return entry
}

func signWithKeyVersion(t *testing.T, manager *credentials.Manager, version, payload string) string {
	t.Helper()
	provider, err := commandledger.NewCredentialKeyProvider(manager)
	if err != nil {
		t.Fatalf("NewCredentialKeyProvider: %v", err)
	}
	req := commandledger.LocalReadRequest{
		InvocationID: "018f0000-0000-7000-8000-000000000001",
		Owner: commandledger.Owner{
			UserID:        "018f0000-0000-7000-8000-000000000002",
			AuthContextID: "018f0000-0000-7000-8000-000000000003",
		},
		AuthGeneration:         "a1",
		SecurityGeneration:     "s1",
		RegistryVersion:        "r1",
		GlobalConfigGeneration: "g1",
		ActiveLayersGeneration: "l1",
		CommandID:              "workspace.read",
		SourceType:             "palette",
		CorrelationID:          payload,
		ReceivedAt:             time.Date(2026, 9, 14, 10, 0, 0, 0, time.UTC),
		ExpiresAt:              time.Date(2026, 9, 14, 10, 5, 0, 0, time.UTC),
	}
	signed, err := commandledger.SignLocalRead(context.Background(), req, version, provider)
	if err != nil {
		t.Fatalf("SignLocalRead(%s): %v", version, err)
	}
	return signed.RequestFingerprint
}

func TestKeysPrepareV1ReopenPreservesFingerprintAndCiphertext(t *testing.T) {
	db, path := newKeysTestDB(t)
	dek := fakeDEK()
	seedMasterWrap(t, dek)
	manager := loadedKeysManager(t, dek)

	version, err := PrepareKeys(context.Background(), db, manager)
	if err != nil {
		t.Fatalf("PrepareKeys: %v", err)
	}
	if version != "v1" {
		t.Fatalf("version = %q, want v1", version)
	}
	raw, ok, err := manager.GetInstanceSecret(commandKeyPattern("v1"))
	if err != nil || !ok {
		t.Fatalf("GetInstanceSecret(v1): ok=%v err=%v", ok, err)
	}
	entry := loadCredential(t, db, commandKeyPattern("v1"))
	if entry.TokenEnc == "" || entry.TokenEnc == raw {
		t.Fatalf("segredo não foi persistido como ciphertext: token_enc=%q raw=%q", entry.TokenEnc, raw)
	}
	digest, err := keyDigest(raw)
	if err != nil {
		t.Fatalf("digest da chave v1: %v", err)
	}
	row := loadKeyVersion(t, db, "v1")
	if row.Digest != digest {
		t.Fatalf("digest persistido = %q, want %q", row.Digest, digest)
	}
	if countRows(t, db, &database.CredentialEntry{}) != 1 {
		t.Fatal("PrepareKeys deveria persistir exatamente uma credencial de chave")
	}

	oldSQLDB, err := db.DB()
	if err != nil {
		t.Fatalf("obter conexão para reopen: %v", err)
	}
	if err := oldSQLDB.Close(); err != nil {
		t.Fatalf("fechar DB antes do reopen: %v", err)
	}
	db = reopenKeysTestDB(t, path)
	manager = loadedKeysManager(t, dek)
	version, err = PrepareKeys(context.Background(), db, manager)
	if err != nil {
		t.Fatalf("PrepareKeys após reopen: %v", err)
	}
	if version != "v1" {
		t.Fatalf("version após reopen = %q, want v1", version)
	}
	row = loadKeyVersion(t, db, "v1")
	if row.Digest != digest {
		t.Fatalf("fingerprint após reopen = %q, want %q", row.Digest, digest)
	}
	entry = loadCredential(t, db, commandKeyPattern("v1"))
	if entry.TokenEnc == "" || entry.TokenEnc == raw {
		t.Fatalf("ciphertext após reopen inválido: token_enc=%q raw=%q", entry.TokenEnc, raw)
	}
}

func TestKeysNoPersistenceDoesNotCreateSecret(t *testing.T) {
	db, _ := newKeysTestDB(t)
	dek := fakeDEK()
	seedMasterWrap(t, dek)
	manager := credentials.NewManagerWithStoreAndPersistence(dek, credentials.NewDBStore(), false)
	if err := manager.LoadInstanceSecrets(context.Background()); err != nil {
		t.Fatalf("LoadInstanceSecrets: %v", err)
	}

	if _, err := PrepareKeys(context.Background(), db, manager); err == nil {
		t.Fatal("PrepareKeys deveria falhar sem persistência")
	}
	if got := countRows(t, db, &database.CredentialEntry{}); got != 0 {
		t.Fatalf("credenciais criadas sem persistência: %d", got)
	}
	if got := countRows(t, db, &keyVersion{}); got != 0 {
		t.Fatalf("metadados de versão criados sem persistência: %d", got)
	}
}

func TestKeysRejectTransactionHandleBeforeAccessingVault(t *testing.T) {
	db, manager, _ := preparedKeysFixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if _, err := PrepareKeys(ctx, tx, manager); !errors.Is(err, ErrKeys) {
			t.Fatalf("PrepareKeys em transação: %v", err)
		}
		if _, err := RotateKeys(ctx, tx, manager, "v1"); !errors.Is(err, ErrKeys) {
			t.Fatalf("RotateKeys em transação: %v", err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestKeysRejectPinnedDigestMismatch(t *testing.T) {
	db, manager, _ := preparedKeysFixture(t)
	before := loadCredential(t, db, commandKeyPattern("v1"))
	if err := db.Model(&keyVersion{}).Where("version = ?", "v1").Update("digest", "0000000000000000000000000000000000000000000000000000000000000000").Error; err != nil {
		t.Fatal(err)
	}
	if _, err := PrepareKeys(context.Background(), db, manager); !errors.Is(err, ErrKeys) {
		t.Fatalf("digest divergente aceito: %v", err)
	}
	if after := loadCredential(t, db, commandKeyPattern("v1")); after.TokenEnc != before.TokenEnc {
		t.Fatal("falha de digest substituiu a chave")
	}
}

func TestKeysCorruptedOrMissingCredentialWithMetadataDoesNotRecreate(t *testing.T) {
	t.Run("corrompida", func(t *testing.T) {
		db, _, _ := preparedKeysFixture(t)
		pattern := commandKeyPattern("v1")
		if err := db.Model(&database.CredentialEntry{}).Where("pattern = ?", pattern).Update("token_enc", "corrupted-ciphertext").Error; err != nil {
			t.Fatalf("corromper credential entry: %v", err)
		}

		manager := loadedKeysManager(t, fakeDEK())
		status := manager.IntegrityStatus()
		if len(status.UnreadableCredentialIDs) != 1 {
			t.Fatalf("credential corrompida não marcada na integridade: %+v", status)
		}
		if _, err := PrepareKeys(context.Background(), db, manager); err == nil {
			t.Fatal("PrepareKeys recriou/aceitou segredo corrompido")
		}
		if got := countRows(t, db, &database.CredentialEntry{}); got != 1 {
			t.Fatalf("quantidade de credenciais após corrupção = %d, want 1", got)
		}
		if got := countRows(t, db, &keyVersion{}); got != 1 {
			t.Fatalf("metadata de versão após corrupção = %d, want 1", got)
		}
	})

	t.Run("ausente", func(t *testing.T) {
		db, _, _ := preparedKeysFixture(t)
		pattern := commandKeyPattern("v1")
		if err := db.Where("pattern = ?", pattern).Delete(&database.CredentialEntry{}).Error; err != nil {
			t.Fatalf("remover credential entry: %v", err)
		}

		manager := loadedKeysManager(t, fakeDEK())
		if _, err := PrepareKeys(context.Background(), db, manager); err == nil {
			t.Fatal("PrepareKeys recriou segredo ausente apesar do metadata")
		}
		if got := countRows(t, db, &database.CredentialEntry{}); got != 0 {
			t.Fatalf("credencial recriada após ausência: %d", got)
		}
		if got := countRows(t, db, &keyVersion{}); got != 1 {
			t.Fatalf("metadata de versão após ausência = %d, want 1", got)
		}
	})
}

func TestKeysLegacyLedgerWithoutKeyDoesNotCreateSecret(t *testing.T) {
	db, _ := newKeysTestDB(t)
	dek := fakeDEK()
	seedMasterWrap(t, dek)
	manager := loadedKeysManager(t, dek)
	now := time.Now().UTC()
	if err := db.Exec(`INSERT INTO command_idempotency_keys
		(id, key, invocation_id, auth_context_type, auth_context_id,
		 request_fingerprint_version, request_fingerprint, status, received_at, expires_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		"legacy-id", "legacy-key", "legacy-invocation", "local", "legacy-context",
		"v1", "legacy-fingerprint", "succeeded", now, now.Add(time.Hour)).Error; err != nil {
		t.Fatalf("inserir ledger legado: %v", err)
	}

	if _, err := PrepareKeys(context.Background(), db, manager); err == nil {
		t.Fatal("PrepareKeys deveria falhar fechado com ledger legado sem key metadata")
	}
	if got := countRows(t, db, &database.CredentialEntry{}); got != 0 {
		t.Fatalf("segredo criado apesar do ledger legado: %d", got)
	}
	if got := countRows(t, db, &keyVersion{}); got != 0 {
		t.Fatalf("metadata criado apesar do ledger legado: %d", got)
	}
}

func TestKeysRotationV1ToV2PreservesV1ProviderSigning(t *testing.T) {
	db, manager, _ := preparedKeysFixture(t)
	v1Before := loadCredential(t, db, commandKeyPattern("v1"))
	payload := "request-body-fixture"
	signatureBefore := signWithKeyVersion(t, manager, "v1", payload)

	next, err := RotateKeys(context.Background(), db, manager, "v1")
	if err != nil {
		t.Fatalf("RotateKeys: %v", err)
	}
	if next != "v2" {
		t.Fatalf("RotateKeys = %q, want v2", next)
	}
	if got := signWithKeyVersion(t, manager, "v1", payload); got != signatureBefore {
		t.Fatalf("assinatura v1 mudou após rotação: antes=%s depois=%s", signatureBefore, got)
	}
	if got := signWithKeyVersion(t, manager, "v2", payload); got == signatureBefore {
		t.Fatal("v2 deveria usar uma chave distinta da v1")
	}
	v1After := loadCredential(t, db, commandKeyPattern("v1"))
	if v1After.TokenEnc != v1Before.TokenEnc {
		t.Fatal("rotação alterou o ciphertext persistido da v1")
	}
	v1 := loadKeyVersion(t, db, "v1")
	v2 := loadKeyVersion(t, db, "v2")
	if v1.Active != 0 || v2.Active != 1 {
		t.Fatalf("atividade após rotação: v1=%d v2=%d", v1.Active, v2.Active)
	}
	if countRows(t, db, &database.CredentialEntry{}) != 2 {
		t.Fatal("rotação deveria conservar v1 e persistir v2")
	}
}

func TestKeysRotationRejectsStaleExpected(t *testing.T) {
	db, manager, _ := preparedKeysFixture(t)
	if _, err := RotateKeys(context.Background(), db, manager, "v1"); err != nil {
		t.Fatalf("primeira RotateKeys: %v", err)
	}
	if _, err := RotateKeys(context.Background(), db, manager, "v1"); err == nil {
		t.Fatal("RotateKeys deveria rejeitar expected stale")
	}
	if got := countRows(t, db, &keyVersion{}); got != 2 {
		t.Fatalf("versões após expected stale = %d, want 2", got)
	}
	if got := countRows(t, db, &database.CredentialEntry{}); got != 2 {
		t.Fatalf("credenciais após expected stale = %d, want 2", got)
	}
	if active := loadKeyVersion(t, db, "v2"); active.Active != 1 {
		t.Fatalf("expected stale alterou a versão ativa: %+v", active)
	}
}

func TestKeysContextCancellationDoesNotPersist(t *testing.T) {
	db, _ := newKeysTestDB(t)
	dek := fakeDEK()
	seedMasterWrap(t, dek)
	manager := loadedKeysManager(t, dek)

	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := PrepareKeys(canceled, db, manager); !errors.Is(err, context.Canceled) {
		t.Fatalf("PrepareKeys com contexto cancelado = %v, want context.Canceled", err)
	}
	if countRows(t, db, &database.CredentialEntry{}) != 0 || countRows(t, db, &keyVersion{}) != 0 {
		t.Fatal("PrepareKeys cancelado persistiu estado")
	}
	if _, err := PrepareKeys(context.Background(), db, manager); err != nil {
		t.Fatalf("PrepareKeys de controle: %v", err)
	}
	canceled, cancel = context.WithCancel(context.Background())
	cancel()
	if _, err := RotateKeys(canceled, db, manager, "v1"); !errors.Is(err, context.Canceled) {
		t.Fatalf("RotateKeys com contexto cancelado = %v, want context.Canceled", err)
	}
	if countRows(t, db, &database.CredentialEntry{}) != 1 || countRows(t, db, &keyVersion{}) != 1 {
		t.Fatal("RotateKeys cancelado persistiu rotação")
	}
}

func TestKeysRotationTransactionRollbackKeepsOrphanForRetry(t *testing.T) {
	db, manager, _ := preparedKeysFixture(t)
	if err := db.Exec(`CREATE TRIGGER fail_command_key_v2
		BEFORE INSERT ON command_key_versions
		WHEN NEW.version = 'v2'
		BEGIN SELECT RAISE(ABORT, 'forced key version insert failure'); END`).Error; err != nil {
		t.Fatalf("criar trigger de falha: %v", err)
	}

	if _, err := RotateKeys(context.Background(), db, manager, "v1"); err == nil {
		t.Fatal("RotateKeys deveria falhar dentro da transação")
	}
	if active := loadKeyVersion(t, db, "v1"); active.Active != 1 {
		t.Fatalf("rollback não restaurou active v1: %+v", active)
	}
	orphan := loadCredential(t, db, commandKeyPattern("v2"))
	if orphan.TokenEnc == "" {
		t.Fatal("falha transacional não deixou a chave v2 órfã persistida")
	}
	if got := countRows(t, db, &keyVersion{}); got != 1 {
		t.Fatalf("metadata após rollback = %d, want 1", got)
	}

	if err := db.Exec("DROP TRIGGER fail_command_key_v2").Error; err != nil {
		t.Fatalf("remover trigger de falha: %v", err)
	}
	next, err := RotateKeys(context.Background(), db, manager, "v1")
	if err != nil {
		t.Fatalf("retry de RotateKeys: %v", err)
	}
	if next != "v2" {
		t.Fatalf("retry RotateKeys = %q, want v2", next)
	}
	if got := loadCredential(t, db, commandKeyPattern("v2")); got.TokenEnc != orphan.TokenEnc {
		t.Fatal("retry substituiu a chave órfã em vez de preservá-la")
	}
	if active := loadKeyVersion(t, db, "v2"); active.Active != 1 {
		t.Fatalf("retry não ativou v2: %+v", active)
	}
	if countRows(t, db, &database.CredentialEntry{}) != 2 || countRows(t, db, &keyVersion{}) != 2 {
		t.Fatal("retry criou estado duplicado após rollback")
	}
}
