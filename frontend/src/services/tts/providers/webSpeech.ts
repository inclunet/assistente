/**
 * WebSpeech TTS Provider
 * Usa a SpeechSynthesis API nativa do navegador
 */

import { BaseTTSProvider } from './base';
import { TTSProvider, TTSVoice } from '../types';

export class WebSpeechProvider extends BaseTTSProvider {
  readonly name = TTSProvider.WEBSPEECH;
  
  private synth: SpeechSynthesis;
  private voice: SpeechSynthesisVoice | null = null;
  private _isSpeaking: boolean = false;
  private utterance: SpeechSynthesisUtterance | null = null;
  private pitch: number = 1.0;
  
  constructor() {
    super();
    this.synth = window.speechSynthesis;
    this._rate = 1.0;
    this._volume = 1.0;
  }
  
  async initialize(): Promise<void> {
    try {
      this._isAvailable = 'speechSynthesis' in window;
      if (this._isAvailable) {
        await this.loadVoices();
      }
    } catch (error) {
      console.error('[WebSpeech] Initialization error:', error);
      this._isAvailable = false;
    }
  }
  
  private async loadVoices(): Promise<void> {
    return new Promise((resolve) => {
      const voices = this.synth.getVoices();
      if (voices.length > 0) {
        resolve();
        return;
      }
      
      // Alguns navegadores disparam evento quando vozes carregam
      const handler = () => {
        this.synth.removeEventListener('voiceschanged', handler);
        resolve();
      };
      
      this.synth.addEventListener('voiceschanged', handler);
      
      // Timeout para garantir que não trave
      setTimeout(() => {
        this.synth.removeEventListener('voiceschanged', handler);
        resolve();
      }, 1000);
    });
  }
  
  async getVoices(): Promise<TTSVoice[]> {
    const voices = this.synth.getVoices();
    return voices.map(v => ({
      id: v.name,
      name: v.name,
      language: v.lang,
      provider: TTSProvider.WEBSPEECH,
      localService: v.localService,
      description: `${v.name} (${v.lang})`
    }));
  }

  getNativeVoices(): SpeechSynthesisVoice[] {
    return this.synth.getVoices();
  }
  
  async setVoice(voiceName: string): Promise<void> {
    this._currentVoice = voiceName;
    const voices = this.synth.getVoices();
    const voice = voices.find(v => v.name === voiceName);
    
    if (voice) {
      this.voice = voice;
    } else {
      console.warn('[WebSpeech] Voz não encontrada:', voiceName);
      // Tenta novamente após carregar vozes
      if (voices.length === 0) {
        await this.loadVoices();
        await this.setVoice(voiceName);
      }
    }
  }
  
  async setRate(rate: number): Promise<void> {
    // WebSpeech usa escala 0.1 a 10
    // Converte de -10/10 para 0.1/10
    if (rate >= -10 && rate <= 10) {
      // -10 -> 0.1, 0 -> 1, 10 -> 10
      this._rate = rate <= 0 
        ? 1 + (rate * 0.09)  // -10 a 0 vira 0.1 a 1
        : 1 + (rate * 0.9);  // 0 a 10 vira 1 a 10
    } else {
      // Já está em escala WebSpeech
      this._rate = Math.max(0.1, Math.min(10, rate));
    }
  }
  
  async setVolume(volume: number): Promise<void> {
    // WebSpeech usa 0-1
    if (volume > 1) {
      // Converte de 0-100 para 0-1
      this._volume = volume / 100;
    } else {
      this._volume = Math.max(0, Math.min(1, volume));
    }
  }
  
  setPitch(pitch: number): void {
    this.pitch = Math.max(0, Math.min(2, pitch));
  }
  
  async speak(text: string): Promise<void> {
    if (!text || text.trim().length === 0) {
      return;
    }
    
    // Cancela fala anterior
    this.stop();
    
    // Cria nova utterance
    this.utterance = new SpeechSynthesisUtterance(text);
    this.utterance.rate = this._rate;
    this.utterance.pitch = this.pitch;
    this.utterance.volume = this._volume;
    
    // Aplica voz
    if (this.voice) {
      this.utterance.voice = this.voice;
    } else if (this._currentVoice) {
      // Tenta carregar a voz agora
      const voices = this.synth.getVoices();
      const voice = voices.find(v => v.name === this._currentVoice);
      if (voice) {
        this.voice = voice;
        this.utterance.voice = voice;
      }
    }
    
    // Eventos
    this.utterance.onstart = () => {
      this._isSpeaking = true;
      this.dispatchEvent('start', { text });
    };
    
    this.utterance.onend = () => {
      this._isSpeaking = false;
      this.utterance = null;
      this.dispatchEvent('end', undefined);
    };
    
    this.utterance.onpause = () => {
      this.dispatchEvent('pause', undefined);
    };
    
    this.utterance.onresume = () => {
      this.dispatchEvent('resume', undefined);
    };
    
    this.utterance.onerror = (event) => {
      this._isSpeaking = false;
      this.utterance = null;
      const error = new Error(`WebSpeech error: ${event.error}`);
      this.dispatchEvent('error', { error });
    };
    
    // Inicia fala
    this.synth.speak(this.utterance);
  }
  
  stop(): void {
    if (this._isSpeaking) {
      this.synth.cancel();
      this._isSpeaking = false;
      this.utterance = null;
    }
  }
  
  pause(): void {
    if (this._isSpeaking) {
      this.synth.pause();
    }
  }
  
  resume(): void {
    if (this._isSpeaking && this.synth.paused) {
      this.synth.resume();
    }
  }
  
  isSpeaking(): boolean {
    return this._isSpeaking;
  }
  
  dispose(): void {
    this.stop();
  }
}
