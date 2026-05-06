/**
 * TTS Service - Serviço de Text-to-Speech com múltiplos provedores
 * Suporta WebSpeech API e OpenAI TTS (SAPI5 foi unificado via backend_audio)
 */

import { TTSProvider, ITTSProvider, TTSVoice, TTSConfig } from './types';
import type { TTSModel } from './types';
import { ttsFactory } from './factory';
import { getStreamPlayer } from './streamPlayer';
import { calcTTSTimeoutMs } from '../../lib/audioUtils';

type WailsApp = {
  go?: {
    app?: {
      App?: {
        GetTTSModels?: (providerId: string) => Promise<BackendModel[]>;
        GetTTSVoices?: (providerId: string, modelId: string) => Promise<BackendVoice[]>;
        SpeakPreview?: (providerId: string, model: string, voiceId: string, rate: number, volume: number, language: string, text: string, sessionId: string) => Promise<void>;
      };
    };
    main?: {
      App?: {
        GetTTSModels?: (providerId: string) => Promise<BackendModel[]>;
        GetTTSVoices?: (providerId: string, modelId: string) => Promise<BackendVoice[]>;
        SpeakPreview?: (providerId: string, model: string, voiceId: string, rate: number, volume: number, language: string, text: string, sessionId: string) => Promise<void>;
      };
    };
  };
};

const getWailsApp = () => {
  const go = (window as unknown as WailsApp).go;
  return go?.app?.App ?? go?.main?.App;
};

type BackendVoice = {
  id: string;
  name: string;
  gender?: string;
  description?: string;
  provider?: string;
  model_id?: string;
};

type BackendModel = {
  id: string;
  name: string;
  provider?: string;
  selection_mode: 'model_and_voice' | 'model_only';
  description?: string;
};

const getTTSModels = async (providerId: string): Promise<BackendModel[]> => {
  const app = getWailsApp();
  if (!app?.GetTTSModels) return [];
  return app.GetTTSModels(providerId);
};

const getTTSVoices = async (providerId: string, modelId: string): Promise<BackendVoice[]> => {
  const app = getWailsApp();
  if (!app?.GetTTSVoices) return [];
  return app.GetTTSVoices(providerId, modelId);
};

/** Configuração de voz por role (assistant, user, system) */
export interface RoleVoiceConfig {
  providerId: string;   // "webspeech" ou LLM provider ID ("sapi5" delega ao backend)
  voiceId: string;      // ID da voz
  model: string;        // modelo TTS (ex: "tts-1")
  selectionMode?: 'model_and_voice' | 'model_only';
  rate: number;
  pitch: number;        // 0.5–2.0 (tom da voz)
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
  private activeStreamPlayer: { stop: () => void } | null = null;
  private activeStreamAbort: AbortController | null = null;
  private providerListeners: { provider: ITTSProvider; start: () => void; end: () => void; error: (e: CustomEvent<{ error: Error }>) => void } | null = null;
  /** Guard: previne chamadas simultâneas a speakWithOverride (webspeech path) */
  private overrideLock: Promise<void> | null = null;
  
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
    // Remove listeners do provider anterior (evita acúmulo em singletons)
    if (this.providerListeners) {
      const { provider: old, start, end, error } = this.providerListeners;
      old.removeEventListener?.('start', start);
      old.removeEventListener?.('end', end);
      old.removeEventListener?.('error', error);
      this.providerListeners = null;
    }

    if (!this.currentProvider) return;

    const onStart = () => { this.emit('speakStart'); };
    const onEnd = () => { this.emit('speakEnd'); };
    const onError = (event: CustomEvent<{ error: Error }>) => {
      this.emit('speakError', event.detail.error);
    };

    this.currentProvider.addEventListener?.('start', onStart);
    this.currentProvider.addEventListener?.('end', onEnd);
    this.currentProvider.addEventListener?.('error', onError);

    this.providerListeners = {
      provider: this.currentProvider,
      start: onStart,
      end: onEnd,
      error: onError,
    };
  }
  
  /**
   * Detecta o provider de uma voz pelo nome
   * Suporta formato "provider:voiceId" ou nome de voz direto
   */
  private detectProviderFromVoice(voiceName: string): TTSProvider {
    // Formato provider:voiceId (ex: "openai:alloy")
    if (voiceName.includes(':')) {
      const [provider] = voiceName.split(':');
      if (provider === 'openai') return TTSProvider.OPENAI;
      if (provider === 'webspeech') return TTSProvider.WEBSPEECH;
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
    this.emit('voiceConfigChanged');
  }

  /**
   * Remove a configuração de voz de uma role.
   */
  clearRoleConfig(role: VoiceRole): void {
    this.roleConfigs.delete(role);
    this.emit('voiceConfigChanged');
  }

  /**
   * Remove todas as configurações de role.
   */
  clearAllRoleConfigs(): void {
    this.roleConfigs.clear();
    this.emit('voiceConfigChanged');
  }

  /**
   * Verifica se há configuração de voz para uma role (ou qualquer role se omitida).
   * Usado pela UI para decidir se mostra botões de reprodução.
   */
  hasVoiceConfig(role?: VoiceRole): boolean {
    if (role) return this.roleConfigs.has(role);
    return this.roleConfigs.size > 0;
  }

  getRoleConfig(role: VoiceRole): RoleVoiceConfig | undefined {
    return this.roleConfigs.get(role);
  }

  /**
   * Retorna os parâmetros de provider TTS para uma role, prontos para
   * passar ao messageAudioService. Retorna undefined se a role não tem config.
   * Centraliza a montagem para evitar duplicação nos callers.
   */
  getVoiceContext(role: VoiceRole): { providerId: string; voiceId: string; model: string; rate: number } | undefined {
    const rc = this.roleConfigs.get(role);
    if (!rc) return undefined;
    return { providerId: rc.providerId, voiceId: rc.voiceId, model: rc.model, rate: rc.rate };
  }

  /**
   * Ponto único de reprodução de TTS.
   *
   * Usa a configuração de voz da role do perfil ativo.
   * Se não houver config para a role → retorna silenciosamente (no-op).
   * NÃO checa config.enabled — quem chama decide se deve reproduzir
   * (auto-read checa isAutoReadEnabled(), on-demand não checa nada).
   */
  async speakAsRole(text: string, role: VoiceRole): Promise<void> {
    const roleConfig = this.roleConfigs.get(role);
    if (!roleConfig) return;

    await this.speakWithOverride(text, {
      providerId: roleConfig.providerId,
      voiceName: roleConfig.voiceId,
      rate: roleConfig.rate,
      pitch: roleConfig.pitch,
      volume: roleConfig.volume,
      ttsModel: roleConfig.model,
    });
  }
  
  /**
   * Para toda reprodução de áudio (providers locais e stream player do backend)
   */
  stop(): void {
    if (this.currentProvider) {
      this.currentProvider.stop();
    }
    if (this.activeStreamPlayer) {
      this.activeStreamPlayer.stop();
      this.activeStreamPlayer = null;
    }
    if (this.activeStreamAbort) {
      this.activeStreamAbort.abort();
      this.activeStreamAbort = null;
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
   * Verifica se está falando (providers locais, API ou streaming backend)
   */
  isSpeaking(): boolean {
    if (this.activeStreamPlayer !== null) return true;
    if (this.activeStreamAbort !== null) return true;
    return this.currentProvider?.isSpeaking() || false;
  }
  
  /**
   * Verifica se TTS está habilitado
   */
  isEnabled(): boolean {
    return this.config.enabled;
  }
  
  /**
   * Verifica se auto-read está habilitado e há voz configurada
   */
  isAutoReadEnabled(): boolean {
    return this.config.enabled && this.config.autoRead && this.roleConfigs.size > 0;
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
   * Verifica se TTS está habilitado para mensagens do usuário e há voz configurada
   */
  isEnabledForUser(): boolean {
    return this.config.enabled && this.config.enabledForUser && this.roleConfigs.has('user');
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
   * Para "webspeech", usa o provider frontend.
   * Para "sapi5" e providers LLM, delega ao backend via SpeakPreview.
   */
  async speakWithOverride(text: string, options: { voiceName?: string; providerId?: string; rate?: number; pitch?: number; volume?: number; ttsModel?: string; language?: string }): Promise<void> {
    const voiceId = options.voiceName ? this.extractVoiceId(options.voiceName) : undefined;

    // Resolve o tipo de provider: webspeech ou LLM/sapi5 (backend)
    const resolvedProvider = this.resolveProviderType(options.providerId);

    // LLM providers (OpenAI-like) → delega ao backend que sabe criar o TTSClient correto
    if (resolvedProvider === 'llm') {
      await this.speakWithBackendPreview(
        text,
        options.providerId || '',
        options.ttsModel || '',
        voiceId || '',
        options.rate ?? 1.0,
        options.volume ?? 1.0,
        options.language ?? '',
      );
      return;
    }

    // WebSpeech → usar provider frontend (funciona bem localmente)
    // Guard contra chamadas simultâneas que corromperiam o config global
    if (this.overrideLock) {
      try { await this.overrideLock; } catch { /* ignorar */ }
    }
    
    let unlockOverride: () => void;
    this.overrideLock = new Promise<void>(resolve => { unlockOverride = resolve; });
    
    const backupConfig = { ...this.config };
    const backupProvider = this.config.provider;
    let providerChanged = false;

    try {
      const targetProvider = TTSProvider.WEBSPEECH;
      if (targetProvider !== this.config.provider) {
        await this.selectProvider(targetProvider);
        providerChanged = true;
      }

      if (this.currentProvider) {
        if (voiceId) await this.currentProvider.setVoice(voiceId);
        if (options.rate !== undefined) await this.currentProvider.setRate(options.rate);
        if (options.pitch !== undefined && typeof this.currentProvider.setPitch === 'function') await this.currentProvider.setPitch(options.pitch);
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
            // Timeout proporcional ao tamanho do texto (60s base + 30s por 4000 chars)
            const timeoutMs = calcTTSTimeoutMs(text.length);
            const timeout = setTimeout(() => { cleanup(); resolve(); }, timeoutMs);
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
      unlockOverride!();
      this.overrideLock = null;
    }
  }

  /**
   * Resolve o tipo de provider para roteamento.
   * Retorna 'webspeech' ou 'llm'.
   * SAPI5 é tratado como 'llm' — preview via backend SpeakPreview.
   */
  private resolveProviderType(providerId?: string): 'webspeech' | 'llm' {
    if (providerId) {
      if (providerId === 'webspeech') return 'webspeech';
      return 'llm';
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
    model: string,
    voiceId: string,
    rate: number,
    volume: number,
    language: string,
  ): Promise<void> {
    const app = getWailsApp();
    const speakPreview = app?.SpeakPreview as ((
      providerId: string, model: string, voiceId: string, rate: number, volume: number, language: string, text: string, sessionId: string,
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
      const streamPlayer = getStreamPlayer();
      this.activeStreamPlayer = streamPlayer;

      const abort = new AbortController();
      this.activeStreamAbort = abort;

      const streamPromise = new Promise<void>((resolve) => {
        const cleanup = () => {
          clearTimeout(timeout);
          abort.signal.removeEventListener('abort', onAbort);
          this.activeStreamPlayer = null;
          this.activeStreamAbort = null;
        };
        // Resolve imediatamente se stop() for chamado externamente
        const onAbort = () => { cleanup(); resolve(); };
        abort.signal.addEventListener('abort', onAbort, { once: true });

        // Timeout proporcional ao tamanho do texto
        const timeoutMs = calcTTSTimeoutMs(text.length);
        const timeout = setTimeout(() => { cleanup(); streamPlayer.stop(); resolve(); }, timeoutMs);

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

      try {
        await speakPreview(providerId, model, voiceId, rate, volume, language, text, sessionId);
        await streamPromise;
      } catch (error) {
        streamPlayer.stop();
        if (this.activeStreamAbort === abort) {
          this.activeStreamAbort = null;
        }
        if (this.activeStreamPlayer === streamPlayer) {
          this.activeStreamPlayer = null;
        }
        this.emit('speakEnd');
        throw error;
      }
    } else {
      // Fallback simples: só chama o backend (sem aguardar)
      await speakPreview(providerId, model, voiceId, rate, volume, language, text, sessionId);
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
   * Retorna lista de modelos TTS disponíveis de um provedor.
   */
  async getModelsForProvider(providerId: string): Promise<TTSModel[]> {
    await ttsFactory.initialize();

    if (!providerId || providerId === 'webspeech' || providerId === 'sapi5') {
      return [];
    }

    try {
      const models = await getTTSModels(providerId);
      return (models || []).map((m) => ({
        id: m.id,
        name: m.name,
        provider: m.provider || providerId,
        selectionMode: m.selection_mode,
        description: m.description || m.name,
      }));
    } catch (error) {
      console.error(`[TTSService] Erro ao buscar modelos TTS para ${providerId}:`, error);
      return [];
    }
  }

  /**
   * Retorna lista de vozes disponíveis para um provedor e modelo.
   */
  async getVoicesForProvider(providerId: string, modelId: string = ''): Promise<TTSVoice[]> {
    await ttsFactory.initialize();

    if (providerId === 'webspeech') {
      const p = ttsFactory.getProvider(TTSProvider.WEBSPEECH);
      return p ? await p.getVoices() : [];
    }
    if (providerId === 'sapi5') {
      try {
        const voices = await getTTSVoices(providerId, '');
        return (voices || []).map((v) => ({
          id: v.id,
          name: v.name,
          language: 'multilingual',
          provider: providerId,
          gender: (v.gender || 'neutral').toLowerCase() as 'neutral' | 'male' | 'female',
          premium: false,
          localService: true,
          description: v.description || v.name
        }));
      } catch (error) {
        console.error('[TTSService] Erro ao buscar vozes SAPI5:', error);
        return [];
      }
    }

    // Se for um provedor LLM registrado, busca via backend
    if (!modelId) return [];
    try {
      const voices = await getTTSVoices(providerId, modelId);
      return (voices || []).map((v) => ({
        id: v.id,
        name: v.name,
        language: 'multilingual',
        provider: providerId,
        modelId: v.model_id || modelId,
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

// Re-export types para backward compatibility
export type { TTSConfig } from './types';
