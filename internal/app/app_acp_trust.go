package app

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"assistente/internal/acptrust"
	"assistente/internal/profiles"
)

// AgentPermissionView é uma autorização permanente como a tela a mostra
// (AEP-0084 D9).
type AgentPermissionView struct {
	// ProfileSlug identifica o perfil e é o que volta na revogação.
	ProfileSlug string `json:"profileSlug"`
	// ProfileName é o nome que a pessoa deu ao perfil. Some quando o perfil
	// foi apagado com a autorização ainda guardada — e aí a tela mostra o
	// slug, que é o que sobrou dele.
	ProfileName string `json:"profileName,omitempty"`
	// Action é a classe da ação como o arquivo a guarda, e é ela que volta na
	// revogação. Vai como código: quem exibe traduz, e o que não estiver no
	// conjunto que a interface conhece vira a frase genérica em vez de aparecer
	// cru na tela.
	Action string `json:"action"`
	// GrantedAt é quando a pessoa autorizou, em RFC 3339.
	GrantedAt string `json:"grantedAt"`
}

// GetAgentPermissions lista o que os perfis autorizaram aos agentes de código
// sem perguntar de novo.
//
// Lista todos os perfis, e não só o ativo: uma autorização esquecida num perfil
// que não se usa há meses volta a valer no dia em que ele for aberto, e quem
// revoga precisa enxergá-la antes disso.
func (a *App) GetAgentPermissions() []AgentPermissionView {
	if a == nil || a.acpTrust == nil {
		return nil
	}
	names := a.profileNames()
	var out []AgentPermissionView
	for _, slug := range a.acpTrust.Profiles() {
		for _, entry := range a.acpTrust.List(slug) {
			out = append(out, AgentPermissionView{
				ProfileSlug: slug,
				ProfileName: names[slug],
				// A classe vai como está guardada, e não pelo conjunto que o
				// app conhece hoje: o que ele não reconhecesse viraria "other"
				// na tela e na revogação, que então não casaria com a entrada
				// do arquivo — a linha ficaria lá, impossível de tirar.
				Action:      strings.ToLower(strings.TrimSpace(entry.Kind)),
				GrantedAt:   entry.GrantedAt.Format(time.RFC3339),
			})
		}
	}
	// Ordem estável: a lista é lida em sequência por leitor de telas, e uma
	// ordem que muda a cada carregamento faria a pessoa procurar de novo o que
	// acabou de encontrar.
	sort.Slice(out, func(i, j int) bool {
		if out[i].ProfileSlug != out[j].ProfileSlug {
			return out[i].ProfileSlug < out[j].ProfileSlug
		}
		return out[i].Action < out[j].Action
	})
	return out
}

// RevokeAgentPermission tira a autorização permanente: o agente volta a
// perguntar antes de agir daquela classe naquele perfil.
func (a *App) RevokeAgentPermission(profileSlug, action string) error {
	if a == nil || a.acpTrust == nil {
		return errors.New("as autorizações do agente não estão disponíveis")
	}
	if err := a.acpTrust.Revoke(profileSlug, action); err != nil {
		if errors.Is(err, acptrust.ErrEntryNotFound) {
			// Dizer "revogado" sem ter revogado nada faria a pessoa acreditar
			// que fechou uma porta que continua aberta.
			return fmt.Errorf("essa autorização não existe mais")
		}
		return err
	}
	return nil
}

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
