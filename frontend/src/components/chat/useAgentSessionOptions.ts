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
  kind?: string;
  currentBool?: boolean;
  values: AgentConfigValue[];
}

export interface AgentSessionOptionsEvent {
  conversationId: string;
  options: AgentConfigOption[];
  snapshot?: boolean;
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
  // Versão de qualquer mudança de estado; protege leituras completas antigas.
  const optionsVersionRef = useRef(0);
  // Versão apenas dos snapshots; um aviso parcial de outra categoria não torna
  // obsoleta a resposta da opção alterada pela pessoa.
  const snapshotVersionRef = useRef(0);
  const categoryVersionsRef = useRef(new Map<string, number>());

  useEffect(() => {
    announceRef.current = announce;
    tRef.current = t;
  }, [announce, t]);

  useEffect(() => {
    optionsRef.current = options;
  }, [options]);

  useEffect(() => {
    conversationRef.current = conversationId ?? '';
    optionsVersionRef.current += 1;
    snapshotVersionRef.current += 1;
    categoryVersionsRef.current.clear();
    // Trocar de conversa zera o "trocando": o pedido em voo é da conversa
    // anterior, e deixar a marca de pé travaria os seletores desta.
    setChanging(false);
    if (!conversationId) {
      setOptions([]);
      return;
    }
    let current = true;
    const requestVersion = optionsVersionRef.current;
    GetAgentSessionOptions(conversationId)
      .then((state) => {
        if (!current || optionsVersionRef.current !== requestVersion) return;
        const fresh = state?.options ?? [];
        // A resposta pode chegar fora de ordem: um evento do agente ou o
        // refetch de fim de turno podem ter trazido opções enquanto esta
        // leitura, mais lenta, ainda voltava. Escrever o vazio dela agora
        // apagaria seletores que já descrevem a sessão de verdade.
        if (fresh.length === 0 && optionsRef.current.length > 0) return;
        setOptions(fresh);
      })
      .catch((error: unknown) => {
        if (!current || optionsVersionRef.current !== requestVersion) return;
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
    const requestVersion = optionsVersionRef.current;
    GetAgentSessionOptions(conversationId)
      .then((state) => {
        if (!current || optionsVersionRef.current !== requestVersion) return;
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
      if (event.snapshot || fresh.length > 0) {
        snapshotVersionRef.current += 1;
      }
      optionsVersionRef.current += 1;
      if (!event.snapshot && fresh.length === 0) {
        if (event.model) bumpCategoryVersion(categoryVersionsRef.current, AGENT_OPTION_MODEL);
        if (event.mode) bumpCategoryVersion(categoryVersionsRef.current, AGENT_OPTION_MODE);
      }
      // config_option_update é um snapshot completo; avisos legados podem só
      // atualizar o modelo ou modo e manter a lista de opções já conhecida.
      const updated = event.snapshot || fresh.length > 0
        ? fresh
        : withAgentValues(optionsRef.current, event);
      const focusedOption = event.snapshot ? focusedOptionRemovedBySnapshot(updated) : null;
      optionsRef.current = updated;
      setOptions(updated);
      if (focusedOption) {
        requestAnimationFrame(() => restoreToolbarFocus(focusedOption.toolbar, focusedOption.id));
      }
      if (!event.announce) return;
      // A pessoa precisa saber com quem está falando: o agente troca de modelo
      // por conta própria quando bate num limite de uso, e descobrir isso pela
      // resposta estranha é pior do que ouvir a troca.
      if (event.modelChanged && event.model) {
        // Pelo rótulo da lista, e não pelo identificador do protocolo: falado,
        // `claude-sonnet-4-5-20250929` é ilegível. Esta troca é a que mais
        // depende do anúncio — a pessoa não a viu acontecer.
        announceRef.current(tRef.current('chat.agentOptions.modelChangedByAgent', {
          model: labelOfValue(optionByCategory(updated, AGENT_OPTION_MODEL), event.model),
        }));
      }
      if (event.modeChanged && event.mode) {
        // O agente manda o modo pelo valor do protocolo (`plan`, `ask`), e em
        // geral sem rótulo nenhum. Falar isso cru faria o leitor de telas ler
        // inglês no meio do português.
        announceRef.current(tRef.current('chat.agentOptions.modeChangedByAgent', {
          mode: labelOfValue(
            optionByCategory(updated, AGENT_OPTION_MODE),
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
    const requestVersion = snapshotVersionRef.current;
    const requestCategoryVersions = new Map(categoryVersionsRef.current);
    const startingOptions = [...optionsRef.current];
    const requestedOption = optionsRef.current.find((option) => option.id === optionId);
    const requestedCategory = requestedOption
      ? isCategory(requestedOption, AGENT_OPTION_MODEL)
        ? AGENT_OPTION_MODEL
        : isCategory(requestedOption, AGENT_OPTION_MODE) ? AGENT_OPTION_MODE : optionId
      : optionId;
    const requestCategoryVersion = categoryVersionsRef.current.get(requestedCategory) ?? 0;
    setChanging(true);
    try {
      const state = await SetAgentSessionOption(requested, optionId, value);
      const applied = state?.options ?? [];
      // A conversa da tela pode ter mudado enquanto o agente respondia. Escrever
      // agora poria o modelo de uma conversa no seletor de outra.
      if (conversationRef.current !== requested) return null;
      // Um evento pode confirmar a troca antes da resposta da chamada. O
      // backend suprime o anúncio do eco, então retornar null aqui deixaria a
      // troca bem-sucedida sem confirmação acessível. A releitura abaixo obtém
      // o estado efetivo quando o evento e a resposta divergem.
      const snapshotChanged = snapshotVersionRef.current !== requestVersion;
      const categoryChanged =
        (categoryVersionsRef.current.get(requestedCategory) ?? 0) !== requestCategoryVersion;
      if (snapshotChanged || categoryChanged) {
        const current = optionsRef.current.find((option) => option.id === optionId);
        const appliedOption = applied.find((option) => option.id === optionId);
        const valueOf = (option: AgentConfigOption | undefined): string | undefined => {
          if (!option) return undefined;
          if (option.kind === 'boolean') {
            // currentBool usa omitempty no DTO Go; false chega sem a chave e
            // corresponde ao mesmo estado desmarcado que a Checkbox renderiza.
            return String(!!option.currentBool);
          }
          return option.currentValue;
        };
        const currentValue = valueOf(current);
        const startingValue = valueOf(requestedOption);
        const appliedValue = valueOf(appliedOption);
        const currentMoved = currentValue !== undefined && currentValue !== startingValue;

        // Um snapshot completo pode retirar a opção enquanto a troca está em
        // voo. A resposta atrasada não pode ressuscitar um controle removido.
        if (snapshotChanged && !current) return null;

        if (currentValue === value && appliedOption && appliedValue !== value) {
          // O eco do agente pode confirmar a seleção, mas um resultado
          // acomodado da chamada pode contradizê-lo. Uma releitura após a
          // resposta resolve pelo estado atual da sessão antes do anúncio.
          const snapshotVersionBeforeRefresh = snapshotVersionRef.current;
          const categoryVersionsBeforeRefresh = new Map(categoryVersionsRef.current);
          let refreshed: Awaited<ReturnType<typeof GetAgentSessionOptions>>;
          try {
            refreshed = await GetAgentSessionOptions(requested);
          } catch (error: unknown) {
            logger.warn('[AgentOptions] não foi possível confirmar a troca após atualização concorrente:', error);
            announceRef.current(tRef.current('chat.agentOptions.changeUnknownState'));
            return null;
          }
          if (conversationRef.current !== requested) return null;
          if (snapshotVersionRef.current !== snapshotVersionBeforeRefresh) {
            return optionsRef.current.some((option) => option.id === optionId)
              ? optionsRef.current
              : null;
          }
          const fresh = refreshed?.options ?? [];
          if (fresh.length === 0) return null;
          // Mudanças de outras categorias durante a chamada continuam valendo.
          // Para a categoria pedida, a releitura substitui o evento anterior;
          // somente eventos ainda mais novos, durante a própria releitura,
          // prevalecem.
          const beforeRefresh = new Map(requestCategoryVersions);
          beforeRefresh.set(
            requestedCategory,
            categoryVersionsBeforeRefresh.get(requestedCategory) ?? 0,
          );
          const reconciled = preserveChangedCategories(
            fresh,
            optionsRef.current,
            beforeRefresh,
            categoryVersionsRef.current,
          );
          optionsVersionRef.current += 1;
          snapshotVersionRef.current += 1;
          optionsRef.current = reconciled;
          setOptions(reconciled);
          return reconciled.some((option) => option.id === optionId) ? reconciled : null;
        }

        // Um snapshot que mudou a própria opção é mais novo que uma resposta
        // atrasada que não confirma o valor pedido; mantém-se e anuncia-se o
        // estado que o agente publicou.
        if (snapshotChanged && currentMoved && currentValue !== value) {
          return current ? optionsRef.current : null;
        }
      }
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
      const responseOptions = snapshotChanged
        ? reconcileSnapshotResponse(applied, startingOptions, optionsRef.current)
        : applied;
      const reconciled = preserveChangedCategories(
        responseOptions,
        optionsRef.current,
        requestCategoryVersions,
        categoryVersionsRef.current,
      );
      optionsVersionRef.current += 1;
      snapshotVersionRef.current += 1;
      optionsRef.current = reconciled;
      setOptions(reconciled);
      return reconciled;
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

/** Mantém o foco dentro da barra quando um snapshot remove o controle focado. */
function focusedOptionRemovedBySnapshot(
  updated: AgentConfigOption[],
): { id: string; toolbar: HTMLElement } | null {
  if (typeof document === 'undefined') return null;
  const active = document.activeElement;
  if (!(active instanceof HTMLElement)) return null;
  const option = active.closest<HTMLElement>('[data-agent-option-id]');
  if (!option) return null;
  const id = option.dataset.agentOptionId;
  const toolbar = option.closest<HTMLElement>('[role="toolbar"]');
  return id && toolbar && !updated.some((candidate) => candidate.id === id) ? { id, toolbar } : null;
}

function restoreToolbarFocus(toolbar: HTMLElement, removedOptionId: string): void {
  if (!toolbar.isConnected) return;
  // O foco pode ter sido movido manualmente entre a atualização do snapshot e
  // este callback. Só repor quando a remoção deixou o documento sem foco útil.
  if (document.activeElement !== document.body && document.activeElement !== document.documentElement) return;
  const candidates = toolbar.querySelectorAll<HTMLElement>(
    'button:not(:disabled), input:not(:disabled), select:not(:disabled), [tabindex]:not([tabindex="-1"])',
  );
  const fallback = Array.from(candidates).find((candidate) => {
    const option = candidate.closest<HTMLElement>('[data-agent-option-id]');
    return option?.dataset.agentOptionId !== removedOptionId
      && candidate.getAttribute('aria-disabled') !== 'true'
      && !candidate.hasAttribute('disabled');
  });
  fallback?.focus();
}

/** Mantém mudanças e remoções de opções publicadas por snapshot após o pedido. */
function reconcileSnapshotResponse(
  response: AgentConfigOption[],
  before: AgentConfigOption[],
  latest: AgentConfigOption[],
): AgentConfigOption[] {
  const reconciled = response.filter((option) => {
    // Opção conhecida antes da chamada e ausente no snapshot foi removida; a
    // resposta atrasada não pode reinscrevê-la. IDs novos da resposta continuam
    // válidos, pois o agente pode oferecer controles adicionais ao acomodar a
    // alteração pedida.
    return !before.some((candidate) => candidate.id === option.id) ||
      latest.some((candidate) => candidate.id === option.id);
  });

  for (const current of latest) {
    const previous = before.find((option) => option.id === current.id);
    if (previous && sameOptionState(previous, current)) continue;
    const index = reconciled.findIndex((option) => option.id === current.id);
    if (index === -1) reconciled.push(current);
    else reconciled[index] = current;
  }
  return reconciled;
}

function sameOptionState(left: AgentConfigOption, right: AgentConfigOption): boolean {
  const leftValue = left.kind === 'boolean' ? String(!!left.currentBool) : left.currentValue;
  const rightValue = right.kind === 'boolean' ? String(!!right.currentBool) : right.currentValue;
  return left.name === right.name && left.category === right.category && left.kind === right.kind &&
    leftValue === rightValue && left.values.length === right.values.length &&
    left.values.every((value, index) => {
      const candidate = right.values[index];
      return candidate !== undefined && value.value === candidate.value && value.name === candidate.name;
    });
}

/** Mantém as opções conhecidas quando o agente só anuncia um valor corrente. */
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

function bumpCategoryVersion(versions: Map<string, number>, category: string): void {
  versions.set(category, (versions.get(category) ?? 0) + 1);
}

function preserveChangedCategories(
  response: AgentConfigOption[],
  latest: AgentConfigOption[],
  before: Map<string, number>,
  after: Map<string, number>,
): AgentConfigOption[] {
  const reconciled = [...response];
  for (const [category, version] of after) {
    if (version === (before.get(category) ?? 0)) continue;
    const current = latest.find((option) => isCategory(option, category));
    if (!current) continue;
    const index = reconciled.findIndex((option) => isCategory(option, category));
    if (index === -1) reconciled.push(current);
    else reconciled[index] = current;
  }
  return reconciled;
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
