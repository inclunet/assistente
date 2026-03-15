/**
 * Audio Recorder
 * 
 * Grava áudio usando MediaRecorder API.
 * Usado para enviar áudio para APIs como Whisper.
 */

import { AudioRecorderOptions } from './types';

type WebkitAudioWindow = Window & { webkitAudioContext?: typeof AudioContext };
type MediaRecorderErrorEvent = Event & { error?: DOMException | Error };

export class AudioRecorder {
  private mimeType: string;
  private audioBitsPerSecond: number;
  
  private mediaRecorder: MediaRecorder | null = null;
  private stream: MediaStream | null = null;
  private chunks: Blob[] = [];
  private _isRecording: boolean = false;
  
  // Callbacks
  private onStart: () => void;
  private onStop: (blob: Blob) => void;
  private onData: (data: Blob) => void;
  private onError: (error: Error | string) => void;
  
  // Visualização
  private analyser: AnalyserNode | null = null;
  private audioContext: AudioContext | null = null;

  constructor(options: AudioRecorderOptions = {}) {
    this.mimeType = options.mimeType || this.getPreferredMimeType();
    this.audioBitsPerSecond = options.audioBitsPerSecond || 128000;
    
    this.onStart = options.onStart || (() => {});
    this.onStop = options.onStop || (() => {});
    this.onData = options.onData || (() => {});
    this.onError = options.onError || (() => {});
  }

  /**
   * Obtém o melhor MIME type suportado
   */
  private getPreferredMimeType(): string {
    // Ordem de preferência para compatibilidade com Whisper
    const types = [
      'audio/webm;codecs=opus',
      'audio/webm',
      'audio/ogg;codecs=opus',
      'audio/mp4',
      'audio/wav',
    ];
    
    for (const type of types) {
      if (MediaRecorder.isTypeSupported(type)) {
        return type;
      }
    }
    
    return 'audio/webm';
  }

  /**
   * Verifica se o navegador suporta MediaRecorder
   */
  static isSupported(): boolean {
    return typeof window !== 'undefined' &&
           'MediaRecorder' in window && 
           'getUserMedia' in navigator.mediaDevices;
  }

  /**
   * Verifica suporte
   */
  get isSupported(): boolean {
    return AudioRecorder.isSupported();
  }

  /**
   * Solicita permissão do microfone e prepara para gravar
   */
  async init(existingStream?: MediaStream): Promise<boolean> {
    try {
      if (existingStream) {
        this.stream = existingStream;
      } else {
        this.stream = await navigator.mediaDevices.getUserMedia({ 
          audio: {
            channelCount: 1,
            sampleRate: 16000,
            echoCancellation: true,
            noiseSuppression: true,
            autoGainControl: true,
          } 
        });
      }
      
      this.mediaRecorder = new MediaRecorder(this.stream, {
        mimeType: this.mimeType,
        audioBitsPerSecond: this.audioBitsPerSecond,
      });
      
      this.mediaRecorder.ondataavailable = (event: BlobEvent) => {
        if (event.data.size > 0) {
          this.chunks.push(event.data);
          this.onData(event.data);
        }
      };
      
      this.mediaRecorder.onstop = () => {
        const blob = new Blob(this.chunks, { type: this.mimeType });
        this.chunks = [];
        this._isRecording = false;
        this.onStop(blob);
      };
      
      this.mediaRecorder.onerror = (event: Event) => {
        this._isRecording = false;
        const error = (event as MediaRecorderErrorEvent).error || new Error('Erro no MediaRecorder');
        this.onError(error);
      };
      
      // Setup para visualização de áudio
      this.setupAnalyser();
      
      return true;
    } catch (error) {
      const message = error instanceof Error 
        ? (error.name === 'NotAllowedError' 
          ? 'Permissão de microfone negada'
          : 'Erro ao acessar microfone: ' + error.message)
        : 'Erro desconhecido';
      this.onError(message);
      return false;
    }
  }

  /**
   * Configura o analyser para visualização
   */
  private setupAnalyser(): void {
    if (!this.stream) return;
    
    try {
      const AudioContextClass = window.AudioContext || (window as WebkitAudioWindow).webkitAudioContext;
      if (!AudioContextClass) {
        return;
      }
      this.audioContext = new AudioContextClass();
      this.analyser = this.audioContext.createAnalyser();
      this.analyser.fftSize = 256;
      
      const source = this.audioContext.createMediaStreamSource(this.stream);
      source.connect(this.analyser);
    } catch {
      // best-effort
    }
  }

  /**
   * Inicia a gravação
   */
  start(): boolean {
    if (!this.mediaRecorder) {
      this.onError('Recorder não inicializado. Chame init() primeiro.');
      return false;
    }
    
    if (this._isRecording) {
      return false;
    }
    
    this.chunks = [];
    this.mediaRecorder.start(100); // Envia dados a cada 100ms
    this._isRecording = true;
    this.onStart();
    return true;
  }

  /**
   * Para a gravação
   */
  stop(): void {
    if (!this.mediaRecorder || !this._isRecording) {
      return;
    }
    
    this.mediaRecorder.stop();
  }

  /**
   * Retorna nível de áudio atual (0-1)
   */
  getLevel(): number {
    if (!this.analyser) {
      return 0;
    }
    
    const dataArray = new Uint8Array(this.analyser.frequencyBinCount);
    this.analyser.getByteFrequencyData(dataArray);
    
    const average = dataArray.reduce((a, b) => a + b, 0) / dataArray.length;
    return average / 255;
  }

  /**
   * Retorna dados de frequência para visualização
   */
  getFrequencyData(): Uint8Array {
    if (!this.analyser) {
      return new Uint8Array(0);
    }
    
    const dataArray = new Uint8Array(this.analyser.frequencyBinCount);
    this.analyser.getByteFrequencyData(dataArray);
    return dataArray;
  }

  /**
   * Libera apenas o stream do microfone, mantendo o objeto reutilizável.
   * Útil para modos PTT/Toggle onde queremos desligar o microfone após cada gravação.
   */
  releaseStream(): void {
    if (this._isRecording) {
      return;
    }

    if (this.stream) {
      this.stream.getTracks().forEach(track => track.stop());
      this.stream = null;
    }
    
    if (this.audioContext) {
      this.audioContext.close();
      this.audioContext = null;
    }
    
    this.mediaRecorder = null;
    this.analyser = null;
  }

  /**
   * Verifica se o stream está ativo
   */
  get hasActiveStream(): boolean {
    return this.stream !== null && this.stream.active;
  }

  /**
   * Libera recursos
   */
  destroy(): void {
    if (this.mediaRecorder && this._isRecording) {
      this.mediaRecorder.stop();
    }
    
    if (this.stream) {
      this.stream.getTracks().forEach(track => track.stop());
      this.stream = null;
    }
    
    if (this.audioContext) {
      this.audioContext.close();
      this.audioContext = null;
    }
    
    this.mediaRecorder = null;
    this.analyser = null;
    this.chunks = [];
  }

  /**
   * Se está gravando
   */
  get isRecording(): boolean {
    return this._isRecording;
  }

  /**
   * MIME type em uso
   */
  get currentMimeType(): string {
    return this.mimeType;
  }

  /**
   * Stream de áudio
   */
  get mediaStream(): MediaStream | null {
    return this.stream;
  }
}

export default AudioRecorder;
