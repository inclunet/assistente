package apidto

// AgentConfigValue é um valor que o agente oferece para uma opção.
type AgentConfigValue struct {
	Value string `json:"value"`
	// Name é o rótulo do agente. Pode vir vazio — o modo no formato anterior não
	// traz um —, e aí quem exibe cai no próprio valor.
	Name string `json:"name,omitempty"`
}

// AgentConfigOption é uma escolha que o agente expõe para a sessão: o modelo, o
// modo (AEP-0084 D6).
type AgentConfigOption struct {
	ID   string `json:"id"`
	Name string `json:"name,omitempty"`
	// Category diz o que a opção significa (`model`, `mode`). É por ela que a
	// tela sabe que seletor está desenhando: o identificador é escolha do agente
	// e varia entre implementações.
	Category     string             `json:"category,omitempty"`
	CurrentValue string             `json:"currentValue"`
	Values       []AgentConfigValue `json:"values"`
}

// AgentSessionOptions é o que a conversa pode escolher no agente agora (borda Wails).
type AgentSessionOptions struct {
	ConversationID string `json:"conversationId"`
	// Available é falso quando não há o que escolher: conversa que não é de
	// agente, sessão que ainda não nasceu, agente que decide o próprio modelo. A
	// tela esconde o seletor em vez de mostrar um controle vazio.
	Available bool                `json:"available"`
	Options   []AgentConfigOption `json:"options"`
}
