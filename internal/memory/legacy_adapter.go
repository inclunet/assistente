package memory

import (
	"context"
	"log/slog"
	"strings"

	"assistente/internal/configdir"
)

const legacyMemoryAdapterEvent = "legacy_memory_markdown_adapter_applied"

func legacyMemoryAdapterInstructions() string {
	return `<memory_instructions>
Database-backed durable memory requires an authenticated user. You are in legacy compatibility mode: legacy memory.md content may appear as read-only context.
Do not call the memory tool to create, update, or delete records until the user is authenticated. You may propose memory.md additions for the user to apply manually.
</memory_instructions>`
}

func buildLegacyMemoryPromptBlock(budgetChars int) string {
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
	logLegacyMemoryAdapterApplied()
	return prefix + content + suffix
}

// logLegacyMemoryAdapterApplied usa apenas valores fixos e não recebe o
// contexto da requisição para não registrar conteúdo, paths, payloads ou IDs.
func logLegacyMemoryAdapterApplied() {
	slog.LogAttrs(
		context.Background(),
		slog.LevelInfo,
		legacyMemoryAdapterEvent,
		slog.String("component", "context_compatibility"),
		slog.String("adapter", "memory_markdown_without_auth"),
		slog.Int("schema_version", 1),
	)
}
