import { useCallback, useRef } from 'react';
import { useTranslation } from 'react-i18next';
import { RetryUserRuntimeInit } from '@wailsjs/go/app/App';
import { logger } from '../utils/logger';
import { useWailsEvent } from './useWails';
import { useAnnouncer } from './useAnnouncer';
import { useUIStore } from '../store/uiStore';
import {
  RUNTIME_PARTIAL_INIT_EVENT,
  RUNTIME_SUBSYSTEMS,
  type RuntimePartialInitPayload,
} from '../types/runtime';

// Janela curta para o evento runtime:partial-init (re)propagar após o retry
// antes de declararmos sucesso. O backend emite o evento de forma síncrona
// durante o reload, então este tick é folga suficiente para distinguir
// "ainda falhando" de "subiu tudo agora".
const RETRY_SETTLE_MS = 400;

/**
 * Assina (uma única vez) o evento `runtime:partial-init` emitido pelo backend
 * quando o reload do runtime user-scoped pós-login termina com subsistemas
 * falhos (issue #250). Exibe um aviso NÃO-bloqueante: announce acessível
 * (assertivo) + toast persistente com ação "Tentar novamente", que rechama o
 * mesmo pipeline de reload no backend (RetryUserRuntimeInit).
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

  const retryingRef = useRef(false);
  // Conta eventos partial-init recebidos, para detectar de forma determinística
  // se um retry ainda resultou em falha (evento novo) ou em sucesso (silêncio).
  const eventCountRef = useRef(0);

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
      removeToast(toastId);

      const countBefore = eventCountRef.current;
      announce(t('runtimeStatus.partialInit.retrying'), 'polite');

      try {
        await RetryUserRuntimeInit();
      } catch (err) {
        logger.error('[partialInit] retry falhou:', err);
        announce(t('runtimeStatus.partialInit.retryError'), 'assertive');
        addToast(t('runtimeStatus.partialInit.retryError'), 'error');
        retryingRef.current = false;
        return;
      }

      // Dá tempo para um eventual novo runtime:partial-init chegar. Se nenhum
      // chegou, o reload subiu tudo: feedback positivo. Se chegou, o próprio
      // listener já reexibiu o aviso (não duplicamos mensagem aqui).
      await new Promise((resolve) => setTimeout(resolve, RETRY_SETTLE_MS));
      if (eventCountRef.current === countBefore) {
        announce(t('runtimeStatus.partialInit.retrySuccess'), 'polite');
        addToast(t('runtimeStatus.partialInit.retrySuccess'), 'success', 4000);
      }
      retryingRef.current = false;
    },
    [announce, addToast, removeToast, t],
  );

  useWailsEvent<RuntimePartialInitPayload>(RUNTIME_PARTIAL_INIT_EVENT, (data) => {
    if (!data || !Array.isArray(data.subsystems) || data.subsystems.length === 0) {
      return;
    }
    eventCountRef.current += 1;

    const names = data.subsystems.map((s) => labelFor(s.subsystem));
    const list = names.join(', ');
    const message = t('runtimeStatus.partialInit.message', { subsystems: list });

    announce(t('runtimeStatus.partialInit.announce', { subsystems: list }), 'assertive');

    // toastId só existe após addToast retornar; a ação é chamada depois, então
    // a closure captura o valor já atribuído.
    let toastId = '';
    toastId = addToast(message, 'warning', 0, {
      label: t('runtimeStatus.partialInit.retry'),
      onClick: () => {
        void handleRetry(toastId);
      },
    });
  });
}
