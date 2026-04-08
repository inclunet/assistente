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
} from '@wailsjs/go/main/App';

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
  private _isSpeaking: boolean = false;
  
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
    } catch {
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
    } catch {
      return [];
    }
  }
  
  async setVoice(voiceName: string): Promise<void> {
    this._currentVoice = voiceName;
  }
  
  async setRate(rate: number): Promise<void> {
    // SAPI5 usa -10 a 10
    this._rate = Math.max(-10, Math.min(10, Math.round(rate)));
    
    if (this._isAvailable) {
      try {
        await SetSAPI5Rate(this._rate);
      } catch {
        // best-effort
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
      } catch {
        // best-effort
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
      
      // Inicia monitoramento do estado
      this.startSpeakingMonitor();
      
    } catch (error) {
      this.dispatchEvent('error', { error: error as Error });
    }
  }
  
  private startSpeakingMonitor(): void {
    // Para monitor anterior se existir
    this.stopSpeakingMonitor();
    
    this._isSpeaking = true;
    
    // Monitora estado de fala
    this.checkInterval = window.setInterval(async () => {
      try {
        const speaking = await IsSAPI5Speaking();
        if (!speaking) {
          // Fala terminou
          this._isSpeaking = false;
          this.stopSpeakingMonitor();
          this.dispatchEvent('end', undefined);
        }
      } catch {
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
      this._isSpeaking = false;
      this.stopSpeakingMonitor();
      
      StopSAPI5()
        .then(() => undefined)
        .catch(() => undefined);
    }
  }
  
  pause(): void {
    // SAPI5 não tem pause/resume nativo via Wails
    // Poderia implementar no backend se necessário
    return;
  }
  
  resume(): void {
    return;
  }
  
  isSpeaking(): boolean {
    return this._isSpeaking;
  }
  
  dispose(): void {
    this.stop();
  }
}
