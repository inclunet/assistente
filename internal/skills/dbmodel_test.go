package skills

import (
	"reflect"
	"testing"

	"assistente/internal/database"
)

func boolPtr(b bool) *bool { return &b }

// fullSkill cria um Skill com campos diversos (escalares, slices, ponteiros e
// permissões de tools) para validar roundtrip fiel.
func fullSkill() *Skill {
	return &Skill{
		Slug:    "coding-helper",
		Content: "# Coding Helper\n\nFaça coisas.",
		SkillMetadata: SkillMetadata{
			Name:        "coding-helper",
			Version:     "1.2.3",
			Description: "Um skill de exemplo para roundtrip de banco",
			DisplayName: "Coding Helper",
			Author:      "Equipe",
			AuthorEmail: "dev@example.com",
			AuthorURL:   "https://example.com",
			License:     "MIT",
			Repository:  "https://github.com/x/y",
			Homepage:    "https://example.com/home",
			Keywords:    []string{"go", "lint"},
			Category:    "dev",
			Subcategory: "backend",
			Type:        SkillTypeCommand,
			Difficulty:  DifficultyIntermediate,
			Audience:    []string{"devs"},
			MinVersion:  "1.0.0",
			MaxVersion:  "2.0.0",
			Platforms:   []string{"linux", "windows"},
			Languages:   []string{"go"},
			Frameworks:  []string{"wails"},

			AutoLoad:               true,
			DisableModelInvocation: false,
			UserInvocable:          boolPtr(false),
			ArgumentHint:           "[arg]",
			SkillContext:           "fork",
			Agent:                  "Explore",
			Model:                  "gpt-x",

			AllowedTools: "Read, Grep, Glob",
			Tools: &ToolPermissions{
				Allowed: []string{"Read", "Grep", "Glob"},
				Denied:  []string{"Delete"},
				BashCommands: &BashCommands{
					Allowed: []string{"ls"},
					Denied:  []string{"rm"},
				},
			},
			Filesystem: &FilesystemPermissions{
				Read:  []string{"/a"},
				Write: []string{"/b"},
				Deny:  []string{"/c"},
			},
			Network: &NetworkPermissions{
				AllowedHosts: []string{"example.com"},
				DeniedHosts:  []string{"evil.com"},
			},
			Input: &InputConfig{
				Arguments: []ArgumentDef{{Name: "issue", Type: "string", Required: true}},
			},
			Output: &OutputConfig{Format: "markdown"},
			Behavior: &BehaviorConfig{
				Timeout: 30,
				Retry:   &RetryConfig{MaxAttempts: 3, BackoffMs: 100},
			},
			Triggers: &TriggerConfig{
				Events:   []string{"PreToolUse"},
				Priority: 5,
			},
			Dependencies: &DependenciesConfig{
				NPM:      []string{"eslint"},
				Commands: []string{"git"},
				Skills:   []string{"memory"},
			},
			MCP: &MCPConfig{
				Server: &MCPServerConfig{Command: "node", Args: []string{"server.js"}},
				Tools:  []MCPToolDef{{Name: "do", Description: "does"}},
			},

			ContextBudget:      1200,
			AutoloadReason:     "always needed for coding context",
			RequiresTools:      true,
			RequiresFilesystem: true,
			RequiresNetwork:    false,
			RequiresMCP:        true,
		},
	}
}

func TestSkillRoundtrip(t *testing.T) {
	orig := fullSkill()

	model, err := skillToModel(orig)
	if err != nil {
		t.Fatalf("skillToModel: %v", err)
	}

	got, err := skillFromModel(model)
	if err != nil {
		t.Fatalf("skillFromModel: %v", err)
	}

	if got.Slug != orig.Slug {
		t.Errorf("slug: got %q want %q", got.Slug, orig.Slug)
	}
	if got.Content != orig.Content {
		t.Errorf("content: got %q want %q", got.Content, orig.Content)
	}
	if !reflect.DeepEqual(got.SkillMetadata, orig.SkillMetadata) {
		t.Errorf("metadata roundtrip mismatch:\n got: %+v\nwant: %+v", got.SkillMetadata, orig.SkillMetadata)
	}
}

func TestSkillRoundtripMinimal(t *testing.T) {
	orig := &Skill{
		Slug:    "minimal",
		Content: "corpo",
		SkillMetadata: SkillMetadata{
			Name:        "minimal",
			Version:     "0.1.0",
			Description: "skill minimo de teste roundtrip",
		},
	}

	model, err := skillToModel(orig)
	if err != nil {
		t.Fatalf("skillToModel: %v", err)
	}

	// Campos opcionais nil não devem gerar JSON "null".
	if model.ToolsConfig != "" || model.FilesystemConfig != "" || model.Keywords != "" {
		t.Errorf("campos opcionais nil deveriam ser vazios, got tools=%q fs=%q kw=%q",
			model.ToolsConfig, model.FilesystemConfig, model.Keywords)
	}

	got, err := skillFromModel(model)
	if err != nil {
		t.Fatalf("skillFromModel: %v", err)
	}
	if !reflect.DeepEqual(got.SkillMetadata, orig.SkillMetadata) {
		t.Errorf("metadata roundtrip mismatch:\n got: %+v\nwant: %+v", got.SkillMetadata, orig.SkillMetadata)
	}
}

func TestSkillToolRowsDerivation(t *testing.T) {
	orig := fullSkill()
	model, err := skillToModel(orig)
	if err != nil {
		t.Fatalf("skillToModel: %v", err)
	}

	var allowed, denied int
	for _, row := range model.Tools {
		switch row.Relation {
		case database.SkillToolAllowed:
			allowed++
		case database.SkillToolDenied:
			denied++
		default:
			t.Errorf("relation inesperada: %q", row.Relation)
		}
	}
	if allowed != 3 {
		t.Errorf("allowed rows: got %d want 3", allowed)
	}
	if denied != 1 {
		t.Errorf("denied rows: got %d want 1", denied)
	}
}

func TestSkillToolRowsNil(t *testing.T) {
	if rows := skillToolRows(nil); rows != nil {
		t.Errorf("skillToolRows(nil) = %v, want nil", rows)
	}
	if rows := skillToolRows(&ToolPermissions{}); rows != nil {
		t.Errorf("skillToolRows(empty) = %v, want nil", rows)
	}
}

func TestSkillInfoFromModel(t *testing.T) {
	model, err := skillToModel(fullSkill())
	if err != nil {
		t.Fatalf("skillToModel: %v", err)
	}

	model.IsBuiltin = true
	info, err := skillInfoFromModel(model)
	if err != nil {
		t.Fatalf("skillInfoFromModel: %v", err)
	}
	if info.Slug != "coding-helper" {
		t.Errorf("slug: got %q", info.Slug)
	}
	if !info.IsBuiltin {
		t.Errorf("isBuiltin: got false want true")
	}

	model.IsBuiltin = false
	info, _ = skillInfoFromModel(model)
	if info.IsBuiltin {
		t.Errorf("isBuiltin: got true want false")
	}
}
