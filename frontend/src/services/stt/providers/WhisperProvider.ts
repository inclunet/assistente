/**
 * Whisper API STT Provider
 * 
 * Usa OpenAI Whisper API via backend Go para transcrição de voz.
 * Grava áudio localmente e envia para processamento no servidor.
 */

import { STT_PROVIDERS, ISTTProvider, STTProvider } from '../types';
import { AudioRecorder } from '../AudioRecorder';

// Import dinâmico do Wails
let TranscribeWhisper: ((audioBase64: string, filename: string) => Promise<{ text: string }>) | null = null;

async function loadWailsFunctions(): Promise<boolean> {
  try {
    const wails = await import('../../../../wailsjs/go/main/App');
    TranscribeWhisper = wails.TranscribeWhisper;
    return true;
  } catch (e) {
    console.warn('[WhisperProvider] Wails TranscribeWhisper não disponível:', e);
    return false;
  }
}

export interface WhisperProviderOptions {
  language?: string;
  onStart?: () => void;
  onEnd?: (transcript: string) => void;
  onProcessing?: () => void;
  onError?: (message: string, code?: string) => void;
}

export class WhisperProvider implements ISTTProvider {
  readonly name: STTProvider = STT_PROVIDERS.WHISPER_API;
  
  private audioRecorder: AudioRecorder | null = null;
  private _isSupported: boolean = false;
  private _isRecording: boolean = false;
  private _isProcessing: boolean = false;
  private _language: string;
  
  // Callbacks
  private onStart: () => void;
  private onEnd: (transcript: string) => void;
  private onProcessing: () => void;
  private onError: (message: string, code?: string) => void;

  constructor(options: WhisperProviderOptions = {}) {
    this._language = options.language || 'pt-BR';
    this.onStart = options.onStart || (() => {});
    this.onEnd = options.onEnd || (() => {});
    this.onProcessing = options.onProcessing || (() => {});
    this.onError = options.onError || (() => {});
  }

  /**
   * Se está suportado
   */
  get isSupported(): boolean {
    return this._isSupported && AudioRecorder.isSupported();
  }

  /**
   * Verifica suporte estaticamente
   */
  static async checkSupport(): Promise<boolean> {
    if (!AudioRecorder.isSupported()) {
      return false;
    }
    
    await loadWailsFunctions();
    return !!TranscribeWhisper;
  }

  /**
   * Inicializa o provider (sem inicializar o stream do microfone)
   */
  async init(): Promise<boolean> {
    // Carrega funções do Wails
    await loadWailsFunctions();
    
    if (!TranscribeWhisper) {
      console.warn('[WhisperProvider] TranscribeWhisper não disponível');
      this._isSupported = false;
      return false;
    }

    // Cria o gravador de áudio (sem inicializar stream ainda - lazy init)
    this.audioRecorder = new AudioRecorder({
      mimeType: 'audio/webm',
      onStart: () => {
        this._isRecording = true;
        this.onStart();
      },
      onStop: async (blob: Blob) => {
        this._isRecording = false;
        // Libera o microfone após parar a gravação
        this.audioRecorder?.releaseStream();
        if (blob.size > 0) {
          await this.processAudioBlob(blob);
        }
      },
      onError: (error) => {
        this._isRecording = false;
        const message = typeof error === 'string' ? error : error.message;
        this.onError(message, 'recorder-error');
      },
    });

    // NÃO inicializa o stream aqui - será feito sob demanda em start()
    this._isSupported = AudioRecorder.isSupported();
    
    console.log('[WhisperProvider] Inicializado (microfone será ativado sob demanda)');
    return true;
  }

  /**
   * Inicia a gravação
   */
  async start(): Promise<boolean> {
    if (!this.audioRecorder) {
      this.onError('Provider não inicializado', 'not-initialized');
      return false;
    }
    
    if (this._isRecording || this._isProcessing) {
      return false;
    }
    
    // Inicializa stream do microfone se necessário (lazy init)
    if (!this.audioRecorder.hasActiveStream) {
      const success = await this.audioRecorder.init();
      if (!success) {
        this.onError('Falha ao acessar microfone', 'microphone-error');
        return false;
      }
    }
    
    return this.audioRecorder.start();
  }

  /**
   * Para a gravação (dispara processamento)
   */
  stop(): void {
    if (!this.audioRecorder || !this._isRecording) {
      return;
    }
    
    this.audioRecorder.stop();
  }

  /**
   * Aborta sem processar
   */
  abort(): void {
    if (this.audioRecorder && this._isRecording) {
      this.audioRecorder.stop();
    }
    this._isRecording = false;
    this._isProcessing = false;
  }

  /**
   * Define o idioma (usado como hint para o Whisper)
   */
  setLanguage(language: string): void {
    this._language = language;
  }

  /**
   * Retorna o idioma configurado
   */
  get language(): string {
    return this._language;
  }

  /**
   * Processa o blob de áudio com Whisper
   */
  private async processAudioBlob(blob: Blob): Promise<void> {
    if (!TranscribeWhisper) {
      this.onError('Whisper não disponível', 'whisper-unavailable');
      return;
    }

    try {
      this._isProcessing = true;
      this.onProcessing();
      
      // Converte para base64
      const audioBase64 = await this.blobToBase64(blob);
      
      // Envia para o backend
      const result = await TranscribeWhisper(audioBase64, 'audio.webm');
      
      this._isProcessing = false;
      
      if (result && result.text && result.text.trim()) {
        this.onEnd(result.text.trim());
      } else {
        this.onEnd('');
      }
    } catch (error) {
      this._isProcessing = false;
      const message = error instanceof Error ? error.message : 'Erro na transcrição';
      console.error('[WhisperProvider] Erro:', error);
      this.onError(message, 'transcription-error');
    }
  }

  /**
   * Converte Blob para Base64
   */
  private blobToBase64(blob: Blob): Promise<string> {
    return new Promise((resolve, reject) => {
      const reader = new FileReader();
      reader.onloadend = () => {
        const result = reader.result as string;
        const base64 = result.split(',')[1];
        resolve(base64);
      };
      reader.onerror = reject;
      reader.readAsDataURL(blob);
    });
  }

  /**
   * Libera recursos
   */
  destroy(): void {
    if (this.audioRecorder) {
      this.audioRecorder.destroy();
      this.audioRecorder = null;
    }
    this._isRecording = false;
    this._isProcessing = false;
  }

  /**
   * Se está gravando
   */
  get isRecording(): boolean {
    return this._isRecording;
  }

  /**
   * Se está processando
   */
  get isProcessing(): boolean {
    return this._isProcessing;
  }

  /**
   * Nível de áudio atual (0-1)
   */
  getLevel(): number {
    return this.audioRecorder?.getLevel() || 0;
  }
}

export default WhisperProvider;
