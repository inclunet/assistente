package memory

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"assistente/internal/database"
	memorysvc "assistente/internal/memory"
)

type fakeService struct {
	created *memorysvc.RecordInput
	patched *memorysvc.RecordPatch
	listed  memorysvc.Filter
}

func (f *fakeService) List(_ context.Context, filter memorysvc.Filter) (memorysvc.ListResult, error) {
	f.listed = filter
	return memorysvc.ListResult{Records: []database.MemoryRecord{{Content: "found"}}, Total: 1}, nil
}
func (f *fakeService) Get(_ context.Context, id string) (*memorysvc.Record, error) {
	return &database.MemoryRecord{UUIDModel: database.UUIDModel{ID: id}, Content: "detail"}, nil
}
func (f *fakeService) Create(_ context.Context, input memorysvc.RecordInput) (*memorysvc.Record, error) {
	f.created = &input
	return &database.MemoryRecord{Content: input.Content, LoadPolicy: input.LoadPolicy}, nil
}
func (f *fakeService) Update(_ context.Context, id string, input memorysvc.RecordInput) (*memorysvc.Record, error) {
	f.created = &input
	return &database.MemoryRecord{UUIDModel: database.UUIDModel{ID: id}, Content: input.Content}, nil
}
func (f *fakeService) Patch(_ context.Context, id string, patch memorysvc.RecordPatch) (*memorysvc.Record, error) {
	f.patched = &patch
	content := ""
	if patch.Content != nil {
		content = *patch.Content
	}
	return &database.MemoryRecord{UUIDModel: database.UUIDModel{ID: id}, Content: content}, nil
}
func (f *fakeService) Archive(_ context.Context, id string) (*memorysvc.Record, error) {
	return &database.MemoryRecord{UUIDModel: database.UUIDModel{ID: id}, LoadPolicy: memorysvc.LoadPolicyArchived}, nil
}
func (f *fakeService) Unarchive(_ context.Context, id string, loadPolicy string) (*memorysvc.Record, error) {
	return &database.MemoryRecord{UUIDModel: database.UUIDModel{ID: id}, LoadPolicy: loadPolicy}, nil
}
func (f *fakeService) Delete(_ context.Context, id string) error { return nil }
func (f *fakeService) PolicySummary(_ context.Context) (memorysvc.PolicySummary, error) {
	return memorysvc.PolicySummary{Core: 1, Total: 1}, nil
}

func TestToolInfersWriteFromContent(t *testing.T) {
	fake := &fakeService{}
	tool := New(fake)

	result, err := tool.Execute(context.Background(), json.RawMessage(`{"content":"remember this","load_policy":"core"}`))
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected tool error: %s", result.Content)
	}
	if fake.created == nil || fake.created.Content != "remember this" || fake.created.LoadPolicy != "core" {
		t.Fatalf("write not forwarded: %+v", fake.created)
	}
	if !result.Structured || !strings.Contains(result.Content, "remember this") {
		t.Fatalf("unexpected result: %+v", result)
	}
}

func TestToolUsesPatchForExistingRecord(t *testing.T) {
	fake := &fakeService{}
	tool := New(fake)

	result, err := tool.Execute(context.Background(), json.RawMessage(`{"id":"mem-1","content":"updated"}`))
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected tool error: %s", result.Content)
	}
	if fake.patched == nil || fake.patched.Content == nil || *fake.patched.Content != "updated" {
		t.Fatalf("patch not forwarded: %+v", fake.patched)
	}
}

func TestToolSearchUsesFilters(t *testing.T) {
	fake := &fakeService{}
	tool := New(fake)

	result, err := tool.Execute(context.Background(), json.RawMessage(`{"query":"auth","load_policies":["core"],"include_archived":true}`))
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected tool error: %s", result.Content)
	}
	if fake.listed.Query != "auth" || !fake.listed.IncludeArchived || len(fake.listed.LoadPolicies) != 1 {
		t.Fatalf("filters not forwarded: %+v", fake.listed)
	}
}

func TestToolSearchUsesSingularFilters(t *testing.T) {
	fake := &fakeService{}
	tool := New(fake)

	result, err := tool.Execute(context.Background(), json.RawMessage(`{"query":"auth","load_policy":"pinned","kind":"decision","scope":"workspace"}`))
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected tool error: %s", result.Content)
	}
	if len(fake.listed.LoadPolicies) != 1 || fake.listed.LoadPolicies[0] != "pinned" {
		t.Fatalf("load_policy not forwarded: %+v", fake.listed)
	}
	if len(fake.listed.Kinds) != 1 || fake.listed.Kinds[0] != "decision" {
		t.Fatalf("kind not forwarded: %+v", fake.listed)
	}
	if len(fake.listed.Scopes) != 1 || fake.listed.Scopes[0] != "workspace" {
		t.Fatalf("scope not forwarded: %+v", fake.listed)
	}
}
