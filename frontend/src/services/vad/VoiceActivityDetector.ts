/**
 * Voice Activity Detection (VAD)
 * 
 * Detecta início e fim de atividade de voz usando análise de volume do áudio.
 * Usa Web Audio API para analisar o stream de áudio do microfone.
 */

import { VADConfig, VADCallbacks, VADOptions, DEFAULT_VAD_CONFIG } from './types';

export class VoiceActivityDetector {
  private config: VADConfig;
  private callbacks: Required<VADCallbacks>;
  
  // Web Audio API
  private audioContext: AudioContext | null = null;
  private analyser: AnalyserNode | null = null;
  private mediaStream: MediaStream | null = null;
  private sourceNode: MediaStreamAudioSourceNode | null = null;
  
  // Estado
  private _isActive: boolean = false;
  private _isSpeaking: boolean = false;
  private _currentVolume: number = 0;
  
  // Timers
  private checkIntervalId: ReturnType<typeof setInterval> | null = null;
  private silenceStartTime: number | null = null;
  private activityStartTime: number | null = null;
  
  // Buffer para análise FFT
  private dataArray: Uint8Array | null = null;

  constructor(options: VADOptions = {}) {
    // Separa config de callbacks
    const {
      onSilenceStart,
      onSilenceEnd,
      onActivityStart,
      onActivityEnd,
      onVolumeChange,
      ...configOptions
    } = options;

    this.config = {
      ...DEFAULT_VAD_CONFIG,
      ...configOptions,
    };

    this.callbacks = {
      onSilenceStart: onSilenceStart || (() => {}),
      onSilenceEnd: onSilenceEnd || (() => {}),
      onActivityStart: onActivityStart || (() => {}),
      onActivityEnd: onActivityEnd || (() => {}),
      onVolumeChange: onVolumeChange || (() => {}),
    };
  }

  /**
   * Inicializa o VAD com um MediaStream
   * @param stream - Stream de áudio do microfone (opcional, será solicitado se não fornecido)
   */
  async init(stream?: MediaStream): Promise<boolean> {
    try {
      // Obtém o stream do microfone se não fornecido
      if (!stream) {
        stream = await navigator.mediaDevices.getUserMedia({ 
          audio: {
            echoCancellation: true,
            noiseSuppression: true,
            autoGainControl: true,
          }
        });
      }
      this.mediaStream = stream;

      // Cria contexto de áudio
      const AudioContextClass = window.AudioContext || (window as any).webkitAudioContext;
      this.audioContext = new AudioContextClass();
      
      // Cria analisador
      this.analyser = this.audioContext.createAnalyser();
      this.analyser.fftSize = 256;
      this.analyser.smoothingTimeConstant = 0.3;
      
      // Conecta o stream ao analisador
      this.sourceNode = this.audioContext.createMediaStreamSource(stream);
      this.sourceNode.connect(this.analyser);
      
      // Buffer para dados de frequência
      this.dataArray = new Uint8Array(this.analyser.frequencyBinCount);
      
      console.log('[VAD] Inicializado com sucesso');
      return true;
    } catch (error) {
      console.error('[VAD] Erro ao inicializar:', error);
      throw error;
    }
  }

  /**
   * Inicia a detecção de atividade de voz
   */
  start(): void {
    if (this._isActive) return;
    
    if (!this.analyser) {
      console.error('[VAD] Não inicializado. Chame init() primeiro.');
      return;
    }

    this._isActive = true;
    this._isSpeaking = false;
    this.silenceStartTime = null;
    this.activityStartTime = null;

    // Resume o AudioContext se estiver suspenso (necessário após interação do usuário)
    if (this.audioContext?.state === 'suspended') {
      this.audioContext.resume();
    }

    // Inicia loop de verificação
    this.checkIntervalId = setInterval(() => this.checkVolume(), this.config.checkInterval);
    
    console.log('[VAD] Detecção iniciada');
  }

  /**
   * Para a detecção de atividade de voz
   */
  stop(): void {
    if (!this._isActive) return;

    this._isActive = false;
    
    if (this.checkIntervalId) {
      clearInterval(this.checkIntervalId);
      this.checkIntervalId = null;
    }

    // Notifica fim de atividade se estava falando
    if (this._isSpeaking) {
      this._isSpeaking = false;
      this.callbacks.onActivityEnd();
    }

    console.log('[VAD] Detecção parada');
  }

  /**
   * Libera todos os recursos
   */
  destroy(): void {
    this.stop();
    
    if (this.sourceNode) {
      this.sourceNode.disconnect();
      this.sourceNode = null;
    }
    
    if (this.audioContext) {
      this.audioContext.close();
      this.audioContext = null;
    }
    
    if (this.mediaStream) {
      this.mediaStream.getTracks().forEach(track => track.stop());
      this.mediaStream = null;
    }
    
    this.analyser = null;
    this.dataArray = null;
    
    console.log('[VAD] Recursos liberados');
  }

  /**
   * Verifica o volume atual e detecta atividade/silêncio
   */
  private checkVolume(): void {
    if (!this.analyser || !this.dataArray) return;

    // Obtém dados de frequência
    this.analyser.getByteFrequencyData(this.dataArray as Uint8Array<ArrayBuffer>);

    // Calcula volume médio normalizado (0-1)
    let sum = 0;
    for (let i = 0; i < this.dataArray.length; i++) {
      sum += this.dataArray[i];
    }
    const average = sum / this.dataArray.length;
    const volume = average / 255;

    this._currentVolume = volume;

    // Notifica mudança de volume
    this.callbacks.onVolumeChange(volume);

    const now = Date.now();

    if (volume > this.config.activityThreshold) {
      // Há atividade de voz
      
      if (!this._isSpeaking) {
        // Ainda não estava falando, inicia contagem
        if (!this.activityStartTime) {
          this.activityStartTime = now;
        } else if (now - this.activityStartTime >= this.config.activityDuration) {
          // Atividade sustentada por tempo suficiente
          this._isSpeaking = true;
          this.silenceStartTime = null;
          this.activityStartTime = null;
          this.callbacks.onActivityStart();
        }
      } else {
        // Já estava falando, reseta contagem de silêncio
        if (this.silenceStartTime) {
          this.silenceStartTime = null;
          this.callbacks.onSilenceEnd();
        }
      }
    } else {
      // Há silêncio
      this.activityStartTime = null;

      if (this._isSpeaking) {
        // Estava falando, inicia contagem de silêncio
        if (!this.silenceStartTime) {
          this.silenceStartTime = now;
          this.callbacks.onSilenceStart();
        } else if (now - this.silenceStartTime >= this.config.silenceDuration) {
          // Silêncio sustentado por tempo suficiente
          this._isSpeaking = false;
          this.silenceStartTime = null;
          this.callbacks.onActivityEnd();
        }
      }
    }
  }

  /**
   * Retorna o volume atual (0-1)
   */
  getCurrentVolume(): number {
    if (!this.analyser || !this.dataArray) return 0;

    this.analyser.getByteFrequencyData(this.dataArray as Uint8Array<ArrayBuffer>);
    
    let sum = 0;
    for (let i = 0; i < this.dataArray.length; i++) {
      sum += this.dataArray[i];
    }
    
    return (sum / this.dataArray.length) / 255;
  }

  /**
   * Atualiza a configuração em runtime
   */
  updateConfig(newConfig: Partial<VADConfig>): void {
    this.config = {
      ...this.config,
      ...newConfig,
    };
  }

  /**
   * Atualiza os callbacks em runtime
   */
  updateCallbacks(newCallbacks: Partial<VADCallbacks>): void {
    this.callbacks = {
      ...this.callbacks,
      ...newCallbacks,
    };
  }

  // === Getters ===

  /** Se o VAD está ativo (rodando) */
  get isActive(): boolean {
    return this._isActive;
  }

  /** Alias para isActive */
  get active(): boolean {
    return this._isActive;
  }

  /** Se o usuário está falando */
  get isSpeaking(): boolean {
    return this._isSpeaking;
  }

  /** Alias para isSpeaking */
  get speaking(): boolean {
    return this._isSpeaking;
  }

  /** Volume atual (0-1) */
  get currentVolume(): number {
    return this._currentVolume;
  }

  /** Configuração atual */
  get currentConfig(): VADConfig {
    return { ...this.config };
  }
}

export default VoiceActivityDetector;
