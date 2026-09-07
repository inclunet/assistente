import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { GetAgentSessionOptions, SetAgentSessionOption } from '@wailsjs/go/wailsapi/ACPOptions';
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
 * `sessionSettled` diz quando o turno corrente da conversa terminou. Enquanto
 * não terminou, a sessão do agente pode ainda nem existir, e perguntar cedo
 * demais devolve vazio; quando termina com os seletores vazios, pergunta de
 * novo — a sessão agora é de verdade, e a resposta traz o que ela oferece.
 *
 * Quem anuncia é o serviço global de anúncio, não uma região viva por aba
 * (`AGENTS.md`): o agente que troca de modelo sozinho precisa ser ouvido uma
 * vez, e não uma vez por superfície aberta.
 */
export function useAgentSessionOptions(
  conversationId?: string | null,
  sessionSettled?: boolean,
): UseAgentSessionOptionsResult {
  const { t } = useTranslation();
  const { announce } = useAnnouncer();
  const [options, setOptions] = useState<AgentConfigOption[]>([]);
  const [changing, setChanging] = useState(false);
  const announceRef = useRef(announce);
  const tRef = useRef(t);
  // conversationRef diz qual conversa está na tela agora. A troca de opção é uma
  // ida ao agente, e a pessoa pode mudar de conversa antes da volta.
  const conversationRef = useRef(conversationId ?? '');
  // optionsRef é o que os seletores mostram agora. O aviso do agente chega de
  // fora do render e precisa desta lista para achar o rótulo do valor novo
  // quando ele conta a troca sem repetir as opções.
  const optionsRef = useRef<AgentConfigOption[]>([]);

  useEffect(() => {
    announceRef.current = announce;
    tRef.current = t;
  }, [announce, t]);

  useEffect(() => {
    optionsRef.current = options;
  }, [options]);

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
        const fresh = state?.options ?? [];
        // A resposta pode chegar fora de ordem: um evento do agente ou o
        // refetch de fim de turno podem ter trazido opções enquanto esta
        // leitura, mais lenta, ainda voltava. Escrever o vazio dela agora
        // apagaria seletores que já descrevem a sessão de verdade.
        if (fresh.length === 0 && optionsRef.current.length > 0) return;
        setOptions(fresh);
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

  // Rede de segurança: o aviso de opções nasce com a sessão, mas pode chegar
  // antes de alguém escutar (superfície montada no meio do primeiro turno, por
  // exemplo). Terminado o turno com os seletores vazios, pergunta ao backend de
  // novo — agora a sessão existe e a resposta traz o que o agente oferece.
  // Enquanto seguem vazios, repete uma vez a cada fim de turno: é uma consulta
  // local barata, e o custo some no primeiro conjunto de opções que voltar.
  useEffect(() => {
    if (!conversationId || !sessionSettled) return;
    if (optionsRef.current.length > 0) return;
    let current = true;
    GetAgentSessionOptions(conversationId)
      .then((state) => {
        if (!current) return;
        const fresh = state?.options ?? [];
        if (fresh.length > 0) setOptions(fresh);
      })
      .catch(() => {
        // Sem barulho: o fetch inicial já tratou o erro do mesmo jeito.
      });
    return () => {
      current = false;
    };
  }, [conversationId, sessionSettled]);

  useEffect(() => {
    if (!conversationId) return;
    return EventsOn('chat:agent_options', (event: AgentSessionOptionsEvent) => {
      if (!event || event.conversationId !== conversationId) return;
      const fresh = event.options ?? [];
      // Aviso sem opção alguma não descreve seletor: o agente pode contar a
      // troca sem repetir a lista. Escrever o vazio faria os controles sumirem
      // no meio da conversa, então o que a tela já tem continua valendo — com o
      // valor novo anotado, que é o que a pessoa precisa ver.
      const options = fresh.length > 0 ? fresh : withAgentValues(optionsRef.current, event);
      setOptions(options);
      if (!event.announce) return;
      // A pessoa precisa saber com quem está falando: o agente troca de modelo
      // por conta própria quando bate num limite de uso, e descobrir isso pela
      // resposta estranha é pior do que ouvir a troca.
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
      if (applied.length === 0) {
        // Conjunto vazio não descreve seletor nenhum: escrevê-lo faria os
        // controles sumirem no meio da conversa. E como o agente não disse em
        // que estado ficou, não há troca a anunciar — sucesso anunciado com
        // controle desaparecendo é o estado que ninguém consegue explicar
        // depois. Acontece com um agente que responde a troca com opções sem
        // valores para escolher, que o backend descarta por não desenharem
        // seletor.
        logger.warn('[AgentOptions] o agente aceitou a troca sem dizer em que estado a sessão ficou');
        announceRef.current(tRef.current('chat.agentOptions.changeUnknownState'));
        return null;
      }
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

/**
 * isCategory diz se a opção é da categoria pedida. A comparação some com a caixa
 * porque a categoria vem do agente pelo fio; ela mora aqui para que todo mundo
 * neste arquivo decida isso igual — duas funções com critérios diferentes para a
 * mesma entrada é armadilha para quem mexer depois.
 */
function isCategory(option: AgentConfigOption, category: string): boolean {
  return (option.category ?? '').toLowerCase() === category;
}

/** optionByCategory acha a opção de uma categoria do protocolo. */
export function optionByCategory(
  options: AgentConfigOption[],
  category: string,
): AgentConfigOption | undefined {
  return options.find((option) => isCategory(option, category));
}

/**
 * withAgentValues anota na lista que a tela mostra o valor corrente que o agente
 * contou. Serve ao aviso que traz a troca sem trazer as opções: a lista de
 * valores continua sendo a que se conhecia, e só o valor corrente muda.
 */
function withAgentValues(
  options: AgentConfigOption[],
  event: AgentSessionOptionsEvent,
): AgentConfigOption[] {
  if (options.length === 0) return options;
  return options.map((option) => {
    const value = isCategory(option, AGENT_OPTION_MODEL)
      ? event.model
      : isCategory(option, AGENT_OPTION_MODE) ? event.mode : '';
    return value ? { ...option, currentValue: valueAsListed(option, value) } : option;
  });
}

/**
 * valueAsListed devolve o valor do aviso escrito como a lista o escreve.
 *
 * O valor do aviso chega aparado do backend, e o da lista chega como o agente
 * escreveu. Gravar o aparado como valor corrente faria o seletor comparar coisas
 * diferentes: ele acha o item por igualdade exata, e sem achar mostra o
 * identificador cru no lugar do rótulo e deixa a lista inteira sem item marcado
 * — o leitor de telas deixaria de dizer qual é o modelo de agora.
 *
 * Sem correspondência, o valor do aviso é melhor do que nada: é o que o agente
 * disse que vale, ainda que não esteja na lista que ele ofereceu antes.
 */
function valueAsListed(option: AgentConfigOption, value: string): string {
  const wanted = value.trim();
  const item = option.values.find((candidate) => candidate.value.trim() === wanted);
  return item ? item.value : value;
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
  // Aparado dos dois lados: o valor que vem no aviso do agente chega aparado do
  // backend, e o da lista chega como o agente escreveu. Comparando cru, um
  // espaço sobrando na lista faria o anúncio sair pelo valor de protocolo mesmo
  // havendo rótulo escrito na tela.
  const wanted = value.trim();
  const item = option?.values.find((candidate) => candidate.value.trim() === wanted);
  return item?.name || translateValue?.(wanted) || wanted;
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
