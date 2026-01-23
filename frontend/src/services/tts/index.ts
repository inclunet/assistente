/**
 * TTS Service - Serviço de Text-to-Speech com múltiplos provedores
 * Suporta WebSpeech API, SAPI5 (Windows) e OpenAI TTS
 */

import { TTSProvider, ITTSProvider, TTSVoice } from './types';
import { ttsFactory } from './factory';

export interface TTSConfig {
  enabled: boolean;
  autoRead: boolean;
  provider: TTSProvider;
  voiceName?: string;
  rate: number;
  pitch: number;
  volume: number;
}

class TTSService {
  private currentProvider: ITTSProvider | null = null;
  private config: TTSConfig = {
    enabled: false,
    autoRead: false,
    provider: TTSProvider.WEBSPEECH,
    rate: 1.0,
    pitch: 1.0,
    volume: 1.0,
  };
  private listeners: Map<string, Set<Function>> = new Map();
  private initialized: boolean = false;
  
  constructor() {
    this.init();
  }
  
  private async init(): Promise<void> {
    if (this.initialized) {
      return;
    }
    
    console.log('[TTSService] Initializing...');
    
    // Inicializa factory de provedores
    await ttsFactory.initialize();
    
    // Carrega configurações salvas
    this.loadConfig();
    
    // Seleciona provider inicial
    await this.selectProvider(this.config.provider);
    
    this.initialized = true;
    console.log('[TTSService] Initialized with provider:', this.config.provider);
  }
  
  private async selectProvider(type: TTSProvider): Promise<void> {
    // Para provider atual se existir
    if (this.currentProvider) {
      this.currentProvider.stop();
    }
    
    // Obtém novo provider
    this.currentProvider = ttsFactory.getProviderWithFallback(type);
    
    if (!this.currentProvider) {
      console.error('[TTSService] No provider available');
      return;
    }
    
    console.log('[TTSService] Selected provider:', this.currentProvider.name);
    
    // Aplica configurações ao novo provider
    if (this.config.voiceName) {
      await this.currentProvider.setVoice(this.config.voiceName);
    }
    await this.currentProvider.setRate(this.config.rate);
    await this.currentProvider.setVolume(this.config.volume);
    
    // Registra eventos
    this.setupProviderEvents();
  }
  
  private setupProviderEvents(): void {
    if (!this.currentProvider) return;
    
    const provider = this.currentProvider as any;
    
    provider.addEventListener('start', () => {
      this.emit('speakStart');
    });
    
    provider.addEventListener('end', () => {
      this.emit('speakEnd');
    });
    
    provider.addEventListener('error', (event: CustomEvent<{ error: Error }>) => {
      this.emit('speakError', event.detail.error);
    });
  }
  
  private loadConfig(): void {
    try {
      const saved = localStorage.getItem('tts_config');
      if (saved) {
        const config = JSON.parse(saved);
        this.config = { 
          ...this.config, 
          ...config,
          provider: config.provider || TTSProvider.WEBSPEECH 
        };
      }
    } catch (error) {
      console.error('[TTSService] Error loading config:', error);
    }
  }
  
  private saveConfig(): void {
    try {
      localStorage.setItem('tts_config', JSON.stringify(this.config));
    } catch (error) {
      console.error('[TTSService] Error saving config:', error);
    }
  }
  
  /**
   * Detecta o provider de uma voz pelo nome
   */
  private detectProviderFromVoice(voiceName: string): TTSProvider {
    // Padrões conhecidos
    if (voiceName.includes('Microsoft') || voiceName.includes('SAPI')) {
      return TTSProvider.SAPI5;
    }
    if (voiceName.match(/^(alloy|echo|fable|onyx|nova|shimmer)$/)) {
      return TTSProvider.OPENAI;
    }
    return TTSProvider.WEBSPEECH;
  }
  
  /**
   * Fala um texto
   */
  async speak(text: string): Promise<void> {
    if (!this.config.enabled || !this.currentProvider) {
      return;
    }
    
    await this.currentProvider.speak(text);
  }

  /**
   * Sintetiza texto em áudio SEM tocar
   * Retorna Blob para uso com sistema de audio por mensagem
   */
  async synthesizeForMessage(text: string): Promise<Blob | null> {
    console.log('[TTSService] 🎤 synthesizeForMessage chamado:', {
      enabled: this.config.enabled,
      hasProvider: !!this.currentProvider,
      providerName: this.currentProvider?.name
    });
    
    if (!this.config.enabled || !this.currentProvider) {
      console.log('[TTSService] ❌ TTS desabilitado ou sem provider');
      return null;
    }

    // Verifica se provider suporta synthesize
    if (!this.currentProvider.synthesize) {
      console.warn(`[TTSService] ❌ Provider ${this.currentProvider.name} não suporta synthesize`);
      return null;
    }

    try {
      console.log('[TTSService] 🎤 Chamando provider.synthesize...');
      const blob = await this.currentProvider.synthesize(text);
      console.log('[TTSService] ✅ Blob recebido do provider:', blob?.size, 'bytes');
      return blob;
    } catch (error) {
      console.error('[TTSService] ❌ Erro ao sintetizar:', error);
      return null;
    }
  }
  
  /**
   * Para a fala
   */
  stop(): void {
    if (this.currentProvider) {
      this.currentProvider.stop();
    }
  }
  
  /**
   * Pausa a fala
   */
  pause(): void {
    if (this.currentProvider) {
      this.currentProvider.pause();
    }
  }
  
  /**
   * Retoma a fala
   */
  resume(): void {
    if (this.currentProvider) {
      this.currentProvider.resume();
    }
  }
  
  /**
   * Verifica se está falando
   */
  isSpeaking(): boolean {
    return this.currentProvider?.isSpeaking() || false;
  }
  
  /**
   * Verifica se TTS está habilitado
   */
  isEnabled(): boolean {
    return this.config.enabled;
  }
  
  /**
   * Verifica se auto-read está habilitado
   */
  isAutoReadEnabled(): boolean {
    return this.config.enabled && this.config.autoRead;
  }
  
  /**
   * Habilita/desabilita TTS
   */
  setEnabled(enabled: boolean): void {
    this.config.enabled = enabled;
    this.saveConfig();
    this.emit('configChanged', this.config);
    
    if (!enabled) {
      this.stop();
    }
  }
  
  /**
   * Habilita/desabilita leitura automática
   */
  setAutoRead(autoRead: boolean): void {
    this.config.autoRead = autoRead;
    this.saveConfig();
    this.emit('configChanged', this.config);
  }
  
  /**
   * Define o provider
   */
  async setProvider(provider: TTSProvider): Promise<void> {
    this.config.provider = provider;
    await this.selectProvider(provider);
    this.saveConfig();
    this.emit('configChanged', this.config);
  }
  
  /**
   * Define a velocidade de fala
   */
  async setRate(rate: number): Promise<void> {
    this.config.rate = rate;
    if (this.currentProvider) {
      await this.currentProvider.setRate(rate);
    }
    this.saveConfig();
    this.emit('configChanged', this.config);
  }
  
  /**
   * Define o tom de voz (apenas WebSpeech)
   */
  setPitch(pitch: number): void {
    this.config.pitch = pitch;
    // Pitch só funciona com WebSpeech
    if (this.currentProvider?.name === TTSProvider.WEBSPEECH) {
      (this.currentProvider as any).setPitch?.(pitch);
    }
    this.saveConfig();
    this.emit('configChanged', this.config);
  }
  
  /**
   * Define o volume
   */
  async setVolume(volume: number): Promise<void> {
    this.config.volume = volume;
    if (this.currentProvider) {
      await this.currentProvider.setVolume(volume);
    }
    this.saveConfig();
    this.emit('configChanged', this.config);
  }

  /**
   * Obtém o volume atual
   */
  getVolume(): number {
    return this.config.volume;
  }
  
  /**
   * Define a voz (detecta provider automaticamente)
   */
  async setVoice(voiceName: string): Promise<void> {
    console.log('[TTSService] Definindo voz:', voiceName);
    
    this.config.voiceName = voiceName;
    
    // Detecta provider da voz
    const detectedProvider = this.detectProviderFromVoice(voiceName);
    
    // Troca de provider se necessário
    if (detectedProvider !== this.config.provider) {
      console.log('[TTSService] Trocando provider para:', detectedProvider);
      this.config.provider = detectedProvider;
      await this.selectProvider(detectedProvider);
    }
    
    // Define a voz no provider
    if (this.currentProvider) {
      await this.currentProvider.setVoice(voiceName);
    }
    
    this.saveConfig();
    this.emit('configChanged', this.config);
  }
  
  /**
   * Retorna lista de vozes disponíveis de TODOS os provedores
   */
  async getVoices(): Promise<TTSVoice[]> {
    const allVoices: TTSVoice[] = [];
    
    const providers = ttsFactory.getAvailableProviders();
    
    for (const providerType of providers) {
      const provider = ttsFactory.getProvider(providerType);
      if (provider) {
        try {
          const voices = await provider.getVoices();
          allVoices.push(...voices);
        } catch (error) {
          console.error(`[TTSService] Error getting voices from ${providerType}:`, error);
        }
      }
    }
    
    return allVoices;
  }
  
  /**
   * Retorna vozes de um provider específico (compatibilidade)
   */
  getVoicesByProvider(provider: TTSProvider): SpeechSynthesisVoice[] {
    // Apenas para compatibilidade com código antigo que espera SpeechSynthesisVoice[]
    // Só funciona com WebSpeech
    if (provider === TTSProvider.WEBSPEECH && this.currentProvider?.name === TTSProvider.WEBSPEECH) {
      return (this.currentProvider as any).synth?.getVoices() || [];
    }
    return [];
  }
  
  /**
   * Retorna configuração atual
   */
  getConfig(): TTSConfig {
    return { ...this.config };
  }
  
  /**
   * Retorna provider atual
   */
  getCurrentProvider(): TTSProvider {
    return this.config.provider;
  }
  
  /**
   * Retorna provedores disponíveis
   */
  getAvailableProviders(): TTSProvider[] {
    return ttsFactory.getAvailableProviders();
  }
  
  /**
   * Verifica se o sistema suporta TTS
   */
  isSupported(): boolean {
    return ttsFactory.getAvailableProviders().length > 0;
  }
  
  // Event emitter methods
  private emit(event: string, data?: any): void {
    const listeners = this.listeners.get(event);
    if (listeners) {
      listeners.forEach(listener => listener(data));
    }
  }
  
  on(event: string, listener: Function): void {
    if (!this.listeners.has(event)) {
      this.listeners.set(event, new Set());
    }
    this.listeners.get(event)!.add(listener);
  }
  
  off(event: string, listener: Function): void {
    const listeners = this.listeners.get(event);
    if (listeners) {
      listeners.delete(listener);
    }
  }
}

// Singleton
export const ttsService = new TTSService();
