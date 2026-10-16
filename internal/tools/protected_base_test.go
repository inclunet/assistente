package tools

import (
	"context"
	"testing"
)

func TestIsProtectedBaseTool(t *testing.T) {
	protected := []string{
		ToolCatalogName,
		LoadSkillName,
		"memory",
		"task",
		"task_list",
		"task_note",
		"update_plan",
		"read_tool_result",
	}
	for _, name := range protected {
		if !IsProtectedBaseTool(name) {
			t.Errorf("esperava %q no conjunto base protegido", name)
		}
	}
	for _, name := range []string{"read_file", "shell", "web_fetch", ""} {
		if IsProtectedBaseTool(name) {
			t.Errorf("não esperava %q no conjunto base protegido", name)
		}
	}
}

// A allowlist da skill faz narrowing das tools de domínio, mas não pode remover
// implicitamente o conjunto base protegido.
func TestValidateExecutionContextToolAccess_ProtectedBaseAllowedDespiteAllowlist(t *testing.T) {
	ctx := WithExecutionContext(context.Background(), ExecutionContext{
		InvokedSkillSlug: "pesquisa",
		AllowedTools:     []string{"web_fetch"}, // allowlist focada no domínio, omite as base
	})
	for _, name := range []string{"memory", ToolCatalogName, LoadSkillName, "update_plan"} {
		if err := validateExecutionContextToolAccess(ctx, name); err != nil {
			t.Errorf("tool base %q deveria ser permitida mesmo fora da allowlist: %v", name, err)
		}
	}
}

// Deny explícito da skill vence o conjunto base protegido.
func TestValidateExecutionContextToolAccess_ExplicitDenyBeatsProtectedBase(t *testing.T) {
	ctx := WithExecutionContext(context.Background(), ExecutionContext{
		InvokedSkillSlug: "restrita",
		AllowedTools:     []string{"web_fetch"},
		DeniedTools:      []string{"memory"},
	})
	if err := validateExecutionContextToolAccess(ctx, "memory"); err == nil {
		t.Fatal("deny explícito deveria bloquear a tool base 'memory'")
	}
}

// Tool de domínio fora da allowlist continua bloqueada.
func TestValidateExecutionContextToolAccess_DomainToolOutsideAllowlistBlocked(t *testing.T) {
	ctx := WithExecutionContext(context.Background(), ExecutionContext{
		InvokedSkillSlug: "pesquisa",
		AllowedTools:     []string{"web_fetch"},
	})
	if err := validateExecutionContextToolAccess(ctx, "shell"); err == nil {
		t.Fatal("tool de domínio 'shell' fora da allowlist deveria ser bloqueada")
	}
	// A tool que está na allowlist continua permitida.
	if err := validateExecutionContextToolAccess(ctx, "web_fetch"); err != nil {
		t.Fatalf("tool 'web_fetch' na allowlist não deveria ser bloqueada: %v", err)
	}
}
