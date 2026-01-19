/**
 * STTService - Serviço unificado de Speech-to-Text
 * 
 * Suporta múltiplos backends:
 * - WebSpeech (navegador)
 * - Whisper (OpenAI via Wails)
 * 
 * Modos de gravação:
 * - PTT (Push-to-Talk): segura para gravar
 * - Toggle: clique para iniciar/parar
 * - VAD Silence: clique + detecta silêncio para parar
 * - VAD Activity: full auto - detecta início e fim de fala
 * 
 * Uso:
 *   import { sttService, STT_PROVIDERS, RECORDING_MODES } from '$lib/speech/stt-service.js';
 *   
 *   sttService.addEventListener('transcription', (e) => {
 *     console.log('Texto:', e.detail.text);
 *   });
 *   
 *   sttService.setProvider(STT_PROVIDERS.WHISPER);
 *   sttService.setMode(RECORDING_MODES.PTT);
 *   sttService.startRecording();
 */

import { SpeechRecognitionManager } from './stt-webspeech.js';
import { AudioRecorder } from './audio-recorder.js';
import { VoiceActivityDetector } from './vad.js';

// Import dinâmico do Wails
let TranscribeWhisper = null;

async function loadWailsFunctions() {
  try {
    const wails = await import('../../../wailsjs/go/main/App.js');
    TranscribeWhisper = wails.TranscribeWhisper;
  } catch (e) {
    console.warn('Wails TranscribeWhisper not available:', e.message);
  }
}

// Providers suportados
export const STT_PROVIDERS = {
  WEBSPEECH: 'webspeech',
  WHISPER: 'whisper'
};

// Modos de gravação
export const RECORDING_MODES = {
  PTT: 'ptt',                  // Push-to-talk: segura para gravar
  TOGGLE: 'toggle',            // Clique para iniciar/parar
  VAD_SILENCE: 'vad_silence',  // Clique + detecta silêncio para parar
  VAD_ACTIVITY: 'vad_activity', // Full auto: detecta início e fim de fala
  RECORD_AUDIO: 'record_audio' // Grava áudio como arquivo (não transcreve)
};

// Estados possíveis
export const STT_STATES = {
  IDLE: 'idle',
  LISTENING: 'listening',      // Esperando atividade de voz (VAD Activity)
  RECORDING: 'recording',      // Gravando
  PROCESSING: 'processing',    // Processando transcrição
  ERROR: 'error'
};

/**
 * Serviço de STT com suporte a múltiplos backends e modos
 */
class STTService extends EventTarget {
  constructor() {
    super();
    
    // Configuração
    this._provider = STT_PROVIDERS.WEBSPEECH;
    this._mode = RECORDING_MODES.PTT;
    this._language = 'pt-BR';
    this._silenceDuration = 1500; // ms para detectar fim de fala
    
    // Estado
    this._state = STT_STATES.IDLE;
    this._interimText = '';
    this._currentVolume = 0;
    
    // Managers
    this._sttManager = null;
    this._audioRecorder = null;
    this._vadDetector = null;
    this._mediaStream = null;
    
    // Inicialização
    this._initialized = false;
    this._initPromise = this._init();
  }
  
  async _init() {
    await loadWailsFunctions();
    
    // Inicializa WebSpeech STT
    if (SpeechRecognitionManager.isSupported()) {
      this._sttManager = new SpeechRecognitionManager({
        language: this._language,
        continuous: false,
        interimResults: true,
        onStart: () => this._handleRecordingStart(),
        onEnd: (transcript) => this._handleTranscription(transcript),
        onResult: (transcript) => this._handleFinalResult(transcript),
        onInterim: (text) => this._handleInterim(text),
        onError: (message, code) => this._handleError(message, code)
      });
    }
    
    // Inicializa AudioRecorder para Whisper
    this._audioRecorder = new AudioRecorder({
      mimeType: 'audio/webm',
      onStart: () => this._handleRecordingStart(),
      onStop: async (blob) => {
        if (blob.size > 0) {
          await this._processAudioBlob(blob);
        }
      },
      onError: (error) => this._handleError(error.message || 'Erro ao gravar')
    });
    
    await this._audioRecorder.init();
    
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
  get mode() { return this._mode; }
  get state() { return this._state; }
  get language() { return this._language; }
  get interimText() { return this._interimText; }
  get currentVolume() { return this._currentVolume; }
  
  get isIdle() { return this._state === STT_STATES.IDLE; }
  get isListening() { return this._state === STT_STATES.LISTENING; }
  get isRecording() { return this._state === STT_STATES.RECORDING; }
  get isProcessing() { return this._state === STT_STATES.PROCESSING; }
  
  get isWebSpeechSupported() {
    return SpeechRecognitionManager.isSupported();
  }
  
  get isWhisperSupported() {
    return !!TranscribeWhisper;
  }
  
  /**
   * Verifica se algum provider de STT está disponível
   */
  get isSupported() {
    return this.isWebSpeechSupported || this.isWhisperSupported;
  }
  
  // === Configuração ===
  
  /**
   * Define o provider de STT
   * @param {string} provider - 'webspeech' | 'whisper'
   */
  setProvider(provider) {
    this._provider = provider;
    this._dispatchEvent('providerChange', { provider });
  }
  
  /**
   * Define o modo de gravação
   * @param {string} mode - 'ptt' | 'toggle' | 'vad_silence' | 'vad_activity'
   */
  setMode(mode) {
    this._mode = mode;
    this._dispatchEvent('modeChange', { mode });
  }
  
  /**
   * Define o idioma
   * @param {string} language - Código do idioma (ex: 'pt-BR')
   */
  setLanguage(language) {
    this._language = language;
    if (this._sttManager) {
      this._sttManager.setLanguage(language);
    }
    this._dispatchEvent('languageChange', { language });
  }
  
  /**
   * Define duração de silêncio para VAD
   * @param {number} ms - Milissegundos
   */
  setSilenceDuration(ms) {
    this._silenceDuration = ms;
  }
  
  // === Controle de gravação ===
  
  /**
   * Inicia gravação baseado no modo configurado
   */
  async startRecording() {
    if (this._state === STT_STATES.RECORDING || this._state === STT_STATES.LISTENING) {
      return false;
    }
    
    await this.ready();
    
    try {
      switch (this._mode) {
        case RECORDING_MODES.PTT:
        case RECORDING_MODES.TOGGLE:
        case RECORDING_MODES.RECORD_AUDIO:
          // Inicia gravação imediatamente
          await this._startActualRecording();
          break;
          
        case RECORDING_MODES.VAD_SILENCE:
          // Inicia gravação + ativa VAD para detectar fim
          await this._startWithVAD(false);
          break;
          
        case RECORDING_MODES.VAD_ACTIVITY:
          // Entra em modo "escutando" e espera atividade
          await this._startListening();
          break;
      }
      
      return true;
    } catch (error) {
      this._handleError(error.message || 'Erro ao iniciar gravação');
      return false;
    }
  }
  
  /**
   * Para a gravação e processa
   */
  stopRecording() {
    if (this._state !== STT_STATES.RECORDING && this._state !== STT_STATES.LISTENING) {
      return;
    }
    
    // Para o VAD
    this._stopVAD();
    
    if (this._state === STT_STATES.LISTENING) {
      // Estava só escutando, não iniciou gravação
      this._setState(STT_STATES.IDLE);
      this._dispatchEvent('recordingCancelled', {});
      return;
    }
    
    // Para gravação
    if (this._mode === RECORDING_MODES.RECORD_AUDIO || this._provider === STT_PROVIDERS.WHISPER) {
      if (this._audioRecorder) {
        this._audioRecorder.stop();
      }
    } else {
      if (this._sttManager) {
        this._sttManager.stop();
      }
    }
  }
  
  /**
   * Cancela a gravação sem processar
   */
  cancelRecording() {
    this._stopVAD();
    
    if (this._sttManager) {
      this._sttManager.abort();
    }
    
    if (this._audioRecorder) {
      this._audioRecorder.stop();
    }
    
    this._setState(STT_STATES.IDLE);
    this._interimText = '';
    this._dispatchEvent('recordingCancelled', {});
  }
  
  /**
   * Alias para cancelRecording
   */
  cancel() {
    this.cancelRecording();
  }
  
  /**
   * Toggle gravação (para modos Toggle e VAD)
   */
  toggleRecording() {
    if (this._state === STT_STATES.RECORDING || this._state === STT_STATES.LISTENING) {
      this.stopRecording();
    } else {
      this.startRecording();
    }
  }
  
  // === Métodos internos ===
  
  async _startActualRecording() {
    // Modo record_audio sempre usa AudioRecorder (não transcreve)
    if (this._mode === RECORDING_MODES.RECORD_AUDIO || this._provider === STT_PROVIDERS.WHISPER) {
      if (this._audioRecorder) {
        this._audioRecorder.start();
      }
    } else {
      if (this._sttManager) {
        this._sttManager.start();
      }
    }
  }
  
  async _startWithVAD(waitForActivity) {
    try {
      await this._initVAD();
      
      // Obtém stream para VAD
      this._mediaStream = await navigator.mediaDevices.getUserMedia({ audio: true });
      await this._vadDetector.init(this._mediaStream);
      this._vadDetector.start();
      
      if (waitForActivity) {
        // Modo VAD Activity: entra em "listening" e espera atividade
        this._setState(STT_STATES.LISTENING);
      } else {
        // Modo VAD Silence: inicia gravação imediata
        await this._startActualRecording();
      }
    } catch (error) {
      console.error('[VAD] Erro ao inicializar:', error);
      // Fallback para gravação normal
      await this._startActualRecording();
    }
  }
  
  async _startListening() {
    try {
      await this._initVAD();
      
      this._mediaStream = await navigator.mediaDevices.getUserMedia({ audio: true });
      await this._vadDetector.init(this._mediaStream);
      this._vadDetector.start();
      
      this._setState(STT_STATES.LISTENING);
    } catch (error) {
      console.error('[VAD] Erro ao iniciar listening:', error);
      this._handleError('Erro ao acessar microfone');
    }
  }
  
  async _initVAD() {
    if (this._vadDetector) {
      this._vadDetector.destroy();
    }
    
    this._vadDetector = new VoiceActivityDetector({
      silenceDuration: this._silenceDuration,
      silenceThreshold: 0.01,
      activityThreshold: 0.02,
      activityDuration: 200,
      
      onVolumeChange: (volume) => {
        this._currentVolume = volume;
        this._dispatchEvent('volumeChange', { volume });
      },
      
      onActivityStart: () => {
        if (this._mode === RECORDING_MODES.VAD_ACTIVITY && this._state === STT_STATES.LISTENING) {
          // Modo full auto: inicia gravação quando detecta voz
          this._startActualRecording();
        }
        this._dispatchEvent('activityStart', {});
      },
      
      onActivityEnd: () => {
        if ((this._mode === RECORDING_MODES.VAD_SILENCE || this._mode === RECORDING_MODES.VAD_ACTIVITY) 
            && this._state === STT_STATES.RECORDING) {
          // Para a gravação quando detecta silêncio
          this.stopRecording();
        }
        this._dispatchEvent('activityEnd', {});
      }
    });
  }
  
  _stopVAD() {
    if (this._vadDetector && this._vadDetector.active) {
      this._vadDetector.stop();
    }
    
    if (this._mediaStream) {
      this._mediaStream.getTracks().forEach(track => track.stop());
      this._mediaStream = null;
    }
  }
  
  async _processAudioBlob(blob) {
    // Modo record_audio: apenas dispara evento com arquivo
    if (this._mode === RECORDING_MODES.RECORD_AUDIO) {
      const file = new File([blob], `audio-${Date.now()}.webm`, { type: 'audio/webm' });
      this._dispatchEvent('audioFile', { file, blob });
      this._setState(STT_STATES.IDLE);
      return;
    }
    
    if (this._provider === STT_PROVIDERS.WHISPER && TranscribeWhisper) {
      await this._transcribeWithWhisper(blob);
    } else {
      // Blob de áudio sem transcrição (pode ser usado para anexar)
      this._dispatchEvent('audioData', { blob });
      this._setState(STT_STATES.IDLE);
    }
  }
  
  async _transcribeWithWhisper(audioBlob) {
    try {
      this._setState(STT_STATES.PROCESSING);
      
      // Converte para base64
      const reader = new FileReader();
      const base64Promise = new Promise((resolve, reject) => {
        reader.onloadend = () => {
          const base64 = reader.result.split(',')[1];
          resolve(base64);
        };
        reader.onerror = reject;
      });
      reader.readAsDataURL(audioBlob);
      
      const audioBase64 = await base64Promise;
      const result = await TranscribeWhisper(audioBase64, 'audio.webm');
      
      if (result && result.text && result.text.trim()) {
        this._handleTranscription(result.text);
      } else {
        this._dispatchEvent('noSpeechDetected', {});
        this._setState(STT_STATES.IDLE);
      }
    } catch (error) {
      console.error('Whisper transcription error:', error);
      this._handleError('Erro na transcrição Whisper');
    }
  }
  
  // === Handlers ===
  
  _handleRecordingStart() {
    let message = 'Gravando. Fale agora.';
    if (this._mode === RECORDING_MODES.RECORD_AUDIO) {
      message = 'Gravando áudio. Solte para anexar.';
    } else if (this._provider === STT_PROVIDERS.WHISPER) {
      message = 'Gravando para Whisper. Fale agora.';
    }
    this._setState(STT_STATES.RECORDING, message);
    this._interimText = '';
  }
  
  _handleInterim(text) {
    this._interimText = text;
    this._dispatchEvent('interimResult', { text });
  }
  
  _handleFinalResult(transcript) {
    this._dispatchEvent('partialResult', { text: transcript });
  }
  
  _handleTranscription(transcript) {
    this._stopVAD();
    
    if (transcript && transcript.trim()) {
      this._setState(STT_STATES.IDLE, `Transcrição: ${transcript.trim()}`);
      this._dispatchEvent('transcription', { 
        text: transcript.trim(),
        isFinal: true,
        provider: this._provider 
      });
    } else {
      this._dispatchEvent('noSpeechDetected', {});
      this._setState(STT_STATES.IDLE, 'Nenhuma fala detectada.');
    }
    
    this._interimText = '';
  }
  
  _handleError(message, code = null) {
    this._stopVAD();
    this._setState(STT_STATES.ERROR);
    this._dispatchEvent('error', { message, code });
    
    // Volta para idle após um tempo
    setTimeout(() => {
      if (this._state === STT_STATES.ERROR) {
        this._setState(STT_STATES.IDLE);
      }
    }, 3000);
  }
  
  _setState(state, message = '') {
    const previousState = this._state;
    this._state = state;
    this._dispatchEvent('stateChange', { state, previousState, message });
  }
  
  _dispatchEvent(type, detail) {
    this.dispatchEvent(new CustomEvent(type, { detail }));
  }
  
  // === Cleanup ===
  
  destroy() {
    this._stopVAD();
    
    if (this._sttManager) {
      this._sttManager.abort();
    }
    
    if (this._audioRecorder) {
      this._audioRecorder.stop();
    }
    
    if (this._vadDetector) {
      this._vadDetector.destroy();
    }
  }
}

// Exporta instância singleton
export const sttService = new STTService();

// Exporta classe para quem quiser criar instâncias próprias
export { STTService };

