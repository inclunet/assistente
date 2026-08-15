package app

import (
	"assistente/internal/skills"
)

// ============================================================================
// Settings — Clear*/TestConnection/NativeTTS/ResetConfig → wailsapi.Settings (AEP-0088).
// SendMessageSync → wailsapi.Chat (probe de acessibilidade; lógica em SettingsController).
// ============================================================================

// parseSlashCommand é um shim para manter compatibilidade com testes e código existente.
func parseSlashCommand(content string) (slug string, args string, ok bool) {
	return skills.ParseSlashCommand(content)
}
