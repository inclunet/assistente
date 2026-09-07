package database

import (
	"errors"

	"gorm.io/gorm"
)

// LegacyEditorDocument representa um rascunho preservado no SQLite pela 0.1.9.
type LegacyEditorDocument struct {
	ID       string
	Markdown string
}

// LegacyEditorData contém somente os campos necessários para transportar o
// editor 0.1.9 ao storage por usuário. As tabelas de origem não são apagadas.
type LegacyEditorData struct {
	SessionJSON string
	Documents   []LegacyEditorDocument
}

// LoadLegacyEditorData lê o storage instance-wide do editor 0.1.9. O caller
// deve reservar a adoção para exatamente um usuário antes de usar o retorno.
// A operação é read-only e idempotente.
func LoadLegacyEditorData() (LegacyEditorData, error) {
	if db == nil {
		return LegacyEditorData{}, nil
	}

	var result LegacyEditorData
	if db.Migrator().HasTable("editor_session_states") {
		var row struct {
			JSON string
		}
		err := db.Table("editor_session_states").
			Select("json").
			Where("id = ?", "default").
			Take(&row).Error
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return LegacyEditorData{}, err
		}
		result.SessionJSON = row.JSON
	}

	if db.Migrator().HasTable("editor_documents") {
		if err := db.Table("editor_documents").
			Select("id", "markdown").
			Order("id").
			Scan(&result.Documents).Error; err != nil {
			return LegacyEditorData{}, err
		}
	}
	return result, nil
}
