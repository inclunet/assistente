package main

import (
	"bytes"
	"strings"
	"testing"

	"assistente/controllers"
)

// ---------------------------------------------------------------------------
// Mock toolsBackend
// ---------------------------------------------------------------------------

type mockToolsBackend struct {
	tools []controllers.ToolInfo
}

func (m *mockToolsBackend) GetAvailableTools() []controllers.ToolInfo {
	return m.tools
}

// ---------------------------------------------------------------------------
// runToolsList
// ---------------------------------------------------------------------------

func TestToolsList_Success(t *testing.T) {
	mock := &mockToolsBackend{
		tools: []controllers.ToolInfo{
			{Name: "http_request", SourceLabel: "Local", Description: "Faz requisições HTTP"},
			{Name: "mcp_github__create_issue", SourceLabel: "GitHub", Description: "Cria uma issue no GitHub"},
		},
	}

	var out bytes.Buffer
	err := runToolsList(mock, &out)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := out.String()
	if !strings.Contains(output, "NOME") {
		t.Error("expected header row")
	}
	if !strings.Contains(output, "http_request") {
		t.Error("expected http_request in output")
	}
	if !strings.Contains(output, "mcp_github__create_issue") {
		t.Error("expected MCP tool in output")
	}
	if !strings.Contains(output, "GitHub") {
		t.Error("expected source label 'GitHub' in output")
	}
}

func TestToolsList_Empty(t *testing.T) {
	mock := &mockToolsBackend{
		tools: []controllers.ToolInfo{},
	}

	var out bytes.Buffer
	err := runToolsList(mock, &out)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out.String(), "Nenhuma ferramenta disponível") {
		t.Error("expected empty message")
	}
}

func TestToolsList_LongDescription(t *testing.T) {
	long := strings.Repeat("A", 100)
	mock := &mockToolsBackend{
		tools: []controllers.ToolInfo{
			{Name: "my_tool", SourceLabel: "Local", Description: long},
		},
	}

	var out bytes.Buffer
	err := runToolsList(mock, &out)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := out.String()
	if !strings.Contains(output, "...") {
		t.Error("expected truncated description with '...'")
	}
}
