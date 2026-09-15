package commandbootstrap

import (
	"fmt"
	"strings"

	"assistente/internal/commandconfig"
	"gorm.io/gorm"
)

func legacyConfigObject(obj schemaObject) schemaObject {
	switch obj.Name {
	case "command_layers", "command_bindings", "command_config_generations":
		const opaque = "(length(workspace_id) BETWEEN 1 AND 256 AND trim(workspace_id) = workspace_id AND instr(workspace_id, char(0)) = 0)"
		const old = "(length(%s) = 36 AND length(replace(%s, '-', '')) = 32 AND lower(%s) = %s AND %s NOT GLOB '*[^0-9a-f-]*' AND substr(%s, 9, 1) = '-' AND substr(%s, 14, 1) = '-' AND substr(%s, 19, 1) = '-' AND substr(%s, 24, 1) = '-' AND substr(%s, 15, 1) = '7' AND substr(%s, 20, 1) IN ('8', '9', 'a', 'b'))"
		w := "workspace_id"
		obj.SQL = strings.ReplaceAll(obj.SQL, opaque, fmt.Sprintf(old, w, w, w, w, w, w, w, w, w, w, w))
	case "command_config_mutations":
		obj.SQL = commandconfig.LegacyMutationAuditSchema()
	}
	return obj
}

// v22–v25 já tinham documentos v2, mas ainda não os verbos de regras.
// A comparação continua integral; não se aceita um CHECK arbitrário.
func legacyRulesObject(obj schemaObject) schemaObject {
	if obj.Name == "command_config_mutations" {
		obj.SQL = strings.Replace(obj.SQL, "'rule_create','rule_update','rule_delete','rule_enable','rule_disable','rule_restore',", "", 1)
	}
	return obj
}

// Reconstrói apenas tabelas cuja origem inteira já foi comparada com v20/v21.
// Copia por nomes de coluna, mantém todas as linhas e refaz os índices depois.
func upgradeConfigObjects(tx *gorm.DB, want map[string]schemaObject) error {
	actual, err := objects(tx)
	if err != nil {
		return err
	}
	for _, obj := range actual {
		switch obj.Name {
		case "command_layers", "command_bindings", "command_config_generations", "command_config_mutations":
		default:
			continue
		}
		current := want[obj.Name]
		if normalizeDDL(current) == normalizeDDL(obj) {
			continue
		}
		if normalizeDDL(legacyConfigObject(current)) != normalizeDDL(obj) && normalizeDDL(legacyRulesObject(current)) != normalizeDDL(obj) {
			return ErrStorage
		}
		var columns []struct{ Name string }
		if err := tx.Raw("SELECT name FROM pragma_table_info(?) ORDER BY cid", obj.Name).Scan(&columns).Error; err != nil {
			return err
		}
		if len(columns) == 0 {
			return ErrStorage
		}
		list := make([]string, len(columns))
		for i, c := range columns {
			list[i] = "`" + strings.ReplaceAll(c.Name, "`", "``") + "`"
		}
		projection := strings.Join(list, ",")
		// Nomes são allowlist acima, jamais texto de import/payload.
		backup := obj.Name + "_v22_upgrade"
		if err := tx.Exec("ALTER TABLE " + obj.Name + " RENAME TO " + backup).Error; err != nil {
			return err
		}
		if err := tx.Exec(current.SQL).Error; err != nil {
			return err
		}
		if err := tx.Exec("INSERT INTO " + obj.Name + " (" + projection + ") SELECT " + projection + " FROM " + backup).Error; err != nil {
			return err
		}
		if err := tx.Exec("DROP TABLE " + backup).Error; err != nil {
			return err
		}
	}
	return nil
}
