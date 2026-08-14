package apidto

// AgentCommand é um comando que o agente de código oferece para a conversa — o
// que ele chama de slash command (AEP-0084 D8 / AEP-0088).
type AgentCommand struct {
	// Name é o que se digita depois da barra.
	Name string `json:"name"`
	// Description é a explicação curta que o agente deu, quando ele dá uma.
	Description string `json:"description,omitempty"`
	// AcceptsInput diz que o comando usa o texto escrito depois do nome. Quem
	// escolhe precisa saber se ainda falta escrever alguma coisa.
	AcceptsInput bool `json:"acceptsInput"`
}

// AgentSessionCommands é o que o agente desta conversa oferece agora (borda Wails).
type AgentSessionCommands struct {
	ConversationID string         `json:"conversationId"`
	Commands       []AgentCommand `json:"commands"`
}
