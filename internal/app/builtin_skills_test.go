package app

import (
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"assistente/internal/profiles"
	"assistente/internal/skills"
)

func TestRemoveLegacyContextProviderSkills(t *testing.T) {
	homeDir := t.TempDir()
	for _, slug := range legacyContextProviderSkillSlugs {
		skillDir := filepath.Join(homeDir, slug)
		if err := os.MkdirAll(skillDir, 0755); err != nil {
			t.Fatalf("mkdir %s: %v", slug, err)
		}
		if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("# legacy"), 0644); err != nil {
			t.Fatalf("write %s: %v", slug, err)
		}
	}
	keptDir := filepath.Join(homeDir, "coding")
	if err := os.MkdirAll(keptDir, 0755); err != nil {
		t.Fatalf("mkdir kept skill: %v", err)
	}

	removeLegacyContextProviderSkills(homeDir)

	for _, slug := range legacyContextProviderSkillSlugs {
		if _, err := os.Stat(filepath.Join(homeDir, slug)); !os.IsNotExist(err) {
			t.Fatalf("legacy skill %s should be moved out of active skills, stat err=%v", slug, err)
		}
		matches, err := filepath.Glob(filepath.Join(homeDir, ".legacy-backup", "context-providers-*", slug, "SKILL.md"))
		if err != nil {
			t.Fatalf("glob backup for %s: %v", slug, err)
		}
		if len(matches) != 1 {
			t.Fatalf("legacy skill %s should have one backup, got %d", slug, len(matches))
		}
	}
	if _, err := os.Stat(keptDir); err != nil {
		t.Fatalf("non-legacy skill should remain: %v", err)
	}
	if _, err := os.Stat(filepath.Join(homeDir, legacyContextProviderCleanupMarker)); err != nil {
		t.Fatalf("cleanup marker should be written: %v", err)
	}
}

func TestRemoveLegacyContextProviderSkillsDoesNotRunAfterMarker(t *testing.T) {
	homeDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(homeDir, legacyContextProviderCleanupMarker), []byte("done"), 0644); err != nil {
		t.Fatalf("write marker: %v", err)
	}
	for _, slug := range legacyContextProviderSkillSlugs {
		skillDir := filepath.Join(homeDir, slug)
		if err := os.MkdirAll(skillDir, 0755); err != nil {
			t.Fatalf("mkdir %s: %v", slug, err)
		}
		if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("# custom "+slug), 0644); err != nil {
			t.Fatalf("write %s: %v", slug, err)
		}
	}

	removeLegacyContextProviderSkills(homeDir)

	for _, slug := range legacyContextProviderSkillSlugs {
		if _, err := os.Stat(filepath.Join(homeDir, slug, "SKILL.md")); err != nil {
			t.Fatalf("custom skill %s should remain after marker: %v", slug, err)
		}
	}
}

func TestRemoveLegacyContextProviderSkillsDoesNotWriteMarkerAfterBackupFailure(t *testing.T) {
	homeDir := t.TempDir()
	skillDir := filepath.Join(homeDir, legacyContextProviderSkillSlugs[0])
	if err := os.MkdirAll(skillDir, 0755); err != nil {
		t.Fatalf("mkdir legacy skill: %v", err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("# legacy"), 0644); err != nil {
		t.Fatalf("write legacy skill: %v", err)
	}
	if err := os.WriteFile(filepath.Join(homeDir, ".legacy-backup"), []byte("not a dir"), 0644); err != nil {
		t.Fatalf("write backup blocker: %v", err)
	}

	removeLegacyContextProviderSkills(homeDir)

	if _, err := os.Stat(filepath.Join(homeDir, legacyContextProviderCleanupMarker)); !os.IsNotExist(err) {
		t.Fatalf("cleanup marker should not be written after backup failure, stat err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(skillDir, "SKILL.md")); err != nil {
		t.Fatalf("legacy skill should remain after backup failure: %v", err)
	}
}

func TestBuiltinProfilesDoNotEnableLegacyContextProviderSkills(t *testing.T) {
	entries, err := fs.ReadDir(builtinProfilesFS, "builtin/profiles")
	if err != nil {
		t.Fatalf("read builtin profiles: %v", err)
	}
	legacy := map[string]bool{}
	for _, slug := range legacyContextProviderSkillSlugs {
		legacy[slug] = true
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		data, err := fs.ReadFile(builtinProfilesFS, "builtin/profiles/"+entry.Name())
		if err != nil {
			t.Fatalf("read profile %s: %v", entry.Name(), err)
		}
		var profile struct {
			Chat struct {
				EnabledSkills []string `json:"enabled_skills"`
			} `json:"chat"`
		}
		if err := json.Unmarshal(data, &profile); err != nil {
			t.Fatalf("parse profile %s: %v", entry.Name(), err)
		}
		for _, skill := range profile.Chat.EnabledSkills {
			if legacy[skill] {
				t.Fatalf("builtin profile %s enables legacy context provider skill %q", entry.Name(), skill)
			}
		}
	}
}

func TestBuiltinSlidesRevealMarkdownSkillParses(t *testing.T) {
	data, err := fs.ReadFile(builtinSkillsFS, "builtin/skills/slides-reveal-markdown/SKILL.md")
	if err != nil {
		t.Fatalf("read slides-reveal-markdown skill: %v", err)
	}

	meta, content, err := skills.Parse(string(data))
	if err != nil {
		t.Fatalf("parse slides-reveal-markdown skill: %v", err)
	}
	if meta.Name != "slides-reveal-markdown" {
		t.Fatalf("unexpected skill name: %q", meta.Name)
	}
	if meta.Version != "1.0.0" {
		t.Fatalf("unexpected skill version: %q", meta.Version)
	}
	if meta.Category != "editor" {
		t.Fatalf("unexpected skill category: %q", meta.Category)
	}
	if tools := meta.GetToolsAllowed(); len(tools) != 1 || tools[0] != "text_edit" {
		t.Fatalf("unexpected allowed tools: %#v", meta.GetToolsAllowed())
	}
	for _, required := range []string{
		"surface_context",
		"<selection",
		`<content kind="reveal_slide">`,
		`<metadata key="current_slide_index">`,
		"Criação de deck completo",
		"objetivo, audiência, duração",
		"título, agenda ou contexto",
		"Note:",
		"----",
		"texto alternativo",
	} {
		if !strings.Contains(content, required) {
			t.Fatalf("skill content should mention %q", required)
		}
	}
	for _, forbidden := range []string{"currentSlideIndex", "currentSlideMarkdown"} {
		if strings.Contains(content, forbidden) {
			t.Fatalf("skill content should not mention legacy surface_context key %q", forbidden)
		}
	}
}

func TestBuiltinEditorTextoSkillUsesSurfaceSelection(t *testing.T) {
	data, err := fs.ReadFile(builtinSkillsFS, "builtin/skills/editor-texto/SKILL.md")
	if err != nil {
		t.Fatalf("read editor-texto skill: %v", err)
	}

	meta, content, err := skills.Parse(string(data))
	if err != nil {
		t.Fatalf("parse editor-texto skill: %v", err)
	}
	if meta.Name != "editor-texto" {
		t.Fatalf("unexpected skill name: %q", meta.Name)
	}
	if meta.Version != "2.3.0" {
		t.Fatalf("unexpected skill version: %q", meta.Version)
	}
	allowedTools := meta.GetToolsAllowed()
	if len(allowedTools) != 2 || allowedTools[0] != "text_edit" || allowedTools[1] != "edit_file" {
		t.Fatalf("unexpected allowed tools: %#v", allowedTools)
	}
	for _, required := range []string{
		"<surface_context>",
		`<selection explicit="true">`,
		"alvo principal",
		"não procure um caminho paralelo de contexto",
		`<metadata key="file_path">`,
		"`text_edit`",
		"`edit_file`",
	} {
		if !strings.Contains(content, required) {
			t.Fatalf("editor-texto skill content should mention %q", required)
		}
	}
}

func TestEditorTextoProfileEnablesSlidesRevealMarkdownOnDemand(t *testing.T) {
	data, err := fs.ReadFile(builtinProfilesFS, "builtin/profiles/editor-texto.json")
	if err != nil {
		t.Fatalf("read editor-texto profile: %v", err)
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("parse raw editor-texto profile: %v", err)
	}
	var profile struct {
		BuiltinVersion string `json:"_builtin_version"`
		Chat           struct {
			LLMProvider       string            `json:"llm_provider"`
			Model             string            `json:"model"`
			EnabledTools      []string          `json:"enabled_tools"`
			EnabledSkills     []string          `json:"enabled_skills"`
			ToolPolicy        map[string]string `json:"tool_policy"`
			ToolPolicyDefault string            `json:"tool_policy_default"`
		} `json:"chat"`
	}
	if err := json.Unmarshal(data, &profile); err != nil {
		t.Fatalf("parse editor-texto profile: %v", err)
	}
	if profile.BuiltinVersion != "4.3.0" {
		t.Fatalf("unexpected builtin version: %q", profile.BuiltinVersion)
	}
	if profile.Chat.LLMProvider != profiles.DefaultProviderSentinel {
		t.Fatalf("chat.llm_provider should use default sentinel, got %q", profile.Chat.LLMProvider)
	}
	if profile.Chat.Model != profiles.DefaultProviderSentinel {
		t.Fatalf("chat.model should use default sentinel, got %q", profile.Chat.Model)
	}
	chatRaw, ok := raw["chat"].(map[string]any)
	if !ok {
		t.Fatalf("editor-texto chat should be an object, got %#v", raw["chat"])
	}
	if _, hasLegacyEnabledTools := chatRaw["enabled_tools"]; hasLegacyEnabledTools {
		t.Fatalf("editor-texto should use tool_policy instead of legacy enabled_tools")
	}
	if profile.Chat.ToolPolicyDefault != "disabled" {
		t.Fatalf("editor-texto should fail closed, got default %q", profile.Chat.ToolPolicyDefault)
	}
	wantPolicy := map[string]string{
		"text_edit": "preloaded",
		"edit_file": "preloaded",
	}
	if len(profile.Chat.ToolPolicy) != len(wantPolicy) {
		t.Fatalf("unexpected tool_policy: %#v", profile.Chat.ToolPolicy)
	}
	for tool, want := range wantPolicy {
		if got := profile.Chat.ToolPolicy[tool]; got != want {
			t.Fatalf("tool_policy[%s] = %q, want %q", tool, got, want)
		}
	}
	if len(profile.Chat.EnabledSkills) < 2 {
		t.Fatalf("expected at least base and on-demand skills, got %#v", profile.Chat.EnabledSkills)
	}
	if profile.Chat.EnabledSkills[0] != "editor-texto" {
		t.Fatalf("first skill should remain editor-texto base, got %#v", profile.Chat.EnabledSkills)
	}
	if profile.Chat.EnabledSkills[1] != "slides-reveal-markdown" {
		t.Fatalf("second skill should be slides-reveal-markdown on-demand, got %#v", profile.Chat.EnabledSkills)
	}
}
