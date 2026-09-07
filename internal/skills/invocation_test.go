package skills

import (
	"strings"
	"testing"
)

// ─── ParseSlashCommand ────────────────────────────────────────────────────────

func TestParseSlashCommand_Basic(t *testing.T) {
	slug, args, ok := ParseSlashCommand("/fix-issue 42")
	if !ok {
		t.Fatal("expected slash command to be detected")
	}
	if slug != "fix-issue" {
		t.Errorf("slug = %q, want fix-issue", slug)
	}
	if args != "42" {
		t.Errorf("args = %q, want 42", args)
	}
}

func TestParseSlashCommand_NoArgs(t *testing.T) {
	slug, args, ok := ParseSlashCommand("/memory")
	if !ok || slug != "memory" || args != "" {
		t.Errorf("got (%q, %q, %v), want (memory, \"\", true)", slug, args, ok)
	}
}

func TestParseSlashCommand_MultipleArgs(t *testing.T) {
	slug, args, ok := ParseSlashCommand(`/deploy staging "my app"`)
	if !ok || slug != "deploy" {
		t.Errorf("got slug=%q ok=%v, want deploy/true", slug, ok)
	}
	if args == "" {
		t.Error("args should not be empty")
	}
}

func TestParseSlashCommand_NotSlash(t *testing.T) {
	_, _, ok := ParseSlashCommand("hello world")
	if ok {
		t.Error("should not detect as slash command")
	}
}

func TestParseSlashCommand_EmptySlash(t *testing.T) {
	_, _, ok := ParseSlashCommand("/")
	if ok {
		t.Error("bare / should not be a valid slash command")
	}
}

func TestParseSlashCommand_SpaceAfterSlash(t *testing.T) {
	_, _, ok := ParseSlashCommand("/ something")
	if ok {
		t.Error("/ with leading space should not be valid")
	}
}

func TestParseSlashCommand_SpecialChars(t *testing.T) {
	_, _, ok := ParseSlashCommand("/foo_bar baz")
	if ok {
		t.Error("slug with underscore should not be valid")
	}
}

func TestParseSlashCommand_UpperCase(t *testing.T) {
	slug, _, ok := ParseSlashCommand("/MEMORY")
	if !ok {
		t.Fatal("uppercase slug should be accepted (lowercased)")
	}
	if slug != "memory" {
		t.Errorf("slug = %q, want memory", slug)
	}
}

func TestParseSlashCommand_WithWhitespace(t *testing.T) {
	slug, args, ok := ParseSlashCommand("  /fix-issue 42  ")
	if !ok || slug != "fix-issue" || args != "42" {
		t.Errorf("got (%q, %q, %v)", slug, args, ok)
	}
}

// ─── Invoke ───────────────────────────────────────────────────────────────────

type stubInvokerManager struct {
	skill *Skill
	err   error
	files []string
}

func (m *stubInvokerManager) Get(slug string) (*Skill, error) {
	return m.skill, m.err
}
func (m *stubInvokerManager) GetSkillFiles(slug string) ([]string, error) {
	return m.files, nil
}

func invocableSkill(content string) *Skill {
	return &Skill{
		SkillMetadata: SkillMetadata{
			Name: "test-skill",
			Type: "assistant",
		},
		Content: content,
	}
}

func invocablePolicy(s *Skill) SelectionPolicy {
	s.Slug = "test-skill"
	return SelectionPolicy{OnDemand: []Skill{*s}}
}

func TestInvoke_NotSlashCommand(t *testing.T) {
	_, found, err := Invoke("olá mundo", &stubInvokerManager{}, nil, "1", SelectionPolicy{})
	if err != nil || found {
		t.Errorf("non-slash content: found=%v err=%v", found, err)
	}
}

func TestInvoke_NilManager(t *testing.T) {
	_, found, err := Invoke("/test-skill", nil, nil, "1", SelectionPolicy{})
	if err != nil || found {
		t.Errorf("nil manager: found=%v err=%v", found, err)
	}
}

func TestInvoke_SkillNotFound(t *testing.T) {
	mgr := &stubInvokerManager{err: &skillNotFoundError{}}
	_, found, err := Invoke("/missing", mgr, nil, "1", SelectionPolicy{})
	if err != nil || found {
		t.Errorf("missing skill: found=%v err=%v", found, err)
	}
}

func TestInvoke_ReturnsContent(t *testing.T) {
	s := invocableSkill("conteúdo do skill")
	mgr := &stubInvokerManager{skill: s}
	result, found, err := Invoke("/test-skill", mgr, nil, "42", invocablePolicy(s))
	if err != nil {
		t.Fatal(err)
	}
	if !found {
		t.Fatal("expected found=true")
	}
	if result.SkillSlug != "test-skill" {
		t.Errorf("SkillSlug = %q, want test-skill", result.SkillSlug)
	}
	if result.Content == "" {
		t.Error("Content must not be empty")
	}
	// Deve conter as tags XML
	if len(result.Content) < len("<invoked_skill>") {
		t.Error("Content missing invoked_skill XML wrapper")
	}
}

func TestInvoke_WithFilesystem(t *testing.T) {
	s := invocableSkill("conteúdo")
	s.Filesystem = &FilesystemPermissions{
		Read:  []string{"/home"},
		Write: []string{"/tmp"},
		Deny:  []string{"/etc"},
	}
	mgr := &stubInvokerManager{skill: s}
	result, found, err := Invoke("/test-skill", mgr, nil, "1", invocablePolicy(s))
	if err != nil || !found {
		t.Fatalf("found=%v err=%v", found, err)
	}
	if result.Filesystem == nil {
		t.Fatal("expected Filesystem to be set")
	}
	if len(result.Filesystem.Read) != 1 || result.Filesystem.Read[0] != "/home" {
		t.Errorf("Filesystem.Read = %v, want [/home]", result.Filesystem.Read)
	}
}

func TestInvoke_WithSupplementaryFiles(t *testing.T) {
	mgr := &stubInvokerManager{
		skill: invocableSkill("c"),
		files: []string{"/path/to/extra.md"},
	}
	result, _, _ := Invoke("/test-skill", mgr, nil, "1", invocablePolicy(mgr.skill))
	if result == nil {
		t.Fatal("result must not be nil")
	}
	// Supporting files deve aparecer no conteúdo
	const marker = "Supporting files"
	found := false
	for i := 0; i+len(marker) <= len(result.Content); i++ {
		if result.Content[i:i+len(marker)] == marker {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("supplementary files section not found in content:\n%s", result.Content)
	}
}

func TestInvoke_WithPolicyRejectsDisabledSkill(t *testing.T) {
	mgr := &stubInvokerManager{skill: invocableSkill("conteúdo")}
	_, found, err := Invoke("/test-skill", mgr, nil, "1", SelectionPolicy{})
	if !found {
		t.Fatal("slash command should be found")
	}
	if err == nil {
		t.Fatal("disabled skill should return an error")
	}
}

func TestInvoke_WithPolicyAllowsOnDemandSkill(t *testing.T) {
	s := invocableSkill("conteúdo")
	s.Slug = "test-skill"
	mgr := &stubInvokerManager{skill: s}
	result, found, err := Invoke("/test-skill", mgr, nil, "1", SelectionPolicy{OnDemand: []Skill{*s}})
	if err != nil || !found {
		t.Fatalf("found=%v err=%v", found, err)
	}
	if result.Mode != SkillModeOnDemand {
		t.Fatalf("expected on_demand mode, got %s", result.Mode)
	}
}

func TestInvoke_AllowsTemplateExamplesAsPlainContent(t *testing.T) {
	s := invocableSkill("{{ .ToolCallingEnabled }}")
	s.Slug = "test-skill"
	mgr := &stubInvokerManager{skill: s}
	result, found, err := Invoke("/test-skill", mgr, nil, "1", SelectionPolicy{OnDemand: []Skill{*s}})
	if err != nil || !found {
		t.Fatalf("found=%v err=%v", found, err)
	}
	if result == nil || !strings.Contains(result.Content, "{{ .ToolCallingEnabled }}") {
		t.Fatalf("expected template example to be preserved as plain content, got %#v", result)
	}
}

// skillNotFoundError é um erro stub para testes.
type skillNotFoundError struct{}

func (e *skillNotFoundError) Error() string { return "skill not found" }
