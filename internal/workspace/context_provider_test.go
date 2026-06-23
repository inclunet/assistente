package workspace

import (
	"context"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"assistente/internal/contextprovider"
)

func mustAbsPath(t *testing.T, name string) string {
	t.Helper()
	path, err := filepath.Abs(name)
	if err != nil {
		t.Fatalf("Abs: %v", err)
	}
	return filepath.Clean(path)
}

func TestContextProviderBuildsFastDynamicBlock(t *testing.T) {
	provider := NewContextProvider()
	editorPath := mustAbsPath(t, "main.go")
	blocks, err := provider.Build(context.Background(), contextprovider.BuildRequest{
		WorkspaceName: "Workspace <Atual>",
		TabCount:      4,
		ProviderBudgets: map[string]int{
			"workspace": 2000,
		},
		Tabs: []contextprovider.Tab{
			{Title: "Chat", Type: "chat", ContentID: "conv-1"},
			{Title: "main.go", Type: "editor", ContentID: editorPath, State: map[string]any{"filePath": editorPath}},
			{Title: "Tarefas", Type: "tasklist", ContentID: "tl-1", State: map[string]any{"tasklistId": "tl-1"}, IsActive: true},
			{Title: "Terminal", Type: "terminal", ContentID: "term-1", State: map[string]any{"sessionId": "term-1"}},
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
		"tab_count: 4",
		"assistente://conversation/conv-1",
		"conversation=conv-1",
		"open_editor_file[0]: " + editorPath,
		"assistente://editor/open?file=" + url.QueryEscape(editorPath),
		"file=" + editorPath,
		"assistente://tasklist/tl-1",
		"tasklist=tl-1",
		"assistente://terminal/term-1",
		"session=term-1",
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
	if strings.Contains(block.Content, "\n\n</workspace_context>") {
		t.Fatalf("workspace block has extra blank line before closing tag: %q", block.Content)
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

func TestContextProviderPreservesOpenEditorFilesWhenTruncated(t *testing.T) {
	filePath := mustAbsPath(t, "arquivo.md")
	blocks, err := NewContextProvider().Build(context.Background(), contextprovider.BuildRequest{
		WorkspaceName: "Workspace",
		TabCount:      2,
		ProviderBudgets: map[string]int{
			"workspace": 500,
		},
		Tabs: []contextprovider.Tab{
			{Title: "Arquivo", Type: "editor", ContentID: filePath, State: map[string]any{"filePath": filePath}},
			{Title: strings.Repeat("aba longa ", 30), Type: "chat", ContentID: "conv-1"},
		},
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if len(blocks) != 2 {
		t.Fatalf("len(blocks) = %d, want 2", len(blocks))
	}
	if !strings.Contains(blocks[1].Content, "open_editor_file[0]: "+filePath) {
		t.Fatalf("expected open editor path to survive truncation, got %q", blocks[1].Content)
	}
	if got := len([]rune(blocks[1].Content)); got > 500 {
		t.Fatalf("workspace block length = %d, want <= 500: %q", got, blocks[1].Content)
	}
	if !strings.Contains(blocks[1].Content, "omitted due to context budget") {
		t.Fatalf("expected truncation notice, got %q", blocks[1].Content)
	}
}

func TestContextProviderOpenEditorFilesSkipRelativePaths(t *testing.T) {
	absolutePath := mustAbsPath(t, "absoluto.md")
	blocks, err := NewContextProvider().Build(context.Background(), contextprovider.BuildRequest{
		WorkspaceName: "Workspace",
		TabCount:      2,
		ProviderBudgets: map[string]int{
			"workspace": 1000,
		},
		Tabs: []contextprovider.Tab{
			{Title: "Relativo", Type: "editor", ContentID: "docs/relativo.md", State: map[string]any{"filePath": "docs/relativo.md"}},
			{Title: "Absoluto", Type: "editor", ContentID: absolutePath, State: map[string]any{"filePath": absolutePath}},
		},
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if len(blocks) != 2 {
		t.Fatalf("len(blocks) = %d, want 2", len(blocks))
	}
	if strings.Contains(blocks[1].Content, "open_editor_file[0]: docs/relativo.md") {
		t.Fatalf("relative editor path should not be listed as tool-allowed open editor file: %q", blocks[1].Content)
	}
	if !strings.Contains(blocks[1].Content, "open_editor_file[0]: "+absolutePath) {
		t.Fatalf("absolute editor path should be listed, got %q", blocks[1].Content)
	}
}

func TestContextProviderDoesNotEmitPartialOpenEditorFileWhenBudgetIsTiny(t *testing.T) {
	filePath := mustAbsPath(t, strings.Repeat("arquivo-longo-", 20)+".md")
	blocks, err := NewContextProvider().Build(context.Background(), contextprovider.BuildRequest{
		WorkspaceName: "Workspace",
		TabCount:      1,
		ProviderBudgets: map[string]int{
			"workspace": 220,
		},
		Tabs: []contextprovider.Tab{
			{Title: "Arquivo", Type: "editor", ContentID: filePath, State: map[string]any{"filePath": filePath}},
		},
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if len(blocks) != 2 {
		t.Fatalf("len(blocks) = %d, want 2", len(blocks))
	}
	if strings.Contains(blocks[1].Content, "open_editor_fil") && !strings.Contains(blocks[1].Content, "open_editor_file[0]: "+filePath) {
		t.Fatalf("workspace block contains partial open_editor_file entry: %q", blocks[1].Content)
	}
	if got := len([]rune(blocks[1].Content)); got > 220 {
		t.Fatalf("workspace block length = %d, want <= 220: %q", got, blocks[1].Content)
	}
}

func TestContextProviderOmitsBlockWhenBudgetCannotFitOpeningTag(t *testing.T) {
	blocks, err := NewContextProvider().Build(context.Background(), contextprovider.BuildRequest{
		WorkspaceName: "Workspace",
		ProviderBudgets: map[string]int{
			"workspace": runeLen(workspaceContextTruncationNotice) + runeLen(workspaceContextSuffix) + 5,
		},
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if len(blocks) != 1 {
		t.Fatalf("len(blocks) = %d, want only stable instructions block", len(blocks))
	}
}

func TestContextProviderOmitsBlockWhenBudgetFitsOnlyOpeningTagAndNotice(t *testing.T) {
	blocks, err := NewContextProvider().Build(context.Background(), contextprovider.BuildRequest{
		WorkspaceName: "Workspace",
		ProviderBudgets: map[string]int{
			"workspace": runeLen(workspaceContextPrefix) + runeLen(workspaceContextTruncationNotice) + runeLen(workspaceContextSuffix),
		},
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if len(blocks) != 1 {
		t.Fatalf("len(blocks) = %d, want only stable instructions block", len(blocks))
	}
}
