/**
 * WebSpeech STT Provider
 * 
 * Usa a SpeechRecognition API nativa do navegador para transcrição de voz.
 * Suportado em Chrome, Edge, Safari e outros navegadores modernos.
 */

import { STT_PROVIDERS, WebSpeechProviderOptions, ISTTProvider, STTProvider } from '../types';

// Tipos para a API SpeechRecognition
interface SpeechRecognitionEvent extends Event {
  resultIndex: number;
  results: SpeechRecognitionResultList;
}

interface SpeechRecognitionErrorEvent extends Event {
  error: string;
  message?: string;
}

interface SpeechRecognitionResultList {
  length: number;
  item(index: number): SpeechRecognitionResult;
  [index: number]: SpeechRecognitionResult;
}

interface SpeechRecognitionResult {
  isFinal: boolean;
  length: number;
  item(index: number): SpeechRecognitionAlternative;
  [index: number]: SpeechRecognitionAlternative;
}

interface SpeechRecognitionAlternative {
  transcript: string;
  confidence: number;
}

interface SpeechRecognition extends EventTarget {
  continuous: boolean;
  interimResults: boolean;
  lang: string;
  maxAlternatives: number;
  onstart: ((this: SpeechRecognition, ev: Event) => void) | null;
  onend: ((this: SpeechRecognition, ev: Event) => void) | null;
  onresult: ((this: SpeechRecognition, ev: SpeechRecognitionEvent) => void) | null;
  onerror: ((this: SpeechRecognition, ev: SpeechRecognitionErrorEvent) => void) | null;
  start(): void;
  stop(): void;
  abort(): void;
}

declare global {
  interface Window {
    SpeechRecognition: new () => SpeechRecognition;
    webkitSpeechRecognition: new () => SpeechRecognition;
  }
}

export class WebSpeechProvider implements ISTTProvider {
  readonly name: STTProvider = STT_PROVIDERS.WEBSPEECH;
  
  private recognition: SpeechRecognition | null = null;
  private _isListening: boolean = false;
  private transcript: string = '';
  private interimTranscript: string = '';
  
  // Configuração
  private language: string;
  private continuous: boolean;
  private interimResults: boolean;
  private maxAlternatives: number;
  
  // Callbacks
  private onStart: () => void;
  private onEnd: (transcript: string) => void;
  private onResult: (transcript: string) => void;
  private onInterim: (text: string) => void;
  private onError: (message: string, code: string) => void;

  constructor(options: WebSpeechProviderOptions = {}) {
    this.language = options.language || 'pt-BR';
    this.continuous = options.continuous || false;
    this.interimResults = options.interimResults !== false;
    this.maxAlternatives = options.maxAlternatives || 1;
    
    this.onStart = options.onStart || (() => {});
    this.onEnd = options.onEnd || (() => {});
    this.onResult = options.onResult || (() => {});
    this.onInterim = options.onInterim || (() => {});
    this.onError = options.onError || (() => {});
  }

  /**
   * Verifica se o navegador suporta SpeechRecognition
   */
  get isSupported(): boolean {
    return WebSpeechProvider.checkSupport();
  }

  /**
   * Verifica suporte estaticamente
   */
  static checkSupport(): boolean {
    return typeof window !== 'undefined' && 
           !!(window.SpeechRecognition || window.webkitSpeechRecognition);
  }

  /**
   * Inicializa o provider
   */
  async init(): Promise<boolean> {
    const SpeechRecognitionClass = window.SpeechRecognition || window.webkitSpeechRecognition;
    
    if (!SpeechRecognitionClass) {
      return false;
    }
    
    this.recognition = new SpeechRecognitionClass();
    this.recognition.lang = this.language;
    this.recognition.continuous = this.continuous;
    this.recognition.interimResults = this.interimResults;
    this.recognition.maxAlternatives = this.maxAlternatives;
    
    this.recognition.onstart = () => {
      this._isListening = true;
      this.transcript = '';
      this.interimTranscript = '';
      this.onStart();
    };
    
    this.recognition.onend = () => {
      this._isListening = false;
      this.onEnd(this.transcript);
    };
    
    this.recognition.onresult = (event: SpeechRecognitionEvent) => {
      let finalTranscript = '';
      let interimTranscript = '';
      
      for (let i = event.resultIndex; i < event.results.length; i++) {
        const result = event.results[i];
        if (result.isFinal) {
          finalTranscript += result[0].transcript;
        } else {
          interimTranscript += result[0].transcript;
        }
      }
      
      if (finalTranscript) {
        this.transcript += finalTranscript;
        this.onResult(this.transcript);
      }
      
      if (interimTranscript) {
        this.interimTranscript = interimTranscript;
        this.onInterim(interimTranscript);
      }
    };
    
    this.recognition.onerror = (event: SpeechRecognitionErrorEvent) => {
      this._isListening = false;
      
      // Mapeia erros para mensagens amigáveis
      const errorMessages: Record<string, string> = {
        'no-speech': 'Nenhuma fala detectada. Tente novamente.',
        'audio-capture': 'Microfone não encontrado ou não permitido.',
        'not-allowed': 'Permissão de microfone negada.',
        'network': 'Erro de rede. Verifique sua conexão.',
        'aborted': 'Reconhecimento cancelado.',
        'service-not-allowed': 'Serviço de reconhecimento não disponível.',
      };
      
      const message = errorMessages[event.error] || `Erro: ${event.error}`;
      this.onError(message, event.error);
    };
    
    return true;
  }

  /**
   * Inicia o reconhecimento de voz
   */
  start(): boolean {
    if (!this.recognition) {
      this.onError('SpeechRecognition não inicializado', 'not-initialized');
      return false;
    }
    
    if (this._isListening) {
      return false;
    }
    
    try {
      this.recognition.start();
      return true;
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Erro desconhecido';
      this.onError('Erro ao iniciar reconhecimento: ' + message, 'start-error');
      return false;
    }
  }

  /**
   * Para o reconhecimento de voz
   */
  stop(): void {
    if (!this.recognition || !this._isListening) {
      return;
    }
    
    try {
      this.recognition.stop();
    } catch {
      // best-effort
    }
  }

  /**
   * Aborta o reconhecimento sem processar
   */
  abort(): void {
    if (!this.recognition) {
      return;
    }
    
    try {
      this.recognition.abort();
      this.transcript = '';
      this.interimTranscript = '';
    } catch {
      // best-effort
    }
  }

  /**
   * Atualiza o idioma
   */
  setLanguage(language: string): void {
    this.language = language;
    if (this.recognition) {
      this.recognition.lang = language;
    }
  }

  /**
   * Libera recursos
   */
  destroy(): void {
    this.abort();
    this.recognition = null;
  }

  /**
   * Se está ouvindo
   */
  get isListening(): boolean {
    return this._isListening;
  }

  /**
   * Transcrição atual
   */
  get currentTranscript(): string {
    return this.transcript;
  }

  /**
   * Transcrição parcial
   */
  get currentInterim(): string {
    return this.interimTranscript;
  }
}

export default WebSpeechProvider;
