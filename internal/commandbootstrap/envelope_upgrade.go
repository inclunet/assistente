package commandbootstrap

import (
	"gorm.io/gorm"
	"strings"
)

// A única origem adicional aceita é exatamente o schema v20 conhecido.
// Não se normalizam CHECKs, índices ou tipos para aceitar drift arbitrário.
func legacyEnvelopeObject(obj schemaObject) schemaObject {
	obj = legacyExternalContextObject(obj)
	switch obj.Name {
	case "command_idempotency_keys":
		obj.SQL = strings.ReplaceAll(obj.SQL, "`actor_type` text,", "")
		obj.SQL = strings.ReplaceAll(obj.SQL, "`actor_id` text,", "")
		obj.SQL = strings.ReplaceAll(obj.SQL, "`input_fingerprint` text,", "")
	case "command_decision_receipts":
		obj.SQL = strings.ReplaceAll(obj.SQL, "subject_type IN ('config_mutation','invocation')", "subject_type = 'config_mutation'")
	}
	return obj
}

// Schema imediatamente anterior à v32: subject invocation já era suportado,
// mas receipts eram restritas a uma sessão local.
func legacyExternalContextObject(obj schemaObject) schemaObject {
	if obj.Name == "command_decision_receipts" {
		obj.SQL = strings.ReplaceAll(obj.SQL, "CONSTRAINT `auth_context_subject`", "CONSTRAINT `chk_command_decision_receipts_auth_context_type`")
		obj.SQL = strings.ReplaceAll(obj.SQL, "CONSTRAINT auth_context_subject ", "CONSTRAINT chk_command_decision_receipts_auth_context_type ")
		obj.SQL = strings.ReplaceAll(obj.SQL, `CONSTRAINT "auth_context_subject"`, `CONSTRAINT "chk_command_decision_receipts_auth_context_type"`)
		obj.SQL = strings.ReplaceAll(obj.SQL,
			"auth_context_type IN ('local_session','external_token') AND (auth_context_type <> 'external_token' OR subject_type = 'invocation')",
			"auth_context_type = 'local_session'")
	}
	return obj
}

// Só chamada após comparar TODOS os objetos com as formas atuais/conhecidas.
// Aplica apenas a evolução de colunas da chave de idempotência; receipts têm
// rebuild separado e vinculado ao carimbo da v32.
func upgradeEnvelopeObjects(tx *gorm.DB, want map[string]schemaObject) error {
	actual, err := objects(tx)
	if err != nil {
		return err
	}
	for _, obj := range actual {
		current := want[obj.Name]
		if normalizeDDL(obj) == normalizeDDL(current) {
			continue
		}
		switch obj.Name {
		case "command_idempotency_keys":
			if err := tx.Exec("ALTER TABLE command_idempotency_keys ADD COLUMN `actor_type` text").Error; err != nil {
				return err
			}
			if err := tx.Exec("ALTER TABLE command_idempotency_keys ADD COLUMN `actor_id` text").Error; err != nil {
				return err
			}
			if err := tx.Exec("ALTER TABLE command_idempotency_keys ADD COLUMN `input_fingerprint` text").Error; err != nil {
				return err
			}
		default:
			if obj.Name != "command_decision_receipts" {
				return ErrStorage
			}
		}
	}
	return nil
}

// upgradeDecisionObjects reconstrói somente a tabela receipt cujo DDL antigo
// exato foi aceito por check(). A cópia e a recriação do índice acontecem na
// transação da migração v32; nenhum receipt ganha nova semântica por backfill.
func upgradeDecisionObjects(tx *gorm.DB, want map[string]schemaObject) error {
	actual, err := objects(tx)
	if err != nil {
		return err
	}
	for _, obj := range actual {
		if obj.Name != "command_decision_receipts" {
			continue
		}
		current := want[obj.Name]
		if normalizeDDL(obj) == normalizeDDL(current) {
			continue
		}
		if normalizeDDL(obj) != normalizeDDL(legacyExternalContextObject(current)) &&
			normalizeDDL(obj) != normalizeDDL(legacyEnvelopeObject(current)) {
			return ErrStorage
		}
		if err := rebuildDecisionReceipts(tx, current.SQL); err != nil {
			return err
		}
	}
	return nil
}

func rebuildDecisionReceipts(tx *gorm.DB, targetDDL string) error {
	var columns []struct{ Name string }
	if err := tx.Raw("SELECT name FROM pragma_table_info('command_decision_receipts') ORDER BY cid").Scan(&columns).Error; err != nil {
		return err
	}
	if len(columns) == 0 {
		return ErrStorage
	}
	projection := make([]string, len(columns))
	for i, col := range columns {
		projection[i] = "`" + strings.ReplaceAll(col.Name, "`", "``") + "`"
	}
	columnList := strings.Join(projection, ",")
	if err := tx.Exec("ALTER TABLE command_decision_receipts RENAME TO command_decision_receipts_upgrade").Error; err != nil {
		return err
	}
	if err := tx.Exec(targetDDL).Error; err != nil {
		return err
	}
	if err := tx.Exec("INSERT INTO command_decision_receipts (" + columnList + ") SELECT " + columnList + " FROM command_decision_receipts_upgrade").Error; err != nil {
		return err
	}
	return tx.Exec("DROP TABLE command_decision_receipts_upgrade").Error
}
