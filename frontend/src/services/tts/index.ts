/**
 * TTS Service - Serviço de Text-to-Speech com múltiplos provedores
 * Suporta WebSpeech API e OpenAI TTS (SAPI5 foi unificado via backend_audio)
 */

import { logger } from '../../utils/logger';
import { TTSProvider, ITTSProvider, TTSVoice, TTSConfig } from './types';
import type { TTSModel, TTSSelectionMode } from './types';
import { ttsFactory } from './factory';
import { getStreamPlayer } from './streamPlayer';
import { calcTTSTimeoutMs } from '../../lib/audioUtils';

import { GetTTSModels, GetTTSVoices, SpeakPreview } from '@wailsjs/go/wailsapi/Speech';

export type VoiceSelection = {
 selectionMode: TTSSelectionMode | "";
 completeness: "complete" | "partial" | "unknown";
 voices: TTSVoice[];
};

/** Configuração de voz por role (assistant, user, system) */
export interface RoleVoiceConfig {
  providerId: string;   // "webspeech" ou LLM provider ID ("sapi5" delega ao backend)
  voiceId: string;      // ID da voz
  model: string;        // modelo TTS (ex: "tts-1")
  language?: string;    // idioma do perfil que pediu a fala
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
    this.currentProvider = ttsFactory.getProvider(type);
    
    if (!this.currentProvider) {
      logger.error('[TTSService] No provider available');
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
  getVoiceContext(role: VoiceRole): { providerId: string; voiceId: string; model: string; rate: number; language?: string } | undefined {
    const rc = this.roleConfigs.get(role);
    if (!rc) return undefined;
    return {
      providerId: rc.providerId,
      voiceId: rc.voiceId,
      model: rc.model,
      rate: rc.rate,
      ...(rc.language ? { language: rc.language } : {}),
    };
  }

  /**
   * Ponto único de reprodução de TTS.
   *
   * Usa a configuração de voz da role do perfil ativo.
   * Se não houver config para a role → retorna silenciosamente (no-op).
   * NÃO checa config.enabled — quem chama decide se deve reproduzir
   * (auto-read checa isAutoReadEnabled(), on-demand não checa nada).
   */
  async speakAsRole(text: string, role: VoiceRole, isCurrent?: () => boolean, onStarted?: () => void): Promise<void> {
    if (isCurrent && !isCurrent()) throw new Error('chat-message-stale');
    const roleConfig = this.roleConfigs.get(role);
    if (!roleConfig) return;

    await this.speakWithOverride(text, {
      providerId: roleConfig.providerId,
      voiceName: roleConfig.voiceId,
      rate: roleConfig.rate,
      pitch: roleConfig.pitch,
      volume: roleConfig.volume,
      ttsModel: roleConfig.model,
      isCurrent,
      onStarted,
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
  async speakWithOverride(text: string, options: { voiceName?: string; providerId?: string; rate?: number; pitch?: number; volume?: number; ttsModel?: string; language?: string; isCurrent?: () => boolean; onStarted?: () => void }): Promise<void> {
    const guard = () => { if (options.isCurrent && !options.isCurrent()) throw new Error('chat-message-stale'); };
    guard();
    const voiceId = options.voiceName;

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
        options.isCurrent,
        options.onStarted,
      );
      return;
    }

    // WebSpeech → usar provider frontend (funciona bem localmente)
    // Guard contra chamadas simultâneas que corromperiam o config global
    if (this.overrideLock) {
      try { await this.overrideLock; } catch { /* ignorar */ }
    }
    guard();
    
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
        guard();

        const provider = this.currentProvider;
        const hasEvents = typeof provider.addEventListener === 'function';

        if (hasEvents) {
          await new Promise<void>((resolve) => {
            const cleanup = () => {
              provider.removeEventListener?.('end', onEnd);
              provider.removeEventListener?.('error', onErr);
              provider.removeEventListener?.('start', onStart);
              clearTimeout(timeout);
            };
            const onEnd = () => { cleanup(); resolve(); };
            const onStart = () => { if (!options.isCurrent || options.isCurrent()) options.onStarted?.(); else { provider.stop(); cleanup(); resolve(); } };
            const onErr = () => { cleanup(); resolve(); };
            // Timeout proporcional ao tamanho do texto (60s base + 30s por 4000 chars)
            const timeoutMs = calcTTSTimeoutMs(text.length);
            const timeout = setTimeout(() => { cleanup(); resolve(); }, timeoutMs);
            provider.addEventListener?.('end', onEnd);
            provider.addEventListener?.('start', onStart);
            provider.addEventListener?.('error', onErr);
            provider.speak(text);
          });
        } else {
          const completion = provider.speak(text);
          options.onStarted?.();
          await completion;
        }
      }
    } finally {
      if (providerChanged) {
        await this.selectProvider(backupProvider);
      }
      this.config = backupConfig;
      if (this.currentProvider) {
        if (this.config.voiceName) {
          await this.currentProvider.setVoice(this.config.voiceName);
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
    isCurrent?: () => boolean,
    onStarted?: () => void,
  ): Promise<void> {
    if (isCurrent && !isCurrent()) throw new Error('chat-message-stale');
    const sessionId = `preview-${Date.now()}-${Math.random().toString(36).substr(2, 9)}`;

    this.activeStreamAbort?.abort();
    const streamPlayer = getStreamPlayer();
    this.activeStreamPlayer = streamPlayer;

    const abort = new AbortController();
    this.activeStreamAbort = abort;

    let complete!: () => void;
    const streamPromise = new Promise<void>(resolve => { complete = resolve; });
    const finish = () => { clearTimeout(timeout); complete(); };
    const cancel = () => {
      if (this.activeStreamAbort === abort) streamPlayer.stop();
      finish();
    };
    const timeout = setTimeout(cancel, calcTTSTimeoutMs(text.length));
    abort.signal.addEventListener('abort', cancel, { once: true });

    streamPlayer.startListening(sessionId, {
      isCurrent,
      onStart: () => {
        if (abort.signal.aborted) return;
        if (isCurrent && !isCurrent()) { abort.abort(); return; }
        this.emit('speakStart');
        onStarted?.();
      },
      onEnd: () => { finish(); this.emit('speakEnd'); },
      onError: () => { finish(); this.emit('speakEnd'); },
    });

    try {
      await Promise.race([
        SpeakPreview(providerId, model, voiceId, rate, volume, language, text, sessionId),
        streamPromise,
      ]);
      if (abort.signal.aborted) return;
      if (isCurrent && !isCurrent()) throw new Error('chat-message-stale');
      await streamPromise;
    } catch (error) {
      if (abort.signal.aborted) return;
      abort.abort();
      this.emit('speakEnd');
      throw error;
    } finally {
      clearTimeout(timeout);
      abort.signal.removeEventListener('abort', cancel);
      if (this.activeStreamAbort === abort) {
        this.activeStreamAbort = null;
        this.activeStreamPlayer = null;
      }
    }

  }

  /**   * Obtém o volume atual
   */
  getVolume(): number {
    return this.config.volume;
  }
  
  /**
   * Define o ID exato da voz no provedor explicitamente configurado
   */
  async setVoice(voiceName: string): Promise<void> {
    this.config.voiceName = voiceName;
    
    if (this.currentProvider) {
      await this.currentProvider.setVoice(voiceName);
    }

    this.emit('configChanged', this.config);
  }
  
  /**
   * Retorna lista de modelos TTS disponíveis de um provedor.
   */
  async getModelsForProvider(providerId: string): Promise<TTSModel[]> {

    if (!providerId || providerId === 'webspeech' || providerId === 'sapi5') {
      return [];
    }

    try {
      const models = await GetTTSModels(providerId);
      return (models || []).map((m) => ({
        id: m.id,
        name: m.name,
        provider: m.provider || providerId,
        selectionMode: m.selection_mode as TTSSelectionMode,
        description: m.description || m.name,
      }));
    } catch (error) {
      logger.error(`[TTSService] Erro ao buscar modelos TTS para ${providerId}:`, error);
      return [];
    }
  }

  /**
   * Retorna lista de vozes disponíveis para um provedor e modelo.
   */
  async getVoiceSelectionForProvider(providerId: string, modelId: string = ''): Promise<VoiceSelection> {
    if (providerId === 'webspeech') {
      await ttsFactory.initialize();
      const provider = ttsFactory.getProvider(TTSProvider.WEBSPEECH);
      return { selectionMode: '', completeness: 'complete', voices: provider ? await provider.getVoices() : [] };
    }
    const result = await GetTTSVoices(providerId, modelId);
    return {
      selectionMode: result.selection_mode as TTSSelectionMode | '',
      completeness: result.completeness as VoiceSelection['completeness'],
      voices: result.items.map((voice) => ({
        id: voice.id, name: voice.name, language: voice.language || '',
        provider: providerId, modelId: voice.model_id,
        gender: (voice.gender || 'neutral').toLowerCase() as 'neutral' | 'male' | 'female',
        localService: providerId === 'sapi5', description: voice.description || voice.name,
      })),
    };
  }

  async getVoicesForProvider(providerId: string, modelId: string = ''): Promise<TTSVoice[]> {
    return (await this.getVoiceSelectionForProvider(providerId, modelId)).voices;
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
          logger.error(`[TTSService] Error getting voices from ${providerType}:`, error);
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
