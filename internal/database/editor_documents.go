package database

import (
	"errors"
	"strings"
	"time"

	"gorm.io/gorm"
)

type CleanupOrphanEditorDocumentsArgs struct {
	KeepIDs       []string
	UpdatedBefore time.Time
}

// EditorDocument representa um documento/rascunho do Editor persistido no SQLite.
//
// Nota: nesta fase, usamos o ID como chave estável (ex.: draftId) para suportar
// recuperação pós-crash e merge sessions, mantendo compatibilidade com o modelo atual.
type EditorDocument struct {
	ID string `json:"id" gorm:"primaryKey;size:128"`

	Title    string `json:"title" gorm:"default:''"`
	Mode     string `json:"mode" gorm:"default:'markdown';size:16"`
	FilePath string `json:"filePath" gorm:"index;size:1024"`

	// Conteúdo fonte-de-verdade: Markdown.
	Markdown string `json:"markdown" gorm:"type:text"`

	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

func ensureDB() (*gorm.DB, error) {
	if db == nil {
		return nil, errors.New("database não inicializado")
	}
	return db, nil
}

func normalizeEditorDocumentID(id string) string {
	return strings.TrimSpace(id)
}

// UpsertEditorDocument salva ou atualiza um documento pelo ID.
func UpsertEditorDocument(doc EditorDocument) error {
	db, err := ensureDB()
	if err != nil {
		return err
	}

	doc.ID = normalizeEditorDocumentID(doc.ID)
	if doc.ID == "" {
		return errors.New("editor document id vazio")
	}

	return db.Save(&doc).Error
}

// GetEditorDocumentMarkdown retorna o markdown do documento por ID.
// found=false quando não existe.
func GetEditorDocumentMarkdown(id string) (markdown string, found bool, err error) {
	dbConn, err := ensureDB()
	if err != nil {
		return "", false, err
	}

	normalized := normalizeEditorDocumentID(id)
	if normalized == "" {
		return "", false, errors.New("editor document id vazio")
	}

	var doc EditorDocument
	q := dbConn.Select("markdown").First(&doc, "id = ?", normalized)
	if q.Error != nil {
		if errors.Is(q.Error, gorm.ErrRecordNotFound) {
			return "", false, nil
		}
		return "", false, q.Error
	}

	return doc.Markdown, true, nil
}

// DeleteEditorDocument remove um documento por ID.
func DeleteEditorDocument(id string) error {
	db, err := ensureDB()
	if err != nil {
		return err
	}

	normalized := normalizeEditorDocumentID(id)
	if normalized == "" {
		return errors.New("editor document id vazio")
	}

	return db.Delete(&EditorDocument{}, "id = ?", normalized).Error
}

// CleanupOrphanEditorDocuments remove documentos antigos que não estão mais referenciados.
//
// Uso típico: no startup, passar KeepIDs extraídos da sessão atual do editor e
// UpdatedBefore como um cutoff (grace period) para evitar apagar conteúdo recém-criado.
func CleanupOrphanEditorDocuments(args CleanupOrphanEditorDocumentsArgs) (deleted int64, err error) {
	dbConn, err := ensureDB()
	if err != nil {
		return 0, err
	}

	cutoff := args.UpdatedBefore
	if cutoff.IsZero() {
		return 0, errors.New("UpdatedBefore não pode ser zero")
	}

	keep := make([]string, 0, len(args.KeepIDs))
	seen := map[string]struct{}{}
	for _, id := range args.KeepIDs {
		n := normalizeEditorDocumentID(id)
		if n == "" {
			continue
		}
		if _, ok := seen[n]; ok {
			continue
		}
		seen[n] = struct{}{}
		keep = append(keep, n)
	}

	q := dbConn.Where("updated_at < ?", cutoff)
	if len(keep) > 0 {
		q = q.Where("id NOT IN ?", keep)
	}

	res := q.Delete(&EditorDocument{})
	return res.RowsAffected, res.Error
}
