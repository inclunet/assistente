/**
 * Factory para criação e gerenciamento de provedores TTS
 */

import { TTSProvider, ITTSProvider } from './types';
import { WebSpeechProvider } from './providers/webSpeech';
import { SAPI5Provider } from './providers/sapi5';
import { OpenAIProvider } from './providers/openai';

export class TTSProviderFactory {
  private providers: Map<TTSProvider, ITTSProvider> = new Map();
  private initialized: boolean = false;

  /**
   * Inicializa todos os provedores disponíveis
   */
  async initialize(): Promise<void> {
    if (this.initialized) {
      return;
    }

    // Cria instâncias dos provedores
    const webSpeech = new WebSpeechProvider();
    const sapi5 = new SAPI5Provider();
    const openai = new OpenAIProvider();

    // Inicializa em paralelo
    await Promise.all([
      webSpeech.initialize().catch(err => 
        console.warn('[TTSFactory] WebSpeech init failed:', err)
      ),
      sapi5.initialize().catch(err => 
        console.warn('[TTSFactory] SAPI5 init failed:', err)
      ),
      openai.initialize().catch(err => 
        console.warn('[TTSFactory] OpenAI init failed:', err)
      )
    ]);

    // Registra provedores disponíveis
    if (webSpeech.isAvailable) {
      this.providers.set(TTSProvider.WEBSPEECH, webSpeech);
    }

    if (sapi5.isAvailable) {
      this.providers.set(TTSProvider.SAPI5, sapi5);
    }

    if (openai.isAvailable) {
      this.providers.set(TTSProvider.OPENAI, openai);
    }

    this.initialized = true;
  }

  /**
   * Obtém um provider específico
   */
  getProvider(type: TTSProvider): ITTSProvider | null {
    return this.providers.get(type) || null;
  }

  /**
   * Obtém todos os provedores disponíveis
   */
  getAvailableProviders(): TTSProvider[] {
    return Array.from(this.providers.keys());
  }

  /**
   * Verifica se um provider está disponível
   */
  isProviderAvailable(type: TTSProvider): boolean {
    return this.providers.has(type);
  }

  /**
   * Obtém provider baseado no nome da voz
   * Útil para detectar automaticamente o provider de uma voz
   */
  async getProviderByVoiceName(voiceName: string): Promise<ITTSProvider | null> {
    // Verifica todos os providers para ver qual tem essa voz
    for (const provider of this.providers.values()) {
      try {
        const voices = await provider.getVoices();
        if (voices.some(v => v.id === voiceName || v.name === voiceName)) {
          return provider;
        }
      } catch (error) {
        console.warn('[TTSFactory] Error checking voices for provider:', error);
      }
    }
    
    // Fallback: retorna WebSpeech se disponível
    return this.getProvider(TTSProvider.WEBSPEECH);
  }

  /**
   * Obtém o provider com fallback automático
   * Tenta na ordem: requested -> SAPI5 -> WebSpeech
   */
  getProviderWithFallback(requested: TTSProvider): ITTSProvider | null {
    // Tenta o provider solicitado
    let provider = this.getProvider(requested);
    if (provider) {
      return provider;
    }

    // Fallback para SAPI5 (Windows)
    provider = this.getProvider(TTSProvider.SAPI5);
    if (provider) {
      return provider;
    }

    // Fallback final para WebSpeech
    provider = this.getProvider(TTSProvider.WEBSPEECH);
    if (provider) {
      return provider;
    }

    console.error('[TTSFactory] No providers available');
    return null;
  }

  /**
   * Limpa todos os providers
   */
  dispose(): void {
    for (const provider of this.providers.values()) {
      provider.dispose();
    }
    this.providers.clear();
    this.initialized = false;
  }
}

// Singleton instance
export const ttsFactory = new TTSProviderFactory();
