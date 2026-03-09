package database

import (
	"errors"
	"strings"
	"time"

	"gorm.io/gorm"
)

// EditorSessionState armazena o estado global do Editor (abas abertas, preferências etc.)
// como JSON. Usamos um único registro por enquanto (ID="default").
type EditorSessionState struct {
	ID string `gorm:"primaryKey;size:64"`

	JSON string `gorm:"type:text"`

	CreatedAt time.Time
	UpdatedAt time.Time
}

func normalizeEditorSessionID(id string) string {
	id = strings.TrimSpace(id)
	if id == "" {
		return "default"
	}
	return id
}

// UpsertEditorSessionJSON salva a sessão do editor como JSON.
func UpsertEditorSessionJSON(id string, jsonPayload string) error {
	dbConn, err := ensureDB()
	if err != nil {
		return err
	}

	state := EditorSessionState{
		ID:   normalizeEditorSessionID(id),
		JSON: jsonPayload,
	}

	return dbConn.Save(&state).Error
}

// GetEditorSessionJSON retorna o JSON da sessão. found=false se não existir.
func GetEditorSessionJSON(id string) (jsonPayload string, found bool, err error) {
	dbConn, err := ensureDB()
	if err != nil {
		return "", false, err
	}

	var state EditorSessionState
	q := dbConn.Select("json").First(&state, "id = ?", normalizeEditorSessionID(id))
	if q.Error != nil {
		if errors.Is(q.Error, gorm.ErrRecordNotFound) {
			return "", false, nil
		}
		return "", false, q.Error
	}

	return state.JSON, true, nil
}
