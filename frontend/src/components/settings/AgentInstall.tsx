import { useCallback, useEffect, useId, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import type { TFunction } from 'i18next';
import {
  ACPAgentInstallPlan,
  CancelACPAgentInstall,
  InstallACPAgent,
  RemoveACPAgent,
} from '@wailsjs/go/app/App';
import { EventsOn } from '@wailsjs/runtime/runtime';
import type { app } from '@wailsjs/go/models';
import { Button } from '../ui/Button';
import { Modal } from '../ui/Modal';
import { useAnnouncer } from '../../hooks/useAnnouncer';
import './AgentInstall.css';

/**
 * Evento de progresso da instalação. O nome é o mesmo do backend
 * (`ACPInstallProgressEvent`), e cada marco carrega o agente que o motivou.
 */
const INSTALL_PROGRESS_EVENT = 'acp:install:progress';

type InstallPlan = app.ACPInstallPlan;

/**
 * Marco da instalação, como o backend o emite (`ACPInstallProgress`, em
 * `internal/app/app_acp_install.go`).
 *
 * A forma é escrita aqui à mão, e não importada de `@wailsjs/go/models`, porque
 * o gerador de bindings só escreve os tipos que aparecem em assinatura de
 * método do App: o que viaja apenas em evento nunca chega ao `models.ts`, e
 * apontar para lá quebra o build na primeira regeneração. É a convenção do
 * projeto para payload de evento.
 */
export interface InstallProgress {
  agent_id: string;
  agent?: string;
  /** `started`, `installing`, `verifying`, `done`, `failed` ou `cancelled`. */
  stage: string;
  /** A etapa que falhou; só vem em `failed`. */
  step?: string;
  /** O motivo em texto; só vem em `failed`. */
  reason?: string;
}

/** Mensagem de erro que veio do backend, com recurso para o texto genérico. */
const errorText = (error: unknown, fallback: string): string => {
  const err = error as { message?: unknown } | null;
  return String(err?.message || error || fallback);
};

/**
 * Frase do marco da instalação, em texto (AEP-0086 D13).
 *
 * É a mesma frase para a tela e para o anúncio: um texto só para o anúncio
 * divergiria do que está escrito, e quem usa leitor de telas conferiria a tela
 * para ouvir outra coisa. `failed` nomeia a etapa junto do motivo, porque
 * "falhou" sem etapa não diz o que fazer em seguida.
 */
export const installProgressText = (t: TFunction, progress: InstallProgress): string => {
  const agent = progress.agent || '';
  switch (progress.stage) {
    case 'started':
      return t('providerForm.agent.catalog.stage.started', { agent });
    case 'installing':
      return t('providerForm.agent.catalog.stage.installing', { agent });
    case 'verifying':
      return t('providerForm.agent.catalog.stage.verifying', { agent });
    case 'done':
      return t('providerForm.agent.catalog.stage.done', { agent });
    case 'cancelled':
      return t('providerForm.agent.catalog.stage.cancelled', { agent });
    case 'failed': {
      const reason = progress.reason || t('providerForm.agent.catalog.failedUnknown');
      const step = progress.step ? t(`providerForm.agent.catalog.step.${progress.step}`, '') : '';
      return step
        ? t('providerForm.agent.catalog.failedAtStep', { step, reason })
        : t('providerForm.agent.catalog.failed', { reason });
    }
    default:
      return '';
  }
};

export interface AgentInstallProps {
  /**
   * O agente do registro que está sendo configurado, pelo `id` dele (ex.:
   * `claude-acp`). Vazio é provedor apontado à mão, que não tem linha no
   * catálogo — e aí não há o que instalar por ele.
   */
  agentId: string;

  /**
   * Recebe o comando resolvido da instalação, para os campos do formulário
   * pararem de ser digitação. É o que faz "instalar pelo catálogo" terminar com
   * um provedor pronto, e não com um caminho para alguém copiar.
   */
  onResolved: (command: string, args: string[]) => void;
}

/**
 * Instalação de um agente de código pelo catálogo do registro ACP
 * (AEP-0086 Fase 3).
 *
 * Ela não substitui o campo do comando: quem tem o agente instalado à mão
 * continua detectando ou digitando. O que este bloco resolve é o caso de quem
 * não tem — em vez de mandar a pessoa a um terminal, o app baixa o pacote que o
 * catálogo publica, resolve o comando e confere que ele fala o protocolo.
 *
 * Três decisões do AEP moldam o que aparece aqui:
 *
 *   - D3: nada é baixado sem confirmação, e a confirmação mostra agente, versão,
 *     origem e a linha de comando que será executada.
 *   - D7: sem o runtime não se oferece instalação, e o motivo fica em texto — o
 *     app não instala Node.
 *   - D13: os marcos viram frase na tela e anúncio, o erro nomeia a etapa, e o
 *     cancelamento diz que não sobrou nada no disco.
 */
export const AgentInstall = ({ agentId, onResolved }: AgentInstallProps) => {
  const { t } = useTranslation();
  const { announce } = useAnnouncer();
  const idBase = useId();
  const titleId = `${idBase}-title`;
  const installHelpId = `${idBase}-install-help`;
  const removeHelpId = `${idBase}-remove-help`;
  const confirmId = `${idBase}-confirm`;
  const unverifiedId = `${idBase}-unverified`;

  // O plano guarda de qual agente ele fala, pelo mesmo motivo que a detecção ao
  // lado: trocar o agente não desmonta este bloco, e o plano anterior ofereceria
  // instalar um que não é o que está sendo configurado.
  const [planned, setPlanned] = useState<{ kind: string; plan: InstallPlan } | null>(null);
  const plan = planned?.kind === agentId ? planned.plan : null;
  const [loading, setLoading] = useState(false);
  const [planError, setPlanError] = useState('');
  const [busy, setBusy] = useState(false);
  const [status, setStatus] = useState('');
  const [confirming, setConfirming] = useState(false);
  const [removing, setRemoving] = useState(false);

  const mountedRef = useRef(true);
  useEffect(() => {
    mountedRef.current = true;
    return () => {
      mountedRef.current = false;
    };
  }, []);

  // Tradução e anúncio ficam em refs porque são consumidos por efeitos e por
  // callbacks assíncronos. Como dependência de efeito, um `t` recriado a cada
  // render — o que acontece dependendo de como o i18n é montado — refaria o
  // efeito a cada render, e um efeito que consulta o backend viraria laço.
  const tRef = useRef(t);
  tRef.current = t;
  const announceRef = useRef(announce);
  announceRef.current = announce;

  // O plano só vale se ainda é o último pedido: instalar e remover pedem um
  // plano novo, e uma resposta atrasada devolveria à tela o estado anterior —
  // "instalado" depois de remover, ou o contrário.
  const planSeq = useRef(0);

  // Marca a instalação que já estava em voo quando esta tela abriu. A que começa
  // aqui termina no `finally` de quem a pediu; a adotada não tem quem a espere, e
  // sem isto a tela ficaria ocupada para sempre — botão de instalar desabilitado
  // e o de cancelar oferecendo cancelar o que já acabou.
  const adotadaRef = useRef(false);
  // O agente em refs porque os marcos chegam por evento e as respostas chegam
  // depois: os dois precisam saber de quem esta tela está falando agora, e não
  // de quem ela falava quando o pedido saiu.
  const agentIdRef = useRef(agentId);
  agentIdRef.current = agentId;

  const loadPlan = useCallback(async (kind: string) => {
    const seq = ++planSeq.current;
    const obsoleto = () => seq !== planSeq.current || !mountedRef.current;
    if (!kind) {
      // O plano que estava em voo já foi aposentado pela sequência acima, e o
      // `finally` dele não vai mais mexer em nada — inclusive não vai desligar
      // o carregamento que ele acendeu. Quem desliga é esta chamada.
      setPlanned(null);
      setPlanError('');
      setLoading(false);
      return;
    }
    setLoading(true);
    setPlanError('');
    try {
      const result = await ACPAgentInstallPlan(kind);
      if (obsoleto()) return;
      setPlanned({ kind, plan: result });
      // Instalação em voo sobrevive a esta tela: ela roda no backend, e o app
      // pode ter fechado e reaberto o formulário no meio dela. Quem a começou de
      // outra montagem não tem promessa para esperar aqui, então quem a encerra
      // é o marco de desfecho — e é por isso que ela fica marcada.
      const emVoo = !!result?.installing;
      // O marco também pode não chegar: a instalação adotada pode ter terminado
      // entre o plano e o registro do ouvinte. O plano seguinte é a outra
      // resposta possível, e ele solta a tela do mesmo jeito.
      if (adotadaRef.current && !emVoo) setBusy(false);
      adotadaRef.current = emVoo;
      if (emVoo) setBusy(true);
    } catch (error: unknown) {
      if (obsoleto()) return;
      setPlanned(null);
      setPlanError(errorText(error, tRef.current('providerForm.agent.catalog.planFailed')));
    } finally {
      if (!obsoleto()) setLoading(false);
    }
  }, []);

  useEffect(() => {
    // O que está na tela é do agente que estava nela: a frase do progresso, o
    // diálogo aberto e o botão ocupado passariam a descrever o agente antigo
    // depois da troca. A instalação em voo não é interrompida por isso — ela é
    // do backend —, e o plano do agente novo diz se há alguma dele.
    setStatus('');
    setConfirming(false);
    setRemoving(false);
    setBusy(false);
    adotadaRef.current = false;
    void loadPlan(agentId);
  }, [agentId, loadPlan]);

  // Os marcos chegam por evento porque a instalação é do backend, e ela continua
  // de pé se esta tela for fechada. Cada marco é filtrado pelo agente que o
  // motivou: duas instalações podem estar em voo, e a frase da outra descreveria
  // um agente que não é este. A comparação é direta com o `id` do registro
  // porque é o mesmo identificador dos dois lados desde o D11 — antes era
  // preciso guardar à parte o `agent_id` do plano para ter com o que comparar,
  // e o filtro só passava a valer depois de o plano chegar.
  //
  // Marca que um marco já disse como a instalação terminou. Sem ela, o erro que
  // a chamada devolve depois falaria de novo pelo mesmo desfecho — e no
  // cancelamento ele falaria errado: quem cancelou veria "a instalação falhou:
  // context canceled" no lugar de "cancelada, nada ficou no disco".
  const outcomeRef = useRef(false);
  useEffect(() => {
    return EventsOn(INSTALL_PROGRESS_EVENT, (progress: InstallProgress) => {
      if (!progress || progress.agent_id !== agentIdRef.current) return;
      const text = installProgressText(tRef.current, progress);
      if (!text) return;
      if (progress.stage === 'failed' || progress.stage === 'cancelled') outcomeRef.current = true;
      setStatus(text);
      // A falha interrompe a leitura em curso porque exige decisão; os marcos
      // do meio, e o cancelamento que a própria pessoa pediu, não atropelam
      // quem está lendo outra coisa na mesma tela (AEP-0058).
      announceRef.current(text, progress.stage === 'failed' ? 'assertive' : 'polite');
      // Só a instalação adotada termina por aqui. A que começou nesta tela é
      // encerrada por quem a pediu, que também recarrega o plano; fazer as duas
      // coisas daria dois pedidos ao backend pelo mesmo desfecho.
      if (!adotadaRef.current) return;
      if (progress.stage === 'done' || progress.stage === 'failed' || progress.stage === 'cancelled') {
        adotadaRef.current = false;
        setBusy(false);
        void loadPlan(agentIdRef.current);
      }
    });
  }, [loadPlan]);

  // Diz se a resposta que chegou ainda é da tela que a pediu. Estar montado não
  // basta: trocar o tipo do provedor não desmonta este bloco, e a instalação
  // continua de pé no backend. Sem esta pergunta, o comando resolvido de um
  // agente cairia nos campos de outro, e a frase de erro dele apareceria sob o
  // nome do agente novo.
  const doAgente = useCallback(
    (kind: string) => mountedRef.current && agentIdRef.current === kind,
    [],
  );

  /**
   * Instala o agente do plano em vista.
   *
   * `acceptUnverified` é a resposta à pergunta do artefato sem digest (D4), e
   * ela chega como argumento em vez de ser deduzida do plano de propósito: o
   * plano diz que o app não consegue conferir o arquivo, e isso não é a mesma
   * coisa que alguém ter lido a frase e aceitado. Só o botão afirmativo do
   * diálogo que mostrou a frase concede — outro caminho que chame esta função
   * não concede nada, e o backend recusa.
   */
  const handleInstall = async (acceptUnverified = false) => {
    const agentID = plan?.agent_id;
    if (!agentID) return;
    const kind = agentId;
    setConfirming(false);
    setBusy(true);
    outcomeRef.current = false;
    setStatus(t('providerForm.agent.catalog.stage.started', { agent: plan?.name || '' }));
    try {
      // O que o diálogo mostrou viaja junto: o que seria instalado depende da
      // máquina e do catálogo, e os dois mudam entre mostrar e clicar. O
      // backend recusa em vez de baixar algo que ninguém viu (D3).
      const installation = await InstallACPAgent(agentID, {
        distribution: plan?.distribution || '',
        origin: plan?.origin || '',
        sha256: plan?.sha256 || '',
        // A resposta à pergunta do artefato sem digest viaja com o pedido, e o
        // backend recusa sem ela: a decisão é por instalação, e não uma
        // preferência que fique ligada em algum lugar (D4).
        accept_unverified: acceptUnverified,
      });
      if (!doAgente(kind)) return;
      // O comando resolvido vai para os campos: instalar pelo catálogo termina
      // com um provedor pronto para salvar, e não com um caminho para copiar.
      onResolved(installation.command, installation.args || []);
    } catch (error: unknown) {
      if (!doAgente(kind)) return;
      // O marco de desfecho já disse o que houve, e ele diz melhor: nomeia a
      // etapa, e distingue cancelar de falhar. Sem marco nenhum — a recusa que
      // acontece antes de a instalação começar, como runtime ausente — o texto
      // do erro é tudo o que existe, e ele não pode ficar sem aparecer.
      if (!outcomeRef.current) {
        const message = t('providerForm.agent.catalog.failed', {
          reason: errorText(error, t('providerForm.agent.catalog.failedUnknown')),
        });
        setStatus(message);
        announce(message, 'assertive');
      }
    } finally {
      if (doAgente(kind)) {
        setBusy(false);
        void loadPlan(kind);
      }
    }
  };

  const handleCancel = async () => {
    const agentID = plan?.agent_id;
    if (!agentID) return;
    const kind = agentId;
    try {
      await CancelACPAgentInstall(agentID);
    } catch (error: unknown) {
      if (!doAgente(kind)) return;
      const message = errorText(error, t('providerForm.agent.catalog.cancelFailed'));
      setStatus(message);
      announce(message, 'assertive');
    } finally {
      // Cancelar o que já terminou não é erro — a instalação pode ter acabado
      // entre o render e o clique —, mas deixaria a tela oferecendo cancelar de
      // novo. O plano diz o que de fato está em voo.
      if (doAgente(kind)) void loadPlan(kind);
    }
  };

  const handleRemove = async () => {
    const agentID = plan?.agent_id;
    if (!agentID) return;
    const kind = agentId;
    setRemoving(false);
    try {
      await RemoveACPAgent(agentID);
      if (!doAgente(kind)) return;
      const message = t('providerForm.agent.catalog.removed');
      setStatus(message);
      announce(message, 'polite');
    } catch (error: unknown) {
      if (!doAgente(kind)) return;
      const message = errorText(error, t('providerForm.agent.catalog.removeFailed'));
      setStatus(message);
      announce(message, 'assertive');
    } finally {
      if (doAgente(kind)) void loadPlan(kind);
    }
  };

  // Tipo de provedor sem agente correspondente no catálogo não ganha bloco
  // nenhum: oferecer "instalar pelo catálogo" para o que o catálogo não publica
  // seria um botão que só sabe falhar. A falha de consulta, sim, aparece — quem
  // está sem rede precisa saber por que a oferta não está lá.
  if (planError) {
    return (
      <div className="agent-install" role="group" aria-labelledby={titleId}>
        <p id={titleId} className="agent-install__title">
          {t('providerForm.agent.catalog.title')}
        </p>
        <p className="agent-install__status" data-state="missing">{planError}</p>
      </div>
    );
  }
  if (!plan?.agent_id) return null;

  const installed = plan.installed;
  const runtime = plan.runtime;
  // Falta de runtime só bloqueia quem depende dele. Artefato binário sobe sem
  // Node, e negar o download por um pré-requisito que aquele caminho não usa
  // deixaria de fora justamente os agentes que não têm alternativa npm.
  const runtimeMissing = !!runtime?.required && !runtime.found;
  const binary = plan.distribution === 'binary';
  // Metade dos agentes com binário não publica digest, e o Cursor é um deles.
  // Instalar continua sendo possível — recusar mandaria a pessoa baixar o mesmo
  // arquivo pelo site, sem guarda nenhuma —, com uma confirmação que nomeia o
  // que o app não consegue atestar (D4).
  const unverified = !!plan.unverified;
  // O que ficou no disco também não foi conferido, e a marca não some depois de
  // instalado: quem abrir esta tela amanhã precisa saber o que aquele agente é.
  const installedUnverified = installed?.sha256_origin === 'observed';

  return (
    <div className="agent-install" role="group" aria-labelledby={titleId}>
      <p id={titleId} className="agent-install__title">
        {t('providerForm.agent.catalog.title')}
      </p>

      {installed ? (
        <>
          <p className="agent-install__intro">
            {t('providerForm.agent.catalog.installed', {
              agent: installed.name || plan.name,
              version: installed.version,
            })}
          </p>
          <p className="agent-install__path">
            {t('providerForm.agent.catalog.installedDir', { dir: installed.dir })}
          </p>
          {installedUnverified && (
            <p className="agent-install__unverified">
              {t('providerForm.agent.catalog.installedUnverified')}
            </p>
          )}
          <div className="agent-install__actions">
            <div className="agent-install__action">
              <Button
                type="button"
                variant="secondary"
                onClick={() => {
                  onResolved(installed.command, installed.args || []);
                  // Preencher campo por clique não é visível a quem não vê o
                  // campo: sem o anúncio, o botão pareceria não ter feito nada.
                  announce(
                    t('providerForm.agent.catalog.useAnnounce', { command: installed.command }),
                    'polite',
                  );
                }}
              >
                {t('providerForm.agent.catalog.useBtn')}
              </Button>
            </div>
            <div className="agent-install__action">
              <Button
                type="button"
                variant="danger"
                onClick={() => setRemoving(true)}
                aria-describedby={removeHelpId}
              >
                {t('providerForm.agent.catalog.removeBtn')}
              </Button>
              <p id={removeHelpId} className="agent-install__action-help">
                {t('providerForm.agent.catalog.removeBtnHelp')}
              </p>
            </div>
          </div>
        </>
      ) : (
        <>
          {/*
            Sem nome não há frase: o plano que sobra de uma consulta que falhou
            traz só o identificador e o motivo, e a apresentação viraria "publica
            como pacote, versão" com buracos onde deveriam estar as duas coisas
            que ela existe para dizer. O motivo, esse, aparece abaixo.
          */}
          {!!plan.name && (
            <p className="agent-install__intro">
              {t(
                binary
                  ? 'providerForm.agent.catalog.introBinary'
                  : 'providerForm.agent.catalog.intro',
                { agent: plan.name, version: plan.version },
              )}
            </p>
          )}

          {/*
            Sem runtime a instalação não é oferecida, e o motivo é o texto — não
            um botão cinza (D7). Os lugares consultados vêm junto para "não
            encontrado" ser verificável por quem vai instalar o Node.
          */}
          {runtimeMissing ? (
            <div className="agent-install__blocked">
              <p>{t('providerForm.agent.catalog.runtimeMissing', { runtime: runtime?.name })}</p>
              {!!runtime?.searched?.length && (
                <p className="agent-install__path">
                  {t('providerForm.agent.catalog.runtimeSearched', {
                    runtime: runtime.name,
                    places: runtime.searched.join(', '),
                  })}
                </p>
              )}
            </div>
          ) : plan.can_install ? (
            <div className="agent-install__actions">
              <div className="agent-install__action">
                <Button
                  type="button"
                  variant="secondary"
                  onClick={() => setConfirming(true)}
                  disabled={busy || loading}
                  aria-describedby={installHelpId}
                >
                  {t('providerForm.agent.catalog.installBtn')}
                </Button>
                <p id={installHelpId} className="agent-install__action-help">
                  {t(
                    binary
                      ? 'providerForm.agent.catalog.installBtnHelpBinary'
                      : 'providerForm.agent.catalog.installBtnHelp',
                  )}
                </p>
              </div>
            </div>
          ) : (
            <div className="agent-install__blocked">
              <p>
                {plan.reason
                  ? t('providerForm.agent.catalog.unavailable', { reason: plan.reason })
                  : t('providerForm.agent.catalog.unavailableUnknown')}
              </p>
            </div>
          )}
          {/*
            O cancelar acompanha a instalação, e não o ramo em que a tela caiu:
            o plano pode passar a dizer "indisponível" — Node que sumiu do PATH,
            catálogo que parou de responder — enquanto o npm continua escrevendo
            no disco, e o botão sumiria com algo ainda por cancelar.
          */}
          {busy && (
            <div className="agent-install__action">
              <Button type="button" variant="outline" onClick={handleCancel}>
                {t('providerForm.agent.catalog.cancelBtn')}
              </Button>
            </div>
          )}
        </>
      )}

      {/*
        Estado em texto na tela, com o data-state apenas como reforço visual.
        Não é live region: o anúncio sai pelo announcer global, e uma segunda
        região viva faria o mesmo marco ser lido duas vezes.
      */}
      {!!status && (
        <p className="agent-install__status" data-state={busy ? 'busy' : undefined}>
          {status}
        </p>
      )}

      {/*
        A confirmação mostra o que vai ser baixado e o que vai ser executado
        antes de qualquer byte sair da rede (D3). Os pares ficam em `dl` para o
        rótulo e o valor chegarem ligados a quem usa leitor de telas.
      */}
      <Modal
        isOpen={confirming}
        onClose={() => setConfirming(false)}
        title={t('providerForm.agent.catalog.confirm.title', { agent: plan.name })}
        size="md"
        /*
          A descrição do diálogo inclui o aviso quando ele existe. Um leitor de
          telas lê o que está apontado aqui ao abrir, e deixar de fora a frase
          que nomeia a ausência de verificação a transformaria em texto que só
          quem varre a tela encontra.
        */
        ariaDescribedBy={unverified ? `${confirmId} ${unverifiedId}` : confirmId}
        /*
          O foco começa em cancelar quando não há como conferir o artefato
          (D4). O Enter continua ativando o botão focado, como em qualquer
          diálogo; o que muda é onde ele começa, e instalar passa a exigir mover
          o foco até o outro botão — é esse movimento que separa ler do reflexo
          de confirmar. O seletor é explícito para a regra não depender da ordem
          em que os botões estão no DOM.
        */
        initialFocusSelector={unverified ? '[data-confirm-cancel]' : undefined}
      >
        {/*
          A abertura descreve o que vai acontecer, e o artefato sem digest tem a
          sua: a do binário promete conferir o arquivo contra o código publicado
          pelo registro, e repeti-la aqui diria o contrário do aviso logo abaixo.
        */}
        <p id={confirmId} className="agent-install__confirm-intro">
          {t(
            unverified
              ? 'providerForm.agent.catalog.confirm.introUnverified'
              : binary
                ? 'providerForm.agent.catalog.confirm.introBinary'
                : 'providerForm.agent.catalog.confirm.intro',
          )}
        </p>
        {/*
          A frase nomeia a ausência, e é texto: ícone de alerta como único sinal
          não chega a quem não vê a tela, e "não verificado" sem dizer o que isso
          significa não é informação (D4).
        */}
        {unverified && (
          <p id={unverifiedId} className="agent-install__unverified">
            {t('providerForm.agent.catalog.confirm.unverified')}
          </p>
        )}
        <dl className="agent-install__details">
          <dt>{t('providerForm.agent.catalog.confirm.agent')}</dt>
          <dd>{plan.name}</dd>
          <dt>{t('providerForm.agent.catalog.confirm.version')}</dt>
          <dd>{plan.version}</dd>
          <dt>{t('providerForm.agent.catalog.confirm.origin')}</dt>
          <dd className="agent-install__details-code">{plan.origin}</dd>
          <dt>{t('providerForm.agent.catalog.confirm.dir')}</dt>
          <dd className="agent-install__details-code">{plan.dir}</dd>
          {/*
            O que é baixado depende da distribuição, e o diálogo diz o que de
            fato vai acontecer (D3): o pacote é instalado por uma linha de
            comando, enquanto o artefato binário é um arquivo por plataforma
            cujo digest será conferido contra o que chegar.
          */}
          {binary ? (
            <>
              <dt>{t('providerForm.agent.catalog.confirm.target')}</dt>
              <dd className="agent-install__details-code">{plan.target}</dd>
              <dt>{t('providerForm.agent.catalog.confirm.digest')}</dt>
              <dd className={unverified ? undefined : 'agent-install__details-code'}>
                {unverified
                  ? t('providerForm.agent.catalog.confirm.digestMissing')
                  : plan.sha256}
              </dd>
            </>
          ) : (
            <>
              <dt>{t('providerForm.agent.catalog.confirm.command')}</dt>
              <dd className="agent-install__details-code">{plan.install_command}</dd>
            </>
          )}
        </dl>
        <div className="agent-install__confirm-actions">
          <Button
            type="button"
            variant="outline"
            data-confirm-cancel=""
            onClick={() => setConfirming(false)}
          >
            {t('providerForm.agent.catalog.confirm.cancelBtn')}
          </Button>
          {/*
            Confirmar duas vezes é um clique repetido, e não dois pedidos: o
            diálogo fecha no primeiro, mas o segundo pode chegar antes disso.

            O rótulo do artefato sem digest diz o que está sendo aceito, e não
            "confirmar": num leitor de telas o nome do botão é o que se ouve
            antes de acioná-lo, e ele é a última chance de a frase acima não ter
            passado batida.
          */}
          <Button
            type="button"
            variant="primary"
            onClick={() => void handleInstall(unverified)}
            disabled={busy}
          >
            {t(
              unverified
                ? 'providerForm.agent.catalog.confirm.confirmUnverifiedBtn'
                : 'providerForm.agent.catalog.confirm.confirmBtn',
            )}
          </Button>
        </div>
      </Modal>

      {/*
        Remover apaga o diretório do agente e deixa o provedor de pé (D5). O
        aviso diz isso: quem espera que o provedor suma junto precisa saber que
        ele fica, com um comando que passou a não existir.
      */}
      <Modal
        isOpen={removing}
        onClose={() => setRemoving(false)}
        title={t('providerForm.agent.catalog.removeConfirm.title', {
          agent: installed?.name || plan.name,
        })}
        size="sm"
      >
        <p className="agent-install__confirm-intro">
          {t('providerForm.agent.catalog.removeConfirm.message', { dir: installed?.dir })}
        </p>
        <div className="agent-install__confirm-actions">
          <Button type="button" variant="outline" onClick={() => setRemoving(false)}>
            {t('providerForm.agent.catalog.confirm.cancelBtn')}
          </Button>
          <Button type="button" variant="danger" onClick={handleRemove}>
            {t('providerForm.agent.catalog.removeConfirm.confirmBtn')}
          </Button>
        </div>
      </Modal>
    </div>
  );
};
