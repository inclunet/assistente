import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { GetAgentSessionOptions, SetAgentSessionOption } from '@wailsjs/go/app/App';
import { EventsOn } from '@wailsjs/runtime/runtime';
import { useAnnouncer } from '../../hooks/useAnnouncer';
import { logger } from '../../utils/logger';

/** Categorias que o protocolo define e esta tela sabe desenhar (AEP-0084 D6). */
export const AGENT_OPTION_MODEL = 'model';
export const AGENT_OPTION_MODE = 'mode';

export interface AgentConfigValue {
  value: string;
  name?: string;
}

export interface AgentConfigOption {
  id: string;
  name?: string;
  category?: string;
  currentValue: string;
  values: AgentConfigValue[];
}

export interface AgentSessionOptionsEvent {
  conversationId: string;
  options: AgentConfigOption[];
  model?: string;
  mode?: string;
  modelChanged: boolean;
  modeChanged: boolean;
  announce: boolean;
}

export interface UseAgentSessionOptionsResult {
  /** Opções que o agente desta conversa oferece. Vazio esconde os seletores. */
  options: AgentConfigOption[];
  /** Verdadeiro enquanto uma troca está indo ao agente. */
  changing: boolean;
  /**
   * Troca uma opção e devolve o estado que o agente confirmou, ou nulo quando a
   * troca não valeu. É o estado dele que volta, e não o valor pedido: o agente
   * pode acomodar o pedido em outro valor, e quem anuncia precisa dizer o que
   * valeu de verdade.
   */
  change: (optionId: string, value: string) => Promise<AgentConfigOption[] | null>;
}

/**
 * useAgentSessionOptions liga a barra de ferramentas ao modelo e ao modo do
 * agente desta conversa.
 *
 * A conversa é a unidade, e não a aba: duas abas podem mostrar a mesma conversa,
 * e o agente do outro lado é o mesmo. Por isso o evento é filtrado por
 * `conversationId` — sem esse filtro, o modelo trocado numa conversa apareceria
 * na outra.
 *
 * Quem anuncia é o serviço global de anúncio, não uma região viva por aba
 * (`AGENTS.md`): o agente que troca de modelo sozinho precisa ser ouvido uma
 * vez, e não uma vez por superfície aberta.
 */
export function useAgentSessionOptions(conversationId?: string | null): UseAgentSessionOptionsResult {
  const { t } = useTranslation();
  const { announce } = useAnnouncer();
  const [options, setOptions] = useState<AgentConfigOption[]>([]);
  const [changing, setChanging] = useState(false);
  const announceRef = useRef(announce);
  const tRef = useRef(t);
  // conversationRef diz qual conversa está na tela agora. A troca de opção é uma
  // ida ao agente, e a pessoa pode mudar de conversa antes da volta.
  const conversationRef = useRef(conversationId ?? '');

  useEffect(() => {
    announceRef.current = announce;
    tRef.current = t;
  }, [announce, t]);

  useEffect(() => {
    conversationRef.current = conversationId ?? '';
    // Trocar de conversa zera o "trocando": o pedido em voo é da conversa
    // anterior, e deixar a marca de pé travaria os seletores desta.
    setChanging(false);
    if (!conversationId) {
      setOptions([]);
      return;
    }
    let current = true;
    GetAgentSessionOptions(conversationId)
      .then((state) => {
        if (!current) return;
        setOptions(state?.options ?? []);
      })
      .catch((error: unknown) => {
        if (!current) return;
        // Conversa que não é de agente é o caso comum, não uma falha: some com
        // os seletores em silêncio em vez de gritar num chat que nunca teve
        // modelo para escolher.
        setOptions([]);
        logger.warn('[AgentOptions] não foi possível ler as opções da sessão:', error);
      });
    return () => {
      current = false;
    };
  }, [conversationId]);

  useEffect(() => {
    if (!conversationId) return;
    return EventsOn('chat:agent_options', (event: AgentSessionOptionsEvent) => {
      if (!event || event.conversationId !== conversationId) return;
      setOptions(event.options ?? []);
      if (!event.announce) return;
      // A pessoa precisa saber com quem está falando: o agente troca de modelo
      // por conta própria quando bate num limite de uso, e descobrir isso pela
      // resposta estranha é pior do que ouvir a troca.
      const options = event.options ?? [];
      if (event.modelChanged && event.model) {
        // Pelo rótulo da lista, e não pelo identificador do protocolo: falado,
        // `claude-sonnet-4-5-20250929` é ilegível. Esta troca é a que mais
        // depende do anúncio — a pessoa não a viu acontecer.
        announceRef.current(tRef.current('chat.agentOptions.modelChangedByAgent', {
          model: labelOfValue(optionByCategory(options, AGENT_OPTION_MODEL), event.model),
        }));
      }
      if (event.modeChanged && event.mode) {
        // O agente manda o modo pelo valor do protocolo (`plan`, `ask`), e em
        // geral sem rótulo nenhum. Falar isso cru faria o leitor de telas ler
        // inglês no meio do português.
        announceRef.current(tRef.current('chat.agentOptions.modeChangedByAgent', {
          mode: labelOfValue(
            optionByCategory(options, AGENT_OPTION_MODE),
            event.mode,
            (value) => agentModeLabel(tRef.current, value),
          ),
        }));
      }
    });
  }, [conversationId]);

  const change = useCallback(async (
    optionId: string,
    value: string,
  ): Promise<AgentConfigOption[] | null> => {
    if (!conversationId) return null;
    const requested = conversationId;
    setChanging(true);
    try {
      const state = await SetAgentSessionOption(requested, optionId, value);
      const applied = state?.options ?? [];
      // A conversa da tela pode ter mudado enquanto o agente respondia. Escrever
      // agora poria o modelo de uma conversa no seletor de outra.
      if (conversationRef.current !== requested) return null;
      setOptions(applied);
      return applied;
    } catch (error: unknown) {
      logger.error('[AgentOptions] falha ao trocar a opção do agente:', error);
      if (conversationRef.current !== requested) return null;
      // Anunciar a falha é obrigatório: sem isso o seletor volta ao valor antigo
      // sem explicação, e a pessoa acharia que errou o clique. Só que anunciar a
      // falha de uma conversa que a pessoa já deixou seria ruído.
      announceRef.current(tRef.current('chat.agentOptions.changeError'));
      return null;
    } finally {
      if (conversationRef.current === requested) setChanging(false);
    }
  }, [conversationId]);

  return useMemo(() => ({ options, changing, change }), [options, changing, change]);
}

/** optionByCategory acha a opção de uma categoria do protocolo. */
export function optionByCategory(
  options: AgentConfigOption[],
  category: string,
): AgentConfigOption | undefined {
  return options.find((option) => (option.category ?? '').toLowerCase() === category);
}

/**
 * labelOfValue é o texto de um valor da opção: o rótulo que o agente mandou;
 * quando ele não manda nenhum — o modo não traz —, a tradução de quem exibe; e o
 * último recurso é o valor cru.
 *
 * Mora aqui, e é usado tanto pelos itens dos seletores quanto pelos anúncios,
 * porque o que a pessoa ouve tem de ser o que está escrito na lista. A opção
 * pode faltar: um aviso do agente sobre uma opção que ele acabou de retirar
 * ainda precisa ser dito, e aí o valor cru é melhor do que o silêncio.
 */
export function labelOfValue(
  option: AgentConfigOption | undefined,
  value: string,
  translateValue?: (value: string) => string,
): string {
  const item = option?.values.find((candidate) => candidate.value === value);
  return item?.name || translateValue?.(value) || value;
}

/** Rótulos dos modos que o protocolo enumera. O agente manda só o valor. */
const MODE_LABEL_KEYS: Record<string, string> = {
  agent: 'chat.agentOptions.mode.agent',
  plan: 'chat.agentOptions.mode.plan',
  ask: 'chat.agentOptions.mode.ask',
};

/**
 * agentModeLabel traduz os modos do protocolo. Um modo que este app ainda não
 * conhece sai pelo próprio valor — melhor o valor cru do que nada dito.
 */
export function agentModeLabel(
  t: (key: string, options?: Record<string, unknown>) => string,
  value: string,
): string {
  const key = MODE_LABEL_KEYS[value.trim().toLowerCase()];
  return key ? t(key) : value;
}
