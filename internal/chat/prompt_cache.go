package chat

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"

	"assistente/internal/profiles"
)

// ResolvePromptCacheHintKey resolve a política de provider hints de cache para
// o turno. A chave nunca contém conteúdo da conversa; só um hash de
// identificadores estáveis e não sensíveis.
func ResolvePromptCacheHintKey(profile *profiles.Profile, profileSlug, conversationID string) string {
	if profile == nil {
		return ""
	}
	cfg := profile.Chat.PromptCache
	if !cfg.Enabled || !cfg.ProviderHints {
		return ""
	}

	parts := []string{
		"assistente-v1",
		strings.TrimSpace(profile.Chat.LLMProvider),
		strings.TrimSpace(profile.Chat.Model),
		strings.TrimSpace(profileSlug),
		strings.TrimSpace(conversationID),
	}
	sum := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return "asst-" + hex.EncodeToString(sum[:16])
}
