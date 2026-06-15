import { useEffect, useRef } from 'react';
import { useTranslation } from 'react-i18next';
import { useWailsEvent } from './useWails';
import { useAnnouncer } from './useAnnouncer';
import { useUIStore } from '../store/uiStore';
import { useAuthStore } from '../store/authStore';
import { useConnectionStore } from '../store/connectionStore';
import { CONNECTION_STATUS_EVENT, type ConnectionStatusPayload } from '../types/connection';

/**
 * Assina (uma única vez) o evento de status de conexão emitido pelo backend,
 * alimenta a connectionStore e anuncia transições de queda/restauração via o
 * announcer global + toast existente.
 *
 * NÃO cria nova live region: a fala vai pelo announcer único (useAnnouncer),
 * respeitando a arbitragem de acessibilidade global do projeto.
 *
 * Deve ser montado uma única vez na árvore (ex.: em App), nunca por aba.
 */
export function useConnectionStatusListener() {
  const { t } = useTranslation();
  const { announce } = useAnnouncer();
  const addToast = useUIStore((s) => s.addToast);
  const setStatus = useConnectionStore((s) => s.setStatus);
  const resetStatus = useConnectionStore((s) => s.reset);
  const isAuthenticated = useAuthStore((s) => s.isAuthenticated);

  // Último estado "estável" (online/offline) já notificado, para anunciar
  // apenas transições reais e ignorar o intermediário "checking".
  const lastStableRef = useRef<'online' | 'offline' | null>(null);

  useWailsEvent<ConnectionStatusPayload>(CONNECTION_STATUS_EVENT, (data) => {
    if (!data || typeof data.state !== 'string') return;
    setStatus(data);

    if (data.state !== 'online' && data.state !== 'offline') return;

    const prev = lastStableRef.current;
    if (prev !== null && prev !== data.state) {
      const providerName = data.providerName || t('connectionStatus.provider');
      if (data.state === 'offline') {
        const message = t('connectionStatus.announce.lost', { provider: providerName });
        announce(message, 'assertive');
        // suppressAnnounce: já anunciamos acima; evita fala duplicada.
        addToast(message, 'error', undefined, undefined, { suppressAnnounce: true });
      } else {
        const message = t('connectionStatus.announce.restored', { provider: providerName });
        announce(message, 'polite');
        addToast(message, 'success', 4000, undefined, { suppressAnnounce: true });
      }
    }
    lastStableRef.current = data.state;
  });

  // O hook é montado em App (fora do AuthGate), então permanece ativo após o
  // logout. Ao perder a sessão, o monitor backend é parado, mas o estado da UI
  // ficaria "herdado" e poderia gerar anúncios/toasts espúrios ao relogar.
  // Por isso, limpamos a connectionStore e o tracking de transição sempre que
  // isAuthenticated passa a false. Também limpa no unmount.
  useEffect(() => {
    if (!isAuthenticated) {
      lastStableRef.current = null;
      resetStatus();
    }
  }, [isAuthenticated, resetStatus]);

  useEffect(() => {
    return () => {
      lastStableRef.current = null;
    };
  }, []);
}
