package skillloader

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"assistente/internal/profiles"
	"assistente/internal/skills"
	"assistente/internal/tools/invocationctx"
)

type fakeSkillManager struct {
	skills []skills.Skill
	err    error
}

func (m fakeSkillManager) GetAllSkillsFull() ([]skills.Skill, error) {
	return m.skills, m.err
}

type fakeProfileManager struct {
	bySlug map[string]*profiles.Profile
	active *profiles.Profile
}

func (m fakeProfileManager) Get(slug string) (*profiles.Profile, error) {
	if p, ok := m.bySlug[slug]; ok {
		return p, nil
	}
	return nil, fmt.Errorf("profile not found: %s", slug)
}

func (m fakeProfileManager) GetActive() (*profiles.Profile, error) {
	return m.active, nil
}

func testSkill(slug string) skills.Skill {
	return skills.Skill{
		SkillMetadata: skills.SkillMetadata{
			Name:        slug,
			DisplayName: slug,
			Description: "Skill de teste",
		},
		Slug:    slug,
		Content: "Use esta skill.",
	}
}

func TestLoadSkillLoadsProfileOnDemandSkill(t *testing.T) {
	profile := profiles.DefaultProfile()
	profile.Chat.EnabledSkills = []string{"base", "review"}
	review := testSkill("review")
	review.Filesystem = &skills.FilesystemPermissions{Read: []string{"src/**"}, Deny: []string{"secrets/**"}}
	tool := New(fakeSkillManager{skills: []skills.Skill{testSkill("base"), review}}, fakeProfileManager{
		bySlug: map[string]*profiles.Profile{"dev": profile},
		active: profile,
	})
	ctx := invocationctx.With(context.Background(), invocationctx.InvocationContext{ProfileSlug: "dev"})

	result, err := tool.Execute(ctx, json.RawMessage(`{"skill":"review","reason":"preciso revisar o código"}`))
	if err != nil {
		t.Fatalf("Execute erro: %v", err)
	}
	if result.IsError {
		t.Fatalf("não esperava erro: %s", result.Content)
	}
	if !strings.Contains(result.Content, "<loaded_skill>") || !strings.Contains(result.Content, "Use esta skill.") {
		t.Fatalf("conteúdo de skill não carregado: %s", result.Content)
	}
	if result.Metadata["skill_slug"] != "review" {
		t.Fatalf("metadata skill_slug inesperado: %#v", result.Metadata)
	}
	read, _ := result.Metadata["filesystem_read"].([]string)
	if len(read) != 1 || read[0] != "src/**" {
		t.Fatalf("filesystem_read inesperado: %#v", result.Metadata["filesystem_read"])
	}
}

func TestLoadSkillRejectsDisabledSkill(t *testing.T) {
	profile := profiles.DefaultProfile()
	profile.Chat.EnabledSkills = []string{"base"}
	tool := New(fakeSkillManager{skills: []skills.Skill{testSkill("base"), testSkill("review")}}, fakeProfileManager{
		bySlug: map[string]*profiles.Profile{"dev": profile},
		active: profile,
	})
	ctx := invocationctx.With(context.Background(), invocationctx.InvocationContext{ProfileSlug: "dev"})

	result, err := tool.Execute(ctx, json.RawMessage(`{"skill":"review"}`))
	if err != nil {
		t.Fatalf("Execute erro: %v", err)
	}
	if !result.IsError {
		t.Fatal("skill fora do enabled_skills deveria ser rejeitada")
	}
}

func TestLoadSkillRejectsNonModelInvocableSkill(t *testing.T) {
	profile := profiles.DefaultProfile()
	profile.Chat.EnabledSkills = []string{"base", "review"}
	review := testSkill("review")
	review.DisableModelInvocation = true
	tool := New(fakeSkillManager{skills: []skills.Skill{testSkill("base"), review}}, fakeProfileManager{
		bySlug: map[string]*profiles.Profile{"dev": profile},
		active: profile,
	})
	ctx := invocationctx.With(context.Background(), invocationctx.InvocationContext{ProfileSlug: "dev"})

	result, err := tool.Execute(ctx, json.RawMessage(`{"skill":"review"}`))
	if err != nil {
		t.Fatalf("Execute erro: %v", err)
	}
	if !result.IsError {
		t.Fatal("skill com disable-model-invocation deveria ser rejeitada")
	}
}
