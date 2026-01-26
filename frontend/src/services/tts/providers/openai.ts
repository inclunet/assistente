/**
 * OpenAI TTS Provider
 * Usa a API OpenAI TTS via backend Wails
 */

import { BaseTTSProvider } from './base';
import { TTSProvider, TTSVoice } from '../types';
import { 
  GetOpenAITTSVoices,
  SynthesizeOpenAIWithVoice,
  SetOpenAITTSSpeed,
  SetOpenAITTSVoice
} from '../../../../wailsjs/go/main/App';
import { main } from '../../../../wailsjs/go/models';

interface OpenAITTSVoiceInfo {
  id: string;
  name: string;
  description: string;
  gender: string;
  provider: string;
}

// Audio player global - SINGLETON para evitar duplicação de áudio
// Mesmo se múltiplos listeners chamarem speak(), só um áudio toca por vez
let globalAudioPlayer: HTMLAudioElement | null = null;
let globalAudioUrl: string | null = null;

// Promise de síntese pendente - previne chamadas simultâneas
let pendingSynthesis: Promise<void> | null = null;

export class OpenAIProvider extends BaseTTSProvider {
  readonly name = TTSProvider.OPENAI;
  private currentAudio: HTMLAudioElement | null = null;
  private _isSpeaking: boolean = false;
  
  constructor() {
    super();
    this._rate = 1.0; // OpenAI usa 0.25 a 4.0, default 1.0
    this._volume = 1.0; // 0-1 para Audio element
  }
  
  async initialize(): Promise<void> {
    try {
      // Tenta obter vozes para verificar disponibilidade
      const voices = await GetOpenAITTSVoices();
      this._isAvailable = voices && voices.length > 0;
      
      if (this._isAvailable) {
        console.log('[OpenAI] Provider initialized with', voices.length, 'voices');
      } else {
        console.log('[OpenAI] No voices available (API key may not be configured)');
      }
    } catch (error) {
      console.log('[OpenAI] Not available:', error);
      this._isAvailable = false;
    }
  }
  
  async getVoices(): Promise<TTSVoice[]> {
    if (!this._isAvailable) {
      return [];
    }
    
    try {
      const voices = await GetOpenAITTSVoices();
      return voices.map((v: OpenAITTSVoiceInfo) => ({
        id: v.id,
        name: v.name,
        language: 'multilingual', // OpenAI voices são multilíngues
        provider: TTSProvider.OPENAI,
        gender: v.gender.toLowerCase() as 'male' | 'female' | 'neutral',
        premium: true,
        localService: false,
        description: v.description
      }));
    } catch (error) {
      console.error('[OpenAI] Error getting voices:', error);
      return [];
    }
  }
  
  async setVoice(voiceName: string): Promise<void> {
    this._currentVoice = voiceName;
    
    if (this._isAvailable) {
      try {
        await SetOpenAITTSVoice(voiceName);
        console.log('[OpenAI] Voz selecionada:', voiceName);
      } catch (error) {
        console.error('[OpenAI] Error setting voice:', error);
      }
    }
  }
  
  async setRate(rate: number): Promise<void> {
    // OpenAI usa 0.25 a 4.0
    // Detecta se é escala OpenAI (0.25-4.0) ou escala SAPI5 (-10 a 10)
    // Valores entre 0.25 e 4.0 são considerados escala OpenAI
    // Valores inteiros <= -1 ou >= 2 são considerados escala SAPI5
    
    if (rate >= 0.25 && rate <= 4.0) {
      // Já está em escala OpenAI
      this._rate = rate;
    } else if (rate >= -10 && rate <= 10) {
      // Escala SAPI5 (-10 a 10), converte para OpenAI
      // -10 -> 0.25, 0 -> 1.0, 10 -> 4.0
      if (rate <= 0) {
        this._rate = 1 + (rate * 0.075); // -10 a 0 vira 0.25 a 1
      } else {
        this._rate = 1 + (rate * 0.3); // 0 a 10 vira 1 a 4
      }
    } else {
      // Valor fora do range, limita para escala OpenAI
      this._rate = Math.max(0.25, Math.min(4.0, rate));
    }
    
    if (this._isAvailable) {
      try {
        // Backend espera int (parece ser um multiplicador)
        // Converte 0.25-4.0 para escala -10 a 10
        let backendRate: number;
        if (this._rate < 1) {
          backendRate = Math.round((this._rate - 1) / 0.075);
        } else {
          backendRate = Math.round((this._rate - 1) / 0.3);
        }
        await SetOpenAITTSSpeed(backendRate);
      } catch (error) {
        console.error('[OpenAI] Error setting speed:', error);
      }
    }
  }
  
  async setVolume(volume: number): Promise<void> {
    // Audio element usa 0-1
    if (volume > 1) {
      // Converte de 0-100 para 0-1
      this._volume = volume / 100;
    } else {
      this._volume = Math.max(0, Math.min(1, volume));
    }
    
    // Aplica ao audio atual se existir
    if (this.currentAudio) {
      this.currentAudio.volume = this._volume;
    }
  }
  
  /**
   * Sintetiza texto em áudio SEM reproduzir
   * Retorna Blob para uso com sistema de audio por mensagem
   */
  async synthesize(text: string): Promise<Blob> {
    if (!this._isAvailable || !text || text.trim().length === 0) {
      throw new Error('OpenAI TTS not available or empty text');
    }

    // Configura voz e velocidade
    if (this._currentVoice) {
      await SetOpenAITTSVoice(this._currentVoice);
    }
    
    await SetOpenAITTSSpeed(this.calculateBackendRate());

    console.log('[OpenAI] Sintetizando (sem tocar):', text.substring(0, 50), 'voz:', this._currentVoice);

    // Chama backend para sintetizar
    const result: main.SynthesisResultInfo = await SynthesizeOpenAIWithVoice(
      text,
      this._currentVoice || 'nova'
    );

    if (!result.audioBase64) {
      throw new Error('Backend retornou audio vazio');
    }

    // Decodifica base64 para blob
    const audioBytes = atob(result.audioBase64);
    const audioArray = new Uint8Array(audioBytes.length);
    for (let i = 0; i < audioBytes.length; i++) {
      audioArray[i] = audioBytes.charCodeAt(i);
    }

    const audioBlob = new Blob([audioArray], { type: 'audio/mpeg' });
    console.log('[OpenAI] Blob criado:', audioBlob.size, 'bytes');
    
    return audioBlob;
  }

  private calculateBackendRate(): number {
    // OpenAI backend usa escala diferente
    if (this._rate < 1) {
      return Math.round((this._rate - 1) / 0.075);
    } else {
      return Math.round((this._rate - 1) / 0.3);
    }
  }

  async speak(text: string): Promise<void> {
    if (!this._isAvailable || !text || text.trim().length === 0) {
      return;
    }
    
    // CRÍTICO: Se já há uma síntese em andamento, aguarda ela terminar primeiro
    if (pendingSynthesis) {
      console.log('[OpenAI] ⏳ Aguardando síntese anterior terminar...');
      await pendingSynthesis;
    }
    
    // Cria nova Promise para esta síntese
    const synthesisPromise = (async () => {
      try {
        // IMPORTANTE: Para qualquer áudio GLOBAL anterior
      // Isso garante que mesmo se múltiplos listeners chamarem speak(),
      // apenas um áudio tocará por vez (o mais recente)
      if (globalAudioPlayer) {
        globalAudioPlayer.pause();
        globalAudioPlayer.onplay = null;
        globalAudioPlayer.onended = null;
        globalAudioPlayer.onerror = null;
        globalAudioPlayer = null;
      }
      
      if (globalAudioUrl) {
        URL.revokeObjectURL(globalAudioUrl);
        globalAudioUrl = null;
      }
      
      // Configura velocidade e voz
      if (this._currentVoice) {
        await SetOpenAITTSVoice(this._currentVoice);
      }
      
      await SetOpenAITTSSpeed(this.calculateBackendRate());
      
      console.log('[OpenAI] Sintetizando:', text.substring(0, 50), 'com voz:', this._currentVoice);
      
      // Chama backend para sintetizar
      const result: main.SynthesisResultInfo = await SynthesizeOpenAIWithVoice(
        text, 
        this._currentVoice || 'nova'
      );
      
      console.log('[OpenAI] Resposta do backend:', {
        audioBase64Length: result.audioBase64?.length,
        format: result.format,
        provider: result.provider
      });
      
      if (!result.audioBase64) {
        throw new Error('Backend retornou audio vazio');
      }
      
      // Decodifica base64 para blob de audio
      const audioBytes = atob(result.audioBase64);
      const audioArray = new Uint8Array(audioBytes.length);
      for (let i = 0; i < audioBytes.length; i++) {
        audioArray[i] = audioBytes.charCodeAt(i);
      }
      
      const audioBlob = new Blob([audioArray], { type: 'audio/mpeg' });
      globalAudioUrl = URL.createObjectURL(audioBlob);
      
      // IMPORTANTE: Usa player GLOBAL para evitar duplicação
      // Mesmo se múltiplos listeners chamarem speak(), apenas um áudio toca
      globalAudioPlayer = new Audio(globalAudioUrl);
      globalAudioPlayer.volume = this._volume;
      
      // Mantém referência local para compatibilidade
      this.currentAudio = globalAudioPlayer;
      
      // Eventos
      globalAudioPlayer.onplay = () => {
        this._isSpeaking = true;
        this.dispatchEvent('start', { text });
      };
      
      globalAudioPlayer.onended = () => {
        this._isSpeaking = false;
        if (globalAudioUrl) {
          URL.revokeObjectURL(globalAudioUrl);
          globalAudioUrl = null;
        }
        this.currentAudio = null;
        globalAudioPlayer = null;
        this.dispatchEvent('end', undefined);
      };
      
      globalAudioPlayer.onerror = (error) => {
        this._isSpeaking = false;
        if (globalAudioUrl) {
          URL.revokeObjectURL(globalAudioUrl);
          globalAudioUrl = null;
        }
        this.currentAudio = null;
        globalAudioPlayer = null;
        console.error('[OpenAI] Audio playback error:', error);
        this.dispatchEvent('error', { 
          error: new Error(`OpenAI audio playback error`) 
        });
      };
      
      // Inicia reprodução
      await globalAudioPlayer.play();
      
      console.log('[OpenAI] Reproduzindo audio sintetizado');
      
    } catch (error) {
      console.error('[OpenAI] Error speaking:', error);
      this._isSpeaking = false;
      this.dispatchEvent('error', { error: error as Error });
    } finally {
      // Libera a Promise pendente quando termina (sucesso ou erro)
      pendingSynthesis = null;
    }
    })();
    
    // Armazena a Promise pendente
    pendingSynthesis = synthesisPromise;
    
    // Aguarda conclusão
    await synthesisPromise;
  }
  
  stop(): void {
    // Cancela síntese pendente
    pendingSynthesis = null;
    
    // Para o player global primeiro (prioridade)
    if (globalAudioPlayer) {
      globalAudioPlayer.pause();
      globalAudioPlayer.onplay = null;
      globalAudioPlayer.onended = null;
      globalAudioPlayer.onerror = null;
      globalAudioPlayer = null;
    }
    
    if (globalAudioUrl) {
      URL.revokeObjectURL(globalAudioUrl);
      globalAudioUrl = null;
    }
    
    // Limpa referência local (pode ser a mesma que global)
    if (this.currentAudio) {
      // Remove todos os event listeners para evitar chamadas duplicadas
      this.currentAudio.onplay = null;
      this.currentAudio.onended = null;
      this.currentAudio.onerror = null;
      
      this.currentAudio.pause();
      this.currentAudio.currentTime = 0;
      const url = this.currentAudio.src;
      this.currentAudio = null;
      this._isSpeaking = false;
      
      // Libera memória
      if (url && url.startsWith('blob:')) {
        URL.revokeObjectURL(url);
      }
    }
  }
  
  pause(): void {
    if (this.currentAudio && !this.currentAudio.paused) {
      this.currentAudio.pause();
      this.dispatchEvent('pause', undefined);
    }
  }
  
  resume(): void {
    if (this.currentAudio && this.currentAudio.paused) {
      this.currentAudio.play()
        .then(() => {
          this.dispatchEvent('resume', undefined);
        })
        .catch(error => {
          console.error('[OpenAI] Error resuming:', error);
        });
    }
  }
  
  isSpeaking(): boolean {
    return this._isSpeaking;
  }
  
  dispose(): void {
    this.stop();
  }
}
