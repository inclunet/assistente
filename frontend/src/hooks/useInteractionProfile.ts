/**
 * useInteractionProfile - Hook de Orquestração de Interação por Voz
 * 
 * Integra todos os serviços de voz (VAD, STT, Hotkeys, TTS) baseado no
 * perfil ativo global. Este é o hook principal para controlar
 * toda a experiência de voz do assistente.
 * 
 * Agora usa o sistema unificado de perfis (profiles.Profile) em vez
 * da antiga interactionProfileStore.
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
import { GetActiveProfile, GetProfile } from '@wailsjs/go/main/App';
import { EventsOn } from '@wailsjs/runtime/runtime';
import { profiles } from '../../wailsjs/go/models';
import { playSound, SOUND_TYPES } from '../services/audioFeedback';
import { ttsService, type RoleVoiceConfig } from '../services/tts';
import { TTSProvider } from '../services/tts/types';

// Tipos re-exportados do novo sistema de perfis
type Profile = profiles.Profile;
type TriggerConfig = profiles.TriggerConfig;
type InputConfig = profiles.InputConfig;

// Singleton para evitar múltiplas instâncias processando o mesmo evento
let hotkeyEventCleanup: (() => void) | null = null;
let hotkeyEventHandler: ((data: unknown) => void) | null = null;

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

	// Se o runtime do Wails não estiver disponível por algum motivo (ex: rodando fora do app),
	// não registra listener.
	if (!EventsOn) return;

	hotkeyEventCleanup = EventsOn('interaction:hotkey:triggered', (data) => {
    // Throttle no frontend - evita processar eventos muito rápidos
    const now = Date.now();
    if (now - lastHotkeyTime < HOTKEY_THROTTLE_MS) {
      return;
    }
    lastHotkeyTime = now;
    
    hotkeyEventHandler?.(data);
  });
}

export interface UseInteractionProfileOptions {
  /** Slug do perfil efetivo (cascata: tab → workspace → global) */
  effectiveProfileSlug?: string | null;

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
  // Estado do perfil (agora é o perfil unificado completo)
  activeProfile: Profile | null;
  
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
  reloadProfile: () => Promise<void>;
  
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
function getVADConfigFromTriggers(triggers?: TriggerConfig[]): {
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
  const vadTrigger = triggers.find((t: TriggerConfig) => 
    t.enabled && (t.auto_stop || t.type === 'vad')
  );

  if (!vadTrigger) {
    // Se não tem trigger com VAD, verifica se tem button_ptt
    const pttTrigger = triggers.find((t: TriggerConfig) => t.enabled && t.type === 'button_ptt');
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
function getWakewordConfigFromTriggers(triggers?: TriggerConfig[]): {
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
  
  const wakewordTrigger = triggers.find((t: TriggerConfig) => t.enabled && t.type === 'wakeword');
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

// Helpers para acessar input config do perfil unificado
function getInputConfig(profile: Profile | null): InputConfig | null {
  return profile?.input ?? null;
}

function getTriggers(profile: Profile | null): TriggerConfig[] | undefined {
  return profile?.input?.triggers;
}

export function useInteractionProfile(options: UseInteractionProfileOptions = {}): UseInteractionProfileReturn {
  const {
    effectiveProfileSlug,
    onTranscription,
    onActivityStart,
    onActivityEnd,
    onHotkeyActivation,
    onWakeWordDetected,
    onError,
  } = options;

  // Estado local (substitui a store antiga)
  const [activeProfile, setActiveProfileState] = useState<Profile | null>(null);
  const [isLoading, setIsLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [isActive, setIsActive] = useState(false);
  const [interimText, setInterimText] = useState('');
  const [localVolume, setLocalVolume] = useState(0);
  const [localIsListening, setLocalIsListening] = useState(false);
  
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

  // Carrega o perfil efetivo: por slug (tab/workspace) ou global ativo
  const loadActiveProfile = useCallback(async () => {
    setIsLoading(true);
    setError(null);
    try {
      const profile = effectiveProfileSlug
        ? await GetProfile(effectiveProfileSlug)
        : await GetActiveProfile();
      setActiveProfileState(profile);
    } catch (e) {
      console.error('[useInteractionProfile] Erro ao carregar perfil:', e);
      setError(e instanceof Error ? e.message : 'Erro ao carregar perfil');
    } finally {
      setIsLoading(false);
    }
  }, [effectiveProfileSlug]);

  // Input config do perfil ativo
  const inputConfig = getInputConfig(activeProfile);
  const triggers = getTriggers(activeProfile);

  // Mapeia STT provider do perfil para o formato do useSTT
  const mapSTTProvider = (provider: string | undefined): 'webspeech' | 'whisper_api' => {
    if (provider === 'whisper_api') return provider;
    return 'webspeech';
  };

  // Extrai configurações dos triggers
  const vadConfig = getVADConfigFromTriggers(triggers);
  const wakewordConfig = getWakewordConfigFromTriggers(triggers);
  const hasWakewordTrigger = wakewordConfig?.hasWakeword || false;

  // Hook de STT
  const {
    isListening: sttListening,
    isRecording: sttRecording,
    isProcessing: sttProcessing,
    isInitialized: sttInitialized,
    volume,
    interimText: sttInterimText,
    startRecording,
    stopRecording,
    cancelRecording,
    setMode,
    setProvider,
    setLanguage,
    updateConfig,
    init: initSTT,
  } = useSTT({
    provider: mapSTTProvider(inputConfig?.stt_provider),
    language: inputConfig?.language || 'pt-BR',
    mode: vadConfig.mode,
    silenceDuration: vadConfig.silenceDuration,
    silenceThreshold: vadConfig.silenceThreshold,
    activityThreshold: vadConfig.activityThreshold,
    activityDuration: vadConfig.activityDuration,
    
    onTranscription: (text, provider) => {
      setIsActive(false);
      setInterimText('');
      
      // Feedback sonoro
      if (inputConfig?.feedback_sounds) {
        playSound(SOUND_TYPES.RECORD_END);
      }
      
      callbacksRef.current.onTranscription?.(text, provider);
      
      // Se estava em modo wakeword, reinicia escuta após transcrição
      if (shouldRestartWakewordRef.current) {
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
      setLocalVolume(vol);
    },
    
    onError: (message) => {
      setIsActive(false);
      callbacksRef.current.onError?.(message);
    },
    
    onNoSpeechDetected: () => {
      setIsActive(false);
      if (inputConfig?.feedback_sounds) {
        playSound(SOUND_TYPES.ERROR);
      }
    },
    
    onRecordingCancelled: () => {
      setIsActive(false);
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
    language: inputConfig?.language || 'pt-BR',
    sensitivity: wakewordConfig?.sensitivity || 0.5,
    onDetected: async (keyword, _fullText) => {
      // Para a escuta de wakeword temporariamente
      wakewordStopListening();
      
      // Notifica callback
      callbacksRef.current.onWakeWordDetected?.(keyword);
      
      // Feedback sonoro
      if (inputConfig?.feedback_sounds) {
        playSound(SOUND_TYPES.RECORD_START);
      }
      
      // Inicia gravação real
      setIsActive(true);
      
      // Garante que STT está inicializado
      if (!sttInitialized) {
        const success = await initSTT();
        if (!success) {
          console.error('[useInteractionProfile] Falha ao inicializar STT');
          callbacksRef.current.onError?.('Falha ao inicializar reconhecimento de voz');
          setIsActive(false);
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

  // Atualiza config de STT quando perfil muda
  useEffect(() => {
    if (inputConfig) {
      const config = getVADConfigFromTriggers(inputConfig.triggers);
      setProvider(mapSTTProvider(inputConfig.stt_provider));
      setLanguage(inputConfig.language || 'pt-BR');
      setMode(config.mode);
      updateConfig({
        silenceDuration: config.silenceDuration,
        silenceThreshold: config.silenceThreshold,
        activityThreshold: config.activityThreshold,
        activityDuration: config.activityDuration,
      });
    }
  }, [inputConfig, setProvider, setLanguage, setMode, updateConfig]);

  // Sincroniza configurações de TTS quando o perfil ativo muda
  // O ttsService é a única fonte de verdade para TTS — sem intermediários
  useEffect(() => {
    if (!activeProfile) return;

    const assistantVoice = activeProfile.voice?.assistant;
    const isDisabled = !assistantVoice || !assistantVoice.enabled || assistantVoice.provider === 'disabled';

    const syncTTS = async () => {
      if (isDisabled) {
        ttsService.setEnabled(false);
        ttsService.setAutoRead(false);
        ttsService.setEnabledForUser(false);
        return;
      }

      // Mapeia provider do perfil para TTSProvider enum
      const mapTTSProvider = (provider: string): TTSProvider => {
        switch (provider) {
          case 'webspeech': return TTSProvider.WEBSPEECH;
          case 'sapi5': return TTSProvider.SAPI5;
          case 'openai': return TTSProvider.OPENAI;
          default: return TTSProvider.WEBSPEECH;
        }
      };

      // Resolve o providerId efetivo para uso com speakAsRole
      // "openai" + llm_provider_id → usa o llm_provider_id diretamente
      // "webspeech" / "sapi5" → usa como está
      const resolveProviderId = (voice: { provider?: string; llm_provider_id?: string } | undefined): string => {
        if (!voice) return 'webspeech';
        if (voice.provider === 'openai' && voice.llm_provider_id) return voice.llm_provider_id;
        return voice.provider || 'webspeech';
      };

      // Ativa e configura flags globais
      ttsService.setEnabled(true);
      ttsService.setAutoRead(assistantVoice.enabled);
      ttsService.setEnabledForUser(activeProfile.voice?.user?.enabled || false);

      // Configura provider e voz padrão (assistant) para compatibilidade
      const ttsProvider = mapTTSProvider(assistantVoice.provider);
      await ttsService.setProvider(ttsProvider);

      if (assistantVoice.voice_id) {
        await ttsService.setVoice(assistantVoice.voice_id);
      }

      await ttsService.setRate(assistantVoice.rate || 1.0);
      ttsService.setPitch(assistantVoice.pitch || 1.0);
      await ttsService.setVolume(assistantVoice.volume || 1.0);

      // Configura voice configs por role para speakAsRole()
      const makeRoleConfig = (voice: { provider?: string; llm_provider_id?: string; voice_id?: string; model?: string; rate?: number; volume?: number } | undefined): RoleVoiceConfig => ({
        providerId: resolveProviderId(voice),
        voiceId: voice?.voice_id || '',
        model: voice?.model || '',
        rate: voice?.rate || 1.0,
        volume: voice?.volume || 1.0,
      });

      ttsService.setRoleConfig('assistant', makeRoleConfig(assistantVoice));

      const userVoice = activeProfile.voice?.user;
      if (userVoice?.enabled) {
        ttsService.setRoleConfig('user', makeRoleConfig(userVoice));
      }

      const systemVoice = activeProfile.voice?.system;
      if (systemVoice?.enabled) {
        ttsService.setRoleConfig('system', makeRoleConfig(systemVoice));
      }
    };

    syncTTS().catch((err) => {
      console.error('[useInteractionProfile] Erro ao sincronizar TTS:', err);
    });
  }, [activeProfile]);

  // Sincroniza estado de wakeword listening
  useEffect(() => {
    setLocalIsListening(wakewordIsListening);
  }, [wakewordIsListening]);

  // (estado de gravação/processamento vem direto do useSTT hook via sttRecording/sttProcessing)

  // Funções de controle de wakeword
  const startWakewordListening = useCallback(() => {
    shouldRestartWakewordRef.current = true;
    wakewordStartListening();
    
    if (inputConfig?.feedback_sounds) {
      playSound(SOUND_TYPES.RECORD_START);
    }
  }, [wakewordStartListening, inputConfig]);

  const stopWakewordListening = useCallback(() => {
    shouldRestartWakewordRef.current = false;
    wakewordStopListening();
    
    if (inputConfig?.feedback_sounds) {
      playSound(SOUND_TYPES.RECORD_END);
    }
  }, [wakewordStopListening, inputConfig]);

  const toggleWakewordListening = useCallback(() => {
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
      const success = await initSTT();
      if (!success) {
        console.error('[useInteractionProfile] Falha ao inicializar STT');
        callbacksRef.current.onError?.('Falha ao inicializar reconhecimento de voz');
        return;
      }
    }
    
    setIsActive(true);
    
    // Feedback sonoro
    if (inputConfig?.feedback_sounds) {
      playSound(SOUND_TYPES.RECORD_START);
    }
    
    await startRecording();
  }, [isActive, inputConfig, startRecording, sttInitialized, initSTT]);

  // Para interação
  const stopInteraction = useCallback(() => {
    stopRecording();
    // isActive será setado para false quando onTranscription for chamado
  }, [stopRecording]);

  // Cancela interação
  const cancelInteraction = useCallback(() => {
    cancelRecording();
    setIsActive(false);
    setInterimText('');
    
    if (inputConfig?.feedback_sounds) {
      playSound(SOUND_TYPES.ERROR);
    }
  }, [cancelRecording, inputConfig]);

  // Toggle interação - usa sttRecording (estado real) em vez de isActive (estado local)
  // Para modo wakeword, faz toggle da escuta de wakeword (não da gravação)
  const toggleInteraction = useCallback(() => {
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

  // Registra handler de hotkey (usando singleton global)
  useEffect(() => {
    
    
    // Registra este handler como o ativo
    registerGlobalHotkeyHandler((data) => {
      const eventData = data as { 
        triggerId: number;
        profileId: number; 
        triggerType: string;
        bringToFront: boolean;
      };
      callbacksRef.current.onHotkeyActivation?.(eventData.bringToFront);
      
      // Usa ref para garantir versão mais atual da função (evita closure stale)
      toggleInteractionRef.current?.();
    });
    
    // Garante que o listener está ativo
    ensureHotkeyListener();

    return () => undefined;
  }, []); // Registra uma vez por instância

  // Para escuta de wakeword quando perfil muda ou componente desmonta
  useEffect(() => {
    return () => {
      if (wakewordIsListening) {
        wakewordStopListening();
      }
    };
  }, [wakewordIsListening, wakewordStopListening]);

  // Carrega perfil ativo ao montar e ouve mudanças de perfil
  useEffect(() => {
    loadActiveProfile();

    // Escuta eventos de mudança de perfil
    const unsubs: Array<() => void> = [];
    
    const setupListener = async () => {
      try {
        unsubs.push(EventsOn('profile:changed', () => {
          loadActiveProfile();
        }));
        unsubs.push(EventsOn('profile:updated', () => {
          loadActiveProfile();
        }));
      } catch (err) {
        console.warn('[useInteractionProfile] Falha ao registrar listener profile:changed:', err);
      }
    };

    setupListener();

    return () => {
      unsubs.forEach((unsub) => unsub());
    };
  }, [loadActiveProfile]);

  return {
    // Perfil
    activeProfile,
    
    // Estados
    isActive: isActive || wakewordIsListening,
    isListening: sttListening || wakewordIsListening || localIsListening,
    isRecording: sttRecording,
    isProcessing: sttProcessing,
    
    // Dados
    volume: volume || localVolume,
    interimText: interimText || sttInterimText || (wakewordIsListening ? wakewordLastText : ''),
    
    // Ações
    startInteraction,
    stopInteraction,
    cancelInteraction,
    toggleInteraction,
    reloadProfile: loadActiveProfile,
    
    // Wakeword específico
    startWakewordListening,
    stopWakewordListening,
    toggleWakewordListening,
    isWakewordListening: wakewordIsListening,
    
    // Estados de carregamento
    isLoading,
    error,
  };
}

export default useInteractionProfile;
