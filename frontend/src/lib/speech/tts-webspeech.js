/**
 * WebSpeech TTS (Text-to-Speech) Manager
 * Usa a SpeechSynthesis API nativa do navegador
 */

export class SpeechSynthesisManager {
  constructor(options = {}) {
    this.language = options.language || 'pt-BR';
    this.rate = options.rate || 1.0;      // 0.1 a 10
    this.pitch = options.pitch || 1.0;     // 0 a 2
    this.volume = options.volume || 1.0;   // 0 a 1
    this.voiceName = options.voiceName || null;
    
    this.synth = window.speechSynthesis;
    this.voice = null;
    this.isSpeaking = false;
    this.utterance = null;
    
    // Callbacks
    this.onStart = options.onStart || (() => {});
    this.onEnd = options.onEnd || (() => {});
    this.onPause = options.onPause || (() => {});
    this.onResume = options.onResume || (() => {});
    this.onError = options.onError || (() => {});
    this.onBoundary = options.onBoundary || (() => {}); // Palavra/sentença
    
    this._loadVoices();
  }
  
  _loadVoices() {
    // Vozes podem demorar para carregar
    const loadVoices = () => {
      const voices = this.synth.getVoices();
      if (voices.length > 0) {
        this._selectVoice(voices);
      }
    };
    
    loadVoices();
    
    // Alguns navegadores disparam evento quando vozes carregam
    if (this.synth.onvoiceschanged !== undefined) {
      this.synth.onvoiceschanged = loadVoices;
    }
  }
  
  _selectVoice(voices) {
    // Prioridade: nome específico > idioma pt-BR > qualquer pt > padrão
    if (this.voiceName) {
      this.voice = voices.find(v => v.name === this.voiceName);
    }
    
    if (!this.voice) {
      // Prefere vozes locais (não remote) para menor latência
      const ptBRVoices = voices.filter(v => v.lang.startsWith('pt-BR'));
      const localPtBR = ptBRVoices.find(v => v.localService);
      this.voice = localPtBR || ptBRVoices[0];
    }
    
    if (!this.voice) {
      const ptVoices = voices.filter(v => v.lang.startsWith('pt'));
      this.voice = ptVoices[0];
    }
    
    // Fallback para qualquer voz
    if (!this.voice && voices.length > 0) {
      this.voice = voices[0];
    }
  }
  
  /**
   * Verifica se o navegador suporta SpeechSynthesis
   */
  static isSupported() {
    return 'speechSynthesis' in window;
  }
  
  /**
   * Retorna lista de vozes disponíveis
   */
  getVoices() {
    return this.synth.getVoices().map(v => ({
      name: v.name,
      lang: v.lang,
      localService: v.localService,
      default: v.default
    }));
  }
  
  /**
   * Retorna vozes filtradas por idioma
   */
  getVoicesByLanguage(lang = 'pt') {
    return this.getVoices().filter(v => v.lang.startsWith(lang));
  }
  
  /**
   * Define a voz pelo nome
   */
  setVoice(voiceName) {
    this.voiceName = voiceName;
    const voices = this.synth.getVoices();
    this.voice = voices.find(v => v.name === voiceName);
    return !!this.voice;
  }
  
  /**
   * Fala o texto
   */
  speak(text) {
    if (!this.synth) {
      this.onError('SpeechSynthesis não suportado');
      return false;
    }
    
    // Cancela fala anterior se houver
    if (this.isSpeaking) {
      this.stop();
    }
    
    this.utterance = new SpeechSynthesisUtterance(text);
    
    if (this.voice) {
      this.utterance.voice = this.voice;
    }
    
    this.utterance.lang = this.language;
    this.utterance.rate = this.rate;
    this.utterance.pitch = this.pitch;
    this.utterance.volume = this.volume;
    
    this.utterance.onstart = () => {
      this.isSpeaking = true;
      this.onStart();
    };
    
    this.utterance.onend = () => {
      this.isSpeaking = false;
      this.onEnd();
    };
    
    this.utterance.onpause = () => {
      this.onPause();
    };
    
    this.utterance.onresume = () => {
      this.onResume();
    };
    
    this.utterance.onerror = (event) => {
      this.isSpeaking = false;
      this.onError(event.error);
    };
    
    this.utterance.onboundary = (event) => {
      this.onBoundary(event);
    };
    
    this.synth.speak(this.utterance);
    return true;
  }
  
  /**
   * Para a fala
   */
  stop() {
    if (this.synth) {
      this.synth.cancel();
      this.isSpeaking = false;
    }
  }
  
  /**
   * Pausa a fala
   */
  pause() {
    if (this.synth && this.isSpeaking) {
      this.synth.pause();
    }
  }
  
  /**
   * Retoma a fala
   */
  resume() {
    if (this.synth) {
      this.synth.resume();
    }
  }
  
  /**
   * Atualiza configurações
   */
  setRate(rate) {
    this.rate = Math.max(0.1, Math.min(10, rate));
  }
  
  setPitch(pitch) {
    this.pitch = Math.max(0, Math.min(2, pitch));
  }
  
  setVolume(volume) {
    this.volume = Math.max(0, Math.min(1, volume));
  }
}

