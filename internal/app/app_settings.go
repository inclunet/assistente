package app

import (
	"fmt"

	"assistente/internal/skills"
)

// ============================================================================
// Settings — SendMessageSync permanece no App (não é domínio settings na borda).
// Clear*/TestConnection/NativeTTS/ResetConfig → wailsapi.Settings (AEP-0088).
// ============================================================================

func (a *App) SendMessageSync(messages []Message, params ChatParams) (string, error) {
	if a.settingsCtrl == nil {
		return "", fmt.Errorf("nenhum provedor LLM configurado no perfil ativo")
	}
	ctx, err := a.requireAuthenticatedContext()
	if err != nil {
		return "", err
	}
	return a.settingsCtrl.SendMessageSync(ctx, messages, params)
}

// parseSlashCommand é um shim para manter compatibilidade com testes e código existente.
func parseSlashCommand(content string) (slug string, args string, ok bool) {
	return skills.ParseSlashCommand(content)
}
