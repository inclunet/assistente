package prompt_test

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"assistente/internal/chat"
	"assistente/internal/llm"
	"assistente/internal/profiles"
	"assistente/internal/prompt"
	"assistente/internal/skills"
	"assistente/internal/tools"
	"assistente/internal/workspace"
)

type mockSkillReader struct {
	autoSkills      []skills.Skill
	availableSkills []skills.Skill
	allSkillsFull   []skills.Skill
	skillFiles      map[string][]string
	autoErr         error
	availErr        error
	allErr          error
}

func (m *mockSkillReader) GetAutoSkills() ([]skills.Skill, error) {
	return m.autoSkills, m.autoErr
}
func (m *mockSkillReader) GetAvailableSkills() ([]skills.Skill, error) {
	return m.availableSkills, m.availErr
}
func (m *mockSkillReader) GetAllSkillsFull() ([]skills.Skill, error) {
	return m.allSkillsFull, m.allErr
}
func (m *mockSkillReader) GetSkillFiles(slug string) ([]string, error) {
	if m.skillFiles != nil {
		return m.skillFiles[slug], nil
	}
	return nil, nil
}

type mockWorkspaceReader struct{ ws *workspace.Workspace }

func (m *mockWorkspaceReader) Active() *workspace.Workspace { return m.ws }

func makeSkill(slug, name, desc, content string, autoLoad, modelInvocable bool) skills.Skill {
	s := skills.Skill{Slug: slug, Content: content, Path: "/skills/" + slug + "/SKILL.md"}
	s.Name = name
	s.Description = desc
	s.DisableModelInvocation = !modelInvocable
	if autoLoad {
		s.AutoLoad = true
		s.Behavior = &skills.BehaviorConfig{}
	}
	return s
}

func TestBuild_NoSkillsNoSlash_NoSystemMessage(t *testing.T) {
	b := &prompt.Builder{}
	msgs := []llm.Message{{Role: "user", Content: "olá"}}
	result := b.Build(msgs, []string{}, false, nil, "", "")
	if len(result) != 1 || result[0].Role != "user" {
		t.Errorf("Expected unchanged messages, got %v", result)
	}
}

func TestBuild_WithSlashSkill_AddsSystemMessage(t *testing.T) {
	b := &prompt.Builder{}
	msgs := []llm.Message{{Role: "user", Content: "olá"}}
	result := b.Build(msgs, []string{}, false, nil, "slash content", "")
	if len(result) < 2 {
		t.Fatalf("Expected system+user, got %d", len(result))
	}
	if result[0].Role != "system" {
		t.Errorf("Expected system, got %q", result[0].Role)
	}
	sys, ok := result[0].Content.(string)
	if !ok || !strings.Contains(sys, "slash content") {
		t.Error("System message should contain slash content")
	}
}

func TestBuild_WithSummary_InjectsSummaryTag(t *testing.T) {
	b := &prompt.Builder{}
	msgs := []llm.Message{{Role: "user", Content: "oi"}}
	result := b.Build(msgs, nil, false, nil, "slash", "O usuário perguntou sobre finanças.")
	sys := result[0].Content.(string)
	if !strings.Contains(sys, "<conversation_summary>") {
		t.Error("Expected <conversation_summary> tag")
	}
	if !strings.Contains(sys, "O usuário perguntou") {
		t.Error("Expected summary text in system message")
	}
}

func TestBuild_ExistingSystemMessage_Combined(t *testing.T) {
	b := &prompt.Builder{}
	msgs := []llm.Message{{Role: "system", Content: "Existente."}, {Role: "user", Content: "oi"}}
	result := b.Build(msgs, nil, false, nil, "Novo.", "")
	if len(result) != 2 {
		t.Errorf("Expected 2 messages, got %d", len(result))
	}
	sys := result[0].Content.(string)
	if !strings.Contains(sys, "Existente.") {
		t.Error("Original content should be preserved")
	}
	if !strings.Contains(sys, "Novo.") {
		t.Error("New content should be injected")
	}
}

func TestBuild_OpenEditorFiles_InjectsSection(t *testing.T) {
	b := &prompt.Builder{
		Skills: &mockSkillReader{
			autoSkills: []skills.Skill{makeSkill("s1", "s1", "", "skill1", true, true)},
		},
		OpenEditorPaths: func() []string {
			return []string{"/home/user/doc.txt", "/tmp/notes.md"}
		},
	}
	msgs := []llm.Message{{Role: "user", Content: "leia o doc"}}
	result := b.Build(msgs, nil, false, nil, "", "")
	sys := result[0].Content.(string)
	if !strings.Contains(sys, "<open_editor_files>") {
		t.Error("Expected <open_editor_files> tag in system prompt")
	}
	if !strings.Contains(sys, "/home/user/doc.txt") {
		t.Error("Expected file path in open_editor_files section")
	}
	if !strings.Contains(sys, "/tmp/notes.md") {
		t.Error("Expected second file path in open_editor_files section")
	}
	if !strings.Contains(sys, "You CAN read and edit") {
		t.Error("Expected instruction text about reading/editing")
	}
}

func TestBuild_OpenEditorFiles_EmptyPaths_NoSection(t *testing.T) {
	b := &prompt.Builder{
		Skills: &mockSkillReader{
			autoSkills: []skills.Skill{makeSkill("s1", "s1", "", "skill1", true, true)},
		},
		OpenEditorPaths: func() []string { return nil },
	}
	msgs := []llm.Message{{Role: "user", Content: "oi"}}
	result := b.Build(msgs, nil, false, nil, "", "")
	sys := result[0].Content.(string)
	if strings.Contains(sys, "<open_editor_files>") {
		t.Error("Should not include open_editor_files when paths are empty")
	}
}

func TestBuild_OpenEditorFiles_NilFunc_NoSection(t *testing.T) {
	b := &prompt.Builder{
		Skills: &mockSkillReader{
			autoSkills: []skills.Skill{makeSkill("s1", "s1", "", "skill1", true, true)},
		},
	}
	msgs := []llm.Message{{Role: "user", Content: "oi"}}
	result := b.Build(msgs, nil, false, nil, "", "")
	sys := result[0].Content.(string)
	if strings.Contains(sys, "<open_editor_files>") {
		t.Error("Should not include open_editor_files when OpenEditorPaths is nil")
	}
}

func TestBuildSkillsSection_NilSkillReader_ReturnsEmpty(t *testing.T) {
	b := &prompt.Builder{}
	if got := b.BuildSkillsSection(nil, false, nil); got != "" {
		t.Errorf("Expected empty, got %q", got)
	}
}

func TestBuildSkillsSection_EmptyList_ReturnsEmpty(t *testing.T) {
	b := &prompt.Builder{Skills: &mockSkillReader{}}
	if got := b.BuildSkillsSection([]string{}, false, nil); got != "" {
		t.Errorf("Expected empty for disabled skills, got %q", got)
	}
}

func TestBuildSkillsSection_AutoLoad_ContainsAutoSkillsTag(t *testing.T) {
	s := makeSkill("dev", "Dev", "Dev desc", "Conteúdo de dev.", true, false)
	b := &prompt.Builder{Skills: &mockSkillReader{autoSkills: []skills.Skill{s}}}
	result := b.BuildSkillsSection(nil, false, nil)
	if !strings.Contains(result, "<auto_skills>") {
		t.Error("Expected <auto_skills> tag")
	}
	if !strings.Contains(result, "Conteúdo de dev.") {
		t.Error("Expected skill content")
	}
}

func TestBuildSkillsSection_ExplicitList_OrderRespected(t *testing.T) {
	s1 := makeSkill("alpha", "Alpha", "A", "Conteúdo A.", true, false)
	s2 := makeSkill("beta", "Beta", "B", "Conteúdo B.", true, false)
	b := &prompt.Builder{Skills: &mockSkillReader{allSkillsFull: []skills.Skill{s1, s2}}}
	result := b.BuildSkillsSection([]string{"beta", "alpha"}, true, nil)
	betaIdx := strings.Index(result, "Conteúdo B.")
	alphaIdx := strings.Index(result, "Conteúdo A.")
	if betaIdx == -1 || alphaIdx == -1 {
		t.Fatal("Both skills should appear")
	}
	if betaIdx > alphaIdx {
		t.Error("Beta should appear before Alpha")
	}
}

func TestBuildSkillsSection_AvailableSkills_ContainsTag(t *testing.T) {
	auto := makeSkill("auto", "Auto", "Auto desc", "Auto content.", true, false)
	avail := makeSkill("avail", "Avail", "Avail desc", "Avail content.", false, true)
	b := &prompt.Builder{Skills: &mockSkillReader{
		autoSkills: []skills.Skill{auto}, availableSkills: []skills.Skill{avail}}}
	result := b.BuildSkillsSection(nil, false, nil)
	if !strings.Contains(result, "<available_skills>") {
		t.Error("Expected <available_skills> tag")
	}
	if !strings.Contains(result, "Avail desc") {
		t.Error("Expected available skill description")
	}
}

func TestBuildSkillsSection_DisableOnDemand_NoAvailableSection(t *testing.T) {
	auto := makeSkill("auto", "Auto", "Auto desc", "Auto content.", true, false)
	avail := makeSkill("avail", "Avail", "Avail desc", "Avail content.", false, true)
	b := &prompt.Builder{Skills: &mockSkillReader{
		autoSkills: []skills.Skill{auto}, availableSkills: []skills.Skill{avail}}}
	result := b.BuildSkillsSection(nil, true, nil)
	if strings.Contains(result, "<available_skills>") {
		t.Error("Should not include <available_skills> when disableOnDemand=true")
	}
}

func TestBuildSkillsSection_SupplementaryFiles_Listed(t *testing.T) {
	s := makeSkill("dev", "Dev", "Dev desc", "Dev content.", true, false)
	b := &prompt.Builder{Skills: &mockSkillReader{
		autoSkills: []skills.Skill{s},
		skillFiles: map[string][]string{"dev": {"/skills/dev/guide.md"}},
	}}
	result := b.BuildSkillsSection(nil, false, nil)
	if !strings.Contains(result, "Supporting files") {
		t.Error("Expected supplementary files section")
	}
	if !strings.Contains(result, "guide.md") {
		t.Error("Expected guide.md listed")
	}
}

func TestBuildTemplateData_NilProfile_NoToolCalling(t *testing.T) {
	b := &prompt.Builder{}
	data := b.BuildTemplateData(nil, llm.ChatParams{ProfileSlug: "default"}, 42)
	if data.ToolCallingEnabled {
		t.Error("ToolCallingEnabled should be false")
	}
	if data.ConversationID != 42 {
		t.Errorf("ConversationID should be 42, got %d", data.ConversationID)
	}
}

func TestBuildTemplateData_WithWorkspace_FillsTabInfo(t *testing.T) {
	ws := &workspace.Workspace{
		Name: "Meu Workspace", Profile: "developer",
		Tabs: workspace.TabsState{
			Active: "tab-2",
			Items: []workspace.Tab{
				{ID: "tab-1", Title: "Terminal", Type: "terminal"},
				{ID: "tab-2", Title: "Editor", Type: "editor", ContentID: "main.go"},
			},
		},
	}
	b := &prompt.Builder{Workspace: &mockWorkspaceReader{ws: ws}}
	data := b.BuildTemplateData(nil, llm.ChatParams{ProfileSlug: "dev"}, 1)
	if data.WorkspaceName != "Meu Workspace" {
		t.Errorf("WorkspaceName: got %q", data.WorkspaceName)
	}
	if data.TabCount != 2 {
		t.Errorf("TabCount should be 2, got %d", data.TabCount)
	}
	if data.ActiveTabTitle != "Editor" {
		t.Errorf("ActiveTabTitle: got %q", data.ActiveTabTitle)
	}
	if data.ActiveTabType != "editor" {
		t.Errorf("ActiveTabType: got %q", data.ActiveTabType)
	}
}

func TestBuildTemplateData_WorkspaceNil_NoTabInfo(t *testing.T) {
	b := &prompt.Builder{Workspace: &mockWorkspaceReader{ws: nil}}
	data := b.BuildTemplateData(nil, llm.ChatParams{ProfileSlug: "dev"}, 1)
	if data.TabCount != 0 {
		t.Errorf("TabCount should be 0, got %d", data.TabCount)
	}
}

func TestBuildTemplateData_WithSurfacePayload(t *testing.T) {
	ws := &workspace.Workspace{
		Name: "Meu Workspace",
		Tabs: workspace.TabsState{
			Active: "tab-2",
			Items: []workspace.Tab{
				{ID: "tab-2", Title: "Editor", Type: "editor", State: map[string]any{"filePath": "/tmp/readme.md", "draftId": "draft-1"}},
			},
		},
	}
	b := &prompt.Builder{Workspace: &mockWorkspaceReader{ws: ws}}
	data := b.BuildTemplateData(nil, llm.ChatParams{
		ProfileSlug:        "editor-texto",
		TabType:            "editor",
		SurfaceStateJSON:   `{"filePath":"/tmp/readme.md","draftId":"draft-1"}`,
		SurfaceContextJSON: `{"selectedText":"hello","selectionEmpty":false}`,
	}, 7)

	if data.Surface == nil {
		t.Fatal("expected Surface to be filled")
	}
	if data.Surface.Type != "editor" {
		t.Fatalf("Surface.Type = %q, want editor", data.Surface.Type)
	}
	if data.Surface.Title != "Editor" {
		t.Fatalf("Surface.Title = %q, want Editor", data.Surface.Title)
	}
	if got := data.Surface.State["filePath"]; got != "/tmp/readme.md" {
		t.Fatalf("Surface.State[filePath] = %v, want /tmp/readme.md", got)
	}
	if got := data.Surface.Context["selectedText"]; got != "hello" {
		t.Fatalf("Surface.Context[selectedText] = %v, want hello", got)
	}
}

func TestBuildTemplateData_InvalidSurfaceLogsIdentifyField(t *testing.T) {
	var output strings.Builder
	logger := func(format string, args ...any) {
		_, _ = fmt.Fprintf(&output, format, args...)
		_, _ = output.WriteString("\n")
	}

	state := chat.DecodeSurfaceJSONMapWithLogger("{", "[prompt] surface state json", logger)
	context := chat.DecodeSurfaceJSONMapWithLogger("{", "[prompt] surface context json", logger)
	if state != nil || context != nil {
		t.Fatalf("expected invalid payloads to decode as nil, got state=%v context=%v", state, context)
	}
	logs := output.String()
	if !strings.Contains(logs, "[prompt] surface state json inválido") {
		t.Fatalf("esperava log do state, recebeu: %s", logs)
	}
	if !strings.Contains(logs, "[prompt] surface context json inválido") {
		t.Fatalf("esperava log do context, recebeu: %s", logs)
	}
}

func TestBuildTemplateData_DoesNotReuseActiveTabStateWhenSurfaceTypeDiffers(t *testing.T) {
	ws := &workspace.Workspace{
		Name: "Meu Workspace",
		Tabs: workspace.TabsState{
			Active: "tab-2",
			Items: []workspace.Tab{
				{ID: "tab-2", Title: "Editor", Type: "editor", State: map[string]any{"filePath": "/tmp/readme.md"}},
			},
		},
	}
	b := &prompt.Builder{Workspace: &mockWorkspaceReader{ws: ws}}
	data := b.BuildTemplateData(nil, llm.ChatParams{
		ProfileSlug: "terminal",
		TabType:     "terminal",
	}, 7)

	if data.Surface == nil {
		t.Fatal("expected Surface to be filled")
	}
	if data.Surface.Type != "terminal" {
		t.Fatalf("Surface.Type = %q, want terminal", data.Surface.Type)
	}
	if data.Surface.Title != "" {
		t.Fatalf("Surface.Title = %q, want empty", data.Surface.Title)
	}
	if data.Surface.State != nil {
		t.Fatalf("Surface.State = %v, want nil", data.Surface.State)
	}
}

func TestComputeEnabledToolNames_DisableTools_ReturnsNil(t *testing.T) {
	reg := tools.NewRegistry()
	_ = reg.Register(&fakeTool{name: "read_file"})
	profile := &profiles.Profile{}
	profile.Chat.DisableTools = true
	b := &prompt.Builder{Tools: reg}
	if names := b.ComputeEnabledToolNames(profile); names != nil {
		t.Errorf("Expected nil, got %v", names)
	}
}

func TestComputeEnabledToolNames_NilRegistry_ReturnsNil(t *testing.T) {
	b := &prompt.Builder{}
	if names := b.ComputeEnabledToolNames(nil); names != nil {
		t.Errorf("Expected nil, got %v", names)
	}
}

func TestComputeEnabledToolNames_AllTools_WhenNoFilter(t *testing.T) {
	reg := tools.NewRegistry()
	_ = reg.Register(&fakeTool{name: "read_file"})
	_ = reg.Register(&fakeTool{name: "write_file"})
	b := &prompt.Builder{Tools: reg}
	names := b.ComputeEnabledToolNames(nil)
	if len(names) != 2 {
		t.Errorf("Expected 2 tools, got %v", names)
	}
}

func TestComputeEnabledToolNames_ProfileFilter_OnlySelected(t *testing.T) {
	reg := tools.NewRegistry()
	_ = reg.Register(&fakeTool{name: "read_file"})
	_ = reg.Register(&fakeTool{name: "write_file"})
	_ = reg.Register(&fakeTool{name: "bash"})
	profile := &profiles.Profile{}
	profile.Chat.EnabledTools = []string{"read_file", "bash"}
	b := &prompt.Builder{Tools: reg}
	names := b.ComputeEnabledToolNames(profile)
	if len(names) != 2 {
		t.Errorf("Expected 2 tools, got %v", names)
	}
	for _, n := range names {
		if n != "read_file" && n != "bash" {
			t.Errorf("Unexpected tool %q", n)
		}
	}
}

type fakeTool struct{ name string }

func (f *fakeTool) Name() string                { return f.name }
func (f *fakeTool) Description() string         { return "fake tool " + f.name }
func (f *fakeTool) Parameters() json.RawMessage { return json.RawMessage(`{}`) }
func (f *fakeTool) Execute(_ context.Context, _ json.RawMessage) (tools.ToolResult, error) {
	return tools.ToolResult{Content: "ok"}, nil
}

// Regression: (*workspace.Manager)(nil) em WorkspaceReader não deve fazer BuildTemplateData panir.
func TestBuildTemplateData_TypedNilWorkspaceManager(t *testing.T) {
	var mgr *workspace.Manager
	b := &prompt.Builder{Workspace: mgr}
	data := b.BuildTemplateData(nil, llm.ChatParams{}, 1)
	if data.WorkspaceName != "" || data.TabCount != 0 {
		t.Fatalf("esperava dados de workspace vazios com manager typed-nil, obteve %+v", data)
	}
}
