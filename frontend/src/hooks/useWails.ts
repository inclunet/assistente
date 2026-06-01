import { logger } from '../utils/logger';
import { useEffect, useCallback, useRef } from 'react';
import { EventsOn } from '@wailsjs/runtime/runtime';

/**
 * Hook para escutar eventos do Wails
 * @param eventName Nome do evento a escutar
 * @param callback Função chamada quando o evento é disparado
 */
export function useWailsEvent<T = unknown>(
  eventName: string,
  callback: (data: T) => void
) {
  const callbackRef = useRef(callback);

  // Mantém callback sempre atualizado
  useEffect(() => {
    callbackRef.current = callback;
  }, [callback]);

  useEffect(() => {
    const handler = (data: T) => {
      callbackRef.current(data);
    };

    const unsub = EventsOn(eventName, handler);

    return unsub;
  }, [eventName]);
}

/**
 * Hook para chamar funções do backend Wails com tratamento de erro
 * @param apiFunction Função do backend a ser chamada
 */
export function useWailsAPI<TArgs extends unknown[], TResult>(
  apiFunction: (...args: TArgs) => Promise<TResult>
) {
  const call = useCallback(
    async (...args: TArgs): Promise<TResult> => {
      try {
        const result = await apiFunction(...args);
        return result;
      } catch (error) {
        logger.error('Erro ao chamar API Wails:', error);
        throw error;
      }
    },
    [apiFunction]
  );

  return call;
}
