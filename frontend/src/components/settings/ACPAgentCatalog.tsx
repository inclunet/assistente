import { useCallback, useEffect, useId, useMemo, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import type { TFunction } from 'i18next';
import { GetACPCatalog, RefreshACPCatalog } from '@wailsjs/go/app/App';
import { Button, Input } from '../';
import { useAnnouncer } from '../../hooks/useAnnouncer';
import { useDebouncedValue } from '../../hooks/useDebouncedValue';
import { formatRelativeTimeLocalized } from '../../lib/dateUtils';
import { playBumpSound } from '../../services/audioFeedback';
import './ACPAgentCatalog.css';

/**
 * Uma linha do catálogo, já resolvida pelo backend.
 *
 * O formato é descrito aqui em vez de vir dos tipos gerados do Wails porque
 * `app.ACPCatalog` é classe com método: um objeto literal de teste não satisfaz
 * o tipo, e o teste passaria a existir para agradar o compilador.
 *
 * Opcional aqui é exatamente o que é opcional no backend: os campos que o Go
 * marca com `omitempty` de fato podem faltar, e os outros sempre chegam. Marcar
 * `runtime_found` como opcional deixaria `undefined` se passar por "runtime
 * ausente" sem ninguém notar que o campo não veio.
 */
export interface CatalogAgent {
  id: string;
  name: string;
  version?: string;
  description?: string;
  authors?: string[];
  license?: string;
  website?: string;
  repository?: string;
  distributions: string[];
  runtime?: string;
  runtime_found: boolean;
  runtime_path?: string;
  integrity: string;
  state: string;
  state_detail?: string;
  detected_version?: string;
  installed_by_app?: boolean;
  installed_version?: string;
  installed_unverified?: boolean;
}

/** O catálogo inteiro, com o que se sabe sobre a própria coleta (D2). */
export interface Catalog {
  version?: string;
  agents: CatalogAgent[];
  fetched_at?: string;
  age_seconds: number;
  from_cache: boolean;
  stale: boolean;
  reason_code?: string;
  reason_detail?: string;
  platform?: string;
}

/**
 * Nome do runtime como as pessoas o chamam: o backend manda o identificador
 * (`node`), e "Node.js" é o que está escrito no site de quem o distribui.
 */
const runtimeName = (t: TFunction, runtime: string): string =>
  t(`acpCatalog.runtime.name.${runtime}`, runtime);

/** Nome de cada forma de distribuição, para o item dizer como o agente chega. */
const distributionName = (t: TFunction, kind: string): string =>
  t(`acpCatalog.distribution.${kind}`, kind);

/**
 * O pré-requisito de runtime em texto (AEP-0086 D7). Ele aparece sempre, e não
 * só quando falta: saber que o agente não exige nada é informação, e saber onde
 * o app achou o Node é o que resolve o caso de quem tem duas versões na máquina.
 */
export const runtimeText = (t: TFunction, agent: CatalogAgent): string => {
  if (!agent.runtime) return t('acpCatalog.runtime.none');
  const runtime = runtimeName(t, agent.runtime);
  if (!agent.runtime_found) return t('acpCatalog.runtime.missing', { runtime });
  return agent.runtime_path
    ? t('acpCatalog.runtime.foundAt', { runtime, path: agent.runtime_path })
    : t('acpCatalog.runtime.found', { runtime });
};

/**
 * O estado do agente nesta máquina em texto — nunca só em cor.
 *
 * Os seis estados existem porque três não cobrem o que o app sabe: ele tem
 * detecção escrita à mão para dois agentes do catálogo (D1), e dizer "não
 * encontrado" para os outros alegaria uma procura que não aconteceu.
 */
export const stateText = (t: TFunction, agent: CatalogAgent): string => {
  switch (agent.state) {
    case 'installed':
      // Quem instalou muda a frase porque muda o que se pode fazer com o
      // agente: o que o app pôs ali, ele sabe onde está e sabe remover; o que
      // veio de fora, ele apenas reconheceu.
      if (agent.installed_by_app) {
        return agent.installed_version
          ? t('acpCatalog.state.installedByAppVersion', { version: agent.installed_version })
          : t('acpCatalog.state.installedByApp');
      }
      return agent.detected_version
        ? t('acpCatalog.state.installedVersion', { version: agent.detected_version })
        : t('acpCatalog.state.installed');
    case 'requirement_missing':
      return t('acpCatalog.state.requirementMissing', {
        runtime: runtimeName(t, agent.runtime || ''),
      });
    case 'no_platform_target':
      return t('acpCatalog.state.noPlatformTarget');
    case 'not_installed':
      return t('acpCatalog.state.notInstalled');
    case 'no_detection':
      return t('acpCatalog.state.noDetection');
    case 'detection_failed':
      return t('acpCatalog.state.detectionFailed');
    default:
      // Estado que este frontend não conhece: dizer o identificador cru é
      // melhor do que inventar uma frase que pode descrever outra coisa.
      return t('acpCatalog.state.unknown', { state: agent.state });
  }
};

/**
 * O que se sabe sobre conferir o artefato binário nesta plataforma (D4).
 *
 * Agente que não é distribuído como binário não rende frase: não há digest de
 * que falar, e uma linha dizendo isso seria ruído nos 21 agentes de pacote.
 */
export const integrityText = (t: TFunction, agent: CatalogAgent): string => {
  switch (agent.integrity) {
    case 'digest':
      return t('acpCatalog.integrity.digest');
    case 'no_digest':
      return t('acpCatalog.integrity.noDigest');
    case 'no_platform_target':
      return t('acpCatalog.integrity.noPlatformTarget');
    default:
      return '';
  }
};

/**
 * O que a instalação que existe aqui vale (D4).
 *
 * É outra frase que a da integridade, e as duas convivem: aquela fala do que o
 * registro publica hoje, e esta fala do arquivo que já está no disco. Um agente
 * pode ter passado a publicar soma de verificação depois de alguém instalar a
 * versão que não tinha nenhuma, e é a instalação que continua sem conferência.
 */
export const installedIntegrityText = (t: TFunction, agent: CatalogAgent): string =>
  agent.installed_unverified ? t('acpCatalog.state.installedUnverified') : '';

/**
 * A frase inteira do item, que é o que um leitor de telas lê ao chegar nele.
 *
 * Ela existe porque o critério da Fase 2 é que cada item seja lido inteiro: sem
 * um nome acessível montado, quem navega ouviria o nome do agente e teria de
 * percorrer o item campo por campo para saber se dá para usá-lo aqui. É o mesmo
 * texto que está na tela, pela mesma razão que o resultado do teste de agente já
 * segue: um texto só para o anúncio divergiria do que está escrito.
 */
export const catalogItemLabel = (t: TFunction, agent: CatalogAgent): string => {
  const partes = [
    agent.version ? t('acpCatalog.item.nameVersion', { name: agent.name, version: agent.version }) : agent.name,
    agent.description,
    stateText(t, agent),
    installedIntegrityText(t, agent),
    runtimeText(t, agent),
    agent.distributions?.length
      ? t('acpCatalog.item.distributions', {
          list: agent.distributions.map((kind) => distributionName(t, kind)).join(', '),
        })
      : '',
    integrityText(t, agent),
    agent.authors?.length ? t('acpCatalog.item.authors', { list: agent.authors.join(', ') }) : '',
    agent.license ? t('acpCatalog.item.license', { license: agent.license }) : '',
  ];
  return partes.filter(Boolean).join('. ');
};

/**
 * Filtra pelo que está escrito no item. A busca cobre identificador, nome,
 * descrição, autores e licença porque é por qualquer um deles que se procura um
 * agente num catálogo de 38 — e o identificador entra porque é ele que aparece
 * na documentação de quem publica o agente.
 */
export const matchesSearch = (agent: CatalogAgent, term: string): boolean => {
  const busca = term.trim().toLowerCase();
  if (busca === '') return true;
  const campos = [
    agent.name,
    agent.id,
    agent.description ?? '',
    agent.license ?? '',
    ...(agent.authors ?? []),
  ];
  return campos.some((campo) => campo.toLowerCase().includes(busca));
};

/** A frase que explica por que o catálogo não pôde ser atualizado (D2). */
export const reasonText = (t: TFunction, catalog: Catalog): string => {
  switch (catalog.reason_code) {
    case 'unsupported_version':
      return t('acpCatalog.reason.unsupportedVersion');
    case 'malformed_index':
      return t('acpCatalog.reason.malformedIndex');
    case 'canceled':
      return t('acpCatalog.reason.canceled');
    case 'timeout':
      return t('acpCatalog.reason.timeout');
    case 'bad_status':
      return catalog.reason_detail
        ? t('acpCatalog.reason.badStatusDetail', { detail: catalog.reason_detail })
        : t('acpCatalog.reason.badStatus');
    case 'unreachable':
      return catalog.reason_detail
        ? t('acpCatalog.reason.unreachableDetail', { detail: catalog.reason_detail })
        : t('acpCatalog.reason.unreachable');
    case undefined:
    case '':
      return '';
    default:
      return t('acpCatalog.reason.unknown');
  }
};

/**
 * Diz se a linha do catálogo deve oferecer "Instalar".
 *
 * Quem já foi instalado pelo app troca o botão por gerenciar no fluxo de
 * instalação; sem alvo de plataforma ou sem runtime, o plano recusaria o
 * download — o botão não aparece, e o estado em texto já explica o motivo.
 */
export function catalogOffersInstall(agent: CatalogAgent): boolean {
  if (agent.installed_by_app) return false;
  if (agent.state === 'requirement_missing' || agent.state === 'no_platform_target') {
    return false;
  }
  return true;
}

export interface ACPAgentCatalogProps {
  /**
   * Escolher um agente da lista, em vez de só lê-la. É por aqui que o
   * formulário do provedor descobre qual agente ele está configurando
   * (AEP-0086 D11) — o seletor de tipo tem uma entrada só para os 38, e a lista
   * deles, com busca e estado por máquina, é esta.
   */
  onSelect?: (agent: CatalogAgent) => void;

  /** O agente já escolhido, para a lista dizer qual é sem depender de cor. */
  selectedId?: string;

  /**
   * Abrir o formulário de um provedor novo já apontando para este agente.
   * Só no modo browse da tela de provedores — no picker (`onSelect`) a escolha
   * já preenche o formulário que está aberto.
   */
  onUseAgent?: (agent: CatalogAgent) => void;

  /**
   * Abrir a instalação deste agente sem criar o provedor ainda. Reusa o mesmo
   * `AgentInstall` do formulário; o catálogo só dispara.
   */
  onInstallAgent?: (agent: CatalogAgent) => void;

  /**
   * Quando muda, o catálogo recarrega a cópia em cache (ex.: depois de instalar
   * pelo modal da tela de provedores). Zero/undefined não dispara nada além da
   * carga inicial.
   */
  refreshNonce?: number;
}

/**
 * O catálogo de agentes do registro oficial do ACP (AEP-0086).
 *
 * Três modos, uma lista:
 * - Com `onSelect`: caixa de listagem do formulário do provedor.
 * - Com `onUseAgent` / `onInstallAgent`: browse acionável na tela de provedores
 *   (instalar ou criar o provedor a partir da linha).
 * - Sem nenhum dos dois: só leitura (legado / testes).
 *
 * As três formas compartilham busca, estado e textos de propósito — quem age
 * precisa das mesmas informações de quem só lia.
 */
export const ACPAgentCatalog = ({
  onSelect,
  selectedId,
  onUseAgent,
  onInstallAgent,
  refreshNonce = 0,
}: ACPAgentCatalogProps = {}) => {
  const { t, i18n } = useTranslation();
  const { announce } = useAnnouncer();
  const navHelpId = `${useId()}-nav-help`;

  const [catalog, setCatalog] = useState<Catalog | null>(null);
  const [loading, setLoading] = useState(true);
  const [refreshing, setRefreshing] = useState(false);
  // `null` é "sem erro"; string vazia é "falhou sem dizer o quê". Separar os dois
  // deixa a tradução da frase genérica no render, e é isso que mantém `load` fora
  // da dependência de `t` — que muda de identidade a cada troca de idioma e faria
  // a carga recomeçar sozinha.
  const [loadError, setLoadError] = useState<string | null>(null);
  const [searchTerm, setSearchTerm] = useState('');
  const [activeIndex, setActiveIndex] = useState(0);

  const actionable = !onSelect && (!!onUseAgent || !!onInstallAgent);

  const itemRefs = useRef<Array<HTMLLIElement | null>>([]);
  const useBtnRefs = useRef<Array<HTMLButtonElement | null>>([]);
  // A resposta de uma carga que já não é a última não fala pela tela, e a de um
  // componente desmontado não fala por ninguém: o modal fecha, e um `setState`
  // depois disso descreveria uma tela que saiu do ar.
  const loadSeq = useRef(0);
  const mountedRef = useRef(true);
  useEffect(() => {
    mountedRef.current = true;
    return () => {
      mountedRef.current = false;
    };
  }, []);

  const load = useCallback(async (mode: 'open' | 'refresh') => {
    const seq = ++loadSeq.current;
    const obsoleta = () => seq !== loadSeq.current || !mountedRef.current;
    if (mode === 'refresh') setRefreshing(true);
    else setLoading(true);
    setLoadError(null);
    try {
      const result = mode === 'refresh' ? await RefreshACPCatalog() : await GetACPCatalog();
      if (obsoleta()) return;
      setCatalog(result);
    } catch (error: unknown) {
      if (obsoleta()) return;
      const err = error as { message?: unknown } | null;
      setCatalog(null);
      setLoadError(String(err?.message ?? error ?? ''));
    } finally {
      if (!obsoleta()) {
        setLoading(false);
        setRefreshing(false);
      }
    }
  }, []);

  useEffect(() => {
    void load('open');
  }, [load]);

  // Recarrega depois de uma instalação feita fora deste componente (modal da
  // tela de provedores), sem remontar a lista e perder busca/foco.
  useEffect(() => {
    if (!refreshNonce) return;
    void load('refresh');
  }, [load, refreshNonce]);

  const erroTexto = loadError === null ? '' : loadError || t('acpCatalog.error.loadFailed');

  // A falha vai para o announcer global em vez de uma live region local: é a mesma
  // arbitragem que o resto do app usa, e duas regiões vivas competiriam.
  useEffect(() => {
    if (loadError === null) return;
    announce(erroTexto, 'assertive');
  }, [announce, erroTexto, loadError]);

  const rows = useMemo(
    () => (catalog?.agents ?? []).filter((agent) => matchesSearch(agent, searchTerm)),
    [catalog, searchTerm]
  );

  // O índice ativo é sempre uma linha que existe: filtrar encurta a lista, e um
  // índice preso ao tamanho anterior deixaria o roving tabindex sem parada.
  const safeIndex = rows.length === 0 ? 0 : Math.min(activeIndex, rows.length - 1);

  // O número de resultados é anunciado depois de a digitação parar. A cada tecla
  // ele atropelaria a própria leitura do campo de busca — a mesma disciplina de
  // arbitragem que vale para o anúncio de progresso.
  const termoEstavel = useDebouncedValue(searchTerm, 500);
  const anunciadoRef = useRef<string | null>(null);
  useEffect(() => {
    if (loading || !catalog) return;
    const termo = termoEstavel.trim();
    if (termo === '') {
      anunciadoRef.current = null;
      return;
    }
    if (anunciadoRef.current === termo) return;
    anunciadoRef.current = termo;
    announce(t('acpCatalog.search.results', { total: rows.length }), 'polite');
  }, [announce, catalog, loading, rows.length, t, termoEstavel]);

  const handleSearchChange = (value: string) => {
    setSearchTerm(value);
    // A lista muda de conteúdo: continuar na posição de antes apontaria para
    // outro agente sem ninguém ter navegado.
    setActiveIndex(0);
  };

  const focusRow = (index: number) => {
    setActiveIndex(index);
    if (actionable) {
      useBtnRefs.current[index]?.focus();
      return;
    }
    itemRefs.current[index]?.focus();
  };

  const moveFocusByKey = (key: string): boolean => {
    if (rows.length === 0) return false;
    let next = safeIndex;
    switch (key) {
      case 'ArrowDown':
        next = safeIndex + 1;
        break;
      case 'ArrowUp':
        next = safeIndex - 1;
        break;
      case 'Home':
        next = 0;
        break;
      case 'End':
        next = rows.length - 1;
        break;
      default:
        return false;
    }
    if (next < 0 || next >= rows.length || next === safeIndex) {
      playBumpSound();
      return true;
    }
    focusRow(next);
    return true;
  };

  const handleListKeyDown = (event: React.KeyboardEvent<HTMLUListElement>) => {
    if (rows.length === 0) return;
    if (event.key === 'Enter' || event.key === ' ') {
      // Escolher com o teclado é o mesmo gesto de qualquer caixa de listagem.
      // Sem `onSelect` a lista ou é de leitura (Enter passa) ou acionável
      // (Enter no botão "Usar" já dispara o próprio botão — não engolir aqui).
      if (!onSelect) return;
      event.preventDefault();
      onSelect(rows[safeIndex]);
      return;
    }
    if (moveFocusByKey(event.key)) {
      event.preventDefault();
    }
  };

  const motivo = catalog ? reasonText(t, catalog) : '';
  // A idade sai do instante da coleta, e não de `age_seconds`: a idade é de
  // quando o backend respondeu, e subtraí-la do relógio de agora produziria um
  // instante que anda junto com ele — a tela aberta diria "há cinco minutos"
  // para sempre, enquanto o catálogo envelhece.
  const coletadoEm = catalog?.fetched_at ? Date.parse(catalog.fetched_at) : NaN;
  const coletado = Number.isNaN(coletadoEm)
    ? ''
    : formatRelativeTimeLocalized(coletadoEm, i18n.language);

  const status = (() => {
    if (erroTexto) return erroTexto;
    if (!catalog) return '';
    if (catalog.agents.length === 0) {
      return motivo
        ? t('acpCatalog.status.emptyReason', { reason: motivo })
        : t('acpCatalog.status.empty');
    }
    const partes = [
      coletado ? t('acpCatalog.status.collected', { when: coletado }) : '',
      catalog.stale ? t('acpCatalog.status.stale') : '',
      motivo ? t('acpCatalog.status.notUpdated', { reason: motivo }) : '',
    ];
    return partes.filter(Boolean).join(' ');
  })();

  const statusState = erroTexto || catalog?.agents.length === 0 || motivo ? 'attention' : 'ok';

  return (
    // Ocupado enquanto a lista é carregada ou recarregada: o conteúdo inteiro
    // troca, e quem estiver atravessando o catálogo nesse instante leria metade
    // da lista velha e metade da nova.
    <div className="acp-catalog" aria-busy={loading || refreshing}>
      <p className="acp-catalog__intro">
        {onSelect
          ? t('acpCatalog.introSelect')
          : actionable
            ? t('acpCatalog.introActions')
            : t('acpCatalog.intro')}
      </p>

      <div className="acp-catalog__controls">
        <Input
          label={t('acpCatalog.search.label')}
          value={searchTerm}
          onChange={(event) => handleSearchChange(event.target.value)}
          placeholder={t('acpCatalog.search.placeholder')}
          type="search"
          fullWidth
        />
        <Button type="button" variant="secondary" onClick={() => void load('refresh')} disabled={refreshing || loading}>
          {refreshing ? t('acpCatalog.actions.refreshing') : t('acpCatalog.actions.refresh')}
        </Button>
      </div>

      {/*
        O estado do catálogo em texto, e não live region: o anúncio de erro sai
        pelo announcer global, e uma segunda região viva aqui competiria com ele.
      */}
      {!!status && (
        <p className="acp-catalog__status" data-state={statusState}>
          {status}
        </p>
      )}

      {/*
        Sem catálogo a tela não some nem vira erro (D2): quem está sem rede
        continua podendo apontar comando e argumentos à mão no formulário do
        provedor, e é isso que a frase diz.
      */}
      {!loading && (!catalog || catalog.agents.length === 0) && (
        <p className="acp-catalog__manual">{t('acpCatalog.manualPath')}</p>
      )}

      {loading && <p className="acp-catalog__loading">{t('acpCatalog.loading')}</p>}

      {!loading && !!catalog && catalog.agents.length > 0 && (
        <>
          <p className="acp-catalog__count">
            {t('acpCatalog.count', { shown: rows.length, total: catalog.agents.length })}
          </p>
          <p id={navHelpId} className="acp-catalog__nav-help">
            {onSelect
              ? t('acpCatalog.navHelpSelect')
              : actionable
                ? t('acpCatalog.navHelpActions')
                : t('acpCatalog.navHelp')}
          </p>
          {rows.length === 0 ? (
            <p className="acp-catalog__no-match">{t('acpCatalog.search.noMatch', { term: searchTerm })}</p>
          ) : (
            /*
              Lista de uma parada de Tab só, com setas navegando entre os itens.
              Cada item recebe foco e um nome acessível que é a frase inteira
              dele: é o que faz "lido inteiro" acontecer sem depender de o leitor
              de telas percorrer o item campo por campo.
            */
            <ul
              className="acp-catalog__list"
              role={onSelect ? 'listbox' : undefined}
              aria-label={t('acpCatalog.listLabel')}
              aria-describedby={navHelpId}
              onKeyDown={handleListKeyDown}
            >
              {rows.map((agent, index) => {
                // As duas frases de integridade saem daqui, e não do meio do
                // JSX: cada uma aparece na condição e no conteúdo, e montá-las
                // duas vezes por item traduziria o mesmo texto duas vezes.
                const naoVerificada = installedIntegrityText(t, agent);
                const integridade = integrityText(t, agent);
                const podeInstalar = actionable && !!onInstallAgent && catalogOffersInstall(agent);
                const ativo = index === safeIndex;
                return (
                  <li
                    key={agent.id}
                    ref={(node) => {
                      itemRefs.current[index] = node;
                    }}
                    className="acp-catalog__item"
                    role={onSelect ? 'option' : undefined}
                    aria-selected={onSelect ? agent.id === selectedId : undefined}
                    // No modo acionável o foco mora no botão "Usar" (e Tab vai
                    // ao "Instalar"); a linha continua sendo o agrupador visual.
                    tabIndex={actionable ? undefined : ativo ? 0 : -1}
                    aria-label={actionable ? undefined : catalogItemLabel(t, agent)}
                    onFocus={() => setActiveIndex(index)}
                    onClick={onSelect ? () => onSelect(agent) : undefined}
                  >
                    <h3 className="acp-catalog__name" id={actionable ? `acp-catalog-name-${agent.id}` : undefined}>
                      {agent.name}
                      {!!agent.version && (
                        <span className="acp-catalog__version">
                          {t('acpCatalog.item.version', { version: agent.version })}
                        </span>
                      )}
                    </h3>
                    <p className="acp-catalog__id">{agent.id}</p>
                    {!!agent.description && <p className="acp-catalog__description">{agent.description}</p>}

                    <p className="acp-catalog__state" data-state={agent.state}>
                      <span className="acp-catalog__state-term">{t('acpCatalog.item.state')}</span>{' '}
                      {stateText(t, agent)}
                    </p>
                    {!!agent.state_detail && <p className="acp-catalog__detail">{agent.state_detail}</p>}
                    {!!naoVerificada && <p className="acp-catalog__unverified">{naoVerificada}</p>}

                    <p className="acp-catalog__runtime" data-missing={agent.runtime && !agent.runtime_found ? 'true' : undefined}>
                      {runtimeText(t, agent)}
                    </p>

                    <dl className="acp-catalog__facts">
                      {!!agent.distributions?.length && (
                        <div className="acp-catalog__fact">
                          <dt>{t('acpCatalog.item.distributionsTerm')}</dt>
                          <dd>{agent.distributions.map((kind) => distributionName(t, kind)).join(', ')}</dd>
                        </div>
                      )}
                      {!!integridade && (
                        <div className="acp-catalog__fact">
                          <dt>{t('acpCatalog.item.integrityTerm')}</dt>
                          <dd>{integridade}</dd>
                        </div>
                      )}
                      {!!agent.authors?.length && (
                        <div className="acp-catalog__fact">
                          <dt>{t('acpCatalog.item.authorsTerm')}</dt>
                          <dd>{agent.authors.join(', ')}</dd>
                        </div>
                      )}
                      {!!agent.license && (
                        <div className="acp-catalog__fact">
                          <dt>{t('acpCatalog.item.licenseTerm')}</dt>
                          <dd>{agent.license}</dd>
                        </div>
                      )}
                      {!!agent.website && (
                        <div className="acp-catalog__fact">
                          <dt>{t('acpCatalog.item.websiteTerm')}</dt>
                          <dd className="acp-catalog__url">{agent.website}</dd>
                        </div>
                      )}
                      {!!agent.repository && (
                        <div className="acp-catalog__fact">
                          <dt>{t('acpCatalog.item.repositoryTerm')}</dt>
                          <dd className="acp-catalog__url">{agent.repository}</dd>
                        </div>
                      )}
                    </dl>

                    {actionable && (
                      <div
                        className="acp-catalog__actions"
                        role="group"
                        aria-labelledby={`acp-catalog-name-${agent.id}`}
                      >
                        {onUseAgent && (
                          <Button
                            ref={(node) => {
                              useBtnRefs.current[index] = node;
                            }}
                            type="button"
                            variant="primary"
                            tabIndex={ativo ? 0 : -1}
                            aria-label={t('acpCatalog.row.useAria', { name: agent.name })}
                            onFocus={() => setActiveIndex(index)}
                            onKeyDown={(event) => {
                              // Setas no botão navegam a lista; sem isso o
                              // foco preso no botão perderia o atalho das setas.
                              if (moveFocusByKey(event.key)) {
                                event.preventDefault();
                              }
                            }}
                            onClick={() => onUseAgent(agent)}
                          >
                            {t('acpCatalog.row.use')}
                          </Button>
                        )}
                        {podeInstalar && (
                          <Button
                            type="button"
                            variant="secondary"
                            tabIndex={ativo ? 0 : -1}
                            aria-label={t('acpCatalog.row.installAria', { name: agent.name })}
                            onFocus={() => setActiveIndex(index)}
                            onClick={() => onInstallAgent?.(agent)}
                          >
                            {t('acpCatalog.row.install')}
                          </Button>
                        )}
                      </div>
                    )}
                  </li>
                );
              })}
            </ul>
          )}
        </>
      )}
    </div>
  );
};
