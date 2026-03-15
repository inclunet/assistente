/**
 * useTTS Hook
 * Hook React para integração com o serviço de Text-to-Speech com múltiplos provedores
 */

import { useEffect, useState, useCallback } from 'react';
import { ttsService, TTSConfig } from '../services/tts';
import { TTSVoice } from '../services/tts/types';

interface UseTTSReturn {
  speak: (text: string) => Promise<void>;
  stop: () => void;
  pause: () => void;
  resume: () => void;
  isSpeaking: boolean;
  isEnabled: boolean;
  isAutoReadEnabled: boolean;
  config: TTSConfig;
  voices: TTSVoice[];
  setEnabled: (enabled: boolean) => void;
  setAutoRead: (autoRead: boolean) => void;
  setRate: (rate: number) => Promise<void>;
  setPitch: (pitch: number) => void;
  setVolume: (volume: number) => Promise<void>;
  setVoice: (voiceName: string) => Promise<void>;
  isSupported: boolean;
  reloadVoices: () => Promise<void>;
}

export function useTTS(): UseTTSReturn {
  const [isSpeaking, setIsSpeaking] = useState(false);
  const [config, setConfig] = useState<TTSConfig>(ttsService.getConfig());
  const [voices, setVoices] = useState<TTSVoice[]>([]);
  
  useEffect(() => {
    // Carrega vozes de todos os provedores
    const loadVoices = async () => {
      try {
        const allVoices = await ttsService.getVoices();
        setVoices(allVoices);
      } catch (error) {
        console.error('[useTTS] Error loading voices:', error);
      }
    };
    
    loadVoices();
    
    // Listeners para eventos do TTS
    const handleSpeakStart = () => setIsSpeaking(true);
    const handleSpeakEnd = () => setIsSpeaking(false);
    const handleConfigChanged = (payload?: unknown) => {
      if (payload && typeof payload === 'object') {
        setConfig(payload as TTSConfig);
      }
    };
    
    ttsService.on('speakStart', handleSpeakStart);
    ttsService.on('speakEnd', handleSpeakEnd);
    ttsService.on('configChanged', handleConfigChanged);
    
    return () => {
      ttsService.off('speakStart', handleSpeakStart);
      ttsService.off('speakEnd', handleSpeakEnd);
      ttsService.off('configChanged', handleConfigChanged);
    };
  }, []);
  
  const speak = useCallback(async (text: string) => {
    await ttsService.speak(text);
  }, []);
  
  const stop = useCallback(() => {
    ttsService.stop();
  }, []);
  
  const pause = useCallback(() => {
    ttsService.pause();
  }, []);
  
  const resume = useCallback(() => {
    ttsService.resume();
  }, []);
  
  const setEnabled = useCallback((enabled: boolean) => {
    ttsService.setEnabled(enabled);
  }, []);
  
  const setAutoRead = useCallback((autoRead: boolean) => {
    ttsService.setAutoRead(autoRead);
  }, []);
  
  const setRate = useCallback(async (rate: number) => {
    await ttsService.setRate(rate);
  }, []);
  
  const setPitch = useCallback((pitch: number) => {
    ttsService.setPitch(pitch);
  }, []);
  
  const setVolume = useCallback(async (volume: number) => {
    await ttsService.setVolume(volume);
  }, []);
  
  const setVoice = useCallback(async (voiceName: string) => {
    await ttsService.setVoice(voiceName);
  }, []);
  
  const reloadVoices = useCallback(async () => {
    try {
      const allVoices = await ttsService.getVoices();
      setVoices(allVoices);
    } catch (error) {
      console.error('[useTTS] Error reloading voices:', error);
    }
  }, []);
  
  return {
    speak,
    stop,
    pause,
    resume,
    isSpeaking,
    isEnabled: config.enabled,
    isAutoReadEnabled: config.autoRead,
    config,
    voices,
    setEnabled,
    setAutoRead,
    setRate,
    setPitch,
    setVolume,
    setVoice,
    reloadVoices,
    isSupported: ttsService.isSupported(),
  };
}
