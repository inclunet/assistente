package commandbootstrap

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gorm.io/gorm"
)

type envelopeV20Fixture struct {
	db         *gorm.DB
	path       string
	ledgerID   string
	receiptID  string
	layerID    string
	invocation string
}

// makeKnownV20Fixture cria primeiro o schema atual em um SQLite temporário,
// deriva os DDLs conhecidos e reconstrói o banco legado na mesma ordem de
// objects. Isso evita copiar DDL manual e garante que o fixture acompanhe o
// contrato atual; as transformações conhecidas removem as ampliações v21/v22
// e as tabelas v23, sem admitir DDL arbitrária no migrador de produção.
func makeKnownV20Fixture(t *testing.T) envelopeV20Fixture {
	t.Helper()
	currentPath := filepath.Join(t.TempDir(), "current-reference.db")
	current := openBootstrapTestDB(t, currentPath)
	if err := Migrate(context.Background(), current); err != nil {
		t.Fatalf("criar referência atual: %v", err)
	}
	known, err := objects(current)
	if err != nil {
		t.Fatalf("ler objetos atuais: %v", err)
	}

	path := filepath.Join(t.TempDir(), "known-v20.db")
	legacy := openBootstrapTestDB(t, path)
	for _, currentObject := range known {
		if !preActivationObject(currentObject) {
			continue
		}
		legacyObject := legacyEnvelopeObject(legacyConfigObject(currentObject))
		if strings.TrimSpace(legacyObject.SQL) == "" {
			continue
		}
		if err := legacy.Exec(legacyObject.SQL).Error; err != nil {
			t.Fatalf("reconstruir objeto v20 %s: %v\n%s", legacyObject.Name, err, legacyObject.SQL)
		}
	}
	if err := legacy.Exec(`CREATE TABLE schema_migrations (
		version INTEGER PRIMARY KEY,
		name TEXT NOT NULL,
		applied_at DATETIME NOT NULL
	)`).Error; err != nil {
		t.Fatalf("criar carimbos v20: %v", err)
	}
	if err := legacy.Exec(`INSERT INTO schema_migrations (version, name, applied_at) VALUES (21, 'command_storage_initial', ?)`, time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC)).Error; err != nil {
		t.Fatalf("carimbar v20: %v", err)
	}

	fixture := envelopeV20Fixture{db: legacy, path: path, ledgerID: bootstrapUUID7(t), receiptID: bootstrapUUID7(t), layerID: bootstrapUUID7(t), invocation: bootstrapUUID7(t)}
	userID, authID := bootstrapUUID7(t), bootstrapUUID7(t)
	if err := legacy.Exec(`INSERT INTO command_layers
		(id, user_id, name, description, enabled, source, resolution_priority, created_at, updated_at)
		VALUES (?, ?, 'v20 fixture layer', 'preserved data', 1, 'test', 2, ?, ?)`, fixture.layerID, userID, time.Date(2026, 9, 14, 11, 0, 0, 0, time.UTC), time.Date(2026, 9, 14, 11, 0, 0, 0, time.UTC)).Error; err != nil {
		t.Fatalf("semear dado v20: %v", err)
	}
	if err := legacy.Exec(`INSERT INTO command_idempotency_keys
		(id, key, invocation_id, user_id, auth_context_type, auth_context_id, source_type,
		 request_fingerprint_version, request_fingerprint, status, received_at, expires_at)
		VALUES (?, ?, ?, ?, 'local_session', ?, 'palette', 'v1', 'fp-v20-ledger', 'evaluating', ?, ?)`,
		fixture.ledgerID, "invocation:"+fixture.invocation, fixture.invocation, userID, authID,
		time.Date(2026, 9, 14, 11, 0, 0, 0, time.UTC), time.Date(2026, 9, 14, 13, 0, 0, 0, time.UTC)).Error; err != nil {
		t.Fatalf("semear ledger v20: %v", err)
	}
	if err := legacy.Exec(`INSERT INTO command_decision_receipts
		(decision_id, subject_id, user_id, auth_context_id, request_fingerprint,
		 auth_generation, security_generation, expires_at, status, auth_context_type,
		 subject_type, allowed_action_ids, accepted_action_id, responded_at, consumed_at)
		VALUES (?, ?, ?, ?, 'fp-v20-receipt', 'auth-v20', 'security-v20', ?, 'accepted',
		 'local_session', 'config_mutation', '["apply","deny"]', 'apply', ?, NULL)`,
		fixture.receiptID, bootstrapUUID7(t), userID, authID,
		time.Date(2026, 9, 14, 13, 0, 0, 0, time.UTC), time.Date(2026, 9, 14, 11, 1, 0, 0, time.UTC)).Error; err != nil {
		t.Fatalf("semear receipt v20: %v", err)
	}
	if err := legacy.Exec(`INSERT INTO command_decision_receipt_events
		(id, decision_id, state, occurred_ms) VALUES (?, ?, 'accepted', ?)`,
		bootstrapUUID7(t), fixture.receiptID, time.Date(2026, 9, 14, 11, 1, 0, 0, time.UTC).UnixMilli()).Error; err != nil {
		t.Fatalf("semear evento da receipt v20: %v", err)
	}
	return fixture
}

func migrationStamp(t *testing.T, db *gorm.DB, version int, name string) int64 {
	t.Helper()
	var count int64
	if err := db.Raw("SELECT COUNT(*) FROM schema_migrations WHERE version = ? AND name = ?", version, name).Scan(&count).Error; err != nil {
		t.Fatalf("ler carimbo %d/%s: %v", version, name, err)
	}
	return count
}

func hasColumn(t *testing.T, db *gorm.DB, table, column string) bool {
	t.Helper()
	var count int64
	if err := db.Raw("SELECT COUNT(*) FROM pragma_table_info(?) WHERE name = ?", table, column).Scan(&count).Error; err != nil {
		t.Fatalf("ler coluna %s.%s: %v", table, column, err)
	}
	return count == 1
}

func splitDDLDefinitions(body string) []string {
	parts := make([]string, 0, 8)
	depth, from := 0, 0
	var quote byte
	for i := 0; i < len(body); i++ {
		c := body[i]
		if quote != 0 {
			if c == quote {
				if i+1 < len(body) && body[i+1] == quote {
					i++
				} else {
					quote = 0
				}
			}
			continue
		}
		switch c {
		case '\'', '"', '`':
			quote = c
		case '[':
			quote = ']'
		case '(':
			depth++
		case ')':
			depth--
		case ',':
			if depth == 0 {
				parts = append(parts, strings.TrimSpace(body[from:i]))
				from = i + 1
			}
		}
	}
	return append(parts, strings.TrimSpace(body[from:]))
}

func reorderKnownV20ReceiptColumns(t *testing.T, db *gorm.DB) {
	t.Helper()
	known, err := objects(db)
	if err != nil {
		t.Fatalf("ler objetos para fixture reordenada: %v", err)
	}
	var receipt schemaObject
	for _, object := range known {
		if object.Name == "command_decision_receipts" {
			receipt = legacyEnvelopeObject(object)
			break
		}
	}
	if receipt.SQL == "" {
		t.Fatal("DDL v20 da receipt não encontrado")
	}
	var receiptIndexes []schemaObject
	for _, object := range known {
		if object.Type == "index" && object.TblName == "command_decision_receipts" && object.SQL != "" {
			receiptIndexes = append(receiptIndexes, object)
		}
	}
	start, end := strings.IndexByte(receipt.SQL, '('), strings.LastIndexByte(receipt.SQL, ')')
	if start < 0 || end <= start {
		t.Fatalf("DDL v20 da receipt sem corpo: %s", receipt.SQL)
	}
	parts := splitDDLDefinitions(receipt.SQL[start+1 : end])
	if len(parts) < 2 {
		t.Fatalf("DDL v20 da receipt não tem definições suficientes: %s", receipt.SQL)
	}
	constraintStart := len(parts)
	for i, part := range parts {
		upper := strings.ToUpper(strings.TrimSpace(part))
		if strings.HasPrefix(upper, "PRIMARY KEY") || strings.HasPrefix(upper, "CONSTRAINT ") || strings.HasPrefix(upper, "UNIQUE ") || strings.HasPrefix(upper, "CHECK ") || strings.HasPrefix(upper, "FOREIGN KEY") {
			constraintStart = i
			break
		}
	}
	if constraintStart < 2 {
		t.Fatalf("DDL v20 da receipt não separou colunas e constraints: %s", receipt.SQL)
	}
	reordered := append([]string{}, parts[1:constraintStart]...)
	reordered = append(reordered, parts[0])
	reordered = append(reordered, parts[constraintStart:]...)
	reorderedDDL := receipt.SQL[:start+1] + strings.Join(reordered, ",") + receipt.SQL[end:]

	var columns []struct {
		Name string `gorm:"column:name"`
	}
	if err := db.Raw("SELECT name FROM pragma_table_info('command_decision_receipts') ORDER BY cid").Scan(&columns).Error; err != nil {
		t.Fatalf("ler colunas da receipt v20: %v", err)
	}
	if len(columns) < 2 {
		t.Fatalf("receipt v20 sem colunas suficientes: %d", len(columns))
	}
	projection := make([]string, len(columns))
	for i, column := range columns {
		projection[i] = "`" + strings.ReplaceAll(column.Name, "`", "``") + "`"
	}
	columnList := strings.Join(projection, ",")

	for _, index := range receiptIndexes {
		quotedName := "`" + strings.ReplaceAll(index.Name, "`", "``") + "`"
		if err := db.Exec("DROP INDEX " + quotedName).Error; err != nil {
			t.Fatalf("remover índice %s da receipt v20: %v", index.Name, err)
		}
	}
	if err := db.Exec("ALTER TABLE command_decision_receipts RENAME TO command_decision_receipts_reordered_source").Error; err != nil {
		t.Fatalf("renomear receipt v20 para fixture reordenada: %v", err)
	}
	if err := db.Exec(reorderedDDL).Error; err != nil {
		t.Fatalf("criar receipt v20 com colunas reordenadas: %v\n%s", err, reorderedDDL)
	}
	if err := db.Exec("INSERT INTO command_decision_receipts (" + columnList + ") SELECT " + columnList + " FROM command_decision_receipts_reordered_source").Error; err != nil {
		t.Fatalf("copiar receipt v20 reordenada: %v", err)
	}
	if err := db.Exec("DROP TABLE command_decision_receipts_reordered_source").Error; err != nil {
		t.Fatalf("remover receipt fonte da fixture reordenada: %v", err)
	}
	for _, index := range receiptIndexes {
		if err := db.Exec(index.SQL).Error; err != nil {
			t.Fatalf("recriar índice %s da receipt v20: %v\n%s", index.Name, err, index.SQL)
		}
	}
}

func TestMigrateKnownV20ToV21PreservesLedgerReceiptsDataStampsAndReopen(t *testing.T) {
	fixture := makeKnownV20Fixture(t)
	if migrationStamp(t, fixture.db, 21, "command_storage_initial") != 1 || migrationStamp(t, fixture.db, 22, "command_envelope_ownership") != 0 {
		t.Fatal("fixture não está exatamente em v20")
	}
	if hasColumn(t, fixture.db, "command_idempotency_keys", "actor_type") || hasColumn(t, fixture.db, "command_idempotency_keys", "actor_id") || hasColumn(t, fixture.db, "command_idempotency_keys", "input_fingerprint") {
		t.Fatal("fixture v20 já contém colunas v21")
	}

	if err := Migrate(context.Background(), fixture.db); err != nil {
		t.Fatalf("upgrade v20→v21: %v", err)
	}
	if migrationStamp(t, fixture.db, 21, "command_storage_initial") != 1 || migrationStamp(t, fixture.db, 22, "command_envelope_ownership") != 1 {
		t.Fatal("carimbos não preservaram v20 e não publicaram v21")
	}
	if !hasColumn(t, fixture.db, "command_idempotency_keys", "actor_type") || !hasColumn(t, fixture.db, "command_idempotency_keys", "actor_id") || !hasColumn(t, fixture.db, "command_idempotency_keys", "input_fingerprint") {
		t.Fatal("upgrade não adicionou ownership do ledger")
	}
	var inputFingerprint *string
	if err := fixture.db.Raw("SELECT input_fingerprint FROM command_idempotency_keys WHERE id = ?", fixture.ledgerID).Scan(&inputFingerprint).Error; err != nil {
		t.Fatalf("ler input_fingerprint nullable: %v", err)
	}
	if inputFingerprint != nil {
		t.Fatalf("input_fingerprint legado deveria permanecer NULL: %q", *inputFingerprint)
	}
	var ledgerKey, receiptStatus, layerName string
	if err := fixture.db.Raw("SELECT key FROM command_idempotency_keys WHERE id = ?", fixture.ledgerID).Scan(&ledgerKey).Error; err != nil {
		t.Fatal(err)
	}
	if err := fixture.db.Raw("SELECT status FROM command_decision_receipts WHERE decision_id = ?", fixture.receiptID).Scan(&receiptStatus).Error; err != nil {
		t.Fatal(err)
	}
	if err := fixture.db.Raw("SELECT name FROM command_layers WHERE id = ?", fixture.layerID).Scan(&layerName).Error; err != nil {
		t.Fatal(err)
	}
	if ledgerKey != "invocation:"+fixture.invocation || receiptStatus != "accepted" || layerName != "v20 fixture layer" {
		t.Fatalf("dados v20 alterados: ledger=%q receipt=%q layer=%q", ledgerKey, receiptStatus, layerName)
	}
	var eventCount int64
	if err := fixture.db.Raw("SELECT COUNT(*) FROM command_decision_receipt_events WHERE decision_id = ?", fixture.receiptID).Scan(&eventCount).Error; err != nil {
		t.Fatal(err)
	}
	if eventCount != 1 {
		t.Fatalf("eventos da receipt não preservados: %d", eventCount)
	}

	sqlDB, err := fixture.db.DB()
	if err != nil {
		t.Fatal(err)
	}
	if err := sqlDB.Close(); err != nil {
		t.Fatal(err)
	}
	reopened := openBootstrapTestDB(t, fixture.path)
	if err := Migrate(context.Background(), reopened); err != nil {
		t.Fatalf("reabrir schema v21: %v", err)
	}
	if migrationStamp(t, reopened, 21, "command_storage_initial") != 1 || migrationStamp(t, reopened, 22, "command_envelope_ownership") != 1 {
		t.Fatal("reabertura duplicou ou removeu carimbo")
	}
	if !hasColumn(t, reopened, "command_idempotency_keys", "actor_type") {
		t.Fatal("ownership desapareceu na reabertura")
	}
}

func TestMigrateKnownV20DriftFailsClosedAndRollsBackUpgrade(t *testing.T) {
	fixture := makeKnownV20Fixture(t)
	if err := fixture.db.Exec("DROP INDEX ux_command_layers_user_global_name").Error; err != nil {
		t.Fatal(err)
	}
	if err := fixture.db.Exec("CREATE INDEX ux_command_layers_user_global_name ON command_layers (name)").Error; err != nil {
		t.Fatal(err)
	}
	if err := Migrate(context.Background(), fixture.db); !errors.Is(err, ErrStorage) {
		t.Fatalf("drift aceito: %v", err)
	}
	if migrationStamp(t, fixture.db, 21, "command_storage_initial") != 1 || migrationStamp(t, fixture.db, 22, "command_envelope_ownership") != 0 {
		t.Fatal("falha de drift publicou carimbo")
	}
	if hasColumn(t, fixture.db, "command_idempotency_keys", "actor_type") || hasColumn(t, fixture.db, "command_idempotency_keys", "actor_id") || hasColumn(t, fixture.db, "command_idempotency_keys", "input_fingerprint") {
		t.Fatal("rollback deixou alteração parcial do ledger")
	}
	var layerName string
	if err := fixture.db.Raw("SELECT name FROM command_layers WHERE id = ?", fixture.layerID).Scan(&layerName).Error; err != nil {
		t.Fatal(err)
	}
	if layerName != "v20 fixture layer" {
		t.Fatalf("drift alterou dados: %q", layerName)
	}
}

func TestMigrateKnownV20ReceiptReorderedColumnsPreservesData(t *testing.T) {
	fixture := makeKnownV20Fixture(t)
	reorderKnownV20ReceiptColumns(t, fixture.db)
	if hasColumn(t, fixture.db, "command_idempotency_keys", "input_fingerprint") {
		t.Fatal("fixture v20 reordenada já contém input_fingerprint v21")
	}

	if err := Migrate(context.Background(), fixture.db); err != nil {
		t.Fatalf("upgrade v20→v21 com colunas de receipt reordenadas: %v", err)
	}
	if migrationStamp(t, fixture.db, 21, "command_storage_initial") != 1 || migrationStamp(t, fixture.db, 22, "command_envelope_ownership") != 1 {
		t.Fatal("upgrade reordenado não publicou os carimbos esperados")
	}
	var inputFingerprint *string
	if err := fixture.db.Raw("SELECT input_fingerprint FROM command_idempotency_keys WHERE id = ?", fixture.ledgerID).Scan(&inputFingerprint).Error; err != nil {
		t.Fatalf("ler input_fingerprint nullable após upgrade reordenado: %v", err)
	}
	if inputFingerprint != nil {
		t.Fatalf("input_fingerprint legado deveria permanecer NULL no upgrade reordenado: %q", *inputFingerprint)
	}

	var subjectType, status, fingerprint string
	if err := fixture.db.Raw("SELECT subject_type, status, request_fingerprint FROM command_decision_receipts WHERE decision_id = ?", fixture.receiptID).Row().Scan(&subjectType, &status, &fingerprint); err != nil {
		t.Fatalf("ler receipt preservada após upgrade reordenado: %v", err)
	}
	if subjectType != "config_mutation" || status != "accepted" || fingerprint != "fp-v20-receipt" {
		t.Fatalf("dados da receipt reordenada alterados: subject=%q status=%q fingerprint=%q", subjectType, status, fingerprint)
	}
	var eventCount int64
	if err := fixture.db.Raw("SELECT COUNT(*) FROM command_decision_receipt_events WHERE decision_id = ?", fixture.receiptID).Scan(&eventCount).Error; err != nil {
		t.Fatalf("contar eventos após upgrade reordenado: %v", err)
	}
	if eventCount != 1 {
		t.Fatalf("eventos da receipt reordenada não preservados: %d", eventCount)
	}
}
