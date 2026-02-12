/**
 * TTS Service - Serviço de Text-to-Speech com múltiplos provedores
 * Suporta WebSpeech API, SAPI5 (Windows) e OpenAI TTS
 */

import { TTSProvider, ITTSProvider, TTSVoice } from './types';
import { ttsFactory } from './factory';

export interface TTSConfig {
  enabled: boolean;
  autoRead: boolean;           // Leitura automática de mensagens do assistente
  enabledForUser: boolean;     // TTS para mensagens do usuário
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
    enabledForUser: false,
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
    if (this.initialized) return;
    
    // Inicializa factory de provedores
    await ttsFactory.initialize();
    
    // Seleciona provider inicial
    await this.selectProvider(this.config.provider);
    
    this.initialized = true;
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
  
  /**
   * Detecta o provider de uma voz pelo nome
   * Suporta formato "provider:voiceId" ou nome de voz direto
   */
  private detectProviderFromVoice(voiceName: string): TTSProvider {
    // Formato provider:voiceId (ex: "openai:alloy", "sapi5:Microsoft David")
    if (voiceName.includes(':')) {
      const [provider] = voiceName.split(':');
      if (provider === 'openai') return TTSProvider.OPENAI;
      if (provider === 'sapi5') return TTSProvider.SAPI5;
      if (provider === 'webspeech') return TTSProvider.WEBSPEECH;
    }
    
    // Formato legado - detecta pelo nome da voz
    if (voiceName.includes('Microsoft') || voiceName.includes('SAPI')) {
      return TTSProvider.SAPI5;
    }
    if (voiceName.match(/^(alloy|echo|fable|onyx|nova|shimmer)$/)) {
      return TTSProvider.OPENAI;
    }
    return TTSProvider.WEBSPEECH;
  }
  
  /**
   * Extrai apenas o ID da voz (remove prefixo provider:)
   */
  private extractVoiceId(voiceName: string): string {
    if (voiceName.includes(':')) {
      return voiceName.split(':').slice(1).join(':');
    }
    return voiceName;
  }
  
  /**
   * Fala um texto (somente se TTS estiver habilitado para leitura automática)
   */
  async speak(text: string): Promise<void> {
    if (!this.config.enabled || !this.currentProvider) return;
    await this.currentProvider.speak(text);
  }

  /**
   * Fala um texto sob demanda (ignora config.enabled)
   * Usado para ações explícitas do usuário como "Ouvir mensagem" ou tecla Espaço
   */
  async speakOnDemand(text: string): Promise<void> {
    if (!this.currentProvider) {
      console.warn('[TTSService] Nenhum provider configurado para fala sob demanda');
      return;
    }
    console.log('[TTSService] Fala sob demanda:', text.substring(0, 50) + '...');
    await this.currentProvider.speak(text);
  }

  /**
   * Sintetiza texto em áudio SEM tocar (somente se TTS estiver habilitado)
   * Retorna Blob para uso com sistema de audio por mensagem
   */
  async synthesizeForMessage(text: string): Promise<Blob | null> {
    if (!this.config.enabled || !this.currentProvider) return null;

    // Verifica se provider suporta synthesize
    if (!this.currentProvider.synthesize) return null;

    try {
      return await this.currentProvider.synthesize(text);
    } catch (error) {
      console.error('[TTSService] Erro ao sintetizar:', error);
      return null;
    }
  }

  /**
   * Sintetiza texto em áudio sob demanda (ignora config.enabled)
   * Usado para ações explícitas do usuário como "Baixar áudio"
   */
  async synthesizeOnDemand(text: string): Promise<Blob | null> {
    if (!this.currentProvider) {
      console.warn('[TTSService] Nenhum provider configurado para síntese sob demanda');
      return null;
    }

    // Verifica se provider suporta synthesize
    if (!this.currentProvider.synthesize) {
      console.warn('[TTSService] Provider não suporta síntese:', this.config.provider);
      return null;
    }

    try {
      console.log('[TTSService] Síntese sob demanda:', text.substring(0, 50) + '...');
      return await this.currentProvider.synthesize(text);
    } catch (error) {
      console.error('[TTSService] Erro ao sintetizar sob demanda:', error);
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
    this.emit('configChanged', this.config);
    
    if (!enabled) {
      this.stop();
    }
  }
  
  /**
   * Habilita/desabilita leitura automática (mensagens do assistente)
   */
  setAutoRead(autoRead: boolean): void {
    this.config.autoRead = autoRead;
    this.emit('configChanged', this.config);
  }
  
  /**
   * Verifica se TTS está habilitado para mensagens do usuário
   */
  isEnabledForUser(): boolean {
    return this.config.enabled && this.config.enabledForUser;
  }
  
  /**
   * Habilita/desabilita TTS para mensagens do usuário
   */
  setEnabledForUser(enabled: boolean): void {
    this.config.enabledForUser = enabled;
    this.emit('configChanged', this.config);
  }
  
  /**
   * Verifica se deve usar aria-live para mensagens do assistente
   * (usa aria-live quando TTS NÃO está ativo para o assistente)
   */
  shouldUseAriaLiveForAgent(): boolean {
    return !this.isAutoReadEnabled();
  }
  
  /**
   * Verifica se deve usar aria-live para mensagens do usuário
   * (usa aria-live quando TTS NÃO está ativo para o usuário)
   */
  shouldUseAriaLiveForUser(): boolean {
    return !this.isEnabledForUser();
  }
  
  /**
   * Define o provider
   */
  async setProvider(provider: TTSProvider): Promise<void> {
    this.config.provider = provider;
    await this.selectProvider(provider);
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
    this.emit('configChanged', this.config);
  }
  
  /**
   * Define o tom de voz (apenas WebSpeech)
   */
  setPitch(pitch: number): void {
    this.config.pitch = pitch;
    if (this.currentProvider?.name === TTSProvider.WEBSPEECH) {
      (this.currentProvider as any).setPitch?.(pitch);
    }
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
   * Aceita formato "provider:voiceId" ou nome de voz direto
   */
  async setVoice(voiceName: string): Promise<void> {
    this.config.voiceName = voiceName;
    
    // Detecta provider da voz
    const detectedProvider = this.detectProviderFromVoice(voiceName);
    
    // Extrai apenas o ID da voz (remove prefixo provider:)
    const voiceId = this.extractVoiceId(voiceName);
    
    console.log('[TTSService] setVoice:', { voiceName, detectedProvider, voiceId });
    
    // Troca de provider se necessário
    if (detectedProvider !== this.config.provider) {
      this.config.provider = detectedProvider;
      await this.selectProvider(detectedProvider);
    }
    
    // Define a voz no provider (usa o ID limpo, sem prefixo)
    if (this.currentProvider) {
      await this.currentProvider.setVoice(voiceId);
    }
    
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
