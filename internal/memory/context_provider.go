package memory

import (
	"context"
	"strings"

	"assistente/internal/configdir"
	"assistente/internal/contextprovider"
	"assistente/internal/database"
)

const defaultPromptBudget = 1200

func (s *Service) Name() string { return "memory" }

func (s *Service) Metadata() contextprovider.ProviderMetadata {
	return contextprovider.ProviderMetadata{
		Name:             s.Name(),
		DisplayName:      "Memory",
		Description:      "Saved user, project, and conversation facts that can enter the prompt automatically.",
		DefaultEnabled:   true,
		DefaultBudget:    defaultPromptBudget,
		SupportsSettings: false,
	}
}

func (s *Service) Build(ctx context.Context, req contextprovider.BuildRequest) ([]contextprovider.Block, error) {
	instructions := memoryInstructionsBlock
	if _, ok := database.UserIDFromContext(ctx); !ok {
		instructions = legacyMemoryInstructionsBlock
	}
	blocks := []contextprovider.Block{{
		Provider:   s.Name(),
		Name:       "memory_instructions",
		Volatility: contextprovider.VolatilityStable,
		Priority:   10,
		Content:    instructions(),
	}}
	block, err := s.promptBlock(ctx, req, req.Budget(s.Name(), defaultPromptBudget))
	if err != nil {
		return blocks, err
	}
	if block == "" {
		return blocks, nil
	}
	blocks = append(blocks, contextprovider.Block{
		Provider:   s.Name(),
		Name:       "user_memory",
		Volatility: contextprovider.VolatilitySlowDynamic,
		Priority:   100,
		Content:    block,
	})
	return blocks, nil
}

func memoryInstructionsBlock() string {
	return `<memory_instructions>
Use the memory tool for durable user/project facts, preferences, corrections, conventions, and decisions that should survive future conversations.
Prefer database-backed memory records. Use load policies deliberately: core/pinned for facts that should enter context, auto for relevant contextual recall, retrievable for searchable history, and archived for disabled records.
Do not write new memories to legacy Markdown files. In unauthenticated legacy compatibility flows, memory.md may appear as read-only context until the user recomposes it into database records.
</memory_instructions>`
}

func legacyMemoryInstructionsBlock() string {
	return `<memory_instructions>
Database-backed durable memory requires an authenticated user. You are in legacy compatibility mode: legacy memory.md content may appear as read-only context.
Do not call the memory tool to create, update, or delete records until the user is authenticated. You may propose memory.md additions for the user to apply manually.
</memory_instructions>`
}

func (s *Service) PromptBlock(ctx context.Context, budgetChars int) (string, error) {
	return s.promptBlock(ctx, contextprovider.BuildRequest{}, budgetChars)
}

func (s *Service) promptBlock(ctx context.Context, req contextprovider.BuildRequest, budgetChars int) (string, error) {
	if budgetChars <= 0 {
		budgetChars = defaultPromptBudget
	}
	if _, ok := database.UserIDFromContext(ctx); !ok {
		return legacyPromptBlock(budgetChars), nil
	}
	records, err := s.store.PromptCandidates(ctx, PromptCandidateFilter{
		LoadPolicies:   []string{LoadPolicyCore, LoadPolicyPinned, LoadPolicyAuto},
		ConversationID: req.ConversationID,
		WorkspaceID:    firstNonEmpty(req.WorkspaceID, surfaceString(req.Surface, "workspaceId")),
		ProjectID:      firstNonEmpty(req.ProjectID, surfaceString(req.Surface, "projectId")),
		RelevanceText:  req.CurrentUserText,
		Limit:          200,
	})
	if err != nil {
		return "", err
	}
	if len(records) == 0 {
		if hasDBMemory, err := s.hasAnyMemoryRecord(ctx); err != nil || hasDBMemory {
			return "", err
		}
		return "", nil
	}

	lines := NewPromptSelector().Select(records, req, budgetChars)
	if len(lines) == 0 {
		return "", nil
	}

	var sb strings.Builder
	sb.WriteString("<user_memory>\n")
	sb.WriteString("Use these saved memory records when relevant. Do not expose private metadata unless asked.\n")
	for _, line := range lines {
		sb.WriteString(line.Line)
	}
	sb.WriteString("</user_memory>")
	return sb.String(), nil
}

func (s *Service) hasAnyMemoryRecord(ctx context.Context) (bool, error) {
	result, err := s.store.List(ctx, Filter{
		IncludeArchived: true,
		Limit:           1,
	})
	if err != nil {
		return false, err
	}
	return len(result.Records) > 0, nil
}

func legacyPromptBlock(budgetChars int) string {
	data, _, err := configdir.NewResolver("memory").Read("memory.md")
	if err != nil {
		return ""
	}
	content := strings.TrimSpace(string(data))
	if content == "" || strings.Contains(content, "Ainda não há memórias salvas") {
		return ""
	}
	const prefix = "<legacy_user_memory>\nRead-only legacy memory.md content. Durable database-backed memory requires authentication; propose changes for the user to apply manually.\n"
	const suffix = "\n</legacy_user_memory>"
	contentBudget := budgetChars - len([]rune(prefix)) - len([]rune(suffix))
	if contentBudget <= 0 {
		return ""
	}
	runes := []rune(content)
	if len(runes) > contentBudget {
		content = strings.TrimSpace(string(runes[:contentBudget]))
	}
	if content == "" {
		return ""
	}
	return prefix + content + suffix
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
