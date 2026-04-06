/**
 * TTS Service - Serviço de Text-to-Speech com múltiplos provedores
 * Suporta WebSpeech API, SAPI5 (Windows) e OpenAI TTS
 */

import { TTSProvider, ITTSProvider, TTSVoice } from './types';
import { ttsFactory } from './factory';

type WailsApp = {
  go?: {
    main?: {
      App?: {
        GetTTSVoices?: (profileId: string, providerId: string) => Promise<BackendVoice[]>;
        SpeakPreview?: (providerId: string, voiceId: string, model: string, rate: number, volume: number, text: string, sessionId: string) => Promise<void>;
      };
    };
  };
};

type BackendVoice = {
  id: string;
  name: string;
  gender?: string;
  description?: string;
};

const getTTSVoices = async (profileId: string, providerId: string): Promise<BackendVoice[]> => {
  const app = (window as unknown as WailsApp).go?.main?.App;
  if (!app?.GetTTSVoices) return [];
  return app.GetTTSVoices(profileId, providerId);
};

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

/** Configuração de voz por role (assistant, user, system) */
export interface RoleVoiceConfig {
  providerId: string;   // "webspeech", "sapi5", ou LLM provider ID
  voiceId: string;      // ID da voz
  model: string;        // modelo TTS (ex: "tts-1")
  rate: number;
  volume: number;
}

export type VoiceRole = 'assistant' | 'user' | 'system';

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
  private roleConfigs: Map<VoiceRole, RoleVoiceConfig> = new Map();
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

    this.currentProvider.addEventListener?.('start', () => {
      this.emit('speakStart');
    });

    this.currentProvider.addEventListener?.('end', () => {
      this.emit('speakEnd');
    });

    this.currentProvider.addEventListener?.('error', (event: CustomEvent<{ error: Error }>) => {
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
    if (voiceName.match(/^(alloy|ash|ballad|coral|echo|fable|nova|onyx|sage|shimmer|verse)$/)) {
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
   * Configura a voz de uma role específica (assistant, user, system).
   * Usado pelo hook de perfil para sincronizar voice configs do perfil ativo.
   */
  setRoleConfig(role: VoiceRole, config: RoleVoiceConfig): void {
    this.roleConfigs.set(role, config);
  }

  /**
   * Fala texto usando a voz configurada para uma role específica.
   * Delega para speakWithOverride com as configs da role.
   */
  async speakAsRole(text: string, role: VoiceRole): Promise<void> {
    if (!this.config.enabled) return;

    const roleConfig = this.roleConfigs.get(role);
    if (!roleConfig) {
      // Fallback: usa o provider padrão (assistant config)
      if (this.currentProvider) {
        await this.currentProvider.speak(text);
      }
      return;
    }

    await this.speakWithOverride(text, {
      providerId: roleConfig.providerId,
      voiceName: roleConfig.voiceId,
      rate: roleConfig.rate,
      volume: roleConfig.volume,
      ttsModel: roleConfig.model,
    });
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
      this.currentProvider.setPitch?.(pitch);
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

  /**   * Executa uma fala com configurações temporárias (override)
   * Útil para previews em telas de configuração sem alterar o estado global.
   *
   * Se `providerId` é fornecido, usa-o diretamente em vez de adivinhar pelo nome da voz.
   * Para providers LLM (ex: "openai-default-xxx"), delega ao backend via SpeakPreview.
   * Para "webspeech" e "sapi5", usa os providers frontend.
   */
  async speakWithOverride(text: string, options: { voiceName?: string; providerId?: string; rate?: number; volume?: number; ttsModel?: string }): Promise<void> {
    const voiceId = options.voiceName ? this.extractVoiceId(options.voiceName) : undefined;

    // Resolve o tipo de provider: webspeech, sapi5, ou LLM (OpenAI-like)
    const resolvedProvider = this.resolveProviderType(options.providerId, options.voiceName);

    // LLM providers (OpenAI-like) → delega ao backend que sabe criar o TTSClient correto
    if (resolvedProvider === 'llm') {
      await this.speakWithBackendPreview(
        text,
        options.providerId || '',
        voiceId || '',
        options.ttsModel || '',
        options.rate ?? 1.0,
        options.volume ?? 1.0,
      );
      return;
    }

    // WebSpeech e SAPI5 → usar providers frontend (funcionam bem localmente)
    const backupConfig = { ...this.config };
    const backupProvider = this.config.provider;
    let providerChanged = false;

    try {
      const targetProvider = resolvedProvider === 'sapi5' ? TTSProvider.SAPI5 : TTSProvider.WEBSPEECH;
      if (targetProvider !== this.config.provider) {
        await this.selectProvider(targetProvider);
        providerChanged = true;
      }

      if (this.currentProvider) {
        if (voiceId) await this.currentProvider.setVoice(voiceId);
        if (options.rate !== undefined) await this.currentProvider.setRate(options.rate);
        if (options.volume !== undefined) await this.currentProvider.setVolume(options.volume);

        const provider = this.currentProvider;
        const hasEvents = typeof provider.addEventListener === 'function';

        if (hasEvents) {
          await new Promise<void>((resolve) => {
            const cleanup = () => {
              provider.removeEventListener?.('end', onEnd);
              provider.removeEventListener?.('error', onErr);
              clearTimeout(timeout);
            };
            const onEnd = () => { cleanup(); resolve(); };
            const onErr = () => { cleanup(); resolve(); };
            const timeout = setTimeout(() => { cleanup(); resolve(); }, 30000);
            provider.addEventListener?.('end', onEnd);
            provider.addEventListener?.('error', onErr);
            provider.speak(text);
          });
        } else {
          await provider.speak(text);
        }
      }
    } finally {
      if (providerChanged) {
        await this.selectProvider(backupProvider);
      }
      this.config = backupConfig;
      if (this.currentProvider) {
        if (this.config.voiceName) {
          await this.currentProvider.setVoice(this.extractVoiceId(this.config.voiceName));
        }
        await this.currentProvider.setRate(this.config.rate);
        await this.currentProvider.setVolume(this.config.volume);
      }
    }
  }

  /**
   * Resolve o tipo de provider para roteamento.
   * Retorna 'webspeech', 'sapi5' ou 'llm'.
   */
  private resolveProviderType(providerId?: string, voiceName?: string): 'webspeech' | 'sapi5' | 'llm' {
    // Se há providerId explícito, usa-o
    if (providerId) {
      if (providerId === 'webspeech') return 'webspeech';
      if (providerId === 'sapi5') return 'sapi5';
      // Qualquer outro ID é um LLM provider (ex: "openai-default-xxx", "litellm-123")
      return 'llm';
    }

    // Fallback: detecta pelo nome da voz (compatibilidade com chamadas legadas)
    if (voiceName) {
      const detected = this.detectProviderFromVoice(voiceName);
      if (detected === TTSProvider.SAPI5) return 'sapi5';
      if (detected === TTSProvider.OPENAI) return 'llm';
    }

    return 'webspeech';
  }

  /**
   * Executa preview de voz via backend (para providers LLM/OpenAI-like).
   * O backend cria o TTSClient com as credenciais corretas do provider.
   */
  private async speakWithBackendPreview(
    text: string,
    providerId: string,
    voiceId: string,
    model: string,
    rate: number,
    volume: number,
  ): Promise<void> {
    const app = (window as unknown as WailsApp).go?.main?.App;
    const speakPreview = app?.SpeakPreview as ((
      providerId: string, voiceId: string, model: string, rate: number, volume: number, text: string, sessionId: string,
    ) => Promise<void>) | undefined;

    if (!speakPreview) {
      console.error('[TTSService] SpeakPreview não disponível no backend');
      return;
    }

    const sessionId = `preview-${Date.now()}-${Math.random().toString(36).substr(2, 9)}`;

    // Usa o OpenAI provider do frontend para gerenciar o streaming player
    const openaiProvider = ttsFactory.getProvider(TTSProvider.OPENAI);
    if (openaiProvider && 'streamPlayer' in openaiProvider) {
      // Reutiliza a infraestrutura de streaming do OpenAI provider
      const { getStreamPlayer } = await import('./streamPlayer');
      const streamPlayer = getStreamPlayer();

      const streamPromise = new Promise<void>((resolve) => {
        const cleanup = () => clearTimeout(timeout);
        const timeout = setTimeout(() => { cleanup(); streamPlayer.stop(); resolve(); }, 30000);

        streamPlayer.startListening(sessionId, {
          onStart: () => {
            this.emit('speakStart');
          },
          onEnd: () => {
            cleanup();
            this.emit('speakEnd');
            resolve();
          },
          onError: () => {
            cleanup();
            this.emit('speakEnd');
            resolve();
          },
        });
      });

      await speakPreview(providerId, voiceId, model, rate, volume, text, sessionId);
      await streamPromise;
    } else {
      // Fallback simples: só chama o backend (sem aguardar)
      await speakPreview(providerId, voiceId, model, rate, volume, text, sessionId);
    }
  }

  /**   * Obtém o volume atual
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
   * Retorna lista de vozes disponíveis de um provedor e perfil específicos
   */
  async getVoicesForProvider(providerId: string, profileId: string): Promise<TTSVoice[]> {
    await ttsFactory.initialize();

    if (providerId === 'webspeech') {
      const p = ttsFactory.getProvider(TTSProvider.WEBSPEECH);
      return p ? await p.getVoices() : [];
    }
    if (providerId === 'sapi5') {
      try {
        const voices = await getTTSVoices(profileId, providerId);
        if (voices && voices.length > 0) {
          return voices.map((v) => ({
            id: v.name,
            name: v.name,
            language: 'multilingual',
            provider: providerId,
            gender: (v.gender || 'neutral').toLowerCase() as 'neutral' | 'male' | 'female',
            premium: false,
            localService: true,
            description: v.description || v.name
          }));
        }
      } catch (error) {
        console.error('[TTSService] Erro ao buscar vozes SAPI5:', error);
      }

      const p = ttsFactory.getProvider(TTSProvider.SAPI5);
      return p ? await p.getVoices() : [];
    }

    // Se for um provedor LLM registrado, busca via backend
    try {
      const voices = await getTTSVoices(profileId, providerId);
      return (voices || []).map((v) => ({
        id: v.id,
        name: v.name,
        language: 'multilingual',
        provider: providerId,
        gender: (v.gender || 'neutral').toLowerCase() as 'neutral' | 'male' | 'female',
        premium: true,
        localService: false,
        description: v.description || v.name
      }));
    } catch (error) {
      console.error(`[TTSService] Erro ao buscar vozes para ${providerId}:`, error);
      return [];
    }
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
      return this.currentProvider.getNativeVoices?.() || [];
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
  private emit(event: string, data?: unknown): void {
    const listeners = this.listeners.get(event);
    if (listeners) {
      listeners.forEach(listener => listener(data));
    }
  }

  on(event: string, listener: (payload?: unknown) => void): void {
    if (!this.listeners.has(event)) {
      this.listeners.set(event, new Set());
    }
    this.listeners.get(event)!.add(listener);
  }

  off(event: string, listener: (payload?: unknown) => void): void {
    const listeners = this.listeners.get(event);
    if (listeners) {
      listeners.delete(listener);
    }
  }
}

// Singleton
export const ttsService = new TTSService();
