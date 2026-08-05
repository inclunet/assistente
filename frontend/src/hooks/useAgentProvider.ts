import { useEffect, useState } from 'react';
import { GetLLMProvidersWithStatus } from '@wailsjs/go/app/App';
import { AGENT_API_FORMAT } from '../config/providers';

const DEFAULT_PROVIDER_SENTINEL = '$default';

/**
 * useAgentProvider diz se o provedor escolhido é um agente de código. Quem
 * pergunta é o editor de perfil: com um agente, guias e campos inteiros deixam
 * de ter efeito e ssomem da tela (AEP-0084, Fase 8).
 *
 * A resposta começa em `false` e só vira `true` quando a consulta volta. É de
 * propósito: enquanto não se sabe, a tela mostra o formulário completo, que é o
 * caso da maioria dos provedores — o contrário faria as guias sumirem e voltarem
 * a cada abertura do editor.
 */
export function useAgentProvider(providerID: string): boolean {
  const [isAgent, setIsAgent] = useState(false);

  useEffect(() => {
    if (!providerID) {
      setIsAgent(false);
      return;
    }
    let current = true;
    void (async () => {
      try {
        const providers = (await GetLLMProvidersWithStatus()) || [];
        const chosen = providers.find((provider: Record<string, unknown>) =>
          providerID === DEFAULT_PROVIDER_SENTINEL
            ? provider.is_default === true
            : provider.id === providerID,
        );
        if (current) setIsAgent(chosen?.api_format === AGENT_API_FORMAT);
      } catch {
        // Sem resposta, o editor segue completo: esconder guia por causa de
        // uma consulta que falhou tiraria da pessoa configuração que ela tem.
        if (current) setIsAgent(false);
      }
    })();
    return () => {
      current = false;
    };
  }, [providerID]);

  return isAgent;
}
