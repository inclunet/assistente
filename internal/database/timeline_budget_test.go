package database

import (
	"assistente/internal/sqlite"
	"errors"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"strings"
	"testing"
)

func TestTimelineIdentitySelectionMasksBeforeScan(t *testing.T) {
	// Scan usa Recorder interno e não respeita ParameterizedQueries do logger.
	// Esta prova deliberadamente pesada não imprime o ID de 8 MiB como slow SQL;
	// erros de SQL continuam visíveis e todas as consultas têm erro verificado.
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Error)})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := raw.Close(); err != nil {
			t.Error(err)
		}
	})
	repo := NewMessageRepository(db)
	// A consulta seleciona dois itens, incluindo um ID com NUL; LENGTH(TEXT)
	// contaria só seu prefixo e deixaria a identidade excessiva passar.
	query := `SELECT 'message' kind, ? id, '' message_id, '' turn_id, '2026' created_at, 'anchor' anchor_id, 0 invalid_turn_reference, 0 slot, 'message:a' item_key, 12 window_total UNION ALL SELECT 'message', 'small', '', '', '2025', 'anchor', 0, 0, 'message:b', 12`
	huge := "prefix\x00" + strings.Repeat("i", MaxTimelineSourceBytes)
	var captured string
	if err := db.Callback().Row().After("gorm:row").Register("capture_identity_mask", func(tx *gorm.DB) { captured = tx.Statement.SQL.String() }); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.scanTimelineItems(query, []interface{}{huge}, timelineOrderAscending); !errors.Is(err, ErrTimelineSourceLimit) {
		t.Fatal(err)
	}
	// Executa o SQL capturado como linha crua: a identidade já sai vazia do banco,
	// não é removida por um guard Go posterior.
	var rows []struct {
		ID            string
		IdentityBytes int64
	}
	if err := db.Raw(captured, huge).Scan(&rows).Error; err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].ID != "" || rows[0].IdentityBytes <= MaxTimelineSourceBytes {
		t.Fatal("identidades excessivas atravessaram SQL")
	}
	// A recusa retorna somente o orçamento, sem transportar uma linha por item.
	// Mesmo as identidades redundantes, omitidas da projeção, mantêm sua cota.
	anchorQuery := `SELECT 'message' kind, 'small' id, 'small' message_id, '' turn_id, '2026' created_at, ? anchor_id, 0 invalid_turn_reference, 0 slot, 'message:small' item_key, 1 window_total`
	if _, err := repo.scanTimelineItems(anchorQuery, []interface{}{huge}, timelineOrderAscending); !errors.Is(err, ErrTimelineSourceLimit) {
		t.Fatal("âncora excedente deixou de participar do orçamento", err)
	}
	if items, err := repo.scanTimelineItems(anchorQuery+" WHERE 0", []interface{}{"small"}, timelineOrderAscending); err != nil || len(items) != 0 {
		t.Fatal("página vazia ganhou item de controle", items, err)
	}
	items, err := repo.scanTimelineItems(query, []interface{}{"normal"}, timelineOrderAscending)
	if err != nil || len(items) != 2 || items[0].ID != "small" || items[1].ID != "normal" || items[0].WindowTotal != 12 {
		t.Fatal("ordenação/contagem alteradas", err)
	}
}
