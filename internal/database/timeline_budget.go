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

var timelineIdentitySelect = func() string {
	columns := []string{"kind", "id", "message_id", "turn_id", "created_at", "anchor_id"}
	sizes, masks := make([]string, 0, len(columns)), make([]string, 0, len(columns))
	for _, column := range columns {
		sizes = append(sizes, "COALESCE(LENGTH(CAST("+column+" AS BLOB)),0)")
		masks = append(masks, "CASE WHEN identity_bytes <= "+strconv.Itoa(MaxTimelineSourceBytes)+" THEN COALESCE("+column+", '') ELSE '' END AS "+column)
	}
	return "SELECT " + strings.Join(masks, ",") + ", invalid_turn_reference, window_total, identity_bytes FROM (SELECT selected.*, SUM(1024 + " + strings.Join(sizes, "+") + ") OVER () AS identity_bytes FROM ("
}()

// A subconsulta contém a seleção paginada única. A soma e o mascaramento
// acontecem no SQLite, antes de qualquer identidade excessiva chegar ao Go.
func (r *MessageRepository) scanTimelineItems(query string, args []interface{}, order string) ([]MessageWindowItem, error) {
	sql := timelineIdentitySelect + query + ") selected) bounded"
	if order != "" {
		sql += " ORDER BY " + order
	}
	rows, err := r.db.Raw(sql, args...).Rows()
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	items := make([]MessageWindowItem, 0)
	for rows.Next() {
		var item MessageWindowItem
		var identityBytes int64
		if err := rows.Scan(&item.Kind, &item.ID, &item.MessageID, &item.TurnID, &item.AnchorCreatedAt, &item.AnchorID, &item.InvalidTurnReference, &item.WindowTotal, &identityBytes); err != nil {
			return nil, err
		}
		if identityBytes > MaxTimelineSourceBytes {
			return nil, ErrTimelineSourceLimit
		}
		if len(items) == 0 && item.WindowTotal > 0 {
			items = make([]MessageWindowItem, 0, min(item.WindowTotal, MaxMessageWindowRows))
		}
		items = append(items, item)
	}
	return items, rows.Err()
}
