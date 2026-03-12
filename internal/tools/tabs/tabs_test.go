package tabs

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

type fakeTabManager struct {
	tabs             []TabInfo
	activeTab        *TabInfo
	closedIDs        []uint
	updatedTitles    map[uint]string
	getActiveTabErr  error
	getAllTabsErr    error
	updateTitleErr   error
	closeTabErr      error
}

func newFakeTabManager(tabs ...TabInfo) *fakeTabManager {
	var active *TabInfo
	for i := range tabs {
		if tabs[i].IsActive {
			active = &tabs[i]
			break
		}
	}
	return &fakeTabManager{
		tabs:          tabs,
		activeTab:     active,
		updatedTitles: make(map[uint]string),
	}
}

func (f *fakeTabManager) GetAllTabs() ([]TabInfo, error) {
	if f.getAllTabsErr != nil {
		return nil, f.getAllTabsErr
	}
	return f.tabs, nil
}

func (f *fakeTabManager) GetActiveTab() (*TabInfo, error) {
	if f.getActiveTabErr != nil {
		return nil, f.getActiveTabErr
	}
	if f.activeTab == nil {
		return nil, fmt.Errorf("nenhuma aba ativa")
	}
	return f.activeTab, nil
}

func (f *fakeTabManager) UpdateTabTitle(id uint, title string) error {
	if f.updateTitleErr != nil {
		return f.updateTitleErr
	}
	f.updatedTitles[id] = title
	return nil
}

func (f *fakeTabManager) CloseTab(id uint) error {
	if f.closeTabErr != nil {
		return f.closeTabErr
	}
	f.closedIDs = append(f.closedIDs, id)
	return nil
}

// ==================== RenameConversation Tests ====================

func TestRenameConversation_Name(t *testing.T) {
	tool := NewRenameConversation(nil)
	if tool.Name() != "rename_conversation" {
		t.Fatalf("expected name 'rename_conversation', got '%s'", tool.Name())
	}
}

func TestRenameConversation_EmptyTitle(t *testing.T) {
	mgr := newFakeTabManager(TabInfo{ID: 1, Title: "Aba 1", IsActive: true})
	tool := NewRenameConversation(mgr)

	result, err := tool.Execute(context.Background(), json.RawMessage(`{"title":""}`))
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error for empty title")
	}
}

func TestRenameConversation_WhitespaceTitle(t *testing.T) {
	mgr := newFakeTabManager(TabInfo{ID: 1, Title: "Aba 1", IsActive: true})
	tool := NewRenameConversation(mgr)

	result, err := tool.Execute(context.Background(), json.RawMessage(`{"title":"   "}`))
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error for whitespace-only title")
	}
}

func TestRenameConversation_NoActiveTab(t *testing.T) {
	mgr := newFakeTabManager()
	tool := NewRenameConversation(mgr)

	result, err := tool.Execute(context.Background(), json.RawMessage(`{"title":"Novo Título"}`))
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error when no active tab")
	}
}

func TestRenameConversation_Success(t *testing.T) {
	mgr := newFakeTabManager(TabInfo{ID: 42, Title: "Antiga", IsActive: true})
	tool := NewRenameConversation(mgr)

	result, err := tool.Execute(context.Background(), json.RawMessage(`{"title":"Deploy v2.0"}`))
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if result.IsError {
		t.Fatalf("expected success, got error: %s", result.Content)
	}
	if !strings.Contains(result.Content, "Deploy v2.0") {
		t.Fatalf("expected title in response, got: %s", result.Content)
	}
	if mgr.updatedTitles[42] != "Deploy v2.0" {
		t.Fatalf("expected UpdateTabTitle called with correct title, got: %v", mgr.updatedTitles)
	}
}

func TestRenameConversation_UpdateError(t *testing.T) {
	mgr := newFakeTabManager(TabInfo{ID: 1, Title: "Aba", IsActive: true})
	mgr.updateTitleErr = fmt.Errorf("db error")
	tool := NewRenameConversation(mgr)

	result, err := tool.Execute(context.Background(), json.RawMessage(`{"title":"Teste"}`))
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error when update fails")
	}
}

func TestRenameConversation_InvalidJSON(t *testing.T) {
	mgr := newFakeTabManager()
	tool := NewRenameConversation(mgr)

	result, err := tool.Execute(context.Background(), json.RawMessage(`{invalid}`))
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error for invalid JSON")
	}
}

// ==================== CloseConversation Tests ====================

func TestCloseConversation_Name(t *testing.T) {
	tool := NewCloseConversation(nil)
	if tool.Name() != "close_conversation" {
		t.Fatalf("expected name 'close_conversation', got '%s'", tool.Name())
	}
}

func TestCloseConversation_CloseCurrent(t *testing.T) {
	mgr := newFakeTabManager(
		TabInfo{ID: 10, Title: "Aba A", IsActive: true, Position: 0},
		TabInfo{ID: 20, Title: "Aba B", IsActive: false, Position: 1},
	)
	tool := NewCloseConversation(mgr)

	result, err := tool.Execute(context.Background(), json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if result.IsError {
		t.Fatalf("expected success, got error: %s", result.Content)
	}
	if len(mgr.closedIDs) != 1 || mgr.closedIDs[0] != 10 {
		t.Fatalf("expected conversation 10 closed, got: %v", mgr.closedIDs)
	}
}

func TestCloseConversation_CloseByName_ExactMatch(t *testing.T) {
	mgr := newFakeTabManager(
		TabInfo{ID: 1, Title: "Deploy", IsActive: true, Position: 0},
		TabInfo{ID: 2, Title: "Revisão de código", IsActive: false, Position: 1},
	)
	tool := NewCloseConversation(mgr)

	result, err := tool.Execute(context.Background(), json.RawMessage(`{"name":"Revisão de código"}`))
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if result.IsError {
		t.Fatalf("expected success, got error: %s", result.Content)
	}
	if len(mgr.closedIDs) != 1 || mgr.closedIDs[0] != 2 {
		t.Fatalf("expected conversation 2 closed, got: %v", mgr.closedIDs)
	}
}

func TestCloseConversation_CloseByName_CaseInsensitive(t *testing.T) {
	mgr := newFakeTabManager(
		TabInfo{ID: 1, Title: "Deploy Prod", IsActive: true, Position: 0},
	)
	tool := NewCloseConversation(mgr)

	result, err := tool.Execute(context.Background(), json.RawMessage(`{"name":"deploy prod"}`))
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if result.IsError {
		t.Fatalf("expected success, got error: %s", result.Content)
	}
	if len(mgr.closedIDs) != 1 || mgr.closedIDs[0] != 1 {
		t.Fatalf("expected conversation 1 closed, got: %v", mgr.closedIDs)
	}
}

func TestCloseConversation_CloseByName_PartialMatch(t *testing.T) {
	mgr := newFakeTabManager(
		TabInfo{ID: 1, Title: "Análise de performance", IsActive: true, Position: 0},
		TabInfo{ID: 2, Title: "Bug fix #123", IsActive: false, Position: 1},
	)
	tool := NewCloseConversation(mgr)

	result, err := tool.Execute(context.Background(), json.RawMessage(`{"name":"performance"}`))
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if result.IsError {
		t.Fatalf("expected success, got error: %s", result.Content)
	}
	if len(mgr.closedIDs) != 1 || mgr.closedIDs[0] != 1 {
		t.Fatalf("expected conversation 1 closed, got: %v", mgr.closedIDs)
	}
}

func TestCloseConversation_CloseByName_NotFound(t *testing.T) {
	mgr := newFakeTabManager(
		TabInfo{ID: 1, Title: "Aba 1", IsActive: true, Position: 0},
	)
	tool := NewCloseConversation(mgr)

	result, err := tool.Execute(context.Background(), json.RawMessage(`{"name":"inexistente"}`))
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error when conversation not found by name")
	}
	if !strings.Contains(result.Content, "Nenhuma conversa encontrada") {
		t.Fatalf("expected 'not found' message, got: %s", result.Content)
	}
}

func TestCloseConversation_CloseByName_MultipleMatches(t *testing.T) {
	mgr := newFakeTabManager(
		TabInfo{ID: 1, Title: "Deploy Staging", IsActive: true, Position: 0},
		TabInfo{ID: 2, Title: "Deploy Prod", IsActive: false, Position: 1},
	)
	tool := NewCloseConversation(mgr)

	result, err := tool.Execute(context.Background(), json.RawMessage(`{"name":"Deploy"}`))
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error for ambiguous name match")
	}
	if !strings.Contains(result.Content, "Múltiplas conversas") {
		t.Fatalf("expected ambiguity message, got: %s", result.Content)
	}
}

func TestCloseConversation_CloseByIndex(t *testing.T) {
	mgr := newFakeTabManager(
		TabInfo{ID: 10, Title: "Primeira", IsActive: true, Position: 0},
		TabInfo{ID: 20, Title: "Segunda", IsActive: false, Position: 1},
		TabInfo{ID: 30, Title: "Terceira", IsActive: false, Position: 2},
	)
	tool := NewCloseConversation(mgr)

	result, err := tool.Execute(context.Background(), json.RawMessage(`{"index":2}`))
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if result.IsError {
		t.Fatalf("expected success, got error: %s", result.Content)
	}
	if len(mgr.closedIDs) != 1 || mgr.closedIDs[0] != 20 {
		t.Fatalf("expected conversation 20 (index 2) closed, got: %v", mgr.closedIDs)
	}
}

func TestCloseConversation_CloseByIndex_OutOfRange(t *testing.T) {
	mgr := newFakeTabManager(
		TabInfo{ID: 1, Title: "Única", IsActive: true, Position: 0},
	)
	tool := NewCloseConversation(mgr)

	result, err := tool.Execute(context.Background(), json.RawMessage(`{"index":5}`))
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error for out-of-range index")
	}
}

func TestCloseConversation_CloseByIndex_Zero(t *testing.T) {
	mgr := newFakeTabManager(
		TabInfo{ID: 1, Title: "Aba", IsActive: true, Position: 0},
	)
	tool := NewCloseConversation(mgr)

	result, err := tool.Execute(context.Background(), json.RawMessage(`{"index":0}`))
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error for index 0")
	}
}

func TestCloseConversation_BothNameAndIndex(t *testing.T) {
	mgr := newFakeTabManager()
	tool := NewCloseConversation(mgr)

	result, err := tool.Execute(context.Background(), json.RawMessage(`{"name":"test","index":1}`))
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error when both name and index provided")
	}
}

func TestCloseConversation_InvalidJSON(t *testing.T) {
	mgr := newFakeTabManager()
	tool := NewCloseConversation(mgr)

	result, err := tool.Execute(context.Background(), json.RawMessage(`{invalid}`))
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestCloseConversation_CloseError(t *testing.T) {
	mgr := newFakeTabManager(TabInfo{ID: 1, Title: "Aba", IsActive: true, Position: 0})
	mgr.closeTabErr = fmt.Errorf("cannot close")
	tool := NewCloseConversation(mgr)

	result, err := tool.Execute(context.Background(), json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error when close fails")
	}
}

func TestCloseConversation_EmptyName(t *testing.T) {
	mgr := newFakeTabManager()
	tool := NewCloseConversation(mgr)

	result, err := tool.Execute(context.Background(), json.RawMessage(`{"name":""}`))
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error for empty name")
	}
}

func TestCloseConversation_CloseCurrent_GetActiveTabError(t *testing.T) {
	mgr := newFakeTabManager()
	mgr.getActiveTabErr = fmt.Errorf("db connection lost")
	tool := NewCloseConversation(mgr)

	result, err := tool.Execute(context.Background(), json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error when GetActiveTab fails")
	}
	if !strings.Contains(result.Content, "db connection lost") {
		t.Fatalf("expected original error in message, got: %s", result.Content)
	}
}

func TestCloseConversation_CloseByName_GetAllTabsError(t *testing.T) {
	mgr := newFakeTabManager()
	mgr.getAllTabsErr = fmt.Errorf("db timeout")
	tool := NewCloseConversation(mgr)

	result, err := tool.Execute(context.Background(), json.RawMessage(`{"name":"qualquer"}`))
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error when GetAllTabs fails")
	}
	if !strings.Contains(result.Content, "db timeout") {
		t.Fatalf("expected original error in message, got: %s", result.Content)
	}
}

func TestCloseConversation_CloseByName_CloseError(t *testing.T) {
	mgr := newFakeTabManager(
		TabInfo{ID: 5, Title: "Minha Aba", IsActive: true, Position: 0},
	)
	mgr.closeTabErr = fmt.Errorf("permission denied")
	tool := NewCloseConversation(mgr)

	result, err := tool.Execute(context.Background(), json.RawMessage(`{"name":"Minha Aba"}`))
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error when close fails after name match")
	}
	if !strings.Contains(result.Content, "permission denied") {
		t.Fatalf("expected original error in message, got: %s", result.Content)
	}
}

func TestCloseConversation_CloseByIndex_GetAllTabsError(t *testing.T) {
	mgr := newFakeTabManager()
	mgr.getAllTabsErr = fmt.Errorf("disk full")
	tool := NewCloseConversation(mgr)

	result, err := tool.Execute(context.Background(), json.RawMessage(`{"index":1}`))
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error when GetAllTabs fails")
	}
	if !strings.Contains(result.Content, "disk full") {
		t.Fatalf("expected original error in message, got: %s", result.Content)
	}
}

func TestCloseConversation_CloseByIndex_CloseError(t *testing.T) {
	mgr := newFakeTabManager(
		TabInfo{ID: 7, Title: "Aba Protegida", IsActive: true, Position: 0},
	)
	mgr.closeTabErr = fmt.Errorf("tab is pinned")
	tool := NewCloseConversation(mgr)

	result, err := tool.Execute(context.Background(), json.RawMessage(`{"index":1}`))
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error when close fails after index lookup")
	}
	if !strings.Contains(result.Content, "tab is pinned") {
		t.Fatalf("expected original error in message, got: %s", result.Content)
	}
}

func TestCloseConversation_CloseByIndex_Negative(t *testing.T) {
	mgr := newFakeTabManager(
		TabInfo{ID: 1, Title: "Aba", IsActive: true, Position: 0},
	)
	tool := NewCloseConversation(mgr)

	result, err := tool.Execute(context.Background(), json.RawMessage(`{"index":-1}`))
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error for negative index")
	}
}
