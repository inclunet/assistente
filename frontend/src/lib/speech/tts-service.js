/**
 * TTSService - Serviço unificado de Text-to-Speech
 * 
 * Suporta múltiplos backends:
 * - WebSpeech (navegador)
 * - SAPI5 (Windows via Wails)
 * - OpenAI TTS (via backend)
 * 
 * Uso:
 *   import { ttsService } from '$lib/speech/tts-service.js';
 *   
 *   ttsService.addEventListener('speakStart', () => console.log('Falando...'));
 *   ttsService.addEventListener('speakEnd', () => console.log('Terminou'));
 *   
 *   ttsService.setProvider('openai', { voiceId: 'alloy' });
 *   ttsService.speak('Olá mundo!');
 */

import { SpeechSynthesisManager } from './tts-webspeech.js';

// Imports do Wails serão resolvidos dinamicamente para evitar erros em ambientes sem Wails
let SpeakSAPI5 = null;
let StopSAPI5 = null;
let SetSAPI5Volume = null;
let SetSAPI5Rate = null;
let SynthesizeOpenAIWithVoice = null;
let SetOpenAITTSSpeed = null;

// Carrega funções do Wails se disponíveis
async function loadWailsFunctions() {
  try {
    const wails = await import('../../../wailsjs/go/main/App.js');
    SpeakSAPI5 = wails.SpeakSAPI5;
    StopSAPI5 = wails.StopSAPI5;
    SetSAPI5Volume = wails.SetSAPI5Volume;
    SetSAPI5Rate = wails.SetSAPI5Rate;
    SynthesizeOpenAIWithVoice = wails.SynthesizeOpenAIWithVoice;
    SetOpenAITTSSpeed = wails.SetOpenAITTSSpeed;
  } catch (e) {
    console.warn('Wails functions not available:', e.message);
  }
}

// Providers suportados
export const TTS_PROVIDERS = {
  DISABLED: 'disabled',
  WEBSPEECH: 'webspeech',
  SAPI5: 'sapi5',
  OPENAI: 'openai'
};

/**
 * Serviço de TTS com suporte a múltiplos backends
 */
class TTSService extends EventTarget {
  constructor() {
    super();
    
    // Estado
    this._provider = TTS_PROVIDERS.DISABLED;
    this._voice = null;
    this._voiceId = null; // ID específico (ex: openai voice id)
    this._volume = 100; // 0-100
    this._rate = 0; // -10 a 10
    this._isSpeaking = false;
    
    // Managers
    this._webSpeechManager = null;
    this._currentAudio = null; // Para OpenAI TTS
    
    // Inicialização
    this._initialized = false;
    this._initPromise = this._init();
  }
  
  async _init() {
    // Carrega funções do Wails
    await loadWailsFunctions();
    
    // Inicializa WebSpeech se suportado
    if (SpeechSynthesisManager.isSupported()) {
      this._webSpeechManager = new SpeechSynthesisManager({
        language: 'pt-BR',
        onStart: () => this._handleSpeakStart(),
        onEnd: () => this._handleSpeakEnd(),
        onError: (error) => this._handleError(error)
      });
    }
    
    this._initialized = true;
  }
  
  /**
   * Aguarda inicialização completa
   */
  async ready() {
    await this._initPromise;
  }
  
  // === Getters ===
  
  get provider() { return this._provider; }
  get voice() { return this._voice; }
  get volume() { return this._volume; }
  get rate() { return this._rate; }
  get isSpeaking() { return this._isSpeaking; }
  get isDisabled() { return this._provider === TTS_PROVIDERS.DISABLED; }
  
  /**
   * Verifica se WebSpeech está disponível
   */
  get isWebSpeechSupported() {
    return SpeechSynthesisManager.isSupported();
  }
  
  /**
   * Verifica se SAPI5 está disponível (Windows + Wails)
   */
  get isSAPI5Supported() {
    return !!SpeakSAPI5;
  }
  
  /**
   * Verifica se OpenAI TTS está disponível (precisa de API key)
   */
  get isOpenAISupported() {
    return !!SynthesizeOpenAIWithVoice;
  }
  
  // === Setters / Configuração ===
  
  /**
   * Define o provider de TTS
   * @param {string} provider - 'disabled', 'webspeech', 'sapi5', 'openai'
   * @param {Object} options - Opções específicas do provider
   * @param {string} options.voice - Nome da voz
   * @param {string} options.voiceId - ID da voz (para OpenAI)
   */
  setProvider(provider, options = {}) {
    this._provider = provider;
    
    if (options.voice) {
      this._voice = options.voice;
    }
    
    if (options.voiceId) {
      this._voiceId = options.voiceId;
    }
    
    // Configura WebSpeech se for o provider
    if (provider === TTS_PROVIDERS.WEBSPEECH && this._webSpeechManager && this._voice) {
      this._webSpeechManager.setVoice(this._voice);
    }
    
    this._dispatchEvent('providerChange', { provider, voice: this._voice });
  }
  
  /**
   * Define a voz
   * @param {string} voice - Nome da voz
   * @param {string} [voiceId] - ID da voz (para OpenAI)
   */
  setVoice(voice, voiceId = null) {
    this._voice = voice;
    this._voiceId = voiceId;
    
    if (this._provider === TTS_PROVIDERS.WEBSPEECH && this._webSpeechManager) {
      this._webSpeechManager.setVoice(voice);
    }
    
    this._dispatchEvent('voiceChange', { voice, voiceId });
  }
  
  /**
   * Define o volume
   * @param {number} volume - 0 a 100
   */
  async setVolume(volume) {
    this._volume = Math.max(0, Math.min(100, volume));
    
    // Aplica no WebSpeech
    if (this._webSpeechManager) {
      this._webSpeechManager.setVolume(this._volume / 100);
    }
    
    // Aplica no SAPI5
    if (this._provider === TTS_PROVIDERS.SAPI5 && SetSAPI5Volume) {
      try {
        await SetSAPI5Volume(this._volume);
      } catch (e) {
        console.error('Failed to set SAPI5 volume:', e);
      }
    }
    
    // OpenAI não tem controle de volume no backend, aplica no áudio
    if (this._currentAudio) {
      this._currentAudio.volume = this._volume / 100;
    }
    
    this._dispatchEvent('volumeChange', { volume: this._volume });
  }
  
  /**
   * Define a velocidade
   * @param {number} rate - -10 a 10
   */
  async setRate(rate) {
    this._rate = Math.max(-10, Math.min(10, rate));
    
    // Aplica no WebSpeech (converte de -10/10 para 0.1/10)
    if (this._webSpeechManager) {
      const webRate = this._rate <= 0 
        ? 1 + (this._rate * 0.09) 
        : 1 + (this._rate * 0.9);
      this._webSpeechManager.setRate(Math.max(0.1, Math.min(10, webRate)));
    }
    
    // Aplica no SAPI5 (já usa -10 a 10)
    if (this._provider === TTS_PROVIDERS.SAPI5 && SetSAPI5Rate) {
      try {
        await SetSAPI5Rate(this._rate);
      } catch (e) {
        console.error('Failed to set SAPI5 rate:', e);
      }
    }
    
    // Aplica no OpenAI TTS
    if (this._provider === TTS_PROVIDERS.OPENAI && SetOpenAITTSSpeed) {
      try {
        await SetOpenAITTSSpeed(this._rate);
      } catch (e) {
        console.error('Failed to set OpenAI TTS speed:', e);
      }
    }
    
    this._dispatchEvent('rateChange', { rate: this._rate });
  }
  
  // === Métodos principais ===
  
  /**
   * Fala o texto usando o provider configurado
   * @param {string} text - Texto para falar
   * @returns {Promise<boolean>} - true se iniciou com sucesso
   */
  async speak(text) {
    if (!text || this._provider === TTS_PROVIDERS.DISABLED) {
      return false;
    }
    
    // Para fala anterior se houver
    if (this._isSpeaking) {
      await this.stop();
    }
    
    await this.ready();
    
    try {
      switch (this._provider) {
        case TTS_PROVIDERS.OPENAI:
          return await this._speakOpenAI(text);
          
        case TTS_PROVIDERS.SAPI5:
          return await this._speakSAPI5(text);
          
        case TTS_PROVIDERS.WEBSPEECH:
          return this._speakWebSpeech(text);
          
        default:
          return false;
      }
    } catch (error) {
      this._handleError(error.message || 'Erro ao falar');
      return false;
    }
  }
  
  /**
   * Para a fala atual
   */
  async stop() {
    // Para OpenAI
    if (this._currentAudio) {
      this._currentAudio.pause();
      this._currentAudio.currentTime = 0;
      this._currentAudio = null;
    }
    
    // Para SAPI5
    if (this._provider === TTS_PROVIDERS.SAPI5 && StopSAPI5) {
      try {
        await StopSAPI5();
      } catch (e) {
        console.error('SAPI5 stop error:', e);
      }
    }
    
    // Para WebSpeech
    if (this._webSpeechManager) {
      this._webSpeechManager.stop();
    }
    
    if (this._isSpeaking) {
      this._isSpeaking = false;
      this._dispatchEvent('speakEnd', { interrupted: true });
    }
  }
  
  /**
   * Pausa a fala (apenas WebSpeech)
   */
  pause() {
    if (this._provider === TTS_PROVIDERS.WEBSPEECH && this._webSpeechManager) {
      this._webSpeechManager.pause();
    } else if (this._currentAudio) {
      this._currentAudio.pause();
    }
  }
  
  /**
   * Retoma a fala (apenas WebSpeech)
   */
  resume() {
    if (this._provider === TTS_PROVIDERS.WEBSPEECH && this._webSpeechManager) {
      this._webSpeechManager.resume();
    } else if (this._currentAudio) {
      this._currentAudio.play();
    }
  }
  
  // === Métodos de backend ===
  
  async _speakOpenAI(text) {
    if (!SynthesizeOpenAIWithVoice || !this._voiceId) {
      // Fallback para WebSpeech
      return this._speakWebSpeech(text);
    }
    
    try {
      this._handleSpeakStart();
      
      const result = await SynthesizeOpenAIWithVoice(text, this._voiceId);
      
      if (result && result.audioBase64) {
        this._currentAudio = new Audio(`data:audio/mp3;base64,${result.audioBase64}`);
        this._currentAudio.volume = this._volume / 100;
        
        this._currentAudio.onended = () => {
          this._currentAudio = null;
          this._handleSpeakEnd();
        };
        
        this._currentAudio.onerror = (e) => {
          this._currentAudio = null;
          this._handleError('Erro ao reproduzir áudio');
        };
        
        await this._currentAudio.play();
        return true;
      }
      
      return false;
    } catch (e) {
      console.error('OpenAI TTS error:', e);
      // Fallback para WebSpeech
      return this._speakWebSpeech(text);
    }
  }
  
  async _speakSAPI5(text) {
    if (!SpeakSAPI5) {
      // Fallback para WebSpeech
      return this._speakWebSpeech(text);
    }
    
    try {
      this._handleSpeakStart();
      await SpeakSAPI5(text, this._voice);
      this._handleSpeakEnd();
      return true;
    } catch (e) {
      console.error('SAPI5 speak error:', e);
      // Fallback para WebSpeech
      return this._speakWebSpeech(text);
    }
  }
  
  _speakWebSpeech(text) {
    if (!this._webSpeechManager) {
      this._handleError('WebSpeech não suportado');
      return false;
    }
    
    return this._webSpeechManager.speak(text);
  }
  
  // === Handlers internos ===
  
  _handleSpeakStart() {
    this._isSpeaking = true;
    this._dispatchEvent('speakStart', {});
  }
  
  _handleSpeakEnd() {
    this._isSpeaking = false;
    this._dispatchEvent('speakEnd', { interrupted: false });
  }
  
  _handleError(message) {
    this._isSpeaking = false;
    this._dispatchEvent('error', { message });
  }
  
  _dispatchEvent(type, detail) {
    this.dispatchEvent(new CustomEvent(type, { detail }));
  }
  
  // === Utilitários ===
  
  /**
   * Lista vozes disponíveis do WebSpeech
   */
  getWebSpeechVoices() {
    if (!this._webSpeechManager) return [];
    return this._webSpeechManager.getVoices();
  }
  
  /**
   * Lista vozes do WebSpeech por idioma
   */
  getWebSpeechVoicesByLanguage(lang = 'pt') {
    if (!this._webSpeechManager) return [];
    return this._webSpeechManager.getVoicesByLanguage(lang);
  }
  
  /**
   * Limpa texto para fala (remove markdown, etc.)
   */
  static cleanTextForSpeech(text) {
    if (!text) return '';
    
    return text
      .replace(/```[\s\S]*?```/g, 'bloco de código omitido')
      .replace(/`[^`]+`/g, (match) => match.slice(1, -1))
      .replace(/#{1,6}\s/g, '')
      .replace(/\*\*([^*]+)\*\*/g, '$1')
      .replace(/\*([^*]+)\*/g, '$1')
      .replace(/__([^_]+)__/g, '$1')
      .replace(/_([^_]+)_/g, '$1')
      .replace(/~~([^~]+)~~/g, '$1')
      .replace(/\[([^\]]+)\]\([^)]+\)/g, '$1')
      .replace(/[-*+]\s/g, '')
      .replace(/\n+/g, '. ')
      .trim();
  }
}

// Exporta instância singleton
export const ttsService = new TTSService();

// Exporta classe para quem quiser criar instâncias próprias
export { TTSService };




