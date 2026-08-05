package app

import (
	"errors"
	"strings"

	"assistente/internal/acp"
)

// AgentCommand é um comando que o agente de código oferece para a conversa — o
// que ele chama de slash command (AEP-0084 D8).
type AgentCommand struct {
	// Name é o que se digita depois da barra.
	Name string `json:"name"`
	// Description é a explicação curta que o agente deu, quando ele dá uma.
	Description string `json:"description,omitempty"`
	// AcceptsInput diz que o comando usa o texto escrito depois do nome. Quem
	// escolhe precisa saber se ainda falta escrever alguma coisa.
	AcceptsInput bool `json:"acceptsInput"`
}

// AgentSessionCommands é o que o agente desta conversa oferece agora.
type AgentSessionCommands struct {
	ConversationID string         `json:"conversationId"`
	Commands       []AgentCommand `json:"commands"`
}

// AgentSessionCommandsEvent é o payload de `chat:agent_commands`: o agente
// contou quais comandos existem nesta conversa.
//
// Ele manda isso assim que a sessão abre — antes de qualquer turno — e refaz
// quando a lista muda. Não é anunciado ao leitor de telas: é uma lista que
// aparece quando alguém digita a barra, e falar dela sem que ninguém tenha
// pedido atropelaria a leitura do que está em curso.
type AgentSessionCommandsEvent struct {
	ConversationID string         `json:"conversationId"`
	Commands       []AgentCommand `json:"commands"`
}

// GetAgentSessionCommands devolve os comandos que o agente desta conversa
// oferece. Como o seletor de modelo, de propósito não sobe processo nem abre
// sessão: abrir o menu de comandos não pode fazer nascer um agente de código.
func (a *App) GetAgentSessionCommands(conversationID string) (AgentSessionCommands, error) {
	conversationID = strings.TrimSpace(conversationID)
	out := AgentSessionCommands{ConversationID: conversationID, Commands: []AgentCommand{}}
	if _, err := a.requireAuthenticatedContext(); err != nil {
		return out, err
	}
	if conversationID == "" {
		return out, errors.New("conversa sem identificador")
	}
	if a.acpMgr == nil {
		return out, nil
	}
	out.Commands = agentCommandsFrom(a.acpMgr.ConversationCommands(conversationID))
	return out, nil
}

// agentSessionCommandsChanged leva ao frontend a lista que o agente contou. Roda
// na goroutine de entrega do transporte: só monta o payload e emite.
func (a *App) agentSessionCommandsChanged(event acp.SessionCommandsEvent) {
	if a == nil || a.emitter == nil {
		return
	}
	// A lista vazia é emitida de propósito, ao contrário do que acontece com as
	// opções: ela diz que o agente deixou de oferecer comandos, e calar isso
	// deixaria no menu comandos que já não existem.
	a.emitter.Emit("chat:agent_commands", AgentSessionCommandsEvent{
		ConversationID: event.ConversationID,
		Commands:       agentCommandsFrom(event.Commands),
	})
}

func agentCommandsFrom(commands []acp.Command) []AgentCommand {
	out := make([]AgentCommand, 0, len(commands))
	for _, command := range commands {
		out = append(out, AgentCommand{
			Name:         command.Name,
			Description:  command.Description,
			AcceptsInput: command.AcceptsInput,
		})
	}
	return out
}
