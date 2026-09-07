package apidto

// AgentWorkDir descreve onde o agente de código desta conversa trabalha
// (AEP-0084 D5 / AEP-0088). O diretório é o alcance do que ele pode ler e
// editar, e por isso fica visível na barra da conversa em vez de implícito.
type AgentWorkDir struct {
	ConversationID string `json:"conversationId"`
	// Available é falso quando não há diretório de agente a mostrar: conversa
	// que nunca falou com agente de código e nunca escolheu diretório. A tela
	// esconde o controle em vez de mostrar o caminho do workspace numa conversa
	// que não tem agente nenhum agindo sobre ele.
	Available bool `json:"available"`
	// Dir é o diretório que vale para o próximo turno.
	Dir string `json:"dir"`
	// WorkspaceDir é o diretório do app, que é o padrão de quem não escolheu.
	WorkspaceDir string `json:"workspaceDir"`
	// Pinned diz que esta conversa escolheu o diretório dela, e por isso não
	// acompanha mais a troca de workspace do app.
	Pinned bool `json:"pinned"`
	// SessionDir é o diretório da sessão que o agente tem de pé para esta
	// conversa, quando há uma. Diferente de Dir, ele conta o que ainda vale: a
	// escolha nova só chega ao agente quando a sessão for recriada.
	SessionDir string `json:"sessionDir,omitempty"`
}
