/**
 * Factory para criação e gerenciamento de provedores TTS
 */

import { logger } from '../../utils/logger';
import { TTSProvider, ITTSProvider } from './types';
import { WebSpeechProvider } from './providers/webSpeech';
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
    const openai = new OpenAIProvider();

    // Inicializa em paralelo
    await Promise.all([
      webSpeech.initialize().catch(err => 
        logger.warn('[TTSFactory] WebSpeech init failed:', err)
      ),
      openai.initialize().catch(err => 
        logger.warn('[TTSFactory] OpenAI init failed:', err)
      )
    ]);

    // Registra provedores disponíveis
    if (webSpeech.isAvailable) {
      this.providers.set(TTSProvider.WEBSPEECH, webSpeech);
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
