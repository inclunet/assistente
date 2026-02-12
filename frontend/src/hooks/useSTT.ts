/**
 * useSTT - Hook React para Speech-to-Text
 * 
 * Facilita o uso do STTService em componentes React,
 * gerenciando o ciclo de vida e expondo estado reativo.
 * 
 * @example
 * ```tsx
 * function VoiceInput() {
 *   const {
 *     state,
 *     isRecording,
 *     volume,
 *     interimText,
 *     startRecording,
 *     stopRecording,
 *   } = useSTT({
 *     provider: 'webspeech',
 *     mode: 'vad_silence',
 *     onTranscription: (text) => console.log('Transcrição:', text),
 *   });
 * 
 *   return (
 *     <div>
 *       <button 
 *         onPointerDown={startRecording}
 *         onPointerUp={stopRecording}
 *       >
 *         {isRecording ? '🔴' : '🎤'}
 *       </button>
 *       {interimText && <span>{interimText}</span>}
 *       <div style={{ width: `${volume * 100}%` }} />
 *     </div>
 *   );
 * }
 * ```
 */

import { useState, useCallback, useRef, useEffect } from 'react';
import {
  STTService,
  STTOptions,
  STTConfig,
  STTState,
  STTProvider,
  RecordingMode,
  STT_STATES,
} from '../services/stt';

export interface UseSTTOptions extends STTOptions {
  /** Se deve inicializar automaticamente ao montar (default: true) */
  autoInit?: boolean;
}

export interface UseSTTReturn {
  /** Estado atual do STT */
  state: STTState;
  
  /** Provider atual */
  provider: STTProvider;
  
  /** Modo de gravação atual */
  mode: RecordingMode;
  
  /** Se está ocioso */
  isIdle: boolean;
  
  /** Se está ouvindo (VAD Activity) */
  isListening: boolean;
  
  /** Se está gravando */
  isRecording: boolean;
  
  /** Se está processando */
  isProcessing: boolean;
  
  /** Se está em erro */
  isError: boolean;
  
  /** Se está ativo (gravando ou ouvindo) */
  isActive: boolean;
  
  /** Volume atual (0-1) */
  volume: number;
  
  /** Texto parcial (durante gravação) */
  interimText: string;
  
  /** Se está inicializado */
  isInitialized: boolean;
  
  /** Se está inicializando */
  isInitializing: boolean;
  
  /** Erro, se houver */
  error: string | null;
  
  /** Se WebSpeech está suportado */
  isWebSpeechSupported: boolean;
  
  /** Se Whisper está suportado */
  isWhisperSupported: boolean;
  
  /** Inicializa o serviço */
  init: () => Promise<boolean>;
  
  /** Inicia gravação */
  startRecording: () => Promise<boolean>;
  
  /** Para gravação */
  stopRecording: () => void;
  
  /** Cancela gravação */
  cancelRecording: () => void;
  
  /** Toggle gravação */
  toggleRecording: () => void;
  
  /** Define provider */
  setProvider: (provider: STTProvider) => void;
  
  /** Define modo */
  setMode: (mode: RecordingMode) => void;
  
  /** Define idioma */
  setLanguage: (language: string) => void;
  
  /** Atualiza configuração */
  updateConfig: (config: Partial<STTConfig>) => void;
  
  /** Libera recursos */
  destroy: () => void;
}

export function useSTT(options: UseSTTOptions = {}): UseSTTReturn {
  const {
    autoInit = true,
    onTranscription,
    onInterimResult,
    onStateChange,
    onVolumeChange,
    onActivityStart,
    onActivityEnd,
    onError,
    onNoSpeechDetected,
    onAudioFile,
    onRecordingCancelled,
    ...configOptions
  } = options;

  // Estado
  const [state, setState] = useState<STTState>(STT_STATES.IDLE);
  const [provider, setProviderState] = useState<STTProvider>(configOptions.provider || 'webspeech');
  const [mode, setModeState] = useState<RecordingMode>(configOptions.mode || 'ptt');
  const [volume, setVolume] = useState(0);
  const [interimText, setInterimText] = useState('');
  const [isInitialized, setIsInitialized] = useState(false);
  const [isInitializing, setIsInitializing] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [isWebSpeechSupported, setIsWebSpeechSupported] = useState(false);
  const [isWhisperSupported, setIsWhisperSupported] = useState(false);

  // Ref para o serviço
  const sttRef = useRef<STTService | null>(null);

  // Ref síncrona para estado de inicialização (evita race conditions)
  const isInitializedRef = useRef(false);
  const isInitializingRef = useRef(false);

  // Refs para callbacks
  const callbacksRef = useRef({
    onTranscription,
    onInterimResult,
    onStateChange,
    onVolumeChange,
    onActivityStart,
    onActivityEnd,
    onError,
    onNoSpeechDetected,
    onAudioFile,
    onRecordingCancelled,
  });

  // Atualiza refs de callbacks
  useEffect(() => {
    callbacksRef.current = {
      onTranscription,
      onInterimResult,
      onStateChange,
      onVolumeChange,
      onActivityStart,
      onActivityEnd,
      onError,
      onNoSpeechDetected,
      onAudioFile,
      onRecordingCancelled,
    };
  }, [
    onTranscription,
    onInterimResult,
    onStateChange,
    onVolumeChange,
    onActivityStart,
    onActivityEnd,
    onError,
    onNoSpeechDetected,
    onAudioFile,
    onRecordingCancelled,
  ]);

  // Inicializa o serviço
  const init = useCallback(async (): Promise<boolean> => {
    // Usa refs para evitar race conditions
    if (isInitializedRef.current) {
      return true;
    }
    if (isInitializingRef.current) {
      // Aguarda inicialização em progresso
      return new Promise((resolve) => {
        const checkInit = setInterval(() => {
          if (!isInitializingRef.current) {
            clearInterval(checkInit);
            resolve(isInitializedRef.current);
          }
        }, 50);
        // Timeout de segurança
        setTimeout(() => {
          clearInterval(checkInit);
          resolve(isInitializedRef.current);
        }, 5000);
      });
    }

    isInitializingRef.current = true;
    setIsInitializing(true);
    setError(null);

    try {
      sttRef.current = new STTService({
        ...configOptions,
        onTranscription: (text, prov) => {
          callbacksRef.current.onTranscription?.(text, prov);
        },
        onInterimResult: (text) => {
          setInterimText(text);
          callbacksRef.current.onInterimResult?.(text);
        },
        onStateChange: (newState, prevState, message) => {
          setState(newState);
          callbacksRef.current.onStateChange?.(newState, prevState, message);
        },
        onVolumeChange: (vol) => {
          setVolume(vol);
          callbacksRef.current.onVolumeChange?.(vol);
        },
        onActivityStart: () => {
          callbacksRef.current.onActivityStart?.();
        },
        onActivityEnd: () => {
          callbacksRef.current.onActivityEnd?.();
        },
        onError: (msg, code) => {
          setError(msg);
          callbacksRef.current.onError?.(msg, code);
        },
        onNoSpeechDetected: () => {
          callbacksRef.current.onNoSpeechDetected?.();
        },
        onAudioFile: (file, blob) => {
          callbacksRef.current.onAudioFile?.(file, blob);
        },
        onRecordingCancelled: () => {
          setInterimText('');
          callbacksRef.current.onRecordingCancelled?.();
        },
      });

      await sttRef.current.init();

      // Guard: component may have unmounted during async init
      if (!sttRef.current) {
        isInitializingRef.current = false;
        setIsInitializing(false);
        return false;
      }
      
      setIsWebSpeechSupported(sttRef.current.isWebSpeechSupported);
      setIsWhisperSupported(sttRef.current.isWhisperSupported);
      
      // Atualiza ref ANTES do state para evitar race condition
      isInitializedRef.current = true;
      isInitializingRef.current = false;
      setIsInitialized(true);
      setIsInitializing(false);

      return true;
    } catch (err) {
      const errorMessage = err instanceof Error ? err.message : 'Erro ao inicializar STT';
      setError(errorMessage);
      isInitializingRef.current = false;
      setIsInitializing(false);
      console.error('[useSTT] Erro:', err);
      return false;
    }
  }, [configOptions]);

  // Inicia gravação
  const startRecording = useCallback(async (): Promise<boolean> => {
    if (!sttRef.current || !isInitializedRef.current) {
      console.warn('[useSTT] STT não inicializado');
      return false;
    }
    return sttRef.current.startRecording();
  }, []);

  // Para gravação
  const stopRecording = useCallback(() => {
    sttRef.current?.stopRecording();
  }, [isInitialized]);

  // Cancela gravação
  const cancelRecording = useCallback(() => {
    sttRef.current?.cancelRecording();
    setInterimText('');
  }, []);

  // Toggle gravação
  const toggleRecording = useCallback(() => {
    sttRef.current?.toggleRecording();
  }, []);

  // Define provider
  const setProvider = useCallback((prov: STTProvider) => {
    sttRef.current?.setProvider(prov);
    setProviderState(prov);
  }, []);

  // Define modo
  const setMode = useCallback((m: RecordingMode) => {
    sttRef.current?.setMode(m);
    setModeState(m);
  }, []);

  // Define idioma
  const setLanguage = useCallback((lang: string) => {
    sttRef.current?.setLanguage(lang);
  }, []);

  // Atualiza configuração
  const updateConfig = useCallback((config: Partial<STTConfig>) => {
    sttRef.current?.updateConfig(config);
  }, []);

  // Libera recursos
  const destroy = useCallback(() => {
    sttRef.current?.destroy();
    sttRef.current = null;
    setState(STT_STATES.IDLE);
    setVolume(0);
    setInterimText('');
    setIsInitialized(false);
    setError(null);
  }, []);

  // Auto-init
  useEffect(() => {
    if (autoInit && !isInitialized && !isInitializing) {
      init();
    }
  }, [autoInit, isInitialized, isInitializing, init]);

  // Cleanup ao desmontar
  useEffect(() => {
    return () => {
      sttRef.current?.destroy();
      sttRef.current = null;
    };
  }, []);

  // Computed states
  const isIdle = state === STT_STATES.IDLE;
  const isListening = state === STT_STATES.LISTENING;
  const isRecording = state === STT_STATES.RECORDING;
  const isProcessing = state === STT_STATES.PROCESSING;
  const isError = state === STT_STATES.ERROR;
  const isActive = isRecording || isListening;

  return {
    state,
    provider,
    mode,
    isIdle,
    isListening,
    isRecording,
    isProcessing,
    isError,
    isActive,
    volume,
    interimText,
    isInitialized,
    isInitializing,
    error,
    isWebSpeechSupported,
    isWhisperSupported,
    init,
    startRecording,
    stopRecording,
    cancelRecording,
    toggleRecording,
    setProvider,
    setMode,
    setLanguage,
    updateConfig,
    destroy,
  };
}

export default useSTT;
