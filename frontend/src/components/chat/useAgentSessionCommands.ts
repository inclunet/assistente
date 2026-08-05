import { useEffect, useState } from 'react';
import { GetAgentSessionCommands } from '@wailsjs/go/app/App';
import { EventsOn } from '@wailsjs/runtime/runtime';
import { logger } from '../../utils/logger';

export interface AgentCommand {
  name: string;
  description?: string;
  acceptsInput: boolean;
}

export interface AgentSessionCommandsEvent {
  conversationId: string;
  commands: AgentCommand[];
}

/**
 * useAgentSessionCommands traz os comandos que o agente de código desta conversa
 * oferece (AEP-0084 D8), para o menu que abre ao digitar a barra.
 *
 * A conversa é a unidade, e não a aba: duas abas podem mostrar a mesma conversa,
 * e o agente do outro lado é o mesmo. O evento é filtrado por `conversationId`
 * pelo motivo inverso — sem o filtro, os comandos de uma conversa apareceriam no
 * menu de outra, que fala com outro agente.
 *
 * A lista é lida uma vez ao abrir porque o agente a anuncia quando a sessão
 * nasce, muito antes de alguém digitar a barra: quem chega depois disso não
 * ouviria anúncio nenhum.
 *
 * Nada aqui é anunciado ao leitor de telas. É uma lista que só aparece quando
 * alguém a pede, e falar dela sozinha atropelaria a leitura do que está em curso.
 */
export function useAgentSessionCommands(conversationId?: string | null): AgentCommand[] {
  const [commands, setCommands] = useState<AgentCommand[]>([]);

  useEffect(() => {
    if (!conversationId) {
      setCommands([]);
      return;
    }
    let current = true;
    setCommands([]);
    GetAgentSessionCommands(conversationId)
      .then((state) => {
        if (!current) return;
        setCommands(state?.commands ?? []);
      })
      .catch((error: unknown) => {
        if (!current) return;
        // Conversa que não fala com agente de código é o caso comum, não uma
        // falha: fica sem comandos em silêncio.
        setCommands([]);
        logger.warn('[AgentCommands] não foi possível ler os comandos da sessão:', error);
      });
    return () => {
      current = false;
    };
  }, [conversationId]);

  useEffect(() => {
    if (!conversationId) return;
    return EventsOn('chat:agent_commands', (event: AgentSessionCommandsEvent) => {
      if (!event || event.conversationId !== conversationId) return;
      // A lista do agente é o conjunto completo: a vazia tira do menu o que ele
      // deixou de oferecer.
      setCommands(event.commands ?? []);
    });
  }, [conversationId]);

  return commands;
}
