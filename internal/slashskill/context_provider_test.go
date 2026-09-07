package slashskill

import (
	"context"
	"strings"
	"testing"

	"assistente/internal/contextprovider"
)

func TestContextProviderBuildsSlashSkillBlock(t *testing.T) {
	blocks, err := NewContextProvider().Build(context.Background(), contextprovider.BuildRequest{
		SlashSkillContent: "<invoked_skill>\nUse these instructions.\n</invoked_skill>",
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if len(blocks) != 1 {
		t.Fatalf("len(blocks) = %d, want 1", len(blocks))
	}
	block := blocks[0]
	if block.Provider != "slash_skill" || block.Name != "slash_skill" {
		t.Fatalf("unexpected block identity: %+v", block)
	}
	if block.Volatility != contextprovider.VolatilityTurnDynamic || block.Priority != 200 {
		t.Fatalf("unexpected block ordering metadata: %+v", block)
	}
	if !strings.Contains(block.Content, "<invoked_skill>") || !strings.Contains(block.Content, "Use these instructions.") {
		t.Fatalf("unexpected slash skill content: %q", block.Content)
	}
}

func TestContextProviderOmitsEmptySlashSkill(t *testing.T) {
	blocks, err := NewContextProvider().Build(context.Background(), contextprovider.BuildRequest{})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if len(blocks) != 0 {
		t.Fatalf("len(blocks) = %d, want 0: %+v", len(blocks), blocks)
	}
}

func TestContextProviderTruncatesTaggedSlashSkill(t *testing.T) {
	blocks, err := NewContextProvider().Build(context.Background(), contextprovider.BuildRequest{
		SlashSkillContent: "<invoked_skill>\n" + strings.Repeat("Use these instructions. ", 30) + "\n</invoked_skill>",
		ProviderBudgets:   map[string]int{"slash_skill": 160},
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if len(blocks) != 1 {
		t.Fatalf("len(blocks) = %d, want truncated slash skill", len(blocks))
	}
	if contextprovider.RuneLen(blocks[0].Content) > 160 {
		t.Fatalf("slash skill length = %d, want <= 160: %q", contextprovider.RuneLen(blocks[0].Content), blocks[0].Content)
	}
	if !strings.Contains(blocks[0].Content, "omitted due to context budget") {
		t.Fatalf("expected truncation notice: %q", blocks[0].Content)
	}
	if !strings.Contains(blocks[0].Content, "<invoked_skill>") || !strings.Contains(blocks[0].Content, "</invoked_skill>") {
		t.Fatalf("truncated slash skill should keep tags: %q", blocks[0].Content)
	}
}

func TestContextProviderPreservesInvokedSkillArgumentsTag(t *testing.T) {
	blocks, err := NewContextProvider().Build(context.Background(), contextprovider.BuildRequest{
		SlashSkillContent: "<invoked_skill_arguments>\nArguments:\nrevisar login\n</invoked_skill_arguments>",
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if len(blocks) != 1 {
		t.Fatalf("len(blocks) = %d, want 1", len(blocks))
	}
	if !strings.Contains(blocks[0].Content, "<invoked_skill_arguments>") || !strings.Contains(blocks[0].Content, "revisar login") {
		t.Fatalf("unexpected arguments block: %q", blocks[0].Content)
	}
}
