/**
 * SAPI5 TTS Provider (Windows only)
 * Usa funções backend do Wails para acessar SAPI5 do Windows
 */

import { BaseTTSProvider } from './base';
import { TTSProvider, TTSVoice } from '../types';
import { 
  GetSAPI5Voices, 
  SpeakSAPI5, 
  StopSAPI5, 
  SetSAPI5Volume, 
  SetSAPI5Rate,
  IsSAPI5Speaking 
} from '../../../../wailsjs/go/main/App';

interface SAPI5VoiceInfo {
  id: string;
  name: string;
  language: string;
  gender: string;
  age: string;
  vendor: string;
  description: string;
  source: string;
}

export class SAPI5Provider extends BaseTTSProvider {
  readonly name = TTSProvider.SAPI5;
  private checkInterval: number | null = null;
  
  constructor() {
    super();
    this._rate = 0;     // -10 a 10
    this._volume = 100; // 0-100
  }
  
  async initialize(): Promise<void> {
    try {
      // Tenta obter vozes para verificar disponibilidade
      const voices = await GetSAPI5Voices();
      this._isAvailable = voices && voices.length > 0;
      
      if (this._isAvailable) {
        console.log('[SAPI5] Provider initialized with', voices.length, 'voices');
      } else {
        console.log('[SAPI5] No voices available (may not be on Windows)');
      }
    } catch (error) {
      console.log('[SAPI5] Not available:', error);
      this._isAvailable = false;
    }
  }
  
  async getVoices(): Promise<TTSVoice[]> {
    if (!this._isAvailable) {
      return [];
    }
    
    try {
      const voices = await GetSAPI5Voices();
      return voices.map((v: SAPI5VoiceInfo) => ({
        id: v.name,
        name: v.name,
        language: v.language,
        provider: TTSProvider.SAPI5,
        gender: v.gender.toLowerCase() as 'male' | 'female' | 'neutral',
        localService: true,
        description: v.description || `${v.name} (${v.language})`
      }));
    } catch (error) {
      console.error('[SAPI5] Error getting voices:', error);
      return [];
    }
  }
  
  async setVoice(voiceName: string): Promise<void> {
    this._currentVoice = voiceName;
    console.log('[SAPI5] Voz selecionada:', voiceName);
  }
  
  async setRate(rate: number): Promise<void> {
    // SAPI5 usa -10 a 10
    this._rate = Math.max(-10, Math.min(10, Math.round(rate)));
    
    if (this._isAvailable) {
      try {
        await SetSAPI5Rate(this._rate);
      } catch (error) {
        console.error('[SAPI5] Error setting rate:', error);
      }
    }
  }
  
  async setVolume(volume: number): Promise<void> {
    // SAPI5 usa 0-100
    if (volume <= 1) {
      // Converte de 0-1 para 0-100
      this._volume = Math.round(volume * 100);
    } else {
      this._volume = Math.max(0, Math.min(100, Math.round(volume)));
    }
    
    if (this._isAvailable) {
      try {
        await SetSAPI5Volume(this._volume);
      } catch (error) {
        console.error('[SAPI5] Error setting volume:', error);
      }
    }
  }
  
  async speak(text: string): Promise<void> {
    if (!this._isAvailable || !text || text.trim().length === 0) {
      return;
    }
    
    try {
      // Para fala anterior
      await this.stop();
      
      // Aplica configurações
      await SetSAPI5Rate(this._rate);
      await SetSAPI5Volume(this._volume);
      
      // Dispara evento de início
      this.dispatchEvent('start', { text });
      
      // Inicia fala
      const voiceName = this._currentVoice || '';
      await SpeakSAPI5(text, voiceName);
      
      console.log('[SAPI5] Falando:', text.substring(0, 50), 'com voz:', voiceName);
      
      // Inicia monitoramento do estado
      this.startSpeakingMonitor();
      
    } catch (error) {
      console.error('[SAPI5] Error speaking:', error);
      this.dispatchEvent('error', { error: error as Error });
    }
  }
  
  private startSpeakingMonitor(): void {
    // Para monitor anterior se existir
    this.stopSpeakingMonitor();
    
    // Monitora estado de fala
    this.checkInterval = window.setInterval(async () => {
      try {
        const speaking = await IsSAPI5Speaking();
        if (!speaking) {
          // Fala terminou
          this.stopSpeakingMonitor();
          this.dispatchEvent('end', undefined);
        }
      } catch (error) {
        console.error('[SAPI5] Error checking speaking state:', error);
        this.stopSpeakingMonitor();
      }
    }, 100);
  }
  
  private stopSpeakingMonitor(): void {
    if (this.checkInterval !== null) {
      clearInterval(this.checkInterval);
      this.checkInterval = null;
    }
  }
  
  stop(): void {
    if (this._isAvailable) {
      this.stopSpeakingMonitor();
      
      StopSAPI5()
        .then(() => {
          console.log('[SAPI5] Stopped');
        })
        .catch(error => {
          console.error('[SAPI5] Error stopping:', error);
        });
    }
  }
  
  pause(): void {
    // SAPI5 não tem pause/resume nativo via Wails
    // Poderia implementar no backend se necessário
    console.warn('[SAPI5] Pause not implemented');
  }
  
  resume(): void {
    console.warn('[SAPI5] Resume not implemented');
  }
  
  isSpeaking(): boolean {
    // Nota: Esta é uma verificação síncrona, mas SAPI5 é assíncrono
    // Use o evento 'end' para saber quando terminou
    return this.checkInterval !== null;
  }
  
  dispose(): void {
    this.stop();
  }
}
