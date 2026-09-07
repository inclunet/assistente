package tools

import (
	"context"
	"encoding/json"
	"testing"
)

func TestCatalogEntryFromToolUsesDeclaredMetadata(t *testing.T) {
	tool := &catalogMetadataTestTool{
		name: "demo_tool",
		metadata: CatalogMetadata{
			Category: "history",
			Class:    "read_context",
			Package:  "history",
			Risk:     "read",
			Tags:     []string{"demo"},
		},
	}

	entry := CatalogEntryFromTool(tool)
	if entry.Category != "history" || entry.Package != "history" || entry.Class != "read_context" || entry.Risk != "read" {
		t.Fatalf("metadados declarados não foram propagados: %#v", entry)
	}
	if len(entry.Tags) != 1 || entry.Tags[0] != "demo" {
		t.Fatalf("tags declaradas não foram propagadas: %#v", entry.Tags)
	}
	if entry.Origin != ToolOriginBuiltin {
		t.Fatalf("origem esperada builtin, obtido %q", entry.Origin)
	}
	if entry.SchemaHash != SchemaHash(tool.Parameters()) {
		t.Fatalf("schema_hash divergente: %q", entry.SchemaHash)
	}
}

func TestCatalogEntryFromToolFallsBackToDefaultMetadata(t *testing.T) {
	// Tool sem CatalogMetadataProvider recebe o fallback padrão de builtin,
	// preservando o comportamento histórico para tools genéricas de aplicação.
	entry := CatalogEntryFromTool(&plainTestTool{name: "sem_metadata"})
	want := DefaultBuiltinCatalogMetadata()
	if entry.Category != want.Category || entry.Class != want.Class || entry.Package != want.Package || entry.Risk != want.Risk {
		t.Fatalf("fallback padrão não aplicado: %#v", entry)
	}
}

type catalogMetadataTestTool struct {
	name     string
	metadata CatalogMetadata
}

func (t *catalogMetadataTestTool) Name() string        { return t.name }
func (t *catalogMetadataTestTool) Description() string { return "test" }
func (t *catalogMetadataTestTool) Parameters() json.RawMessage {
	return json.RawMessage(`{"type":"object"}`)
}
func (t *catalogMetadataTestTool) Execute(context.Context, json.RawMessage) (ToolResult, error) {
	return ToolResult{Content: "ok"}, nil
}
func (t *catalogMetadataTestTool) CatalogMetadata() CatalogMetadata { return t.metadata }

type plainTestTool struct {
	name string
}

func (t *plainTestTool) Name() string        { return t.name }
func (t *plainTestTool) Description() string { return "test" }
func (t *plainTestTool) Parameters() json.RawMessage {
	return json.RawMessage(`{"type":"object"}`)
}
func (t *plainTestTool) Execute(context.Context, json.RawMessage) (ToolResult, error) {
	return ToolResult{Content: "ok"}, nil
}
