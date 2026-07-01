package database

import (
	"assistente/internal/logging"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
)

// migrateToUUIDv7 converte todas as tabelas de INTEGER PK para TEXT UUIDv7.
// Executada automaticamente no Init() se detectar schema antigo.
// Transação atômica — falha reverte tudo.
func migrateToUUIDv7() error {
	sqlDB, err := db.DB()
	if err != nil {
		return fmt.Errorf("erro ao obter *sql.DB: %w", err)
	}

	// Verificar se a migração é necessária
	needed, err := isUUIDMigrationNeeded(sqlDB)
	if err != nil {
		return fmt.Errorf("erro ao verificar necessidade de migração: %w", err)
	}
	if !needed {
		return nil
	}

	startedAt := time.Now()
	logging.Println(context.Background(), "database.migration-uuid", "[Migration] Iniciando migração de IDs INTEGER → UUIDv7...")
	defer func() {
		logging.Errorf(context.Background(), "database.migration-uuid", "[Migration] migrateToUUIDv7 finalizado em %s", time.Since(startedAt).Truncate(time.Millisecond))
	}()

	// Backup antes de migrar.
	// Decisão de design: falha de backup NÃO aborta a migração.
	// A transação atômica (tx.Begin / tx.Commit) é o mecanismo primário de
	// segurança — qualquer erro durante a migração causa rollback completo.
	// O backup é best-effort para facilitar recuperação manual em cenários
	// onde o SQLite esteja em estado inconsistente (corrupção prévia, etc.).
	if err := createBackup(); err != nil {
		logging.Warnf(context.Background(), "database.migration-uuid", "[Migration] Aviso: não foi possível criar backup: %v", err)
	}

	tx, err := sqlDB.Begin()
	if err != nil {
		return fmt.Errorf("erro ao iniciar transação: %w", err)
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	// Dropar FTS5 e triggers antes de mexer em chat_messages.
	// DROP TABLE chat_messages (abaixo) auto-dropa triggers associados,
	// mas a virtual table FTS5 é independente e precisa ser dropada explicitamente.
	_, _ = tx.Exec(`DROP TABLE IF EXISTS chat_messages_fts`)
	_, _ = tx.Exec(`DROP TRIGGER IF EXISTS chat_messages_fts_insert`)
	_, _ = tx.Exec(`DROP TRIGGER IF EXISTS chat_messages_fts_delete`)
	_, _ = tx.Exec(`DROP TRIGGER IF EXISTS chat_messages_fts_update`)

	// === Fase 1: Tabelas sem dependências ===

	// 1. credential_entries
	credMap, err := migrateTable(tx, "credential_entries", []string{
		"id TEXT PRIMARY KEY",
		"pattern TEXT",
		"auth_type TEXT",
		"token_enc TEXT",
		"username TEXT",
		"password_enc TEXT",
		"headers_enc TEXT",
		"expires_at INTEGER DEFAULT 0",
		"refresh_token_enc TEXT",
		"client_id_enc TEXT",
		"client_secret_enc TEXT",
		"created_at DATETIME",
		"updated_at DATETIME",
	}, nil)
	if err != nil {
		return fmt.Errorf("erro ao migrar credential_entries: %w", err)
	}
	logging.Infof(context.Background(), "database.migration-uuid", "[Migration] credential_entries: %d registros migrados", len(credMap))

	// 2. credential_key_wraps (sem FKs)
	kwMap, err := migrateTable(tx, "credential_key_wraps", []string{
		"id TEXT PRIMARY KEY",
		"kind TEXT",
		"salt TEXT",
		"wrapped_dek TEXT",
		"argon_time INTEGER DEFAULT 0",
		"argon_memory INTEGER DEFAULT 0",
		"argon_threads INTEGER DEFAULT 0",
		"created_at DATETIME",
		"updated_at DATETIME",
	}, nil)
	if err != nil {
		return fmt.Errorf("erro ao migrar credential_key_wraps: %w", err)
	}
	logging.Infof(context.Background(), "database.migration-uuid", "[Migration] credential_key_wraps: %d registros migrados", len(kwMap))

	// 3. conversations
	convMap, err := migrateTable(tx, "conversations", []string{
		"id TEXT PRIMARY KEY",
		"title TEXT",
		"channel TEXT",
		"contact_id TEXT",
		"summary TEXT",
		"summary_up_to_message_id TEXT", // será atualizado no 2° passe
		"summarizing_in_progress INTEGER DEFAULT 0",
		"created_at DATETIME",
		"updated_at DATETIME",
	}, nil)
	if err != nil {
		return fmt.Errorf("erro ao migrar conversations: %w", err)
	}
	logging.Infof(context.Background(), "database.migration-uuid", "[Migration] conversations: %d registros migrados", len(convMap))

	// 4. chat_messages (FK: conversation_id, parent_id, turn_id)
	msgMap, err := migrateTable(tx, "chat_messages", []string{
		"id TEXT PRIMARY KEY",
		"conversation_id TEXT",
		"parent_id TEXT",
		"turn_id TEXT",
		"role TEXT",
		"content TEXT",
		"reasoning TEXT",
		"media TEXT",
		"audio TEXT",
		"audio_mime_type TEXT",
		"tool_calls TEXT",
		"tool_call_id TEXT",
		"prompt_tokens INTEGER DEFAULT 0",
		"completion_tokens INTEGER DEFAULT 0",
		"total_tokens INTEGER DEFAULT 0",
		"model TEXT",
		"source TEXT",
		"created_at DATETIME",
		"updated_at DATETIME",
	}, map[string]map[uint]string{
		"conversation_id": convMap,
		"parent_id":       nil, // self-ref, resolvido inline
		"turn_id":         nil, // self-ref, resolvido inline
	})
	if err != nil {
		return fmt.Errorf("erro ao migrar chat_messages: %w", err)
	}
	logging.Infof(context.Background(), "database.migration-uuid", "[Migration] chat_messages: %d registros migrados", len(msgMap))

	// 5. conversations (2° passe) — atualizar summary_up_to_message_id
	if err := updateConversationSummaryRefs(tx, msgMap); err != nil {
		return fmt.Errorf("erro ao atualizar summary_up_to_message_id: %w", err)
	}

	// 6. task_lists
	tlMap, err := migrateTable(tx, "task_lists", []string{
		"id TEXT PRIMARY KEY",
		"title TEXT",
		"slug TEXT",
		"description TEXT",
		"preferred_view_mode TEXT DEFAULT 'list'",
		"validation_policy TEXT",
		"created_at DATETIME",
		"updated_at DATETIME",
	}, nil)
	if err != nil {
		return fmt.Errorf("erro ao migrar task_lists: %w", err)
	}
	logging.Infof(context.Background(), "database.migration-uuid", "[Migration] task_lists: %d registros migrados", len(tlMap))

	// 7. task_list_workflows (FK: task_list_id)
	wfMap, err := migrateTable(tx, "task_list_workflows", []string{
		"id TEXT PRIMARY KEY",
		"task_list_id TEXT",
		"statuses TEXT",
		"allowed_transitions TEXT",
		"initial_status_id INTEGER DEFAULT 1",
		"created_at DATETIME",
		"updated_at DATETIME",
	}, map[string]map[uint]string{
		"task_list_id": tlMap,
	})
	if err != nil {
		return fmt.Errorf("erro ao migrar task_list_workflows: %w", err)
	}
	logging.Infof(context.Background(), "database.migration-uuid", "[Migration] task_list_workflows: %d registros migrados", len(wfMap))

	// 8. tasks (FK: task_list_id, parent_id self-ref)
	taskMap, err := migrateTable(tx, "tasks", []string{
		"id TEXT PRIMARY KEY",
		"task_list_id TEXT",
		"title TEXT",
		"description TEXT",
		"code TEXT",
		"link TEXT",
		"status_id INTEGER NOT NULL DEFAULT 1",
		"parent_id TEXT",
		"\"order\" INTEGER DEFAULT 0",
		"assignee_name TEXT",
		"assignee_id TEXT",
		"creator_name TEXT",
		"creator_id TEXT",
		"due_date DATETIME",
		"created_at DATETIME",
		"updated_at DATETIME",
		"completed_at DATETIME",
	}, map[string]map[uint]string{
		"task_list_id": tlMap,
		"parent_id":    nil, // self-ref
	})
	if err != nil {
		return fmt.Errorf("erro ao migrar tasks: %w", err)
	}
	logging.Infof(context.Background(), "database.migration-uuid", "[Migration] tasks: %d registros migrados", len(taskMap))

	// 9. task_notes (FK: task_id)
	tnMap, err := migrateTable(tx, "task_notes", []string{
		"id TEXT PRIMARY KEY",
		"task_id TEXT",
		"type INTEGER DEFAULT 1",
		"content TEXT",
		"author_name TEXT",
		"author_id TEXT",
		"external_source TEXT",
		"external_id TEXT",
		"external_parent_id TEXT",
		"external_updated_at DATETIME",
		"created_at DATETIME",
		"updated_at DATETIME",
	}, map[string]map[uint]string{
		"task_id": taskMap,
	})
	if err != nil {
		return fmt.Errorf("erro ao migrar task_notes: %w", err)
	}
	logging.Infof(context.Background(), "database.migration-uuid", "[Migration] task_notes: %d registros migrados", len(tnMap))

	// Normalizar campos booleanos: SQLite não tem tipo bool nativo,
	// valores não-booleanos (ex: 4) causam "couldn't convert X into type bool".
	if _, err := tx.Exec(`UPDATE conversations SET summarizing_in_progress = CASE WHEN summarizing_in_progress > 0 THEN 1 ELSE 0 END WHERE summarizing_in_progress NOT IN (0, 1)`); err != nil {
		logging.Warnf(context.Background(), "database.migration-uuid", "[Migration] Aviso: normalização de summarizing_in_progress: %v", err)
	}

	// Persiste mapa de remapeamento ANTES do commit para minimizar janela de
	// crash sem remap. Se o commit falhar (rollback), o remap fica no disco
	// mas será ignorado na próxima abertura (o banco ainda terá IDs INTEGER,
	// e a migração será retentada do zero). Se o commit tiver sucesso, o remap
	// já estará disponível para o workspace manager.
	if err := persistIDRemapFile(sqlDB, convMap, tlMap); err != nil {
		logging.Warnf(context.Background(), "database.migration-uuid", "[Migration] Aviso: não foi possível salvar mapa de remapeamento: %v", err)
	}

	if err := tx.Commit(); err != nil {
		// Remap órfão pode ficar no disco — será ignorado pois a migração será
		// retentada na próxima abertura (banco permanece com IDs INTEGER).
		return fmt.Errorf("erro ao commit da migração: %w", err)
	}

	logging.Println(context.Background(), "database.migration-uuid", "[Migration] Migração UUIDv7 concluída com sucesso!")
	return nil
}

// isUUIDMigrationNeeded verifica se conversations.id ainda é INTEGER.
func isUUIDMigrationNeeded(sqlDB *sql.DB) (bool, error) {
	// Se a tabela não existe, não precisa migrar (será criada pelo AutoMigrate)
	var count int
	err := sqlDB.QueryRow(`SELECT count(*) FROM sqlite_master WHERE type='table' AND name='conversations'`).Scan(&count)
	if err != nil || count == 0 {
		return false, nil
	}

	rows, err := sqlDB.Query(`PRAGMA table_info(conversations)`)
	if err != nil {
		return false, err
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var cid int
		var name, colType string
		var notNull, pk int
		var dflt sql.NullString
		if err := rows.Scan(&cid, &name, &colType, &notNull, &dflt, &pk); err != nil {
			return false, err
		}
		if name == "id" {
			// Se o tipo é INTEGER, precisa migrar
			return strings.ToUpper(colType) == "INTEGER", nil
		}
	}

	return false, nil
}

// migrateTable migra uma tabela de INTEGER PK para TEXT UUIDv7.
// Retorna mapa old_id → new_uuid para resolução de FKs.
// fkMaps: nome_coluna → mapa de tradução (nil = self-ref, resolvido inline).
func migrateTable(tx *sql.Tx, tableName string, newCols []string, fkMaps map[string]map[uint]string) (map[uint]string, error) {
	// Verificar se tabela existe
	var tableExists int
	if err := tx.QueryRow(`SELECT count(*) FROM sqlite_master WHERE type='table' AND name=?`, tableName).Scan(&tableExists); err != nil || tableExists == 0 {
		return map[uint]string{}, nil
	}

	// Criar tabela nova
	newTableName := tableName + "_uuid_new"
	createSQL := fmt.Sprintf("CREATE TABLE %s (%s)", newTableName, strings.Join(newCols, ", "))
	if _, err := tx.Exec(createSQL); err != nil {
		return nil, fmt.Errorf("erro ao criar %s: %w", newTableName, err)
	}

	// Ler todos os registros antigos
	rows, err := tx.Query(fmt.Sprintf("SELECT * FROM %s ORDER BY id", tableName))
	if err != nil {
		return nil, fmt.Errorf("erro ao ler %s: %w", tableName, err)
	}
	defer func() { _ = rows.Close() }()

	cols, err := rows.Columns()
	if err != nil {
		return nil, err
	}

	// Mapear coluna nome → índice
	colIndex := make(map[string]int)
	for i, c := range cols {
		colIndex[c] = i
	}

	idMap := make(map[uint]string)
	orphanFKCount := make(map[string]int)

	// Determinar colunas da nova tabela (sem defaults/types)
	newColNames := make([]string, len(newCols))
	for i, col := range newCols {
		parts := strings.Fields(col)
		newColNames[i] = strings.Trim(parts[0], "\"")
	}

	// Só inserir colunas que existem em ambas as tabelas
	var insertCols []string
	for _, nc := range newColNames {
		if _, ok := colIndex[nc]; ok {
			insertCols = append(insertCols, nc)
		} else if nc == "id" {
			insertCols = append(insertCols, nc)
		}
	}

	placeholders := make([]string, len(insertCols))
	quotedCols := make([]string, len(insertCols))
	for i := range insertCols {
		placeholders[i] = "?"
		quotedCols[i] = fmt.Sprintf(`"%s"`, insertCols[i])
	}
	insertSQL := fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s)",
		newTableName,
		strings.Join(quotedCols, ", "),
		strings.Join(placeholders, ", "),
	)

	for rows.Next() {
		values := make([]interface{}, len(cols))
		valuePtrs := make([]interface{}, len(cols))
		for i := range values {
			valuePtrs[i] = &values[i]
		}
		if err := rows.Scan(valuePtrs...); err != nil {
			return nil, fmt.Errorf("erro ao ler linha de %s: %w", tableName, err)
		}

		// Obter old ID
		idIdx := colIndex["id"]
		oldID := toUint(values[idIdx])

		// Gerar novo UUID
		newUUID, err := uuid.NewV7()
		if err != nil {
			return nil, fmt.Errorf("erro ao gerar UUID: %w", err)
		}
		newID := newUUID.String()
		idMap[oldID] = newID

		// Construir valores para INSERT
		insertValues := make([]interface{}, len(insertCols))
		for i, colName := range insertCols {
			if colName == "id" {
				insertValues[i] = newID
				continue
			}

			// Verificar se é uma FK que precisa de tradução
			if fkMaps != nil {
				if fkMap, isFK := fkMaps[colName]; isFK {
					srcIdx, exists := colIndex[colName]
					if !exists {
						insertValues[i] = nil
						continue
					}
					oldVal := values[srcIdx]
					if oldVal == nil {
						insertValues[i] = nil
					} else {
						oldFKID := toUint(oldVal)
						if oldFKID == 0 {
							insertValues[i] = nil
						} else if fkMap != nil {
							// FK para outra tabela
							if newFK, ok := fkMap[oldFKID]; ok {
								insertValues[i] = newFK
							} else {
								orphanFKCount[colName]++
								insertValues[i] = nil // FK órfã → NULL
							}
						} else {
							// Self-reference — resolver do próprio idMap
							if newFK, ok := idMap[oldFKID]; ok {
								insertValues[i] = newFK
							} else {
								// Forward ref: gravar old ID como string para o 2° passe resolver
								insertValues[i] = fmt.Sprintf("%d", oldFKID)
							}
						}
					}
					continue
				}
			}

			// Coluna normal — copiar valor
			srcIdx, exists := colIndex[colName]
			if exists {
				insertValues[i] = values[srcIdx]
			} else {
				insertValues[i] = nil
			}
		}

		if _, err := tx.Exec(insertSQL, insertValues...); err != nil {
			return nil, fmt.Errorf("erro ao inserir em %s (old_id=%d): %w", newTableName, oldID, err)
		}
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Resolver self-references que apontam para registros processados depois
	// (caso parent_id > id na tabela original — gravados como string do old ID)
	for colName, fkMap := range fkMaps {
		if fkMap == nil { // self-ref
			for oldFK, newFK := range idMap {
				_, _ = tx.Exec(
					fmt.Sprintf(`UPDATE %s SET "%s" = ? WHERE "%s" = ?`, newTableName, colName, colName),
					newFK, fmt.Sprintf("%d", oldFK),
				)
			}
			// Limpar self-refs órfãs que não foram resolvidas pelo segundo passe
			// (strings numéricas que não estão no idMap → dados inconsistentes pré-migração)
			// Usa NOT GLOB '*-*' para excluir UUIDs (que contêm hífens)
			res, _ := tx.Exec(
				fmt.Sprintf(`UPDATE %s SET "%s" = NULL WHERE typeof("%s") = 'text' AND "%s" GLOB '[0-9]*' AND "%s" NOT GLOB '*-*'`, newTableName, colName, colName, colName, colName),
			)
			if res != nil {
				if cleaned, _ := res.RowsAffected(); cleaned > 0 {
					logging.Infof(context.Background(), "database.migration-uuid", "[Migration] %s.%s: %d self-refs órfãs limpas para NULL", tableName, colName, cleaned)
				}
			}
		}
	}

	// Logar FKs órfãs encontradas durante a migração
	for col, count := range orphanFKCount {
		logging.Infof(context.Background(), "database.migration-uuid", "[Migration] %s.%s: %d FKs órfãs resolvidas para NULL", tableName, col, count)
	}

	// Drop tabela antiga, rename nova
	if _, err := tx.Exec(fmt.Sprintf("DROP TABLE %s", tableName)); err != nil {
		return nil, fmt.Errorf("erro ao dropar %s: %w", tableName, err)
	}
	if _, err := tx.Exec(fmt.Sprintf("ALTER TABLE %s RENAME TO %s", newTableName, tableName)); err != nil {
		return nil, fmt.Errorf("erro ao renomear %s: %w", newTableName, err)
	}

	return idMap, nil
}

// updateConversationSummaryRefs atualiza conversations.summary_up_to_message_id com os novos UUIDs.
func updateConversationSummaryRefs(tx *sql.Tx, msgMap map[uint]string) error {
	for oldMsgID, newMsgID := range msgMap {
		_, err := tx.Exec(
			`UPDATE conversations SET summary_up_to_message_id = ? WHERE summary_up_to_message_id = ?`,
			newMsgID, fmt.Sprintf("%d", oldMsgID),
		)
		if err != nil {
			return err
		}
	}
	return nil
}

// toUint converte um valor de interface{} (retornado pelo Scan) para uint.
func toUint(v interface{}) uint {
	switch val := v.(type) {
	case int64:
		return uint(val)
	case float64:
		return uint(val)
	case int:
		return uint(val)
	case uint:
		return val
	case uint64:
		return uint(val)
	default:
		return 0
	}
}

// createBackup cria uma cópia do banco de dados antes da migração.
func createBackup() error {
	sqlDB, err := db.DB()
	if err != nil {
		return err
	}

	var dbPath string
	if err := sqlDB.QueryRow("PRAGMA database_list").Scan(new(int), new(string), &dbPath); err != nil {
		return err
	}

	if dbPath == "" || dbPath == ":memory:" {
		return nil // Nada para backup
	}

	// Flush WAL para o arquivo principal antes de copiar,
	// garantindo que o backup contenha todas as páginas recentes.
	if _, err := sqlDB.Exec("PRAGMA wal_checkpoint(FULL)"); err != nil {
		logging.Warnf(context.Background(), "database.migration-uuid", "[Migration] Aviso: wal_checkpoint falhou: %v", err)
	}

	backupPath := dbPath + ".pre-uuid.bak"
	src, err := os.ReadFile(dbPath)
	if err != nil {
		return err
	}
	if err := os.WriteFile(backupPath, src, 0600); err != nil {
		return err
	}
	logging.Infof(context.Background(), "database.migration-uuid", "[Migration] Backup criado: %s", backupPath)
	return nil
}

// IDRemapData contém os mapas de remapeamento old (numérico) → new (UUIDv7).
type IDRemapData struct {
	Conversations map[string]string `json:"conversations"` // "42" → "01926b90-..."
	TaskLists     map[string]string `json:"task_lists"`    // "3"  → "01926b90-..."
}

const idRemapFilename = "uuid-migration-remap.json"

// persistIDRemapFile salva os mapas de remapeamento em arquivo JSON
// no diretório do banco, para uso posterior pelo workspace manager.
func persistIDRemapFile(sqlDB *sql.DB, convMap, tlMap map[uint]string) error {
	var dbPath string
	if err := sqlDB.QueryRow("PRAGMA database_list").Scan(new(int), new(string), &dbPath); err != nil {
		return err
	}
	if dbPath == "" || dbPath == ":memory:" {
		return nil
	}

	data := IDRemapData{
		Conversations: make(map[string]string, len(convMap)),
		TaskLists:     make(map[string]string, len(tlMap)),
	}
	for old, newID := range convMap {
		data.Conversations[fmt.Sprintf("%d", old)] = newID
	}
	for old, newID := range tlMap {
		data.TaskLists[fmt.Sprintf("%d", old)] = newID
	}

	b, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return err
	}

	remapPath := filepath.Join(filepath.Dir(dbPath), idRemapFilename)
	if err := os.WriteFile(remapPath, b, 0600); err != nil {
		return err
	}
	logging.Infof(context.Background(), "database.migration-uuid", "[Migration] Mapa de remapeamento salvo: %s", remapPath)
	return nil
}

// LoadIDRemapFile lê o arquivo de remapeamento de IDs, se existir.
// Retorna nil se o arquivo não existir (já consumido ou não houve migração).
func LoadIDRemapFile(dir string) *IDRemapData {
	remapPath := filepath.Join(dir, idRemapFilename)
	b, err := os.ReadFile(remapPath)
	if err != nil {
		return nil
	}
	var data IDRemapData
	if err := json.Unmarshal(b, &data); err != nil {
		logging.Warnf(context.Background(), "database.migration-uuid", "[Migration] Aviso: erro ao ler mapa de remapeamento: %v", err)
		return nil
	}
	return &data
}

// DeleteIDRemapFile remove o arquivo de remapeamento após consumo.
func DeleteIDRemapFile(dir string) {
	remapPath := filepath.Join(dir, idRemapFilename)
	if err := os.Remove(remapPath); err != nil && !os.IsNotExist(err) {
		logging.Warnf(context.Background(), "database.migration-uuid", "[Migration] Aviso: erro ao remover mapa de remapeamento: %v", err)
	}
}
