import { useCallback, useEffect, useRef } from 'react';
import { useTranslation } from 'react-i18next';
import { RetryUserRuntimeInit } from '@wailsjs/go/app/App';
import { logger } from '../utils/logger';
import { useWailsEvent } from './useWails';
import { useAnnouncer } from './useAnnouncer';
import { useUIStore } from '../store/uiStore';
import { useAuthStore } from '../store/authStore';
import {
  RUNTIME_PARTIAL_INIT_EVENT,
  RUNTIME_SUBSYSTEMS,
  type RuntimePartialInitPayload,
  type RuntimeSubsystemFailure,
} from '../types/runtime';

/**
 * Assina (uma única vez) o evento `runtime:partial-init` emitido pelo backend
 * quando o reload do runtime user-scoped pós-login termina com subsistemas
 * falhos (issue #250). Exibe um aviso NÃO-bloqueante: announce acessível
 * (assertivo) + toast persistente com ação "Tentar novamente", que rechama o
 * mesmo pipeline de reload no backend (RetryUserRuntimeInit).
 *
 * O sucesso/falha do retry é determinístico: vem do RETORNO da RPC
 * (lista de subsistemas ainda falhos), não de heurística de timer. O aviso
 * persistente só é removido após o backend confirmar sucesso; em falha da RPC
 * ele é mantido para o usuário tentar de novo.
 *
 * NÃO cria nova live region: a fala vai pelo announcer único (useAnnouncer),
 * respeitando a arbitragem de acessibilidade global do projeto. Deve ser
 * montado uma única vez na árvore (em App), nunca por aba.
 */
export function usePartialRuntimeInitListener() {
  const { t } = useTranslation();
  const { announce } = useAnnouncer();
  const addToast = useUIStore((s) => s.addToast);
  const removeToast = useUIStore((s) => s.removeToast);
  const isAuthenticated = useAuthStore((s) => s.isAuthenticated);

  // Id do toast persistente de aviso atualmente visível (para removê-lo no
  // sucesso do retry ou no logout). null quando não há aviso ativo.
  const partialToastIdRef = useRef<string | null>(null);
  // Evita cliques duplos no "Tentar novamente" enquanto a RPC está em voo.
  const retryingRef = useRef(false);
  // Ref para o showWarning mais recente — quebra o ciclo handleRetry⇄showWarning
  // sem recriar callbacks a cada render.
  const showWarningRef = useRef<(subsystems: RuntimeSubsystemFailure[]) => void>(() => {});

  const labelFor = useCallback(
    (subsystem: string) => {
      const key = (RUNTIME_SUBSYSTEMS as readonly string[]).includes(subsystem)
        ? subsystem
        : 'unknown';
      return t(`runtimeStatus.partialInit.subsystems.${key}`);
    },
    [t],
  );

  const handleRetry = useCallback(
    async (toastId: string) => {
      if (retryingRef.current) return;
      retryingRef.current = true;
      announce(t('runtimeStatus.partialInit.retrying'), 'polite');

      try {
        const result = await RetryUserRuntimeInit();
        const remaining = result.subsystems;

        if (Array.isArray(remaining) && remaining.length > 0) {
          // Ainda falhando: reexibe o aviso persistente com a lista ATUAL
          // (showWarning substitui o toast anterior). Mantém "Tentar novamente".
          showWarningRef.current(remaining);
        } else {
          // Sucesso confirmado pelo backend: só agora remove o aviso.
          // Remove SEMPRE o toast atualmente rastreado E o toastId original (se
          // diferentes): um novo runtime:partial-init pode ter substituído o
          // toast enquanto a RPC estava em voo (ex.: RefreshAuth reemitindo),
          // movendo partialToastIdRef para outro id. Como só rastreamos avisos
          // de partial-init nesse ref, é seguro remover ambos sem afetar toasts
          // legítimos de outras origens.
          const currentId = partialToastIdRef.current;
          removeToast(toastId);
          if (currentId && currentId !== toastId) {
            removeToast(currentId);
          }
          partialToastIdRef.current = null;
          announce(t('runtimeStatus.partialInit.retrySuccess'), 'polite');
          // suppressAnnounce: já anunciamos acima; evita fala duplicada.
          addToast(t('runtimeStatus.partialInit.retrySuccess'), 'success', 4000, undefined, {
            suppressAnnounce: true,
          });
        }
      } catch (err) {
        // RPC falhou: NÃO remove o aviso persistente — o usuário ainda precisa
        // do botão "Tentar novamente". Acrescenta só um toast de erro efêmero.
        logger.error('[partialInit] retry falhou:', err);
        announce(t('runtimeStatus.partialInit.retryError'), 'assertive');
        addToast(t('runtimeStatus.partialInit.retryError'), 'error', undefined, undefined, {
          suppressAnnounce: true,
        });
      } finally {
        retryingRef.current = false;
      }
    },
    [announce, addToast, removeToast, t],
  );

  const showWarning = useCallback(
    (subsystems: RuntimeSubsystemFailure[]) => {
      const list = subsystems.map((s) => labelFor(s.subsystem)).join(', ');
      const message = t('runtimeStatus.partialInit.message', { subsystems: list });

      announce(t('runtimeStatus.partialInit.announce', { subsystems: list }), 'assertive');

      // Substitui um aviso anterior ainda visível, evitando empilhar toasts.
      if (partialToastIdRef.current) {
        removeToast(partialToastIdRef.current);
        partialToastIdRef.current = null;
      }

      // toastId só existe após addToast retornar; a ação é chamada depois, então
      // a closure captura o valor já atribuído.
      let toastId = '';
      toastId = addToast(
        message,
        'warning',
        0,
        {
          label: t('runtimeStatus.partialInit.retry'),
          onClick: () => {
            void handleRetry(toastId);
          },
        },
        // suppressAnnounce: já anunciamos a versão "rica" acima (announce key),
        // evitando fala duplicada com o anúncio automático do toast.
        { suppressAnnounce: true },
      );
      partialToastIdRef.current = toastId;
    },
    [announce, addToast, removeToast, labelFor, t, handleRetry],
  );

  // Atualização SÍNCRONA no render (não em useEffect): garante que a ref sempre
  // aponta para o showWarning mais recente, sem a janela inicial com o no-op em
  // que um clique em "Tentar novamente" deixaria de reexibir o aviso. Escrever
  // em uma ref durante o render é seguro/idiomático para este caso (não dispara
  // re-render nem efeitos colaterais externos).
  showWarningRef.current = showWarning;

  useWailsEvent<RuntimePartialInitPayload>(RUNTIME_PARTIAL_INIT_EVENT, (data) => {
    // Ignora eventos que cheguem após logout (corrida com RefreshAuth/logout):
    // sem isso o aviso poderia renderizar/anunciar na tela de login antes do
    // effect de cleanup rodar. useWailsEvent mantém o callback atualizado por
    // ref, então isAuthenticated é sempre o valor corrente.
    if (!isAuthenticated) {
      return;
    }
    if (!data || !Array.isArray(data.subsystems) || data.subsystems.length === 0) {
      return;
    }
    showWarning(data.subsystems);
  });

  // Logout: remove o aviso persistente e zera o estado para não sobreviver à
  // tela de login nem ser relido ao relogar (mesmo padrão do
  // useConnectionStatusListener).
  useEffect(() => {
    if (!isAuthenticated) {
      if (partialToastIdRef.current) {
        removeToast(partialToastIdRef.current);
        partialToastIdRef.current = null;
      }
      retryingRef.current = false;
    }
  }, [isAuthenticated, removeToast]);
}
