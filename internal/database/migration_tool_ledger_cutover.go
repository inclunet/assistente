package database

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"assistente/internal/logging"

	"gorm.io/gorm"
)

const toolLedgerCutoverVersion = 19

type toolLedgerCutoverBackup struct {
	Path   string
	SHA256 string
	Bytes  int64
}

func migrateToolLedgerPhysicalCutover(database *gorm.DB) error {
	if database == nil {
		return errors.New("database não configurado para o cutover do ledger")
	}
	hasLegacyRuns := database.Migrator().HasColumn("job_runs", "tool_name") ||
		database.Migrator().HasColumn("job_runs", "inputs") ||
		database.Migrator().HasColumn("job_runs", "output")

	startedAt := time.Now()
	if err := migrateToolLedgerBackfill(database); err != nil {
		return fmt.Errorf("gate de backfill: %w", err)
	}
	if err := verifyToolLedgerCutoverGate(database); err != nil {
		return err
	}
	backup, err := createToolLedgerCutoverBackup(database)
	if err != nil {
		return fmt.Errorf("backup pré-cutover: %w", err)
	}

	if err := database.Connection(func(connection *gorm.DB) error {
		var foreignKeysEnabled int
		if err := connection.Raw(`PRAGMA foreign_keys`).Scan(&foreignKeysEnabled).Error; err != nil {
			return fmt.Errorf("consultar estado de foreign keys antes do rebuild: %w", err)
		}
		if err := connection.Exec(`PRAGMA foreign_keys = OFF`).Error; err != nil {
			return fmt.Errorf("desabilitar foreign keys durante rebuild: %w", err)
		}
		rebuildErr := connection.Transaction(func(tx *gorm.DB) error {
			for _, statement := range []string{
				`DROP TRIGGER IF EXISTS chat_messages_fts_insert`,
				`DROP TRIGGER IF EXISTS chat_messages_fts_delete`,
				`DROP TRIGGER IF EXISTS chat_messages_fts_update`,
				`DROP TABLE IF EXISTS chat_messages_fts`,
			} {
				if err := tx.Exec(statement).Error; err != nil {
					return err
				}
			}
			if err := rebuildCanonicalChatMessages(tx); err != nil {
				return err
			}
			if hasLegacyRuns {
				if err := rebuildOperationalJobRuns(tx); err != nil {
					return err
				}
			}
			if err := recreateCutoverIndexes(tx); err != nil {
				return err
			}
			return tx.Model(&ToolLedgerMigrationState{}).
				Where("state = ?", toolLedgerStateBackfilled).
				Update("state", toolLedgerStateCanonical).Error
		})
		restorePragma := `PRAGMA foreign_keys = OFF`
		if foreignKeysEnabled != 0 {
			restorePragma = `PRAGMA foreign_keys = ON`
		}
		if restoreErr := connection.Exec(restorePragma).Error; restoreErr != nil {
			restoreErr = fmt.Errorf("restaurar foreign keys após rebuild: %w", restoreErr)
			if rebuildErr != nil {
				return errors.Join(rebuildErr, restoreErr)
			}
			return restoreErr
		}
		return rebuildErr
	}); err != nil {
		return fmt.Errorf("rebuild físico: %w", err)
	}
	if err := validateToolLedgerCutover(database); err != nil {
		return err
	}
	logging.Infof(
		context.Background(),
		"database.tool-ledger.cutover",
		"cutover concluído version=%d duration_ms=%d backup_bytes=%d backup_sha256=%s",
		toolLedgerCutoverVersion,
		time.Since(startedAt).Milliseconds(),
		backup.Bytes,
		backup.SHA256,
	)
	return nil
}

func verifyToolLedgerCutoverGate(database *gorm.DB) error {
	if !database.Migrator().HasTable(&ToolLedgerMigrationState{}) {
		return errors.New("cutover bloqueado: estado do backfill ausente")
	}
	var blocked int64
	if err := database.Model(&ToolLedgerMigrationState{}).
		Where(`state <> ? OR ambiguous_count <> 0 OR last_error_code <> ''
			OR legacy_input_digest <> ledger_input_digest
			OR legacy_output_digest <> ledger_output_digest`, toolLedgerStateBackfilled).
		Count(&blocked).Error; err != nil {
		return fmt.Errorf("consultar gate do cutover: %w", err)
	}
	if blocked != 0 {
		return fmt.Errorf("cutover bloqueado: %d recursos sem reconciliação integral", blocked)
	}
	return nil
}

func createToolLedgerCutoverBackup(database *gorm.DB) (toolLedgerCutoverBackup, error) {
	sqlDB, err := database.DB()
	if err != nil {
		return toolLedgerCutoverBackup{}, err
	}
	path, err := sqliteMainDatabasePath(sqlDB)
	if err != nil {
		return toolLedgerCutoverBackup{}, err
	}
	if path == "" || path == ":memory:" {
		return toolLedgerCutoverBackup{}, nil
	}
	if _, err := sqlDB.Exec(`PRAGMA wal_checkpoint(FULL)`); err != nil {
		return toolLedgerCutoverBackup{}, fmt.Errorf("checkpoint WAL: %w", err)
	}
	backupPath := path + ".pre-tool-ledger-v19.bak"
	if err := archivePreviousToolLedgerBackup(backupPath); err != nil {
		return toolLedgerCutoverBackup{}, err
	}
	if err := verifyToolLedgerBackupSpace(path, backupPath); err != nil {
		return toolLedgerCutoverBackup{}, err
	}
	if _, err := sqlDB.Exec(`VACUUM INTO '` + strings.ReplaceAll(backupPath, "'", "''") + `'`); err != nil {
		return toolLedgerCutoverBackup{}, err
	}
	digest, size, err := fileSHA256(backupPath)
	if err != nil {
		return toolLedgerCutoverBackup{}, err
	}
	if size == 0 {
		return toolLedgerCutoverBackup{}, errors.New("backup pré-cutover vazio")
	}
	manifest := fmt.Sprintf("sha256=%s\nbytes=%d\nsource=%s\n", digest, size, filepath.Base(path))
	if err := os.WriteFile(backupPath+".manifest", []byte(manifest), 0o600); err != nil {
		return toolLedgerCutoverBackup{}, fmt.Errorf("gravar manifesto do backup: %w", err)
	}
	return toolLedgerCutoverBackup{Path: backupPath, SHA256: digest, Bytes: size}, nil
}

func archivePreviousToolLedgerBackup(backupPath string) error {
	if _, err := os.Stat(backupPath); errors.Is(err, os.ErrNotExist) {
		return nil
	} else if err != nil {
		return fmt.Errorf("inspecionar backup pré-cutover anterior: %w", err)
	}
	suffix := ".previous-" + time.Now().UTC().Format("20060102T150405.000000000Z")
	archivedPath := backupPath + suffix
	if err := os.Rename(backupPath, archivedPath); err != nil {
		return fmt.Errorf("preservar backup pré-cutover anterior: %w", err)
	}
	manifestPath := backupPath + ".manifest"
	if _, err := os.Stat(manifestPath); err == nil {
		if err := os.Rename(manifestPath, archivedPath+".manifest"); err != nil {
			return fmt.Errorf("preservar manifesto pré-cutover anterior: %w", err)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("inspecionar manifesto pré-cutover anterior: %w", err)
	}
	return nil
}

func verifyToolLedgerBackupSpace(sourcePath, backupPath string) error {
	source, err := os.Stat(sourcePath)
	if err != nil {
		return fmt.Errorf("medir banco antes do backup: %w", err)
	}
	requiredBytes := source.Size() + 1024*1024
	probe, err := os.CreateTemp(filepath.Dir(backupPath), ".tool-ledger-space-check-*")
	if err != nil {
		return fmt.Errorf("criar verificação de espaço do backup: %w", err)
	}
	probePath := probe.Name()
	defer func() {
		_ = os.Remove(probePath)
	}()

	chunk := make([]byte, 1024*1024)
	var written int64
	for written < requiredBytes {
		next := int64(len(chunk))
		if remaining := requiredBytes - written; remaining < next {
			next = remaining
		}
		count, writeErr := probe.Write(chunk[:int(next)])
		written += int64(count)
		if writeErr != nil {
			_ = probe.Close()
			return fmt.Errorf(
				"espaço insuficiente para backup pré-cutover (necessários=%d bytes): %w",
				requiredBytes,
				writeErr,
			)
		}
		if count == 0 {
			_ = probe.Close()
			return fmt.Errorf(
				"espaço insuficiente para backup pré-cutover (necessários=%d bytes): %w",
				requiredBytes,
				io.ErrShortWrite,
			)
		}
	}
	if err := probe.Sync(); err != nil {
		_ = probe.Close()
		return fmt.Errorf("confirmar espaço do backup pré-cutover: %w", err)
	}
	if err := probe.Close(); err != nil {
		return fmt.Errorf("fechar verificação de espaço do backup: %w", err)
	}
	return nil
}

func sqliteMainDatabasePath(database *sql.DB) (string, error) {
	rows, err := database.Query(`PRAGMA database_list`)
	if err != nil {
		return "", err
	}
	var mainPath string
	for rows.Next() {
		var sequence int
		var name, path string
		if err := rows.Scan(&sequence, &name, &path); err != nil {
			_ = rows.Close()
			return "", err
		}
		if name == "main" {
			mainPath = path
			break
		}
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return "", err
	}
	if err := rows.Close(); err != nil {
		return "", err
	}
	return mainPath, nil
}

func fileSHA256(path string) (string, int64, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", 0, err
	}
	hash := sha256.New()
	size, err := io.Copy(hash, file)
	if err != nil {
		_ = file.Close()
		return "", 0, err
	}
	if err := file.Close(); err != nil {
		return "", 0, err
	}
	return hex.EncodeToString(hash.Sum(nil)), size, nil
}

func rebuildCanonicalChatMessages(tx *gorm.DB) error {
	var before, technical int64
	if err := tx.Table("chat_messages").Count(&before).Error; err != nil {
		return err
	}
	if err := tx.Table("chat_messages").Where("lower(trim(role)) = 'tool'").Count(&technical).Error; err != nil {
		return err
	}
	statements := []string{
		"CREATE TABLE `chat_messages_cutover` (`id` text,`created_at` datetime,`updated_at` datetime,`conversation_id` text,`parent_id` text,`turn_id` text,`role` text,`content` text,`reasoning` text,`media` text,`audio` text,`audio_mime_type` text,`prompt_tokens` integer,`completion_tokens` integer,`total_tokens` integer,`cache_read_tokens` integer DEFAULT 0,`cache_write_tokens` integer DEFAULT 0,`cache_miss_tokens` integer DEFAULT 0,`model` text,`source` text,`pinned` numeric NOT NULL DEFAULT false,PRIMARY KEY (`id`),CONSTRAINT `fk_conversations_messages` FOREIGN KEY (`conversation_id`) REFERENCES `conversations`(`id`),CONSTRAINT `chk_chat_messages_conversational` CHECK (lower(trim(role)) <> 'tool'))",
		`INSERT INTO chat_messages_cutover (
			id, created_at, updated_at, conversation_id, parent_id, turn_id, role,
			content, reasoning, media, audio, audio_mime_type, prompt_tokens,
			completion_tokens, total_tokens, cache_read_tokens, cache_write_tokens,
			cache_miss_tokens, model, source, pinned
		)
		SELECT id, created_at, updated_at, conversation_id, parent_id, turn_id, role,
			content, reasoning, media, audio, audio_mime_type, prompt_tokens,
			completion_tokens, total_tokens, cache_read_tokens, cache_write_tokens,
			cache_miss_tokens, model, source, pinned
		FROM chat_messages WHERE lower(trim(role)) <> 'tool'`,
		`DROP TABLE chat_messages`,
		`ALTER TABLE chat_messages_cutover RENAME TO chat_messages`,
	}
	for _, statement := range statements {
		if err := tx.Exec(statement).Error; err != nil {
			return err
		}
	}
	var after int64
	if err := tx.Table("chat_messages").Count(&after).Error; err != nil {
		return err
	}
	if after != before-technical {
		return fmt.Errorf("conservação de chat_messages falhou: antes=%d técnicas=%d depois=%d", before, technical, after)
	}
	return nil
}

func rebuildOperationalJobRuns(tx *gorm.DB) error {
	// Cutover legado pode rodar DEPOIS da v24/AutoMigrate. Preservar as
	// colunas operacionais novas evita perdê-las na reconstrução da v19.
	if err := migrateCommandJobQueuedAt(tx); err != nil {
		return err
	}
	for _, column := range []string{"root_origin_type", "root_origin_id", "provenance"} {
		if !tx.Migrator().HasColumn("job_runs", column) {
			if err := tx.Exec("ALTER TABLE job_runs ADD COLUMN " + column + " TEXT").Error; err != nil {
				return err
			}
		}
	}
	var before int64
	if err := tx.Table("job_runs").Count(&before).Error; err != nil {
		return err
	}
	statements := []string{
		"CREATE TABLE `job_runs_cutover` (`id` text,`created_at` datetime,`updated_at` datetime,`user_id` text NOT NULL,`job_id` text NOT NULL,`trigger_id` text NOT NULL,`status` text NOT NULL,`queued_at` datetime NOT NULL,`started_at` datetime,`completed_at` datetime,`duration_ms` integer,`error` text,`retry_count` integer,`is_dry_run` numeric,`trigger_data` text,`events_emitted` text,`root_origin_type` text,`root_origin_id` text,`provenance` text,PRIMARY KEY (`id`),CONSTRAINT `fk_job_runs_user` FOREIGN KEY (`user_id`) REFERENCES `users`(`id`),CONSTRAINT `fk_job_runs_trigger` FOREIGN KEY (`trigger_id`) REFERENCES `job_triggers`(`id`),CONSTRAINT `fk_jobs_runs` FOREIGN KEY (`job_id`) REFERENCES `jobs`(`id`))",
		`INSERT INTO job_runs_cutover (
			id, created_at, updated_at, user_id, job_id, trigger_id, status,
			started_at, completed_at, duration_ms, error, retry_count, is_dry_run,
			trigger_data, events_emitted, queued_at, root_origin_type, root_origin_id, provenance
		)
		SELECT id, created_at, updated_at, user_id, job_id, trigger_id, status,
			started_at, completed_at, duration_ms, error, retry_count, is_dry_run,
			trigger_data, events_emitted, queued_at, root_origin_type, root_origin_id, provenance
		FROM job_runs`,
		`DROP TABLE job_runs`,
		`ALTER TABLE job_runs_cutover RENAME TO job_runs`,
	}
	for _, statement := range statements {
		if err := tx.Exec(statement).Error; err != nil {
			return err
		}
	}
	var after int64
	if err := tx.Table("job_runs").Count(&after).Error; err != nil {
		return err
	}
	if after != before {
		return fmt.Errorf("conservação de job_runs falhou: antes=%d depois=%d", before, after)
	}
	return nil
}

func recreateCutoverIndexes(tx *gorm.DB) error {
	statements := []string{
		`CREATE INDEX IF NOT EXISTS idx_chat_messages_conversation_id ON chat_messages (conversation_id)`,
		`CREATE INDEX IF NOT EXISTS idx_chat_messages_parent_id ON chat_messages (parent_id)`,
		`CREATE INDEX IF NOT EXISTS idx_chat_messages_turn_id ON chat_messages (turn_id)`,
		`CREATE INDEX IF NOT EXISTS idx_chat_messages_window ON chat_messages (conversation_id, parent_id, created_at, id)`,
		`CREATE INDEX IF NOT EXISTS idx_chat_messages_timeline_window ON chat_messages (conversation_id, parent_id, turn_id, created_at, id)`,
		`CREATE INDEX IF NOT EXISTS idx_chat_messages_created_at ON chat_messages (conversation_id, created_at, id)`,
		`CREATE INDEX IF NOT EXISTS idx_chat_messages_updated_at ON chat_messages (conversation_id, updated_at)`,
		`CREATE INDEX IF NOT EXISTS idx_chat_messages_conversation_pinned_created ON chat_messages (conversation_id, pinned, created_at, id)`,
		`CREATE INDEX IF NOT EXISTS idx_job_runs_user_id ON job_runs (user_id)`,
		`CREATE INDEX IF NOT EXISTS idx_job_runs_job_id ON job_runs (job_id)`,
		`CREATE INDEX IF NOT EXISTS idx_job_runs_trigger_id ON job_runs (trigger_id)`,
		`CREATE INDEX IF NOT EXISTS idx_job_runs_status ON job_runs (status)`,
		`CREATE INDEX IF NOT EXISTS idx_job_runs_started_at ON job_runs (started_at)`,
		`CREATE INDEX IF NOT EXISTS idx_job_runs_queued_at ON job_runs (queued_at)`,
		`CREATE INDEX IF NOT EXISTS idx_job_runs_root_origin_type ON job_runs (root_origin_type)`,
		`CREATE INDEX IF NOT EXISTS idx_job_runs_is_dry_run ON job_runs (is_dry_run)`,
		`CREATE INDEX IF NOT EXISTS idx_job_runs_user_job_started_at ON job_runs (user_id, job_id, started_at)`,
		`CREATE INDEX IF NOT EXISTS idx_job_runs_user_started_at ON job_runs (user_id, started_at)`,
	}
	for _, statement := range statements {
		if err := tx.Exec(statement).Error; err != nil {
			return err
		}
	}
	return nil
}

func validateToolLedgerCutover(database *gorm.DB) error {
	for table, columns := range map[string][]string{
		"chat_messages": {"tool_calls", "tool_call_id"},
		"job_runs":      {"tool_name", "inputs", "output"},
	} {
		for _, column := range columns {
			if database.Migrator().HasColumn(table, column) {
				return fmt.Errorf("cutover incompleto: %s.%s ainda existe", table, column)
			}
		}
	}
	var technical int64
	if err := database.Table("chat_messages").Where("lower(trim(role)) = 'tool'").Count(&technical).Error; err != nil {
		return err
	}
	if technical != 0 {
		return fmt.Errorf("cutover incompleto: %d mensagens role=tool", technical)
	}
	var integrity string
	if err := database.Raw(`PRAGMA integrity_check`).Scan(&integrity).Error; err != nil {
		return err
	}
	if integrity != "ok" {
		return fmt.Errorf("integrity_check falhou: %s", integrity)
	}
	type foreignKeyViolation struct {
		Table  string
		RowID  int64
		Parent string
		FKID   int
	}
	var violations []foreignKeyViolation
	if err := database.Raw(`PRAGMA foreign_key_check`).Scan(&violations).Error; err != nil {
		return err
	}
	if len(violations) != 0 {
		return fmt.Errorf("foreign_key_check encontrou %d violações", len(violations))
	}
	return nil
}
