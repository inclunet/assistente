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

var timelineIdentityBudgetSelect = func() string {
	columns := []string{"kind", "id", "message_id", "turn_id", "created_at", "anchor_id"}
	sizes, projections := make([]string, 0, len(columns)), make([]string, 0, len(columns))
	for _, column := range columns {
		sizes = append(sizes, "COALESCE(LENGTH(CAST("+column+" AS BLOB)),0)")
		if column != "turn_id" && column != "anchor_id" {
			projections = append(projections, "COALESCE("+column+", '') AS "+column)
		}
	}
	limit := strconv.Itoa(MaxTimelineSourceBytes)
	return `), identity_budget AS (
 SELECT SUM(1024 + ` + strings.Join(sizes, "+") + `) AS identity_bytes FROM selected
) SELECT ` + strings.Join(projections, ",") + `,
 COALESCE(invalid_turn_reference,0) AS invalid_turn_reference,
 COALESCE(window_total,0) AS window_total, identity_bytes
 FROM identity_budget LEFT JOIN selected ON identity_bytes <= ` + limit + `
 WHERE identity_bytes > ` + limit + ` OR selected.id IS NOT NULL`
}()

// A página é selecionada uma vez. Acima da cota, o JOIN devolve somente o
// orçamento, sem deixar identidades excessivas atravessarem o resultado SQL.
func (r *MessageRepository) scanTimelineItems(query string, args []interface{}, order string) ([]MessageWindowItem, error) {
	sql := timelineIdentitySQL(query, order)
	rows, err := r.db.Raw(sql, args...).Rows()

	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	items := make([]MessageWindowItem, 0)
	for rows.Next() {
		var item MessageWindowItem
		var identityBytes int64
		if err := rows.Scan(&item.Kind, &item.ID, &item.MessageID, &item.AnchorCreatedAt, &item.InvalidTurnReference, &item.WindowTotal, &identityBytes); err != nil {
			return nil, err
		}
		if identityBytes > MaxTimelineSourceBytes {
			return nil, ErrTimelineSourceLimit
		}
		// A classificação canônica ancora mensagem na própria PK e resposta na
		// PK da pergunta; ambas já são ID. Não transporta novamente essas strings.
		item.AnchorID = item.ID
		if item.Kind == MessageWindowItemKindTurn {
			item.TurnID = item.ID
		}
		if len(items) == 0 && item.WindowTotal > 0 {
			items = make([]MessageWindowItem, 0, min(item.WindowTotal, MaxMessageWindowRows))
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func timelineIdentitySQL(query, order string) string {
	sql := "WITH selected AS MATERIALIZED (" + query + timelineIdentityBudgetSelect
	if order != "" {
		sql += " ORDER BY " + order
	}
	return sql
}
