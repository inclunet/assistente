import { useEffect, useMemo, useState } from 'react';
import { GetLLMProvidersWithStatus } from '@wailsjs/go/wailsapi/LLMProviders';
import { AGENT_API_FORMAT } from '../config/providers';

const DEFAULT_PROVIDER_SENTINEL = '$default';

export interface AgentProviderState {
  /** isAgent é `true` quando o provedor escolhido é um agente de código. */
  isAgent: boolean;
  /**
   * resolved diz que a lista de provedores já chegou (ou já falhou). Antes
   * disso `isAgent` é `false` por não se saber, e não por resposta.
   */
  resolved: boolean;
}

/**
 * useAgentProvider diz se o provedor escolhido é um agente de código. Quem
 * pergunta é o editor de perfil: com um agente, guias e campos inteiros deixam
 * de ter efeito e somem da tela (AEP-0084, Fase 8).
 *
 * A lista de provedores é consultada uma vez por montagem, e a resposta sai
 * dela: trocar de provedor no formulário muda `isAgent` no mesmo render, sem
 * janela em que a tela ainda esconde o que o provedor novo usa.
 *
 * Enquanto a lista não chega — e se ela não chegar — a resposta é "não é
 * agente", com `resolved` em `false` para quem precise separar as duas coisas.
 * A tela mostra o formulário completo nesse meio tempo, que é o caso da maioria
 * dos provedores; o contrário faria as guias sumirem e voltarem a cada abertura
 * do editor.
 *
 * O editor é um diálogo que só existe enquanto está aberto, então quem mexer no
 * provedor padrão ou no formato de um provedor em Configurações encontra a
 * lista nova na próxima vez que abrir um perfil.
 */
export function useAgentProvider(providerID: string): AgentProviderState {
  const [agents, setAgents] = useState<{ ids: Set<string>; defaultIsAgent: boolean } | null>(null);

  useEffect(() => {
    let current = true;
    void (async () => {
      let ids = new Set<string>();
      let defaultIsAgent = false;
      try {
        const providers = (await GetLLMProvidersWithStatus()) || [];
        for (const provider of providers as Record<string, unknown>[]) {
          if (provider.api_format !== AGENT_API_FORMAT) continue;
          if (typeof provider.id === 'string') ids.add(provider.id);
          if (provider.is_default === true) defaultIsAgent = true;
        }
      } catch {
        // Sem resposta, o editor segue completo: esconder guia por causa de
        // uma consulta que falhou tiraria da pessoa configuração que ela tem.
        ids = new Set<string>();
        defaultIsAgent = false;
      }
      if (current) setAgents({ ids, defaultIsAgent });
    })();
    return () => {
      current = false;
    };
  }, []);

  return useMemo(() => {
    if (!agents) return { isAgent: false, resolved: false };
    if (!providerID) return { isAgent: false, resolved: true };
    const isAgent =
      providerID === DEFAULT_PROVIDER_SENTINEL ? agents.defaultIsAgent : agents.ids.has(providerID);
    return { isAgent, resolved: true };
  }, [agents, providerID]);
}
