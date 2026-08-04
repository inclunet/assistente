package acp

// SessionCommandsEvent conta quais comandos o agente oferece na sessão de uma
// conversa (AEP-0084 D8).
//
// O agente manda a lista assim que a sessão abre, fora de qualquer turno, e
// refaz quando ela muda. Quem digita a barra no campo de mensagem precisa ver o
// que existe agora, e não o que existia quando a conversa começou.
type SessionCommandsEvent struct {
	ConversationID string
	ProviderID     string

	// Commands é o conjunto completo. Vazio diz que o agente não oferece
	// comando nenhum, e é assim que os anteriores saem da tela.
	Commands []Command
}

// sessionCommandsChanged traduz o aviso do transporte num evento de conversa,
// como o das opções: o transporte só conhece o nome da sessão.
func (m *Manager) sessionCommandsChanged(sessionID string, commands []Command) {
	if m.onCommands == nil {
		return
	}
	m.knownMu.Lock()
	known, ok := m.known[sessionID]
	m.knownMu.Unlock()
	if !ok {
		// Sessão que não é de conversa nenhuma: a de descoberta, ou uma que a
		// conversa já soltou. Não há a quem contar.
		return
	}
	m.onCommands(SessionCommandsEvent{
		ConversationID: known.conversationID,
		ProviderID:     known.providerID,
		Commands:       commands,
	})
}

// ConversationCommands são os comandos da sessão desta conversa, procurada pelo
// identificador. Conversa sem sessão de pé não tem comando nenhum, e nada é
// aberto aqui de propósito: abrir o menu de comandos não pode fazer nascer um
// processo de agente.
func (m *Manager) ConversationCommands(conversationID string) []Command {
	conv := m.lookup(conversationID)
	if conv == nil {
		return nil
	}
	return conv.Commands()
}

// Commands são os comandos que o agente oferece na sessão desta conversa.
func (c *Conversation) Commands() []Command {
	session := c.Session()
	if session == nil {
		return nil
	}
	return session.Commands()
}
