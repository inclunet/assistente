/**
 * useVAD - Hook React para Voice Activity Detection
 * 
 * Facilita o uso do VoiceActivityDetector em componentes React,
 * gerenciando o ciclo de vida e expondo estado reativo.
 * 
 * @example
 * ```tsx
 * function VoiceInput() {
 *   const { 
 *     isActive, 
 *     isSpeaking, 
 *     volume, 
 *     start, 
 *     stop 
 *   } = useVAD({
 *     silenceDuration: 1500,
 *     onActivityEnd: () => console.log('Usuário parou de falar'),
 *   });
 * 
 *   return (
 *     <div>
 *       <button onClick={isActive ? stop : start}>
 *         {isActive ? 'Parar' : 'Iniciar'}
 *       </button>
 *       {isSpeaking && <span>Falando...</span>}
 *       <div style={{ width: `${volume * 100}%` }} />
 *     </div>
 *   );
 * }
 * ```
 */

import { logger } from '../utils/logger';
import { useState, useCallback, useRef, useEffect } from 'react';
import { VoiceActivityDetector, VADOptions, VADConfig } from '../services/vad';

export interface UseVADOptions extends VADOptions {
  /** Se deve inicializar automaticamente ao montar (default: false) */
  autoInit?: boolean;
  
  /** Se deve iniciar detecção automaticamente após init (default: false) */
  autoStart?: boolean;
}

export interface UseVADReturn {
  /** Se o VAD está ativo (detectando) */
  isActive: boolean;
  
  /** Se o usuário está falando */
  isSpeaking: boolean;
  
  /** Volume atual (0-1) */
  volume: number;
  
  /** Se está inicializado */
  isInitialized: boolean;
  
  /** Se está inicializando */
  isInitializing: boolean;
  
  /** Erro, se houver */
  error: Error | null;
  
  /** Inicializa o VAD (solicita permissão de microfone) */
  init: (stream?: MediaStream) => Promise<boolean>;
  
  /** Inicia a detecção */
  start: () => void;
  
  /** Para a detecção */
  stop: () => void;
  
  /** Libera recursos */
  destroy: () => void;
  
  /** Atualiza configuração */
  updateConfig: (config: Partial<VADConfig>) => void;
}

export function useVAD(options: UseVADOptions = {}): UseVADReturn {
  const {
    autoInit = false,
    autoStart = false,
    onActivityStart,
    onActivityEnd,
    onSilenceStart,
    onSilenceEnd,
    onVolumeChange,
    ...configOptions
  } = options;

  // Estado
  const [isActive, setIsActive] = useState(false);
  const [isSpeaking, setIsSpeaking] = useState(false);
  const [volume, setVolume] = useState(0);
  const [isInitialized, setIsInitialized] = useState(false);
  const [isInitializing, setIsInitializing] = useState(false);
  const [error, setError] = useState<Error | null>(null);

  // Ref para o detector
  const vadRef = useRef<VoiceActivityDetector | null>(null);

  // Ref para callbacks (para evitar re-criação do VAD quando callbacks mudam)
  const callbacksRef = useRef({
    onActivityStart,
    onActivityEnd,
    onSilenceStart,
    onSilenceEnd,
    onVolumeChange,
  });

  // Atualiza refs de callbacks
  useEffect(() => {
    callbacksRef.current = {
      onActivityStart,
      onActivityEnd,
      onSilenceStart,
      onSilenceEnd,
      onVolumeChange,
    };
  }, [onActivityStart, onActivityEnd, onSilenceStart, onSilenceEnd, onVolumeChange]);

  // Cria/atualiza callbacks wrappers
  const createCallbacks = useCallback(() => ({
    onActivityStart: () => {
      setIsSpeaking(true);
      callbacksRef.current.onActivityStart?.();
    },
    onActivityEnd: () => {
      setIsSpeaking(false);
      callbacksRef.current.onActivityEnd?.();
    },
    onSilenceStart: () => {
      callbacksRef.current.onSilenceStart?.();
    },
    onSilenceEnd: () => {
      callbacksRef.current.onSilenceEnd?.();
    },
    onVolumeChange: (vol: number) => {
      setVolume(vol);
      callbacksRef.current.onVolumeChange?.(vol);
    },
  }), []);

  // Inicializa o VAD
  const init = useCallback(async (stream?: MediaStream): Promise<boolean> => {
    if (isInitialized || isInitializing) {
      return isInitialized;
    }

    setIsInitializing(true);
    setError(null);

    try {
      // Cria nova instância
      vadRef.current = new VoiceActivityDetector({
        ...configOptions,
        ...createCallbacks(),
      });

      await vadRef.current.init(stream);
      setIsInitialized(true);
      setIsInitializing(false);

      // Auto-start se configurado
      if (autoStart) {
        vadRef.current.start();
        setIsActive(true);
      }

      return true;
    } catch (err) {
      const error = err instanceof Error ? err : new Error('Erro ao inicializar VAD');
      setError(error);
      setIsInitializing(false);
      logger.error('[useVAD] Erro ao inicializar:', error);
      return false;
    }
  }, [isInitialized, isInitializing, configOptions, createCallbacks, autoStart]);

  // Inicia detecção
  const start = useCallback(() => {
    if (!vadRef.current || !isInitialized) {
      logger.warn('[useVAD] VAD não inicializado. Chame init() primeiro.');
      return;
    }

    vadRef.current.start();
    setIsActive(true);
  }, [isInitialized]);

  // Para detecção
  const stop = useCallback(() => {
    if (!vadRef.current) return;

    vadRef.current.stop();
    setIsActive(false);
    setIsSpeaking(false);
  }, []);

  // Libera recursos
  const destroy = useCallback(() => {
    if (vadRef.current) {
      vadRef.current.destroy();
      vadRef.current = null;
    }

    setIsActive(false);
    setIsSpeaking(false);
    setVolume(0);
    setIsInitialized(false);
    setError(null);
  }, []);

  // Atualiza configuração
  const updateConfig = useCallback((config: Partial<VADConfig>) => {
    if (vadRef.current) {
      vadRef.current.updateConfig(config);
    }
  }, []);

  // Auto-init se configurado
  useEffect(() => {
    if (autoInit && !isInitialized && !isInitializing) {
      init();
    }
  }, [autoInit, isInitialized, isInitializing, init]);

  // Cleanup ao desmontar
  useEffect(() => {
    return () => {
      if (vadRef.current) {
        vadRef.current.destroy();
        vadRef.current = null;
      }
    };
  }, []);

  return {
    isActive,
    isSpeaking,
    volume,
    isInitialized,
    isInitializing,
    error,
    init,
    start,
    stop,
    destroy,
    updateConfig,
  };
}

export default useVAD;
