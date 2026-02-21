import { useState, useCallback, useEffect, useRef } from 'react';

let announceFunction: ((message: string, priority?: 'polite' | 'assertive') => void) | null = null;

/**
 * Registra a função de anúncio global
 */
export function registerAnnouncer(fn: (message: string, priority?: 'polite' | 'assertive') => void) {
  announceFunction = fn;
}

/**
 * Remove a função de anúncio global
 */
export function unregisterAnnouncer() {
  announceFunction = null;
}

/**
 * Hook para anunciar mensagens para leitores de tela
 */
export function useAnnouncer() {
  const announce = useCallback((message: string, priority: 'polite' | 'assertive' = 'polite') => {
    if (announceFunction) {
      announceFunction(message, priority);
    }
  }, []);

  return { announce };
}

/**
 * Função global para anunciar mensagens (para uso fora de componentes React)
 */
export function announce(message: string, priority: 'polite' | 'assertive' = 'polite') {
  if (announceFunction) {
    announceFunction(message, priority);
  }
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
    const handleAnnounce = (message: string, priority: 'polite' | 'assertive' = 'polite') => {
      // Timeout scales with message length so screen readers have time to capture it.
      // Minimum 3s for short messages, ~50ms per char for longer text.
      const timeoutMs = Math.max(3000, message.length * 50);

      if (priority === 'assertive') {
        if (assertiveTimeoutRef.current) {
          clearTimeout(assertiveTimeoutRef.current);
        }
        setAssertiveMessage(message);
        assertiveTimeoutRef.current = window.setTimeout(() => {
          setAssertiveMessage('');
        }, timeoutMs);
      } else {
        if (politeTimeoutRef.current) {
          clearTimeout(politeTimeoutRef.current);
        }
        setPoliteMessage(message);
        politeTimeoutRef.current = window.setTimeout(() => {
          setPoliteMessage('');
        }, timeoutMs);
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
