package database

import (
	"errors"
	"strconv"
	"strings"
)

const MaxTimelineSourceBytes = 8 * 1024 * 1024

var ErrTimelineSourceLimit = errors.New("base da projeção excede o limite de memória")

// Limita os parâmetros antes de codificar/copiar identidades opacas.
func CheckTimelineIdentityBudget(sets ...[]string) error {
	remaining := MaxTimelineSourceBytes
	for _, ids := range sets {
		for _, id := range ids {
			if len(id) > remaining/4 {
				return ErrTimelineSourceLimit
			}
			remaining -= 4*len(id) + 128
			if remaining < 0 {
				return ErrTimelineSourceLimit
			}
		}
	}
	return nil
}

// A subconsulta contém a seleção paginada única. A soma e o mascaramento
// acontecem no SQLite, antes de qualquer identidade excessiva chegar ao Go.
func (r *MessageRepository) scanTimelineItems(query string, args []interface{}, order string) ([]MessageWindowItem, error) {
	if !r.includeTimelineTurns {
		var rows []MessageWindowItem
		err := r.db.Raw(query, args...).Scan(&rows).Error
		return rows, err
	}
	columns := []string{"kind", "id", "message_id", "turn_id", "created_at", "anchor_id"}
	sizes, masks := make([]string, 0, len(columns)), make([]string, 0, len(columns))
	for _, column := range columns {
		sizes = append(sizes, "COALESCE(LENGTH(CAST("+column+" AS BLOB)),0)")
		masks = append(masks, "CASE WHEN identity_bytes <= "+strconv.Itoa(MaxTimelineSourceBytes)+" THEN "+column+" ELSE '' END AS "+column)
	}
	sql := "SELECT " + strings.Join(masks, ",") + ", invalid_turn_reference, window_total, identity_bytes FROM (SELECT selected.*, SUM(1024 + " + strings.Join(sizes, "+") + ") OVER () AS identity_bytes FROM (" + query + ") selected) bounded"
	if order != "" {
		sql += " ORDER BY " + order
	}
	var rows []struct {
		MessageWindowItem
		IdentityBytes int64
	}
	if err := r.db.Raw(sql, args...).Scan(&rows).Error; err != nil {
		return nil, err
	}
	items := make([]MessageWindowItem, 0, len(rows))
	for _, row := range rows {
		if row.IdentityBytes > MaxTimelineSourceBytes {
			return nil, ErrTimelineSourceLimit
		}
		items = append(items, row.MessageWindowItem)
	}
	return items, nil
}
