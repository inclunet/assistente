package app

import (
	"strings"

	"assistente/internal/acptrust"
	"assistente/internal/profiles"
)

// GetAgentPermissions / RevokeAgentPermission migraram para wailsapi.ACPTrust
// (AEP-0088). Helpers de nome de perfil e o store acpTrust permanecem no App
// para o wiring e para os handlers de permissão em tempo de turno
// (app_acp_permissions.go).

// profileNames mapeia slug para o nome que a pessoa deu ao perfil. Perfil
// apagado não aparece aqui, e a tela cai no slug.
func (a *App) profileNames() map[string]string {
	if a == nil || a.profileManager == nil {
		return nil
	}
	infos, err := a.profileManager.List()
	if err != nil {
		return nil
	}
	return profileNamesFrom(infos)
}

// profileNamesFrom cruza os perfis do app com o jeito como as autorizações
// guardam o slug. A chave passa pela mesma sanitização do arquivo: um perfil
// escrito com maiúsculas gravaria em "profile-cursor.json" e depois não casaria
// com a própria linha, que então apareceria pelo slug como se ele tivesse sido
// apagado. Nome em branco não conta como nome — a tela cai no slug, que ao
// menos identifica a linha.
func profileNamesFrom(infos []profiles.ProfileInfo) map[string]string {
	names := make(map[string]string, len(infos))
	for _, info := range infos {
		key := acptrust.ProfileKey(info.Slug)
		name := strings.TrimSpace(info.Name)
		if key == "" || name == "" {
			continue
		}
		names[key] = name
	}
	return names
}
