package workspace

import (
	"context"
	"strings"
	"testing"

	"assistente/internal/contextprovider"
)

func TestContextProviderBuildsFastDynamicBlock(t *testing.T) {
	provider := NewContextProvider()
	blocks, err := provider.Build(context.Background(), contextprovider.BuildRequest{
		WorkspaceName: "Workspace <Atual>",
		TabCount:      3,
		ProviderBudgets: map[string]int{
			"workspace": 2000,
		},
		Tabs: []contextprovider.Tab{
			{Title: "Chat", Type: "chat", ContentID: "conv-1"},
			{Title: "Tarefas", Type: "tasklist", ContentID: "tl-1", IsActive: true},
			{Title: "Terminal", Type: "terminal", ContentID: "term-1"},
		},
		Surface: &contextprovider.Surface{
			Type:  "editor",
			Title: "Arquivo",
			State: map[string]any{"filePath": "C:/tmp/readme.md", "tasklistId": "tl-1", "sessionId": "term-1"},
			Context: map[string]any{
				"selectedText":   "seleção",
				"historyPreview": "histórico",
				"tasksPreview":   "tarefas",
			},
		},
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if len(blocks) != 2 {
		t.Fatalf("len(blocks) = %d, want 2", len(blocks))
	}
	if blocks[0].Name != "workspace_instructions" || blocks[0].Volatility != contextprovider.VolatilityStable {
		t.Fatalf("unexpected instructions block: %+v", blocks[0])
	}
	block := blocks[1]
	if block.Provider != "workspace" || block.Volatility != contextprovider.VolatilityFastDynamic {
		t.Fatalf("unexpected block metadata: %+v", block)
	}
	for _, needle := range []string{
		"<workspace_context>",
		"Workspace Atual",
		"tab_count: 3",
		"assistente://conversation/conv-1",
		"assistente://tasklist/tl-1",
		"assistente://terminal/term-1",
		"active_file: C:/tmp/readme.md",
		"active_tasklist: tl-1",
		"active_terminal_session: term-1",
		"selected_text: seleção",
		"history_preview: histórico",
		"tasks_preview: tarefas",
	} {
		if !strings.Contains(block.Content, needle) {
			t.Fatalf("workspace block missing %q: %s", needle, block.Content)
		}
	}
}

func TestContextProviderReturnsNoBlockWithoutWorkspaceState(t *testing.T) {
	blocks, err := NewContextProvider().Build(context.Background(), contextprovider.BuildRequest{})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if len(blocks) != 1 || blocks[0].Name != "workspace_instructions" {
		t.Fatalf("expected only stable instructions block, got %+v", blocks)
	}
}

func TestContextProviderRespectsProfileBudget(t *testing.T) {
	blocks, err := NewContextProvider().Build(context.Background(), contextprovider.BuildRequest{
		WorkspaceName: "Workspace",
		ProviderBudgets: map[string]int{
			"workspace": 160,
		},
		Surface: &contextprovider.Surface{
			Type: "editor",
			Context: map[string]any{
				"selectedText": strings.Repeat("linha com conteúdo ", 20),
			},
		},
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if len(blocks) != 2 {
		t.Fatalf("len(blocks) = %d, want 2", len(blocks))
	}
	if got := len([]rune(blocks[1].Content)); got > 160 {
		t.Fatalf("workspace block length = %d, want <= 160: %q", got, blocks[1].Content)
	}
	if !strings.Contains(blocks[1].Content, "omitted due to context budget") {
		t.Fatalf("expected truncation notice, got %q", blocks[1].Content)
	}
}
