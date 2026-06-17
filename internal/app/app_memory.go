package app

import (
	"assistente/internal/database"
	"assistente/internal/memory"
)

// Re-exporta tipos para geração Wails.
type MemoryRecord = database.MemoryRecord
type MemoryRecordInput = memory.RecordInput
type MemoryFilter = memory.Filter
type MemoryListResult = memory.ListResult
type MemoryPolicySummary = memory.PolicySummary

func (a *App) ListMemoryRecords(filter MemoryFilter) (MemoryListResult, error) {
	ctx, err := a.requireAuthenticatedContext()
	if err != nil {
		return MemoryListResult{}, err
	}
	return a.memoryCtrl.ListMemoryRecords(ctx, filter)
}

func (a *App) SearchMemoryRecords(query string, filter MemoryFilter) (MemoryListResult, error) {
	ctx, err := a.requireAuthenticatedContext()
	if err != nil {
		return MemoryListResult{}, err
	}
	return a.memoryCtrl.SearchMemoryRecords(ctx, query, filter)
}

func (a *App) GetMemoryRecord(id string) (*MemoryRecord, error) {
	ctx, err := a.requireAuthenticatedContext()
	if err != nil {
		return nil, err
	}
	return a.memoryCtrl.GetMemoryRecord(ctx, id)
}

func (a *App) CreateMemoryRecord(input MemoryRecordInput) (*MemoryRecord, error) {
	ctx, err := a.requireAuthenticatedContext()
	if err != nil {
		return nil, err
	}
	return a.memoryCtrl.CreateMemoryRecord(ctx, input)
}

func (a *App) UpdateMemoryRecord(id string, input MemoryRecordInput) (*MemoryRecord, error) {
	ctx, err := a.requireAuthenticatedContext()
	if err != nil {
		return nil, err
	}
	return a.memoryCtrl.UpdateMemoryRecord(ctx, id, input)
}

func (a *App) ArchiveMemoryRecord(id string) (*MemoryRecord, error) {
	ctx, err := a.requireAuthenticatedContext()
	if err != nil {
		return nil, err
	}
	return a.memoryCtrl.ArchiveMemoryRecord(ctx, id)
}

func (a *App) UnarchiveMemoryRecord(id string, loadPolicy string) (*MemoryRecord, error) {
	ctx, err := a.requireAuthenticatedContext()
	if err != nil {
		return nil, err
	}
	return a.memoryCtrl.UnarchiveMemoryRecord(ctx, id, loadPolicy)
}

func (a *App) DeleteMemoryRecord(id string) error {
	ctx, err := a.requireAuthenticatedContext()
	if err != nil {
		return err
	}
	return a.memoryCtrl.DeleteMemoryRecord(ctx, id)
}

func (a *App) GetMemoryPolicySummary() (MemoryPolicySummary, error) {
	ctx, err := a.requireAuthenticatedContext()
	if err != nil {
		return MemoryPolicySummary{}, err
	}
	return a.memoryCtrl.GetMemoryPolicySummary(ctx)
}
