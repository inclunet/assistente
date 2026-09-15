package commandbootstrap

import (
	"gorm.io/gorm"
	"strings"
)

// A única origem adicional aceita é exatamente o schema v20 conhecido.
// Não se normalizam CHECKs, índices ou tipos para aceitar drift arbitrário.
func legacyEnvelopeObject(obj schemaObject) schemaObject {
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

// Só chamada após comparar TODOS os objetos com a referência atual/v20.
// O rebuild da receipt mantém dados/colunas e acontece na transação do carimbo.
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
		case "command_decision_receipts":
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
			if err := tx.Exec(current.SQL).Error; err != nil {
				return err
			}
			// Mesmo conjunto de colunas; SubjectType apenas amplia CHECK. Não
			// converter receipts antigas em autorizações de execução.
			if err := tx.Exec("INSERT INTO command_decision_receipts (" + columnList + ") SELECT " + columnList + " FROM command_decision_receipts_upgrade").Error; err != nil {
				return err
			}
			if err := tx.Exec("DROP TABLE command_decision_receipts_upgrade").Error; err != nil {
				return err
			}
		default:
			return ErrStorage
		}
	}
	return nil
}
