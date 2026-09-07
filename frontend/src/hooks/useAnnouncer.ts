import { useState, useCallback, useEffect, useRef } from 'react';
import {
  announceWithOrigin,
  estimateAnnouncementReadingMs,
  registerAnnouncerSink,
  unregisterAnnouncerSink,
  type AnnouncePriority,
  type VoiceAnnounceRequest,
} from '../services/voiceAccessibility/announcerBroker';

/**
 * Registra a função de anúncio global
 */
export function registerAnnouncer(fn: (message: string, priority?: AnnouncePriority) => void) {
  registerAnnouncerSink((message, priority) => fn(message, priority));
}

/**
 * Remove a função de anúncio global
 */
export function unregisterAnnouncer() {
  unregisterAnnouncerSink();
}

/**
 * Hook para anunciar mensagens para leitores de tela
 */
export function useAnnouncer() {
  const announce = useCallback((message: string, priority: AnnouncePriority = 'polite') => {
    announceWithOrigin({ message, announcePriority: priority, eventType: 'user-action' });
  }, []);

  const announceRequest = useCallback((request: VoiceAnnounceRequest) => announceWithOrigin(request), []);

  return { announce, announceRequest };
}

/**
 * Função global para anunciar mensagens (para uso fora de componentes React)
 *
 * Re-tenta no microtask se o handler ainda não estiver registado (ex.: React Strict Mode
 * entre unmount e remount do ScreenReaderAnnouncer).
 */
export function announce(message: string, priority: AnnouncePriority = 'polite') {
  announceWithOrigin({ message, announcePriority: priority, eventType: 'user-action' });
}

/**
 * Hook interno para o componente ScreenReaderAnnouncer
 */
export function useAnnouncerState() {
  const [politeMessage, setPoliteMessage] = useState('');
  const [assertiveMessage, setAssertiveMessage] = useState('');
  const politeTimeoutRef = useRef<number>();
  const assertiveTimeoutRef = useRef<number>();

  useEffect(() => {
    const scheduleClear = (
      setter: (v: string) => void,
      timeoutRef: { current: number | undefined },
      timeoutMs: number,
    ) => {
      if (timeoutRef.current) {
        clearTimeout(timeoutRef.current);
      }
      timeoutRef.current = window.setTimeout(() => {
        setter('');
      }, timeoutMs);
    };

    const handleAnnounce = (message: string, priority: AnnouncePriority = 'polite') => {
      // Mesma estimativa que o broker usa para proteger a leitura: a região só
      // é limpa depois do tempo em que o leitor de telas ainda pode estar lendo.
      const timeoutMs = estimateAnnouncementReadingMs(message);

      if (priority === 'assertive') {
        setAssertiveMessage('');
        requestAnimationFrame(() => {
          setAssertiveMessage(message);
          scheduleClear(setAssertiveMessage, assertiveTimeoutRef, timeoutMs);
        });
      } else {
        setPoliteMessage('');
        requestAnimationFrame(() => {
          setPoliteMessage(message);
          scheduleClear(setPoliteMessage, politeTimeoutRef, timeoutMs);
        });
      }
    };

    registerAnnouncer(handleAnnounce);

    return () => {
      unregisterAnnouncer();
      if (politeTimeoutRef.current) {
        clearTimeout(politeTimeoutRef.current);
      }
      if (assertiveTimeoutRef.current) {
        clearTimeout(assertiveTimeoutRef.current);
      }
    };
  }, []);

  return { politeMessage, assertiveMessage };
}
