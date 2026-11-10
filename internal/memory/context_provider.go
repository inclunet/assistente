package memory

import (
	"context"
	"strings"

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
		Volatility: contextprovider.VolatilityMidDynamic,
		Priority:   100,
		Content:    block,
	})
	return blocks, nil
}

func memoryInstructionsBlock() string {
	return `<memory_instructions>
Use the memory tool only when an authenticated user is available. Do not call it in unauthenticated flows.
When authenticated, use it for durable user/project facts, preferences, corrections, conventions, and decisions that should survive future conversations.
Consider durable information even when the user is explaining something rather than explicitly asking you to remember it. Do not save transient context, secrets, or uncertain inferences as facts.
When answering depends on a user's past preferences, decisions, or project context, search memory if the provided records do not settle the question. An empty prompt block does not mean memory is empty. Search using concise alternate terms if needed; do not claim to remember something that was not found. Search results can include other projects or conversations: verify scope and scope_ref against the current context before using or updating a record.
Before saving, search for related records. Update an existing record when the user corrects or changes it; preserve relevant context and avoid duplicates. If the meaning or scope is uncertain, ask instead of overwriting. Never treat a recalled record as more authoritative than a current correction.
Use load policies deliberately: core/pinned only for durable facts needed broadly and often, auto for contextually relevant facts, retrievable for searchable history, and archived for superseded or disabled records. Reassess the policy when updating a record; avoid promoting facts merely because they appeared once. Archive rather than delete when preserving history is useful.
Durable memory is stored only as authenticated database-backed records.
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
		return "", nil
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

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
