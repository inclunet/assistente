package portability

import (
	"context"
	"fmt"
	"strings"
	"time"

	"assistente/internal/database"
	memorysvc "assistente/internal/memory"

	"gorm.io/gorm"
)

func buildMemoryRecordExports(ctx context.Context, ids []string, all bool) ([]MemoryRecordExport, error) {
	var rows []database.MemoryRecord
	query := database.ScopeByUser(ctx, database.DB().WithContext(ctx), "user_id").
		Where("expires_at IS NULL OR expires_at > ?", time.Now()).
		Order("updated_at DESC")
	if all {
		if err := query.Find(&rows).Error; err != nil {
			return nil, fmt.Errorf("erro ao buscar memórias para exportação: %w", err)
		}
		return exportMemoryRecords(rows), nil
	}
	if len(ids) == 0 {
		return nil, nil
	}
	uniqueIDs := make([]string, 0, len(ids))
	seen := map[string]struct{}{}
	for _, rawID := range ids {
		id := strings.TrimSpace(rawID)
		if id == "" {
			return nil, fmt.Errorf("memoryRecordId inválido: %q", rawID)
		}
		if _, exists := seen[id]; exists {
			continue
		}
		seen[id] = struct{}{}
		uniqueIDs = append(uniqueIDs, id)
	}
	if err := query.Where("id IN ?", uniqueIDs).Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("erro ao buscar memórias para exportação: %w", err)
	}
	byID := make(map[string]database.MemoryRecord, len(rows))
	for _, row := range rows {
		byID[row.ID] = row
	}
	exports := make([]MemoryRecordExport, 0, len(uniqueIDs))
	for _, id := range uniqueIDs {
		row, ok := byID[id]
		if !ok {
			return nil, fmt.Errorf("erro ao buscar memória %s: %w", id, gorm.ErrRecordNotFound)
		}
		exports = append(exports, exportMemoryRecord(row))
	}
	return exports, nil
}

func exportMemoryRecords(rows []database.MemoryRecord) []MemoryRecordExport {
	exports := make([]MemoryRecordExport, 0, len(rows))
	for _, row := range rows {
		exports = append(exports, exportMemoryRecord(row))
	}
	return exports
}

func exportMemoryRecord(row database.MemoryRecord) MemoryRecordExport {
	return MemoryRecordExport{
		ID:                 row.ID,
		Content:            row.Content,
		Summary:            row.Summary,
		LoadPolicy:         row.LoadPolicy,
		ArchivedFromPolicy: row.ArchivedFromPolicy,
		Kind:               row.Kind,
		Scope:              row.Scope,
		ScopeRef:           row.ScopeRef,
		Tags:               row.Tags,
		Importance:         row.Importance,
		Confidence:         row.Confidence,
		SourceType:         row.SourceType,
		SourceID:           row.SourceID,
		LastUsedAt:         row.LastUsedAt,
		ExpiresAt:          row.ExpiresAt,
		CreatedAt:          row.CreatedAt,
	}
}

func importMemoryRecord(ctx context.Context, svc *memorysvc.Service, exported MemoryRecordExport) (bool, error) {
	id := strings.TrimSpace(exported.ID)
	if id == "" {
		return false, fmt.Errorf("memória sem id não pode ser importada")
	}
	record := database.MemoryRecord{
		UUIDModel: database.UUIDModel{
			ID:        id,
			CreatedAt: exported.CreatedAt,
		},
		Content:            exported.Content,
		Summary:            exported.Summary,
		LoadPolicy:         exported.LoadPolicy,
		ArchivedFromPolicy: exported.ArchivedFromPolicy,
		Kind:               exported.Kind,
		Scope:              exported.Scope,
		ScopeRef:           exported.ScopeRef,
		Tags:               exported.Tags,
		Importance:         exported.Importance,
		Confidence:         exported.Confidence,
		SourceType:         exported.SourceType,
		SourceID:           exported.SourceID,
		LastUsedAt:         exported.LastUsedAt,
		ExpiresAt:          exported.ExpiresAt,
	}
	if _, importErr := svc.Import(ctx, record); importErr != nil {
		return false, importErr
	}
	return true, nil
}
