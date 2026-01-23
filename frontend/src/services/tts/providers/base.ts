/**
 * Classe base abstrata para provedores TTS
 * Define comportamentos comuns e interface que todos os provedores devem implementar
 */

import { TTSProvider, ITTSProvider, TTSVoice, TTSEventMap } from '../types';

export abstract class BaseTTSProvider implements ITTSProvider {
  abstract readonly name: TTSProvider;
  protected _isAvailable: boolean = false;
  protected _currentVoice: string | null = null;
  protected _rate: number = 0;
  protected _volume: number = 100;
  protected eventTarget: EventTarget;

  constructor() {
    this.eventTarget = new EventTarget();
  }

  get isAvailable(): boolean {
    return this._isAvailable;
  }

  // Métodos abstratos que cada provider deve implementar
  abstract initialize(): Promise<void>;
  abstract getVoices(): Promise<TTSVoice[]>;
  abstract setVoice(voiceName: string): Promise<void>;
  abstract setRate(rate: number): Promise<void>;
  abstract setVolume(volume: number): Promise<void>;
  abstract speak(text: string): Promise<void>;
  abstract stop(): void;
  abstract pause(): void;
  abstract resume(): void;
  abstract isSpeaking(): boolean;

  // Métodos de eventos comuns
  addEventListener<K extends keyof TTSEventMap>(
    type: K,
    listener: (event: TTSEventMap[K]) => void,
    options?: boolean | AddEventListenerOptions
  ): void {
    this.eventTarget.addEventListener(type as string, listener as EventListener, options);
  }

  removeEventListener<K extends keyof TTSEventMap>(
    type: K,
    listener: (event: TTSEventMap[K]) => void,
    options?: boolean | EventListenerOptions
  ): void {
    this.eventTarget.removeEventListener(type as string, listener as EventListener, options);
  }

  protected dispatchEvent<K extends keyof TTSEventMap>(
    type: K,
    detail?: TTSEventMap[K]['detail']
  ): void {
    const event = new CustomEvent(type as string, { detail });
    this.eventTarget.dispatchEvent(event);
  }

  // Método de limpeza
  dispose(): void {
    this.stop();
  }
}
