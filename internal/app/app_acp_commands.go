package app

import (
	"assistente/internal/acp"
	"assistente/internal/apidto"
)

// AgentSessionCommandsEvent é o payload de `chat:agent_commands`: o agente
// contou quais comandos existem nesta conversa.
//
// Ele manda isso assim que a sessão abre — antes de qualquer turno — e refaz
// quando a lista muda. Não é anunciado ao leitor de telas: é uma lista que
// aparece quando alguém digita a barra, e falar dela sem que ninguém tenha
// pedido atropelaria a leitura do que está em curso.
//
// GetAgentSessionCommands migrou para wailsapi.ACPCommands (AEP-0088); este
// handler lowercase permanece no App.
type AgentSessionCommandsEvent struct {
	ConversationID string                `json:"conversationId"`
	Commands       []apidto.AgentCommand `json:"commands"`
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

func agentCommandsFrom(commands []acp.Command) []apidto.AgentCommand {
	out := make([]apidto.AgentCommand, 0, len(commands))
	for _, command := range commands {
		out = append(out, apidto.AgentCommand{
			Name:         command.Name,
			Description:  command.Description,
			AcceptsInput: command.AcceptsInput,
		})
	}
	return out
}
