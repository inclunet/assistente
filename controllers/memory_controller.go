package controllers

import (
	"context"

	"assistente/internal/database"
	"assistente/internal/memory"
)

type MemoryControllerConfig struct {
	MemorySvc *memory.Service
}

type MemoryController struct {
	memorySvc *memory.Service
}

func NewMemoryController(cfg MemoryControllerConfig) *MemoryController {
	return &MemoryController{memorySvc: cfg.MemorySvc}
}

func (c *MemoryController) ListMemoryRecords(ctx context.Context, filter memory.Filter) (memory.ListResult, error) {
	return c.memorySvc.List(ctx, filter)
}

func (c *MemoryController) SearchMemoryRecords(ctx context.Context, query string, filter memory.Filter) (memory.ListResult, error) {
	filter.Query = query
	return c.memorySvc.List(ctx, filter)
}

func (c *MemoryController) GetMemoryRecord(ctx context.Context, id string) (*database.MemoryRecord, error) {
	return c.memorySvc.Get(ctx, id)
}

func (c *MemoryController) CreateMemoryRecord(ctx context.Context, input memory.RecordInput) (*database.MemoryRecord, error) {
	return c.memorySvc.Create(ctx, input)
}

func (c *MemoryController) UpdateMemoryRecord(ctx context.Context, id string, input memory.RecordInput) (*database.MemoryRecord, error) {
	return c.memorySvc.Update(ctx, id, input)
}

func (c *MemoryController) ArchiveMemoryRecord(ctx context.Context, id string) (*database.MemoryRecord, error) {
	return c.memorySvc.Archive(ctx, id)
}

func (c *MemoryController) UnarchiveMemoryRecord(ctx context.Context, id string, loadPolicy string) (*database.MemoryRecord, error) {
	return c.memorySvc.Unarchive(ctx, id, loadPolicy)
}

func (c *MemoryController) DeleteMemoryRecord(ctx context.Context, id string) error {
	return c.memorySvc.Delete(ctx, id)
}

func (c *MemoryController) GetMemoryPolicySummary(ctx context.Context) (memory.PolicySummary, error) {
	return c.memorySvc.PolicySummary(ctx)
}
