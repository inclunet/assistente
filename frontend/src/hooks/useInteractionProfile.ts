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
  isActive: boolean;        // Se está em modo ativo (gravando ou ouvindo)
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
    t.enabled && (t.auto_stop || t.type === 'wakeword' || t.type === 'vad')
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
  const mapSTTProvider = (provider: string | undefined): 'webspeech' | 'whisper_api' | 'vosk' => {
    if (provider === 'whisper_api' || provider === 'vosk') return provider;
    return 'webspeech';
  };

  // Extrai configurações dos triggers
  const vadConfig = getVADConfigFromTriggers(activeProfile?.triggers);

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

  // Sincroniza estado
  useEffect(() => {
    setRecording(sttRecording);
    setProcessing(sttProcessing);
  }, [sttRecording, sttProcessing, setRecording, setProcessing]);

  // Inicia interação
  const startInteraction = useCallback(async () => {
    if (isActive) return;
    
    // #region agent log
    fetch('http://127.0.0.1:7242/ingest/c14faa4a-a682-41c0-9f93-65632102ad3e',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({location:'useInteractionProfile:startInteraction',message:'startInteraction called',data:{hasProfile:!!activeProfile,feedbackSounds:activeProfile?.feedback_sounds,profileId:activeProfile?.id,sttInitialized,sttInitializing},timestamp:Date.now(),sessionId:'debug-session',hypothesisId:'H'})}).catch(()=>{});
    // #endregion
    
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
    // #region agent log
    fetch('http://127.0.0.1:7242/ingest/c14faa4a-a682-41c0-9f93-65632102ad3e',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({location:'useInteractionProfile:stopInteraction',message:'stopInteraction called',data:{isActive,sttRecording},timestamp:Date.now(),sessionId:'debug-session',hypothesisId:'G'})}).catch(()=>{});
    // #endregion
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
  const toggleInteraction = useCallback(() => {
    console.log('[useInteractionProfile] Toggle:', { isActive, sttRecording });
    if (sttRecording) {
      stopInteraction();
    } else {
      startInteraction();
    }
  }, [sttRecording, startInteraction, stopInteraction]);

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

  // Escuta eventos de wake word
  useEffect(() => {
    let cleanup: (() => void) | undefined;

    const setupEvents = async () => {
      await loadWailsFunctions();
      
      if (EventsOn) {
        cleanup = EventsOn('interaction:wakeword:detected', (data) => {
          const eventData = data as { triggerId: number; keyword: string };
          console.log('[useInteractionProfile] Wake word detected:', eventData.keyword);
          
          setListening(false);
          callbacksRef.current.onWakeWordDetected?.(eventData.keyword);
          
          // Inicia interação
          startInteraction();
        });
      }
    };

    setupEvents();

    return () => {
      cleanup?.();
    };
  }, [startInteraction, setListening]);

  // Carrega perfis ao montar
  useEffect(() => {
    loadProfiles();
  }, [loadProfiles]);

  return {
    // Perfil
    activeProfile,
    profiles,
    
    // Estados
    isActive,
    isListening: sttListening || useInteractionProfileStore.getState().isListening,
    isRecording: sttRecording,
    isProcessing: sttProcessing,
    
    // Dados
    volume,
    interimText: interimText || sttInterimText,
    
    // Ações
    startInteraction,
    stopInteraction,
    cancelInteraction,
    toggleInteraction,
    setActiveProfile,
    loadProfiles,
    
    // Estados de carregamento
    isLoading: storeLoading,
    error: storeError,
  };
}

export default useInteractionProfile;
