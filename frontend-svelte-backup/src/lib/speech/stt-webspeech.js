/**
 * WebSpeech STT (Speech-to-Text) Manager
 * Usa a SpeechRecognition API nativa do navegador
 */

export class SpeechRecognitionManager {
  constructor(options = {}) {
    this.language = options.language || 'pt-BR';
    this.continuous = options.continuous || false;
    this.interimResults = options.interimResults || true;
    this.maxAlternatives = options.maxAlternatives || 1;
    
    this.recognition = null;
    this.isListening = false;
    this.transcript = '';
    this.interimTranscript = '';
    
    // Callbacks
    this.onStart = options.onStart || (() => {});
    this.onEnd = options.onEnd || (() => {});
    this.onResult = options.onResult || (() => {});
    this.onInterim = options.onInterim || (() => {});
    this.onError = options.onError || (() => {});
    
    this._init();
  }
  
  _init() {
    const SpeechRecognition = window.SpeechRecognition || window.webkitSpeechRecognition;
    
    if (!SpeechRecognition) {
      console.warn('SpeechRecognition API não suportada neste navegador');
      return;
    }
    
    this.recognition = new SpeechRecognition();
    this.recognition.lang = this.language;
    this.recognition.continuous = this.continuous;
    this.recognition.interimResults = this.interimResults;
    this.recognition.maxAlternatives = this.maxAlternatives;
    
    this.recognition.onstart = () => {
      this.isListening = true;
      this.transcript = '';
      this.interimTranscript = '';
      this.onStart();
    };
    
    this.recognition.onend = () => {
      this.isListening = false;
      this.onEnd(this.transcript);
    };
    
    this.recognition.onresult = (event) => {
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
    
    this.recognition.onerror = (event) => {
      this.isListening = false;
      
      // Mapeia erros para mensagens amigáveis
      const errorMessages = {
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
  }
  
  /**
   * Verifica se o navegador suporta SpeechRecognition
   */
  static isSupported() {
    return !!(window.SpeechRecognition || window.webkitSpeechRecognition);
  }
  
  /**
   * Inicia o reconhecimento de voz
   */
  start() {
    if (!this.recognition) {
      this.onError('SpeechRecognition não suportado', 'not-supported');
      return false;
    }
    
    if (this.isListening) {
      return false;
    }
    
    try {
      this.recognition.start();
      return true;
    } catch (error) {
      this.onError('Erro ao iniciar reconhecimento: ' + error.message, 'start-error');
      return false;
    }
  }
  
  /**
   * Para o reconhecimento de voz
   */
  stop() {
    if (!this.recognition || !this.isListening) {
      return;
    }
    
    try {
      this.recognition.stop();
    } catch (error) {
      console.error('Erro ao parar reconhecimento:', error);
    }
  }
  
  /**
   * Aborta o reconhecimento sem processar
   */
  abort() {
    if (!this.recognition) {
      return;
    }
    
    try {
      this.recognition.abort();
      this.transcript = '';
      this.interimTranscript = '';
    } catch (error) {
      console.error('Erro ao abortar reconhecimento:', error);
    }
  }
  
  /**
   * Atualiza o idioma
   */
  setLanguage(lang) {
    this.language = lang;
    if (this.recognition) {
      this.recognition.lang = lang;
    }
  }
}

