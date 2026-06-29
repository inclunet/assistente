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

func TestContextProviderBuildsLowDynamicWorkspaceAndTurnDynamicSurfaceBlocks(t *testing.T) {
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
				"surfaceType":     "editor",
				"surfaceId":       "tab-1",
				"title":           "Arquivo",
				"mode":            "reveal",
				"snapshotVersion": "editor:tab-1:42",
				"selection": map[string]any{
					"kind":     "text",
					"text":     "seleção",
					"explicit": true,
					"range": map[string]any{
						"startLine":   float64(1),
						"startColumn": float64(2),
						"endLine":     float64(3),
						"endColumn":   float64(4),
					},
				},
				"focus": map[string]any{
					"kind":  "slide",
					"label": "Objetivo",
					"entity": map[string]any{
						"slideIndex": float64(2),
					},
				},
				"content": map[string]any{
					"kind":     "reveal_slide",
					"markdown": "## Objetivo",
				},
				"metadata": map[string]any{
					"filePath":          "C:/tmp/readme.md",
					"tasklistId":        "tl-1",
					"unsafeNested":      map[string]any{"secret": "do-not-render"},
					"currentSlideIndex": float64(2),
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if len(blocks) != 3 {
		t.Fatalf("len(blocks) = %d, want 3", len(blocks))
	}
	if blocks[0].Name != "workspace_instructions" || blocks[0].Volatility != contextprovider.VolatilityStable {
		t.Fatalf("unexpected instructions block: %+v", blocks[0])
	}
	if strings.Contains(blocks[0].Content, "link= values") || strings.Contains(blocks[0].Content, "app deep links") {
		t.Fatalf("workspace instructions should not include generic deeplink protocol: %q", blocks[0].Content)
	}
	if !strings.Contains(blocks[0].Content, "open_editor_file") {
		t.Fatalf("workspace instructions should keep open editor file guidance: %q", blocks[0].Content)
	}
	block := blocks[1]
	if block.Provider != "workspace" || block.Volatility != contextprovider.VolatilityLowDynamic {
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
	} {
		if !strings.Contains(block.Content, needle) {
			t.Fatalf("workspace block missing %q: %s", needle, block.Content)
		}
	}
	for _, needle := range []string{
		"<surface_context\n  surface_type=\"editor\"\n  surface_id=\"tab-1\"\n  snapshot_version=\"editor:tab-1:42\"\n  title=\"Arquivo\"\n  mode=\"reveal\"\n>",
		`<selection kind="text" explicit="true" range="1:2-3:4">seleção</selection>`,
		`<focus kind="slide" label="Objetivo" slide_index="2" />`,
		`<content kind="reveal_slide">## Objetivo</content>`,
		`<metadata key="file_path">C:/tmp/readme.md</metadata>`,
		`<metadata key="current_slide_index">2</metadata>`,
	} {
		if !strings.Contains(blocks[2].Content, needle) {
			t.Fatalf("surface block missing %q: %s", needle, blocks[2].Content)
		}
	}
	if strings.Contains(blocks[2].Content, "unsafeNested") || strings.Contains(blocks[2].Content, "do-not-render") || strings.Contains(blocks[2].Content, "tasklistId") {
		t.Fatalf("surface block rendered metadata outside allowlist: %s", blocks[2].Content)
	}
	if blocks[2].Provider != "workspace" || blocks[2].Name != "surface_context" || blocks[2].Volatility != contextprovider.VolatilityTurnDynamic {
		t.Fatalf("unexpected surface block metadata: %+v", blocks[2])
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

func TestContextProviderBuildsSurfaceBlockWithoutWorkspaceBlock(t *testing.T) {
	blocks, err := NewContextProvider().Build(context.Background(), contextprovider.BuildRequest{
		ProviderBudgets: map[string]int{
			"workspace": 1000,
		},
		Surface: &contextprovider.Surface{
			Type:    "editor",
			Title:   "Arquivo",
			Context: map[string]any{"selectedText": "seleção"},
		},
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if len(blocks) != 2 {
		t.Fatalf("len(blocks) = %d, want instructions and surface context", len(blocks))
	}
	if blocks[1].Name != "surface_context" || blocks[1].Volatility != contextprovider.VolatilityTurnDynamic {
		t.Fatalf("unexpected surface block: %+v", blocks[1])
	}
	if strings.Contains(blocks[1].Content, "<workspace_context>") {
		t.Fatalf("surface-only request should not emit workspace context: %q", blocks[1].Content)
	}
	if !strings.Contains(blocks[1].Content, `incomplete="true"`) || !strings.Contains(blocks[1].Content, `<selection kind="text" explicit="true">seleção</selection>`) {
		t.Fatalf("surface block missing selected text: %q", blocks[1].Content)
	}
}

func TestContextProviderPreservesSurfaceContextWhenOpeningTagIsLongUnderTightBudget(t *testing.T) {
	surfaceID := "surface-" + strings.Repeat("x", 120)
	preservedContent := "<surface_context\n  surface_type=\"editor\"\n  surface_id=\"" + surfaceID + "\"\n>"
	budget := runeLen(preservedContent) + runeLen(surfaceContextTruncationNotice) + runeLen(surfaceContextSuffix)

	blocks, err := NewContextProvider().Build(context.Background(), contextprovider.BuildRequest{
		ProviderBudgets: map[string]int{
			"workspace": budget,
		},
		Surface: &contextprovider.Surface{
			Type:  "editor",
			Title: strings.Repeat("Título longo ", 40),
			Context: map[string]any{
				"surfaceType":     "editor",
				"surfaceId":       surfaceID,
				"snapshotVersion": "editor:" + surfaceID + ":" + strings.Repeat("v", 80),
				"title":           strings.Repeat("Título longo ", 40),
			},
		},
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if len(blocks) != 2 {
		t.Fatalf("len(blocks) = %d, want instructions and surface context", len(blocks))
	}
	if blocks[1].Name != "surface_context" {
		t.Fatalf("unexpected surface block: %+v", blocks[1])
	}
	if !strings.Contains(blocks[1].Content, `surface_type="editor"`) || !strings.Contains(blocks[1].Content, `surface_id="`+surfaceID+`"`) {
		t.Fatalf("surface context should preserve identity under tight budget, got %q", blocks[1].Content)
	}
	if !strings.Contains(blocks[1].Content, "omitted due to context budget") {
		t.Fatalf("expected truncation notice, got %q", blocks[1].Content)
	}
	if got := runeLen(blocks[1].Content); got > budget {
		t.Fatalf("surface block length = %d, want <= %d: %q", got, budget, blocks[1].Content)
	}
}

func TestTrimSurfaceContextBlockOmitsUnclosedOpeningTagUnderTightBudget(t *testing.T) {
	preservedContent := "<surface_context\n  surface_type=\"editor\""
	content := preservedContent + "\n  surface_id=\"tab-1\"\n>\nCurrent active surface context. Treat this as turn-specific dynamic state.\n<selection kind=\"text\">seleção</selection>"
	budget := runeLen(preservedContent) + runeLen(surfaceContextTruncationNotice) + runeLen(surfaceContextSuffix)

	if got := trimSurfaceContextBlock(content, budget); got != "" {
		t.Fatalf("expected no malformed surface context block, got %q", got)
	}
}

func TestContextProviderBuildsBlockWhenTabsProvidedWithoutTabCount(t *testing.T) {
	editorPath := mustAbsPath(t, "sem-tab-count.md")
	blocks, err := NewContextProvider().Build(context.Background(), contextprovider.BuildRequest{
		ProviderBudgets: map[string]int{
			"workspace": 1000,
		},
		Tabs: []contextprovider.Tab{
			{Title: "Arquivo", Type: "editor", State: map[string]any{"filePath": editorPath}},
		},
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if len(blocks) != 2 {
		t.Fatalf("len(blocks) = %d, want workspace context block", len(blocks))
	}
	if !strings.Contains(blocks[1].Content, "open_editor_file[0]: "+editorPath) {
		t.Fatalf("expected editor file from tabs even without TabCount, got %q", blocks[1].Content)
	}
}

func TestContextProviderTabLinkFallsBackToStateReference(t *testing.T) {
	editorPath := mustAbsPath(t, "link-state.md")
	blocks, err := NewContextProvider().Build(context.Background(), contextprovider.BuildRequest{
		WorkspaceName: "Workspace",
		ProviderBudgets: map[string]int{
			"workspace": 1200,
		},
		Tabs: []contextprovider.Tab{
			{Title: "Editor", Type: "editor", State: map[string]any{"filePath": editorPath}},
			{Title: "Terminal", Type: "terminal", State: map[string]any{"sessionId": "term-state"}},
			{Title: "Tasks", Type: "tasklist", State: map[string]any{"tasklistId": "tl-state"}},
		},
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if len(blocks) != 2 {
		t.Fatalf("len(blocks) = %d, want workspace context block", len(blocks))
	}
	for _, needle := range []string{
		"link=assistente://editor/open?file=" + url.QueryEscape(editorPath),
		"link=assistente://terminal/term-state",
		"link=assistente://tasklist/tl-state",
	} {
		if !strings.Contains(blocks[1].Content, needle) {
			t.Fatalf("workspace block missing %q: %s", needle, blocks[1].Content)
		}
	}
}

func TestContextProviderRespectsProfileBudget(t *testing.T) {
	const budget = 420
	blocks, err := NewContextProvider().Build(context.Background(), contextprovider.BuildRequest{
		WorkspaceName: "Workspace",
		ProviderBudgets: map[string]int{
			"workspace": budget,
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
	if len(blocks) != 3 {
		t.Fatalf("len(blocks) = %d, want instructions, workspace context and surface context", len(blocks))
	}
	if got := runeLen(blocks[1].Content) + runeLen(blocks[2].Content); got > budget {
		t.Fatalf("workspace provider dynamic block length = %d, want <= %d: workspace=%q surface=%q", got, budget, blocks[1].Content, blocks[2].Content)
	}
	if blocks[2].Name != "surface_context" || !strings.Contains(blocks[2].Content, "omitted due to context budget") {
		t.Fatalf("expected truncated surface context, got %+v", blocks[2])
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

func TestContextProviderOpenEditorFilesSkipUnsafePathsInsteadOfMutating(t *testing.T) {
	safePath := mustAbsPath(t, "seguro.md")
	unsafePath := mustAbsPath(t, "arquivo<injetado>.md")
	blocks, err := NewContextProvider().Build(context.Background(), contextprovider.BuildRequest{
		WorkspaceName: "Workspace",
		TabCount:      2,
		ProviderBudgets: map[string]int{
			"workspace": 1000,
		},
		Tabs: []contextprovider.Tab{
			{Title: "Unsafe", Type: "editor", ContentID: unsafePath, State: map[string]any{"filePath": unsafePath}},
			{Title: "Safe", Type: "editor", ContentID: safePath, State: map[string]any{"filePath": safePath}},
		},
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if len(blocks) != 2 {
		t.Fatalf("len(blocks) = %d, want 2", len(blocks))
	}
	for _, line := range strings.Split(blocks[1].Content, "\n") {
		if strings.Contains(line, "open_editor_file") && strings.Contains(line, "arquivo") && line != "- open_editor_file[0]: "+safePath {
			t.Fatalf("unsafe open editor path should be omitted from open_editor_file entries, got line %q in %q", line, blocks[1].Content)
		}
	}
	if !strings.Contains(blocks[1].Content, "open_editor_file[0]: "+safePath) {
		t.Fatalf("safe open editor path should still be listed, got %q", blocks[1].Content)
	}
}

func TestContextProviderOmitsUnsafeTabStateReferenceInsteadOfMutating(t *testing.T) {
	unsafePath := mustAbsPath(t, "arquivo<injetado>.md")
	blocks, err := NewContextProvider().Build(context.Background(), contextprovider.BuildRequest{
		WorkspaceName: "Workspace",
		ProviderBudgets: map[string]int{
			"workspace": 1000,
		},
		Tabs: []contextprovider.Tab{
			{Title: "Unsafe", Type: "editor", ContentID: unsafePath, State: map[string]any{"filePath": unsafePath}},
		},
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if len(blocks) != 2 {
		t.Fatalf("len(blocks) = %d, want 2", len(blocks))
	}
	if strings.Contains(blocks[1].Content, " file=") {
		t.Fatalf("unsafe state-derived file reference should be omitted, got %q", blocks[1].Content)
	}
	if !strings.Contains(blocks[1].Content, "link=assistente://editor/open?file="+url.QueryEscape(unsafePath)) {
		t.Fatalf("safe URL-encoded editor deep link should remain, got %q", blocks[1].Content)
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

func TestContextProviderOmitsBlockWhenOnlyOpeningTagSurvives(t *testing.T) {
	blocks, err := NewContextProvider().Build(context.Background(), contextprovider.BuildRequest{
		WorkspaceName: "Workspace",
		ProviderBudgets: map[string]int{
			"workspace": runeLen(workspaceContextPrefix) + runeLen(workspaceContextTruncationNotice) + runeLen(workspaceContextSuffix) + 1,
		},
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if len(blocks) != 1 {
		t.Fatalf("len(blocks) = %d, want only stable instructions block", len(blocks))
	}
}
