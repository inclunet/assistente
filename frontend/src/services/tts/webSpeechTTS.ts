/**
 * WebSpeech TTS (Text-to-Speech) Manager
 * Usa a SpeechSynthesis API nativa do navegador
 */

export interface SpeechSynthesisOptions {
  language?: string;
  rate?: number;     // 0.1 a 10
  pitch?: number;    // 0 a 2
  volume?: number;   // 0 a 1
  voiceName?: string | null;
  onStart?: () => void;
  onEnd?: () => void;
  onPause?: () => void;
  onResume?: () => void;
  onError?: (error: Error) => void;
  onBoundary?: (event: SpeechSynthesisEvent) => void;
}

export class SpeechSynthesisManager {
  private language: string;
  private rate: number;
  private pitch: number;
  private volume: number;
  private voiceName: string | null;
  
  private synth: SpeechSynthesis;
  private voice: SpeechSynthesisVoice | null = null;
  private isSpeaking: boolean = false;
  private utterance: SpeechSynthesisUtterance | null = null;
  
  private onStart: () => void;
  private onEnd: () => void;
  private onPause: () => void;
  private onResume: () => void;
  private onError: (error: Error) => void;
  private onBoundary: (event: SpeechSynthesisEvent) => void;
  
  constructor(options: SpeechSynthesisOptions = {}) {
    this.language = options.language || 'pt-BR';
    this.rate = options.rate || 1.0;
    this.pitch = options.pitch || 1.0;
    this.volume = options.volume || 1.0;
    this.voiceName = options.voiceName || null;
    
    this.synth = window.speechSynthesis;
    
    // Callbacks
    this.onStart = options.onStart || (() => {});
    this.onEnd = options.onEnd || (() => {});
    this.onPause = options.onPause || (() => {});
    this.onResume = options.onResume || (() => {});
    this.onError = options.onError || (() => {});
    this.onBoundary = options.onBoundary || (() => {});
    
    this._loadVoices();
  }
  
  private _loadVoices(): void {
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
  
  private _selectVoice(voices: SpeechSynthesisVoice[]): void {
    // Prioridade: nome específico > idioma pt-BR > qualquer pt > padrão
    if (this.voiceName) {
      this.voice = voices.find(v => v.name === this.voiceName) || null;
    }
    
    if (!this.voice) {
      // Prefere vozes locais (não remote) para menor latência
      const ptBRVoices = voices.filter(v => v.lang.startsWith('pt-BR'));
      const localPtBR = ptBRVoices.find(v => v.localService);
      this.voice = localPtBR || ptBRVoices[0] || null;
    }
    
    if (!this.voice) {
      const ptVoices = voices.filter(v => v.lang.startsWith('pt'));
      this.voice = ptVoices[0] || null;
    }
    
    // Fallback para qualquer voz
    if (!this.voice && voices.length > 0) {
      this.voice = voices[0];
    }
  }
  
  /**
   * Verifica se o navegador suporta SpeechSynthesis
   */
  static isSupported(): boolean {
    return 'speechSynthesis' in window;
  }
  
  /**
   * Retorna lista de vozes disponíveis
   */
  getVoices(): SpeechSynthesisVoice[] {
    return this.synth.getVoices();
  }
  
  /**
   * Fala um texto
   */
  speak(text: string): void {
    if (!text || text.trim().length === 0) {
      return;
    }
    
    // Cancela fala anterior se houver
    this.stop();
    
    // Cria nova utterance
    this.utterance = new SpeechSynthesisUtterance(text);
    this.utterance.lang = this.language;
    this.utterance.rate = this.rate;
    this.utterance.pitch = this.pitch;
    this.utterance.volume = this.volume;
    
    // Aplica a voz selecionada
    if (this.voice) {
      this.utterance.voice = this.voice;
      console.log('[TTS] Falando com voz:', this.voice.name, this.voice.lang);
    } else if (this.voiceName) {
      // Se temos um nome de voz mas a voz não foi carregada, tenta carregar agora
      const voices = this.getVoices();
      const voice = voices.find(v => v.name === this.voiceName);
      if (voice) {
        this.voice = voice;
        this.utterance.voice = voice;
        console.log('[TTS] Voz carregada na hora de falar:', voice.name, voice.lang);
      } else {
        console.warn('[TTS] Voz ainda não disponível:', this.voiceName);
      }
    }
    
    // Eventos
    this.utterance.onstart = () => {
      this.isSpeaking = true;
      this.onStart();
    };
    
    this.utterance.onend = () => {
      this.isSpeaking = false;
      this.utterance = null;
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
      this.utterance = null;
      this.onError(new Error(`Speech synthesis error: ${event.error}`));
    };
    
    this.utterance.onboundary = (event) => {
      this.onBoundary(event);
    };
    
    // Inicia fala
    this.synth.speak(this.utterance);
  }
  
  /**
   * Para a fala
   */
  stop(): void {
    if (this.isSpeaking) {
      this.synth.cancel();
      this.isSpeaking = false;
      this.utterance = null;
    }
  }
  
  /**
   * Pausa a fala
   */
  pause(): void {
    if (this.isSpeaking) {
      this.synth.pause();
    }
  }
  
  /**
   * Retoma a fala pausada
   */
  resume(): void {
    if (this.isSpeaking && this.synth.paused) {
      this.synth.resume();
    }
  }
  
  /**
   * Verifica se está falando
   */
  getIsSpeaking(): boolean {
    return this.isSpeaking;
  }
  
  /**
   * Define a taxa de fala (velocidade)
   */
  setRate(rate: number): void {
    this.rate = Math.max(0.1, Math.min(10, rate));
  }
  
  /**
   * Define o tom (pitch)
   */
  setPitch(pitch: number): void {
    this.pitch = Math.max(0, Math.min(2, pitch));
  }
  
  /**
   * Define o volume
   */
  setVolume(volume: number): void {
    this.volume = Math.max(0, Math.min(1, volume));
  }
  
  /**
   * Define a voz por nome
   */
  setVoiceByName(name: string): void {
    this.voiceName = name;
    const voices = this.getVoices();
    const voice = voices.find(v => v.name === name);
    if (voice) {
      this.voice = voice;
      console.log('[TTS] Voz definida:', voice.name, voice.lang);
    } else {
      console.warn('[TTS] Voz não encontrada:', name, 'Vozes disponíveis:', voices.length);
      // Tenta novamente depois que as vozes carregarem
      if (voices.length === 0) {
        setTimeout(() => this.setVoiceByName(name), 100);
      }
    }
  }
  
  /**
   * Define o idioma
   */
  setLanguage(language: string): void {
    this.language = language;
    // Recarrega voz adequada para o idioma
    this._selectVoice(this.getVoices());
  }
}
