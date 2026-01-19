import { useEffect, useCallback, useRef } from 'react';
import { EventsOn, EventsOff } from '../../wailsjs/runtime/runtime';

/**
 * Hook para escutar eventos do Wails
 * @param eventName Nome do evento a escutar
 * @param callback Função chamada quando o evento é disparado
 */
export function useWailsEvent<T = any>(
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

    EventsOn(eventName, handler);

    return () => {
      EventsOff(eventName);
    };
  }, [eventName]);
}

/**
 * Hook para chamar funções do backend Wails com tratamento de erro
 * @param apiFunction Função do backend a ser chamada
 */
export function useWailsAPI<TArgs extends any[], TResult>(
  apiFunction: (...args: TArgs) => Promise<TResult>
) {
  const call = useCallback(
    async (...args: TArgs): Promise<TResult> => {
      try {
        const result = await apiFunction(...args);
        return result;
      } catch (error) {
        console.error('Erro ao chamar API Wails:', error);
        throw error;
      }
    },
    [apiFunction]
  );

  return call;
}
