/**
 * STTService - Serviço Unificado de Speech-to-Text
 * 
 * Integra múltiplos providers (WebSpeech, Whisper) com diferentes modos de gravação
 * (PTT, Toggle, VAD Silence, VAD Activity).
 * 
 * @example
 * ```typescript
 * import { STTService, STT_PROVIDERS, RECORDING_MODES } from '@/services/stt';
 * 
 * const stt = new STTService({
 *   provider: STT_PROVIDERS.WEBSPEECH,
 *   mode: RECORDING_MODES.VAD_SILENCE,
 *   onTranscription: (text) => console.log('Transcrição:', text),
 * });
 * 
 * await stt.init();
 * await stt.startRecording();
 * ```
 */

import {
  STTConfig,
  STTCallbacks,
  STTOptions,
  STTState,
  STTProvider,
  RecordingMode,
  STT_STATES,
  STT_PROVIDERS,
  RECORDING_MODES,
  DEFAULT_STT_CONFIG,
} from './types';
import { VoiceActivityDetector } from '../vad';
import { WebSpeechProvider } from './providers/WebSpeechProvider';
import { WhisperProvider } from './providers/WhisperProvider';
import { AudioRecorder } from './AudioRecorder';

export class STTService extends EventTarget {
  private config: STTConfig;
  private callbacks: Required<STTCallbacks>;
  
  // Estado
  private _state: STTState = STT_STATES.IDLE;
  private _interimText: string = '';
  private _currentVolume: number = 0;
  private _initialized: boolean = false;
  
  // Providers
  private webSpeechProvider: WebSpeechProvider | null = null;
  private whisperProvider: WhisperProvider | null = null;
  
  // VAD e Audio
  private vadDetector: VoiceActivityDetector | null = null;
  private audioRecorder: AudioRecorder | null = null;
  private mediaStream: MediaStream | null = null;

  constructor(options: STTOptions = {}) {
    super();
    
    // Separa config de callbacks
    const {
      onTranscription,
      onInterimResult,
      onStateChange,
      onVolumeChange,
      onActivityStart,
      onActivityEnd,
      onError,
      onNoSpeechDetected,
      onAudioFile,
      onRecordingCancelled,
      ...configOptions
    } = options;

    this.config = {
      ...DEFAULT_STT_CONFIG,
      ...configOptions,
    };

    this.callbacks = {
      onTranscription: onTranscription || (() => {}),
      onInterimResult: onInterimResult || (() => {}),
      onStateChange: onStateChange || (() => {}),
      onVolumeChange: onVolumeChange || (() => {}),
      onActivityStart: onActivityStart || (() => {}),
      onActivityEnd: onActivityEnd || (() => {}),
      onError: onError || (() => {}),
      onNoSpeechDetected: onNoSpeechDetected || (() => {}),
      onAudioFile: onAudioFile || (() => {}),
      onRecordingCancelled: onRecordingCancelled || (() => {}),
    };
  }

  /**
   * Inicializa o serviço
   */
  async init(): Promise<boolean> {
    if (this._initialized) {
      return true;
    }

    try {
      // Inicializa WebSpeech se suportado
      if (WebSpeechProvider.checkSupport()) {
        this.webSpeechProvider = new WebSpeechProvider({
          language: this.config.language,
          onStart: () => this.handleRecordingStart(),
          onEnd: (transcript) => this.handleTranscription(transcript),
          onResult: (transcript) => this.handleFinalResult(transcript),
          onInterim: (text) => this.handleInterim(text),
          onError: (message, code) => this.handleError(message, code),
        });
        await this.webSpeechProvider.init();
      }

      // Inicializa Whisper provider
      this.whisperProvider = new WhisperProvider({
        language: this.config.language,
        onStart: () => this.handleRecordingStart(),
        onEnd: (transcript) => this.handleTranscription(transcript),
        onProcessing: () => this.setState(STT_STATES.PROCESSING),
        onError: (message, code) => this.handleError(message, code),
      });
      await this.whisperProvider.init();

      // Cria AudioRecorder (sem inicializar stream ainda - lazy init)
      this.audioRecorder = new AudioRecorder({
        onStart: () => this.handleRecordingStart(),
        onStop: (blob) => this.handleAudioBlob(blob),
        onError: (error) => {
          const message = typeof error === 'string' ? error : error.message;
          this.handleError(message);
        },
      });
      // NÃO inicializa o stream aqui - será feito sob demanda em startActualRecording()

      this._initialized = true;
      console.log('[STTService] Inicializado');
      return true;
    } catch (error) {
      console.error('[STTService] Erro ao inicializar:', error);
      return false;
    }
  }

  /**
   * Aguarda inicialização
   */
  async ready(): Promise<void> {
    if (!this._initialized) {
      await this.init();
    }
  }

  // === Getters ===

  get state(): STTState { return this._state; }
  get provider(): STTProvider { return this.config.provider; }
  get mode(): RecordingMode { return this.config.mode; }
  get language(): string { return this.config.language; }
  get interimText(): string { return this._interimText; }
  get currentVolume(): number { return this._currentVolume; }

  get isIdle(): boolean { return this._state === STT_STATES.IDLE; }
  get isListening(): boolean { return this._state === STT_STATES.LISTENING; }
  get isRecording(): boolean { return this._state === STT_STATES.RECORDING; }
  get isProcessing(): boolean { return this._state === STT_STATES.PROCESSING; }
  get isError(): boolean { return this._state === STT_STATES.ERROR; }
  get isActive(): boolean { return this.isRecording || this.isListening; }

  get isWebSpeechSupported(): boolean {
    return WebSpeechProvider.checkSupport();
  }

  get isWhisperSupported(): boolean {
    return this.whisperProvider?.isSupported || false;
  }

  get isSupported(): boolean {
    return this.isWebSpeechSupported || this.isWhisperSupported;
  }

  // === Configuração ===

  setProvider(provider: STTProvider): void {
    this.config.provider = provider;
    this.dispatchEvent(new CustomEvent('providerChange', { detail: { provider } }));
  }

  setMode(mode: RecordingMode): void {
    this.config.mode = mode;
    this.dispatchEvent(new CustomEvent('modeChange', { detail: { mode } }));
  }

  setLanguage(language: string): void {
    this.config.language = language;
    this.webSpeechProvider?.setLanguage(language);
    this.whisperProvider?.setLanguage(language);
    this.dispatchEvent(new CustomEvent('languageChange', { detail: { language } }));
  }

  setSilenceDuration(ms: number): void {
    this.config.silenceDuration = ms;
  }

  updateConfig(newConfig: Partial<STTConfig>): void {
    this.config = { ...this.config, ...newConfig };
  }

  // === Controle de Gravação ===

  /**
   * Inicia gravação baseado no modo configurado
   */
  async startRecording(): Promise<boolean> {
    if (this._state === STT_STATES.RECORDING || this._state === STT_STATES.LISTENING) {
      return false;
    }

    await this.ready();

    try {
      switch (this.config.mode) {
        case RECORDING_MODES.PTT:
        case RECORDING_MODES.TOGGLE:
        case RECORDING_MODES.RECORD_AUDIO:
          await this.startActualRecording();
          break;

        case RECORDING_MODES.VAD_SILENCE:
          await this.startWithVAD(false);
          break;

        case RECORDING_MODES.VAD_ACTIVITY:
          await this.startListening();
          break;
      }

      return true;
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Erro ao iniciar gravação';
      this.handleError(message);
      return false;
    }
  }

  /**
   * Para a gravação e processa
   */
  stopRecording(): void {
    if (this._state !== STT_STATES.RECORDING && this._state !== STT_STATES.LISTENING) {
      return;
    }

    this.stopVAD();

    if (this._state === STT_STATES.LISTENING) {
      this.setState(STT_STATES.IDLE);
      this.callbacks.onRecordingCancelled();
      return;
    }

    // Para gravação
    if (this.config.mode === RECORDING_MODES.RECORD_AUDIO) {
      this.audioRecorder?.stop();
    } else if (this.config.provider === STT_PROVIDERS.WHISPER_API) {
      this.whisperProvider?.stop();
    } else {
      this.webSpeechProvider?.stop();
    }
  }

  /**
   * Cancela a gravação sem processar
   */
  cancelRecording(): void {
    this.stopVAD();

    this.webSpeechProvider?.abort();
    this.whisperProvider?.abort();
    this.audioRecorder?.stop();

    this.setState(STT_STATES.IDLE);
    this._interimText = '';
    this.callbacks.onRecordingCancelled();
    
    // Libera o microfone após cancelar
    this.releaseMicrophoneIfNotNeeded();
  }

  /**
   * Alias para cancelRecording
   */
  cancel(): void {
    this.cancelRecording();
  }

  /**
   * Toggle gravação
   */
  toggleRecording(): void {
    if (this._state === STT_STATES.RECORDING || this._state === STT_STATES.LISTENING) {
      this.stopRecording();
    } else {
      this.startRecording();
    }
  }

  // === Métodos Internos ===

  private async startActualRecording(): Promise<void> {
    if (this.config.mode === RECORDING_MODES.RECORD_AUDIO) {
      // Inicializa stream do microfone se necessário (lazy init)
      if (this.audioRecorder && !this.audioRecorder.hasActiveStream) {
        await this.audioRecorder.init();
      }
      this.audioRecorder?.start();
    } else if (this.config.provider === STT_PROVIDERS.WHISPER_API) {
      // WhisperProvider.start() é async para lazy init do microfone
      await this.whisperProvider?.start();
    } else {
      this.webSpeechProvider?.start();
    }
  }

  private async startWithVAD(waitForActivity: boolean): Promise<void> {
    try {
      await this.initVAD();

      this.mediaStream = await navigator.mediaDevices.getUserMedia({ audio: true });
      await this.vadDetector!.init(this.mediaStream);
      this.vadDetector!.start();

      if (waitForActivity) {
        this.setState(STT_STATES.LISTENING);
      } else {
        await this.startActualRecording();
      }
    } catch (error) {
      console.error('[STTService] Erro ao iniciar VAD:', error);
      // Fallback para gravação normal
      await this.startActualRecording();
    }
  }

  private async startListening(): Promise<void> {
    try {
      await this.initVAD();

      this.mediaStream = await navigator.mediaDevices.getUserMedia({ audio: true });
      await this.vadDetector!.init(this.mediaStream);
      this.vadDetector!.start();

      this.setState(STT_STATES.LISTENING);
    } catch (error) {
      console.error('[STTService] Erro ao iniciar listening:', error);
      this.handleError('Erro ao acessar microfone');
    }
  }

  private async initVAD(): Promise<void> {
    if (this.vadDetector) {
      this.vadDetector.destroy();
    }

    this.vadDetector = new VoiceActivityDetector({
      silenceDuration: this.config.silenceDuration,
      silenceThreshold: this.config.silenceThreshold,
      activityThreshold: this.config.activityThreshold,
      activityDuration: this.config.activityDuration,

      onVolumeChange: (volume) => {
        this._currentVolume = volume;
        this.callbacks.onVolumeChange(volume);
        this.dispatchEvent(new CustomEvent('volumeChange', { detail: { volume } }));
      },

      onActivityStart: () => {
        if (this.config.mode === RECORDING_MODES.VAD_ACTIVITY && 
            this._state === STT_STATES.LISTENING) {
          this.startActualRecording();
        }
        this.callbacks.onActivityStart();
        this.dispatchEvent(new CustomEvent('activityStart', {}));
      },

      onActivityEnd: () => {
        if ((this.config.mode === RECORDING_MODES.VAD_SILENCE || 
             this.config.mode === RECORDING_MODES.VAD_ACTIVITY) &&
            this._state === STT_STATES.RECORDING) {
          this.stopRecording();
        }
        this.callbacks.onActivityEnd();
        this.dispatchEvent(new CustomEvent('activityEnd', {}));
      },
    });
  }

  private stopVAD(): void {
    if (this.vadDetector?.active) {
      this.vadDetector.stop();
    }

    if (this.mediaStream) {
      this.mediaStream.getTracks().forEach(track => track.stop());
      this.mediaStream = null;
    }
  }

  /**
   * Libera o microfone se o modo atual não requer escuta contínua.
   * Modos PTT, Toggle e RECORD_AUDIO não precisam do microfone ligado o tempo todo.
   */
  private releaseMicrophoneIfNotNeeded(): void {
    // Modos que NÃO precisam do microfone continuamente ligado
    const modesWithoutContinuousListening: RecordingMode[] = [
      RECORDING_MODES.PTT,
      RECORDING_MODES.TOGGLE,
      RECORDING_MODES.RECORD_AUDIO,
    ];

    if (modesWithoutContinuousListening.includes(this.config.mode)) {
      this.audioRecorder?.releaseStream();
    }
  }

  // === Handlers ===

  private handleRecordingStart(): void {
    let message = 'Gravando. Fale agora.';
    if (this.config.mode === RECORDING_MODES.RECORD_AUDIO) {
      message = 'Gravando áudio. Solte para anexar.';
    } else if (this.config.provider === STT_PROVIDERS.WHISPER_API) {
      message = 'Gravando para Whisper. Fale agora.';
    }
    this.setState(STT_STATES.RECORDING, message);
    this._interimText = '';
  }

  private handleInterim(text: string): void {
    this._interimText = text;
    this.callbacks.onInterimResult(text);
    this.dispatchEvent(new CustomEvent('interimResult', { detail: { text } }));
  }

  private handleFinalResult(transcript: string): void {
    this.dispatchEvent(new CustomEvent('partialResult', { detail: { text: transcript } }));
  }

  private handleTranscription(transcript: string): void {
    this.stopVAD();

    if (transcript && transcript.trim()) {
      this.setState(STT_STATES.IDLE, `Transcrição: ${transcript.trim()}`);
      this.callbacks.onTranscription(transcript.trim(), this.config.provider);
      this.dispatchEvent(new CustomEvent('transcription', {
        detail: { text: transcript.trim(), isFinal: true, provider: this.config.provider }
      }));
    } else {
      this.callbacks.onNoSpeechDetected();
      this.dispatchEvent(new CustomEvent('noSpeechDetected', {}));
      this.setState(STT_STATES.IDLE, 'Nenhuma fala detectada.');
    }

    this._interimText = '';
    
    // Libera o microfone em modos que não precisam de escuta contínua
    this.releaseMicrophoneIfNotNeeded();
  }

  private handleAudioBlob(blob: Blob): void {
    if (this.config.mode === RECORDING_MODES.RECORD_AUDIO) {
      const file = new File([blob], `audio-${Date.now()}.webm`, { type: 'audio/webm' });
      this.callbacks.onAudioFile(file, blob);
      this.dispatchEvent(new CustomEvent('audioFile', { detail: { file, blob } }));
      this.setState(STT_STATES.IDLE);
      
      // Libera o microfone após gravar áudio
      this.releaseMicrophoneIfNotNeeded();
    }
  }

  private handleError(message: string, code?: string): void {
    this.stopVAD();
    this.setState(STT_STATES.ERROR);
    this.callbacks.onError(message, code);
    this.dispatchEvent(new CustomEvent('error', { detail: { message, code } }));

    // Volta para idle após um tempo
    setTimeout(() => {
      if (this._state === STT_STATES.ERROR) {
        this.setState(STT_STATES.IDLE);
      }
    }, 3000);
  }

  private setState(state: STTState, message: string = ''): void {
    const previousState = this._state;
    this._state = state;
    this.callbacks.onStateChange(state, previousState, message);
    this.dispatchEvent(new CustomEvent('stateChange', {
      detail: { state, previousState, message }
    }));
  }

  // === Cleanup ===

  destroy(): void {
    this.stopVAD();

    this.webSpeechProvider?.destroy();
    this.whisperProvider?.destroy();
    this.audioRecorder?.destroy();
    this.vadDetector?.destroy();

    this.webSpeechProvider = null;
    this.whisperProvider = null;
    this.audioRecorder = null;
    this.vadDetector = null;
    this._initialized = false;

    console.log('[STTService] Destruído');
  }
}

export default STTService;
