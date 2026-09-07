package apidto

// AgentPermissionView é uma autorização permanente como a tela a mostra
// (AEP-0084 D9 / AEP-0088).
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
