/**
 * OpenAI TTS Provider
 * Usa a API OpenAI TTS via backend Wails com suporte a streaming
 */

import { BaseTTSProvider } from './base';
import { TTSProvider, TTSVoice } from '../types';
import { base64ToBlob } from '../../../lib/audioUtils';
import { 
  GetOpenAITTSVoices,
  SynthesizeOpenAIWithVoice,
  SynthesizeOpenAIStream,
  SetOpenAITTSSpeed,
  SetOpenAITTSVoice
} from '@wailsjs/go/main/App';
import { main } from '../../../../wailsjs/go/models';
import { getStreamPlayer, TTSStreamPlayer } from '../streamPlayer';

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
// Reject da Promise pendente — permite que stop() resolva imediatamente
let rejectPendingSynthesis: ((reason?: Error) => void) | null = null;

export class OpenAIProvider extends BaseTTSProvider {
  readonly name = TTSProvider.OPENAI;
  private currentAudio: HTMLAudioElement | null = null;
  private _isSpeaking: boolean = false;
  private _useStreaming: boolean = true; // Habilita streaming por padrão
  private streamPlayer: TTSStreamPlayer | null = null;
  private currentSessionId: string | null = null;
  
  constructor() {
    super();
    this._rate = 1.0; // OpenAI usa 0.25 a 4.0, default 1.0
    this._volume = 1.0; // 0-1 para Audio element
  }
  
  /**
   * Habilita ou desabilita streaming
   */
  setUseStreaming(useStreaming: boolean): void {
    this._useStreaming = useStreaming;
  }
  
  async initialize(): Promise<void> {
    try {
      // Tenta obter vozes para verificar disponibilidade
      const voices = await GetOpenAITTSVoices();
      this._isAvailable = voices && voices.length > 0;
    } catch {
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
    } catch {
      return [];
    }
  }
  
  async setVoice(voiceName: string): Promise<void> {
    this._currentVoice = voiceName;
    
    if (this._isAvailable) {
      try {
        await SetOpenAITTSVoice(voiceName);
      } catch {
        // best-effort
      }
    }
  }
  
  async setRate(rate: number): Promise<void> {
    // OpenAI usa 0.25 a 4.0
    // Converte de escala alternativa (-10 a 10) se necessário
    // Valores entre 0.25 e 4.0 são considerados escala OpenAI
    // Valores inteiros <= -1 ou >= 2 são convertidos para escala OpenAI
    
    if (rate >= 0.25 && rate <= 4.0) {
      // Já está em escala OpenAI
      this._rate = rate;
    } else if (rate >= -10 && rate <= 10) {
      // Escala alternativa (-10 a 10), converte para OpenAI (0.25-4.0)
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
      } catch {
        // best-effort
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
    
    // Se já há uma síntese em andamento, aguarda ela terminar primeiro.
    // stop() chama rejectPendingSynthesis, então await não trava indefinidamente.
    if (pendingSynthesis) {
      try {
        await pendingSynthesis;
      } catch {
        // Ignorar rejeição (ex: stop() chamou reject)
      }
    }
    
    // Cria wrapper cancelável para a síntese
    const synthesisPromise = new Promise<void>((resolve, reject) => {
      rejectPendingSynthesis = reject;
      const impl = this._useStreaming
        ? this.speakWithStreaming(text)
        : this.speakWithoutStreaming(text);
      impl.then(resolve, reject);
    });
    
    pendingSynthesis = synthesisPromise;
    try {
      await synthesisPromise;
    } catch {
      // Ignorar rejeição causada por stop()
    } finally {
      if (pendingSynthesis === synthesisPromise) {
        pendingSynthesis = null;
        rejectPendingSynthesis = null;
      }
    }
  }

  /**
   * Fala usando streaming (baixa latência)
   */
  private async speakWithStreaming(text: string): Promise<void> {
    try {
      // Para qualquer streaming anterior
      if (this.streamPlayer) {
        this.streamPlayer.stop();
      }
      
      // Para player global não-streaming também
      if (globalAudioPlayer) {
        globalAudioPlayer.pause();
        globalAudioPlayer = null;
      }
      
      // Configura velocidade e voz
      if (this._currentVoice) {
        await SetOpenAITTSVoice(this._currentVoice);
      }
      await SetOpenAITTSSpeed(this.calculateBackendRate());
      
      // Gera ID único para esta sessão
      this.currentSessionId = `tts-${Date.now()}-${Math.random().toString(36).substr(2, 9)}`;
      
      // Obtém o stream player
      this.streamPlayer = getStreamPlayer();
      
      // Promise que resolve quando o streaming termina
      const streamPromise = new Promise<void>((resolve, reject) => {
        this.streamPlayer!.startListening(this.currentSessionId!, {
          onStart: () => {
            this._isSpeaking = true;
            this.dispatchEvent('start', { text });
          },
          onEnd: () => {
            this._isSpeaking = false;
            this.currentSessionId = null;
            this.dispatchEvent('end', undefined);
            resolve();
          },
          onError: (error) => {
            this._isSpeaking = false;
            this.currentSessionId = null;
            this.dispatchEvent('error', { error });
            reject(error);
          }
        });
      });
      
      // Inicia a síntese no backend (isso vai disparar os eventos)
      await SynthesizeOpenAIStream(text, this._currentVoice || 'nova', this.currentSessionId);
      
      // Aguarda o streaming terminar
      await streamPromise;
      
    } catch (error) {
      console.error('[OpenAI] Streaming error:', error);
      this._isSpeaking = false;
      this.dispatchEvent('error', { error: error as Error });
    }
  }

  /**
   * Fala sem streaming (comportamento original - fallback)
   */
  private async speakWithoutStreaming(text: string): Promise<void> {
    try {
      // IMPORTANTE: Para qualquer áudio GLOBAL anterior
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
      
      // Chama backend para sintetizar
      const result: main.SynthesisResultInfo = await SynthesizeOpenAIWithVoice(
        text, 
        this._currentVoice || 'nova'
      );
      
      if (!result.audioBase64) {
        throw new Error('Backend retornou audio vazio');
      }
      
      // Decodifica base64 para blob de audio
      const audioBlob = base64ToBlob(result.audioBase64, 'audio/mpeg');
      globalAudioUrl = URL.createObjectURL(audioBlob);
      
      globalAudioPlayer = new Audio(globalAudioUrl);
      globalAudioPlayer.volume = this._volume;
      this.currentAudio = globalAudioPlayer;
      
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
      
      await globalAudioPlayer.play();
      
    } catch (error) {
      console.error('[OpenAI] Error speaking:', error);
      this._isSpeaking = false;
      this.dispatchEvent('error', { error: error as Error });
    }
  }
  
  stop(): void {
    // Rejeita síntese pendente para destravar qualquer await pendingSynthesis
    if (rejectPendingSynthesis) {
      rejectPendingSynthesis(new Error('stopped'));
      rejectPendingSynthesis = null;
    }
    pendingSynthesis = null;
    
    // Para streaming se estiver ativo
    if (this.streamPlayer) {
      this.streamPlayer.stop();
      this.streamPlayer = null;
    }
    this.currentSessionId = null;
    
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
      this.currentAudio.onplay = null;
      this.currentAudio.onended = null;
      this.currentAudio.onerror = null;
      
      this.currentAudio.pause();
      this.currentAudio.currentTime = 0;
      const url = this.currentAudio.src;
      this.currentAudio = null;
      this._isSpeaking = false;
      
      if (url && url.startsWith('blob:')) {
        URL.revokeObjectURL(url);
      }
    }
    
    this._isSpeaking = false;
  }
  
  pause(): void {
    // Pausa streaming se ativo
    if (this.streamPlayer && this.streamPlayer.isPlaying()) {
      this.streamPlayer.pause();
      this.dispatchEvent('pause', undefined);
      return;
    }
    
    // Pausa áudio tradicional
    if (this.currentAudio && !this.currentAudio.paused) {
      this.currentAudio.pause();
      this.dispatchEvent('pause', undefined);
    }
  }
  
  resume(): void {
    // Resume streaming se ativo
    if (this.streamPlayer && this.streamPlayer.getState() === 'paused') {
      this.streamPlayer.resume();
      this.dispatchEvent('resume', undefined);
      return;
    }
    
    // Resume áudio tradicional
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
    // Verifica streaming primeiro
    if (this.streamPlayer && this.streamPlayer.isPlaying()) {
      return true;
    }
    return this._isSpeaking;
  }
  
  dispose(): void {
    this.stop();
    if (this.streamPlayer) {
      this.streamPlayer.dispose();
      this.streamPlayer = null;
    }
  }
}
