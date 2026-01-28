/**
 * useInteractionProfile - Hook de Orquestração de Interação por Voz
 * 
 * Integra todos os serviços de voz (VAD, STT, Hotkeys, TTS) baseado no
 * perfil de interação ativo. Este é o hook principal para controlar
 * toda a experiência de voz do assistente.
 * 
 * @example
 * ```tsx
 * function VoiceControls() {
 *   const {
 *     isActive,
 *     isRecording,
 *     volume,
 *     interimText,
 *     activeProfile,
 *     startInteraction,
 *     stopInteraction,
 *   } = useInteractionProfile({
 *     onTranscription: (text) => sendMessage(text),
 *   });
 * 
 *   return (
 *     <VoiceButton
 *       isActive={isActive}
 *       isRecording={isRecording}
 *       volume={volume}
 *       onPointerDown={startInteraction}
 *       onPointerUp={stopInteraction}
 *     />
 *   );
 * }
 * ```
 */

import { useCallback, useEffect, useRef, useState } from 'react';
import { useSTT } from './useSTT';
import { useWakewordDetection } from './useWakewordDetection';
import { 
  useInteractionProfileStore, 
  InteractionProfile,
  InteractionTrigger,
} from '../store/interactionProfileStore';
import { playSound, SOUND_TYPES } from '../services/audioFeedback';

// Import dinâmico do Wails para eventos
let EventsOn: ((event: string, callback: (...args: unknown[]) => void) => () => void) | null = null;

async function loadWailsFunctions() {
  try {
    const events = await import('../../wailsjs/runtime/runtime');
    EventsOn = events.EventsOn;
    return true;
  } catch (e) {
    console.warn('[useInteractionProfile] Wails não disponível:', e);
    return false;
  }
}

// Singleton para evitar múltiplas instâncias processando o mesmo evento
let hotkeyEventCleanup: (() => void) | null = null;
let hotkeyEventHandler: ((data: unknown) => void) | null = null;
let instanceCount = 0;

// Throttle no frontend (1 segundo)
let lastHotkeyTime = 0;
const HOTKEY_THROTTLE_MS = 1000;

// Registra handler global de hotkey (singleton)
function registerGlobalHotkeyHandler(handler: (data: unknown) => void): void {
  hotkeyEventHandler = handler;
}

// Configura listener de eventos uma única vez (singleton)
async function ensureHotkeyListener(): Promise<void> {
  if (hotkeyEventCleanup) return; // Já registrado
  
  await loadWailsFunctions();
  if (!EventsOn) return;
  
  hotkeyEventCleanup = EventsOn('interaction:hotkey:triggered', (data) => {
    // Throttle no frontend - evita processar eventos muito rápidos
    const now = Date.now();
    if (now - lastHotkeyTime < HOTKEY_THROTTLE_MS) {
      console.log('[useInteractionProfile] Hotkey bloqueado por throttle frontend');
      return;
    }
    lastHotkeyTime = now;
    
    hotkeyEventHandler?.(data);
  });
}

export interface UseInteractionProfileOptions {
  /** Callback quando transcrição é finalizada */
  onTranscription?: (text: string, provider: string) => void;
  
  /** Callback quando atividade de voz começa */
  onActivityStart?: () => void;
  
  /** Callback quando atividade de voz termina */
  onActivityEnd?: () => void;
  
  /** Callback quando interação é ativada via hotkey */
  onHotkeyActivation?: (bringToFront: boolean) => void;
  
  /** Callback quando wake word é detectado */
  onWakeWordDetected?: (keyword: string) => void;
  
  /** Callback de erro */
  onError?: (message: string) => void;
}

export interface UseInteractionProfileReturn {
  // Estado do perfil
  activeProfile: InteractionProfile | null;
  profiles: InteractionProfile[];
  
  // Estados de interação
  isActive: boolean;        // Se está em modo ativo (gravando ou ouvindo wakeword)
  isListening: boolean;     // Se está ouvindo (VAD/wakeword)
  isRecording: boolean;     // Se está gravando
  isProcessing: boolean;    // Se está processando transcrição
  
  // Dados de áudio
  volume: number;           // Volume atual (0-1)
  interimText: string;      // Texto parcial durante gravação
  
  // Ações
  startInteraction: () => Promise<void>;
  stopInteraction: () => void;
  cancelInteraction: () => void;
  toggleInteraction: () => void;
  
  // Gestão de perfis
  setActiveProfile: (profileId: number) => Promise<void>;
  loadProfiles: () => Promise<void>;
  
  // Wakeword específico
  startWakewordListening: () => void;
  stopWakewordListening: () => void;
  toggleWakewordListening: () => void;
  isWakewordListening: boolean;
  
  // Estados de carregamento
  isLoading: boolean;
  error: string | null;
}

// Extrai configurações VAD do primeiro trigger que use VAD
function getVADConfigFromTriggers(triggers?: InteractionTrigger[]): {
  silenceDuration: number;
  silenceThreshold: number;
  activityThreshold: number;
  activityDuration: number;
  mode: 'ptt' | 'toggle' | 'vad_silence' | 'vad_activity';
} {
  // Valores padrão
  const defaults = {
    silenceDuration: 1500,
    silenceThreshold: 0.01,
    activityThreshold: 0.02,
    activityDuration: 200,
    mode: 'toggle' as const,
  };

  if (!triggers || triggers.length === 0) return defaults;

  // Procura por um trigger que tenha configurações VAD
  const vadTrigger = triggers.find(t => 
    t.enabled && (t.auto_stop || t.type === 'vad')
  );

  if (!vadTrigger) {
    // Se não tem trigger com VAD, verifica se tem button_ptt
    const pttTrigger = triggers.find(t => t.enabled && t.type === 'button_ptt');
    if (pttTrigger) {
      return { ...defaults, mode: 'ptt' };
    }
    return defaults;
  }

  // Determina o modo baseado no trigger
  let mode: 'ptt' | 'toggle' | 'vad_silence' | 'vad_activity' = 'toggle';
  if (vadTrigger.type === 'button_ptt') {
    mode = 'ptt';
  } else if (vadTrigger.type === 'vad') {
    mode = 'vad_activity';
  } else if (vadTrigger.auto_stop) {
    mode = 'vad_silence';
  }

  return {
    silenceDuration: vadTrigger.vad_silence_duration || defaults.silenceDuration,
    silenceThreshold: vadTrigger.vad_silence_threshold || defaults.silenceThreshold,
    activityThreshold: vadTrigger.vad_activity_threshold || defaults.activityThreshold,
    activityDuration: vadTrigger.vad_activity_duration || defaults.activityDuration,
    mode,
  };
}

// Extrai configuração de wakeword dos triggers
function getWakewordConfigFromTriggers(triggers?: InteractionTrigger[]): {
  hasWakeword: boolean;
  keyword: string;
  sensitivity: number;
  autoStop: boolean;
  vadConfig: {
    silenceDuration: number;
    silenceThreshold: number;
  };
} | null {
  if (!triggers) return null;
  
  const wakewordTrigger = triggers.find(t => t.enabled && t.type === 'wakeword');
  if (!wakewordTrigger) return null;
  
  return {
    hasWakeword: true,
    keyword: wakewordTrigger.wakeword_keyword || 'assistente',
    sensitivity: wakewordTrigger.wakeword_sensitivity || 0.5,
    autoStop: wakewordTrigger.auto_stop || false,
    vadConfig: {
      silenceDuration: wakewordTrigger.vad_silence_duration || 1500,
      silenceThreshold: wakewordTrigger.vad_silence_threshold || 0.01,
    },
  };
}

export function useInteractionProfile(options: UseInteractionProfileOptions = {}): UseInteractionProfileReturn {
  const {
    onTranscription,
    onActivityStart,
    onActivityEnd,
    onHotkeyActivation,
    onWakeWordDetected,
    onError,
  } = options;

  // Store
  const {
    profiles,
    isLoading: storeLoading,
    error: storeError,
    loadProfiles,
    setActiveProfile: storeSetActiveProfile,
    getActiveProfile,
    setRecording,
    setProcessing,
    setListening,
    setVolume: storeSetVolume,
  } = useInteractionProfileStore();

  // Estado local
  const [isActive, setIsActive] = useState(false);
  const [interimText, setInterimText] = useState('');
  // Nota: isWakewordListening vem do hook useWakewordDetection (não precisa de estado local)
  
  // Refs para callbacks
  const callbacksRef = useRef({
    onTranscription,
    onActivityStart,
    onActivityEnd,
    onHotkeyActivation,
    onWakeWordDetected,
    onError,
  });

  // Ref para toggle interaction (será populada depois que a função for criada)
  const toggleInteractionRef = useRef<(() => void) | null>(null);
  // Ref para controle de wakeword (indica se deve reiniciar escuta após gravação)
  const shouldRestartWakewordRef = useRef(false);

  useEffect(() => {
    callbacksRef.current = {
      onTranscription,
      onActivityStart,
      onActivityEnd,
      onHotkeyActivation,
      onWakeWordDetected,
      onError,
    };
  }, [onTranscription, onActivityStart, onActivityEnd, onHotkeyActivation, onWakeWordDetected, onError]);

  // Perfil ativo
  const activeProfile = getActiveProfile();

  // Mapeia STT provider do perfil para o formato do useSTT
  const mapSTTProvider = (provider: string | undefined): 'webspeech' | 'whisper_api' => {
    if (provider === 'whisper_api') return provider;
    return 'webspeech';
  };

  // Extrai configurações dos triggers
  const vadConfig = getVADConfigFromTriggers(activeProfile?.triggers);
  const wakewordConfig = getWakewordConfigFromTriggers(activeProfile?.triggers);
  const hasWakewordTrigger = wakewordConfig?.hasWakeword || false;

  // Hook de STT
  const {
    isListening: sttListening,
    isRecording: sttRecording,
    isProcessing: sttProcessing,
    isInitialized: sttInitialized,
    isInitializing: sttInitializing,
    volume,
    interimText: sttInterimText,
    startRecording,
    stopRecording,
    cancelRecording,
    setMode,
    setProvider,
    updateConfig,
    init: initSTT,
  } = useSTT({
    provider: mapSTTProvider(activeProfile?.stt_provider),
    mode: vadConfig.mode,
    silenceDuration: vadConfig.silenceDuration,
    silenceThreshold: vadConfig.silenceThreshold,
    activityThreshold: vadConfig.activityThreshold,
    activityDuration: vadConfig.activityDuration,
    
    onTranscription: (text, provider) => {
      setIsActive(false);
      setRecording(false);
      setInterimText('');
      
      // Feedback sonoro
      if (activeProfile?.feedback_sounds) {
        playSound(SOUND_TYPES.RECORD_END);
      }
      
      callbacksRef.current.onTranscription?.(text, provider);
      
      // Se estava em modo wakeword, reinicia escuta após transcrição
      if (shouldRestartWakewordRef.current) {
        console.log('[useInteractionProfile] Reiniciando escuta de wakeword após transcrição');
        setTimeout(() => {
          startWakewordListeningRef.current?.();
        }, 500); // Pequeno delay para evitar capturar eco do TTS
      }
    },
    
    onInterimResult: (text) => {
      setInterimText(text);
    },
    
    onActivityStart: () => {
      callbacksRef.current.onActivityStart?.();
    },
    
    onActivityEnd: () => {
      callbacksRef.current.onActivityEnd?.();
    },
    
    onVolumeChange: (vol) => {
      storeSetVolume(vol);
    },
    
    onError: (message) => {
      setIsActive(false);
      setRecording(false);
      callbacksRef.current.onError?.(message);
    },
    
    onNoSpeechDetected: () => {
      setIsActive(false);
      setRecording(false);
      if (activeProfile?.feedback_sounds) {
        playSound(SOUND_TYPES.ERROR);
      }
    },
    
    onRecordingCancelled: () => {
      setIsActive(false);
      setRecording(false);
      setInterimText('');
    },
  });

  // Ref para startWakewordListening (para usar no callback de onTranscription)
  const startWakewordListeningRef = useRef<(() => void) | null>(null);

  // Hook de detecção de wakeword
  const {
    isListening: wakewordIsListening,
    startListening: wakewordStartListening,
    stopListening: wakewordStopListening,
    lastRecognizedText: wakewordLastText,
  } = useWakewordDetection({
    keyword: wakewordConfig?.keyword || 'assistente',
    language: activeProfile?.language || 'pt-BR',
    sensitivity: wakewordConfig?.sensitivity || 0.5,
    onDetected: async (keyword, fullText) => {
      console.log('[useInteractionProfile] 🎯 Wakeword detected:', keyword, 'in:', fullText);
      
      // Para a escuta de wakeword temporariamente
      wakewordStopListening();
      
      // Notifica callback
      callbacksRef.current.onWakeWordDetected?.(keyword);
      
      // Feedback sonoro
      if (activeProfile?.feedback_sounds) {
        playSound(SOUND_TYPES.RECORD_START);
      }
      
      // Inicia gravação real
      setIsActive(true);
      setRecording(true);
      
      // Garante que STT está inicializado
      if (!sttInitialized) {
        console.log('[useInteractionProfile] STT não inicializado, inicializando...');
        const success = await initSTT();
        if (!success) {
          console.error('[useInteractionProfile] Falha ao inicializar STT');
          callbacksRef.current.onError?.('Falha ao inicializar reconhecimento de voz');
          setIsActive(false);
          setRecording(false);
          return;
        }
      }
      
      await startRecording();
    },
    onError: (message) => {
      console.error('[useInteractionProfile] Wakeword error:', message);
      callbacksRef.current.onError?.(message);
    },
  });

  // Atualiza config quando perfil muda
  useEffect(() => {
    if (activeProfile) {
      const config = getVADConfigFromTriggers(activeProfile.triggers);
      setProvider(mapSTTProvider(activeProfile.stt_provider));
      setMode(config.mode);
      updateConfig({
        silenceDuration: config.silenceDuration,
        silenceThreshold: config.silenceThreshold,
        activityThreshold: config.activityThreshold,
        activityDuration: config.activityDuration,
      });
    }
  }, [activeProfile, setProvider, setMode, updateConfig]);

  // Sincroniza estado de wakeword listening com a store
  useEffect(() => {
    setListening(wakewordIsListening);
  }, [wakewordIsListening, setListening]);

  // Sincroniza estado
  useEffect(() => {
    setRecording(sttRecording);
    setProcessing(sttProcessing);
  }, [sttRecording, sttProcessing, setRecording, setProcessing]);

  // Funções de controle de wakeword
  const startWakewordListening = useCallback(() => {
    console.log('[useInteractionProfile] Starting wakeword listening');
    shouldRestartWakewordRef.current = true;
    wakewordStartListening();
    
    if (activeProfile?.feedback_sounds) {
      playSound(SOUND_TYPES.RECORD_START);
    }
  }, [wakewordStartListening, activeProfile]);

  const stopWakewordListening = useCallback(() => {
    console.log('[useInteractionProfile] Stopping wakeword listening');
    shouldRestartWakewordRef.current = false;
    wakewordStopListening();
    
    if (activeProfile?.feedback_sounds) {
      playSound(SOUND_TYPES.RECORD_END);
    }
  }, [wakewordStopListening, activeProfile]);

  const toggleWakewordListening = useCallback(() => {
    console.log('[useInteractionProfile] Toggle wakeword:', { isListening: wakewordIsListening });
    if (wakewordIsListening) {
      stopWakewordListening();
    } else {
      startWakewordListening();
    }
  }, [wakewordIsListening, startWakewordListening, stopWakewordListening]);

  // Atualiza ref para uso no callback de onTranscription
  useEffect(() => {
    startWakewordListeningRef.current = startWakewordListening;
  }, [startWakewordListening]);

  // Inicia interação
  const startInteraction = useCallback(async () => {
    if (isActive) return;
    
    // Garante que o STT está inicializado (initSTT já verifica internamente se já está inicializado)
    if (!sttInitialized) {
      console.log('[useInteractionProfile] STT não inicializado, inicializando...');
      const success = await initSTT();
      if (!success) {
        console.error('[useInteractionProfile] Falha ao inicializar STT');
        callbacksRef.current.onError?.('Falha ao inicializar reconhecimento de voz');
        return;
      }
      console.log('[useInteractionProfile] STT inicializado com sucesso');
    }
    
    setIsActive(true);
    setRecording(true);
    
    // Feedback sonoro
    if (activeProfile?.feedback_sounds) {
      playSound(SOUND_TYPES.RECORD_START);
    }
    
    await startRecording();
  }, [isActive, activeProfile, startRecording, setRecording, sttInitialized, initSTT]);

  // Para interação
  const stopInteraction = useCallback(() => {
    stopRecording();
    // isActive será setado para false quando onTranscription for chamado
  }, [stopRecording, isActive, sttRecording]);

  // Cancela interação
  const cancelInteraction = useCallback(() => {
    cancelRecording();
    setIsActive(false);
    setRecording(false);
    setInterimText('');
    
    if (activeProfile?.feedback_sounds) {
      playSound(SOUND_TYPES.ERROR);
    }
  }, [cancelRecording, activeProfile, setRecording]);

  // Toggle interação - usa sttRecording (estado real) em vez de isActive (estado local)
  // Para modo wakeword, faz toggle da escuta de wakeword (não da gravação)
  const toggleInteraction = useCallback(() => {
    console.log('[useInteractionProfile] Toggle:', { 
      isActive, 
      sttRecording, 
      hasWakewordTrigger, 
      wakewordIsListening 
    });
    
    // Se tem trigger de wakeword, faz toggle da escuta de wakeword
    if (hasWakewordTrigger) {
      toggleWakewordListening();
      return;
    }
    
    // Para outros modos, faz toggle da gravação
    if (sttRecording) {
      stopInteraction();
    } else {
      startInteraction();
    }
  }, [sttRecording, hasWakewordTrigger, wakewordIsListening, startInteraction, stopInteraction, toggleWakewordListening]);

  // Atualiza ref com a versão mais atual de toggleInteraction (evita closure stale em eventos)
  useEffect(() => {
    toggleInteractionRef.current = toggleInteraction;
  }, [toggleInteraction]);

  // Define perfil ativo (store já cuida de registrar hotkeys no backend)
  const setActiveProfile = useCallback(async (profileId: number) => {
    await storeSetActiveProfile(profileId);
  }, [storeSetActiveProfile]);

  // Registra handler de hotkey (usando singleton global)
  useEffect(() => {
    instanceCount++;
    console.log('[useInteractionProfile] Instance mounted, count:', instanceCount);
    
    // Registra este handler como o ativo
    registerGlobalHotkeyHandler((data) => {
      const eventData = data as { 
        triggerId: number;
        profileId: number; 
        triggerType: string;
        bringToFront: boolean;
      };
      console.log('[useInteractionProfile] Hotkey triggered:', eventData);
      
      callbacksRef.current.onHotkeyActivation?.(eventData.bringToFront);
      
      // Usa ref para garantir versão mais atual da função (evita closure stale)
      toggleInteractionRef.current?.();
    });
    
    // Garante que o listener está ativo
    ensureHotkeyListener();

    return () => {
      instanceCount--;
      console.log('[useInteractionProfile] Instance unmounted, count:', instanceCount);
    };
  }, []); // Registra uma vez por instância

  // Para escuta de wakeword quando perfil muda ou componente desmonta
  useEffect(() => {
    return () => {
      if (wakewordIsListening) {
        wakewordStopListening();
      }
    };
  }, [wakewordIsListening, wakewordStopListening]);

  // Carrega perfis ao montar
  useEffect(() => {
    loadProfiles();
  }, [loadProfiles]);

  return {
    // Perfil
    activeProfile,
    profiles,
    
    // Estados
    isActive: isActive || wakewordIsListening,
    isListening: sttListening || wakewordIsListening || useInteractionProfileStore.getState().isListening,
    isRecording: sttRecording,
    isProcessing: sttProcessing,
    
    // Dados
    volume,
    interimText: interimText || sttInterimText || (wakewordIsListening ? wakewordLastText : ''),
    
    // Ações
    startInteraction,
    stopInteraction,
    cancelInteraction,
    toggleInteraction,
    setActiveProfile,
    loadProfiles,
    
    // Wakeword específico
    startWakewordListening,
    stopWakewordListening,
    toggleWakewordListening,
    isWakewordListening: wakewordIsListening,
    
    // Estados de carregamento
    isLoading: storeLoading,
    error: storeError,
  };
}

export default useInteractionProfile;
