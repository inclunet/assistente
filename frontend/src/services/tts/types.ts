/**
 * Tipos e interfaces para o sistema de TTS com múltiplos provedores
 */

export enum TTSProvider {
  DISABLED = 'disabled',
  WEBSPEECH = 'webspeech',
  SAPI5 = 'sapi5',
  OPENAI = 'openai'
}

export interface TTSVoice {
  id: string;
  name: string;
  language: string;
  provider: TTSProvider;
  gender?: 'male' | 'female' | 'neutral';
  premium?: boolean;
  localService?: boolean;
  description?: string;
}

export interface TTSConfig {
  enabled: boolean;
  autoRead: boolean;
  provider: TTSProvider;
  voiceName: string;
  rate: number;    // -10 a 10 (SAPI5/OpenAI) ou 0.1 a 10 (WebSpeech)
  pitch: number;   // 0 a 2 (WebSpeech only)
  volume: number;  // 0-100 (SAPI5) ou 0-1 (WebSpeech/OpenAI)
}

export interface ITTSProvider {
  readonly name: TTSProvider;
  readonly isAvailable: boolean;

  initialize(): Promise<void>;
  getVoices(): Promise<TTSVoice[]>;
  setVoice(voiceName: string): Promise<void>;
  setRate(rate: number): Promise<void>;
  setVolume(volume: number): Promise<void>;
  setPitch?(pitch: number): void;
  speak(text: string): Promise<void>;
  
  // Nova API: sintetizar sem tocar (retorna Blob para sistema de audio por mensagem)
  synthesize?(text: string): Promise<Blob>;
  
  stop(): void;
  pause(): void;
  resume(): void;
  isSpeaking(): boolean;
  dispose(): void;

  addEventListener?<K extends keyof TTSEventMap>(
    type: K,
    listener: (event: TTSEventMap[K]) => void,
    options?: boolean | AddEventListenerOptions
  ): void;
  removeEventListener?<K extends keyof TTSEventMap>(
    type: K,
    listener: (event: TTSEventMap[K]) => void,
    options?: boolean | EventListenerOptions
  ): void;
  getNativeVoices?: () => SpeechSynthesisVoice[];
}

export interface TTSEventMap {
  'start': CustomEvent<{ text: string }>;
  'end': CustomEvent;
  'pause': CustomEvent;
  'resume': CustomEvent;
  'error': CustomEvent<{ error: Error }>;
  'voiceschanged': CustomEvent;
}

export type TTSEventType = keyof TTSEventMap;
