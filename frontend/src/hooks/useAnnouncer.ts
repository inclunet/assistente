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
      if (priority === 'assertive') {
        // Limpa timeout anterior se existir
        if (assertiveTimeoutRef.current) {
          clearTimeout(assertiveTimeoutRef.current);
        }
        
        // Define a mensagem
        setAssertiveMessage(message);
        
        // Limpa após 1 segundo
        assertiveTimeoutRef.current = window.setTimeout(() => {
          setAssertiveMessage('');
        }, 1000);
      } else {
        // Limpa timeout anterior se existir
        if (politeTimeoutRef.current) {
          clearTimeout(politeTimeoutRef.current);
        }
        
        // Define a mensagem
        setPoliteMessage(message);
        
        // Limpa após 1 segundo
        politeTimeoutRef.current = window.setTimeout(() => {
          setPoliteMessage('');
        }, 1000);
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
