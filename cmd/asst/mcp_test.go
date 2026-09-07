package main

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	mcpmgr "assistente/internal/mcp"
)

// ---------------------------------------------------------------------------
// Mock mcpBackend
// ---------------------------------------------------------------------------

type mockMCPBackend struct {
	servers    []mcpmgr.ServerInfo
	tools      []mcpmgr.MCPToolInfo
	listErr    error
	toolsErr   error
	saveErr    error
	connectErr error
	disconnErr error
	deleteErr  error

	// Capture calls
	savedSlug string
	savedCfg  mcpmgr.ServerConfig
	connSlug  string
	discSlug  string
	delSlug   string
}

func (m *mockMCPBackend) ListMCPServers() ([]mcpmgr.ServerInfo, error) {
	return m.servers, m.listErr
}

func (m *mockMCPBackend) SaveMCPServer(slug string, cfg mcpmgr.ServerConfig) error {
	m.savedSlug = slug
	m.savedCfg = cfg
	return m.saveErr
}

func (m *mockMCPBackend) ConnectMCPServer(slug string) error {
	m.connSlug = slug
	return m.connectErr
}

func (m *mockMCPBackend) DisconnectMCPServer(slug string) error {
	m.discSlug = slug
	return m.disconnErr
}

func (m *mockMCPBackend) GetMCPServerTools(slug string) ([]mcpmgr.MCPToolInfo, error) {
	return m.tools, m.toolsErr
}

func (m *mockMCPBackend) DeleteMCPServer(slug string) error {
	m.delSlug = slug
	return m.deleteErr
}

// ---------------------------------------------------------------------------
// runMCPList
// ---------------------------------------------------------------------------

func TestMCPList_Success(t *testing.T) {
	mock := &mockMCPBackend{
		servers: []mcpmgr.ServerInfo{
			{Slug: "filesystem", Name: "Filesystem", Transport: mcpmgr.TransportStdio, Status: "connected", ToolCount: 5, Enabled: true},
			{Slug: "github", Name: "GitHub", Transport: mcpmgr.TransportSSE, Status: "disconnected", ToolCount: 0},
		},
	}

	var out bytes.Buffer
	err := runMCPList(mock, &out)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := out.String()
	if !strings.Contains(output, "SLUG") {
		t.Error("expected header row")
	}
	if !strings.Contains(output, "filesystem") {
		t.Error("expected 'filesystem' in output")
	}
	if !strings.Contains(output, "sim") {
		t.Error("expected 'sim' for enabled server")
	}
}

func TestMCPList_Empty(t *testing.T) {
	mock := &mockMCPBackend{
		servers: []mcpmgr.ServerInfo{},
	}

	var out bytes.Buffer
	err := runMCPList(mock, &out)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out.String(), "Nenhum servidor MCP configurado") {
		t.Error("expected empty message")
	}
}

func TestMCPList_ControllerNotReady(t *testing.T) {
	mock := &mockMCPBackend{listErr: errMCPNotReady}

	var out bytes.Buffer
	err := runMCPList(mock, &out)
	if err == nil {
		t.Fatal("esperado erro quando MCP não está pronto")
	}
	if !strings.Contains(err.Error(), "mcp controller") {
		t.Fatalf("erro inesperado: %v", err)
	}
	if out.Len() != 0 {
		t.Fatalf("não deve imprimir lista vazia ao falhar: %q", out.String())
	}
}

// ---------------------------------------------------------------------------
// runMCPAdd
// ---------------------------------------------------------------------------

func TestMCPAdd_Stdio(t *testing.T) {
	mock := &mockMCPBackend{}

	var out bytes.Buffer
	err := runMCPAdd(mock, &out, "fs", "npx", "@modelcontextprotocol/server-filesystem /tmp", nil, "", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if mock.savedSlug != "fs" {
		t.Errorf("expected slug 'fs', got %q", mock.savedSlug)
	}
	if mock.savedCfg.Transport != mcpmgr.TransportStdio {
		t.Errorf("expected stdio transport, got %q", mock.savedCfg.Transport)
	}
	if mock.savedCfg.Command != "npx" {
		t.Errorf("expected command 'npx', got %q", mock.savedCfg.Command)
	}
	if len(mock.savedCfg.Args) != 2 {
		t.Errorf("expected 2 args, got %d", len(mock.savedCfg.Args))
	}
	if !strings.Contains(out.String(), "adicionado") {
		t.Error("expected success message")
	}
}

func TestMCPAdd_SSE(t *testing.T) {
	mock := &mockMCPBackend{}

	var out bytes.Buffer
	err := runMCPAdd(mock, &out, "remote", "", "", nil, "https://mcp.example.com/sse", "sse")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if mock.savedCfg.Transport != mcpmgr.TransportSSE {
		t.Errorf("expected sse transport, got %q", mock.savedCfg.Transport)
	}
	if mock.savedCfg.URL != "https://mcp.example.com/sse" {
		t.Errorf("expected URL, got %q", mock.savedCfg.URL)
	}
}

func TestMCPAdd_WithEnv(t *testing.T) {
	mock := &mockMCPBackend{}

	var out bytes.Buffer
	err := runMCPAdd(mock, &out, "test", "cmd", "", []string{"KEY=val", "FOO=bar"}, "", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(mock.savedCfg.Env) != 2 {
		t.Fatalf("expected 2 env vars, got %d", len(mock.savedCfg.Env))
	}
	if mock.savedCfg.Env["KEY"] != "val" {
		t.Errorf("expected KEY=val, got %q", mock.savedCfg.Env["KEY"])
	}
}

func TestMCPAdd_Error(t *testing.T) {
	mock := &mockMCPBackend{
		saveErr: fmt.Errorf("save failed"),
	}

	var out bytes.Buffer
	err := runMCPAdd(mock, &out, "fs", "npx", "", nil, "", "")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "erro ao salvar servidor") {
		t.Errorf("unexpected error: %v", err)
	}
}

// ---------------------------------------------------------------------------
// runMCPConnect
// ---------------------------------------------------------------------------

func TestMCPConnect_Success(t *testing.T) {
	mock := &mockMCPBackend{}

	var out bytes.Buffer
	err := runMCPConnect(mock, &out, "filesystem")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mock.connSlug != "filesystem" {
		t.Errorf("expected slug 'filesystem', got %q", mock.connSlug)
	}
	if !strings.Contains(out.String(), "OK") {
		t.Error("expected OK in output")
	}
}

func TestMCPConnect_Error(t *testing.T) {
	mock := &mockMCPBackend{
		connectErr: fmt.Errorf("timeout"),
	}

	var out bytes.Buffer
	err := runMCPConnect(mock, &out, "bad")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(out.String(), "FALHOU") {
		t.Error("expected FALHOU in output")
	}
}

// ---------------------------------------------------------------------------
// runMCPDisconnect
// ---------------------------------------------------------------------------

func TestMCPDisconnect_Success(t *testing.T) {
	mock := &mockMCPBackend{}

	var out bytes.Buffer
	err := runMCPDisconnect(mock, &out, "filesystem")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mock.discSlug != "filesystem" {
		t.Errorf("expected slug 'filesystem', got %q", mock.discSlug)
	}
	if !strings.Contains(out.String(), "desconectado") {
		t.Error("expected success message")
	}
}

func TestMCPDisconnect_Error(t *testing.T) {
	mock := &mockMCPBackend{
		disconnErr: fmt.Errorf("not connected"),
	}

	var out bytes.Buffer
	err := runMCPDisconnect(mock, &out, "bad")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "erro ao desconectar") {
		t.Errorf("unexpected error: %v", err)
	}
}

// ---------------------------------------------------------------------------
// runMCPTools
// ---------------------------------------------------------------------------

func TestMCPTools_Success(t *testing.T) {
	mock := &mockMCPBackend{
		tools: []mcpmgr.MCPToolInfo{
			{Name: "read_file", Description: "Reads a file from disk"},
			{Name: "write_file", Description: "Writes a file to disk"},
		},
	}

	var out bytes.Buffer
	err := runMCPTools(mock, &out, "filesystem")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := out.String()
	if !strings.Contains(output, "read_file") {
		t.Error("expected 'read_file' in output")
	}
	if !strings.Contains(output, "write_file") {
		t.Error("expected 'write_file' in output")
	}
}

func TestMCPTools_Empty(t *testing.T) {
	mock := &mockMCPBackend{
		tools: []mcpmgr.MCPToolInfo{},
	}

	var out bytes.Buffer
	err := runMCPTools(mock, &out, "empty-server")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out.String(), "Nenhuma tool") {
		t.Error("expected empty message")
	}
}

func TestMCPTools_ControllerNotReady(t *testing.T) {
	mock := &mockMCPBackend{toolsErr: errMCPNotReady}

	var out bytes.Buffer
	err := runMCPTools(mock, &out, "filesystem")
	if err == nil {
		t.Fatal("esperado erro quando MCP não está pronto")
	}
	if !strings.Contains(err.Error(), "mcp controller") {
		t.Fatalf("erro inesperado: %v", err)
	}
}

func TestMCPTools_LongDescription(t *testing.T) {
	long := strings.Repeat("B", 100)
	mock := &mockMCPBackend{
		tools: []mcpmgr.MCPToolInfo{
			{Name: "tool1", Description: long},
		},
	}

	var out bytes.Buffer
	err := runMCPTools(mock, &out, "s1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out.String(), "...") {
		t.Error("expected truncated description")
	}
}

// ---------------------------------------------------------------------------
// runMCPRemove
// ---------------------------------------------------------------------------

func TestMCPRemove_Success(t *testing.T) {
	mock := &mockMCPBackend{}

	var out bytes.Buffer
	err := runMCPRemove(mock, &out, "filesystem")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mock.delSlug != "filesystem" {
		t.Errorf("expected slug 'filesystem', got %q", mock.delSlug)
	}
	if !strings.Contains(out.String(), "removido") {
		t.Error("expected success message")
	}
}

func TestMCPRemove_Error(t *testing.T) {
	mock := &mockMCPBackend{
		deleteErr: fmt.Errorf("not found"),
	}

	var out bytes.Buffer
	err := runMCPRemove(mock, &out, "bad")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "erro ao remover servidor") {
		t.Errorf("unexpected error: %v", err)
	}
}
