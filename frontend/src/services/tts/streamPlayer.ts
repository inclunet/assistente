/**
 * TTSStreamPlayer - Player de áudio com streaming usando MediaSource API
 * 
 * Interface unificada para reprodução de áudio em streaming.
 * Recebe chunks de áudio e começa a reproduzir assim que o primeiro chunk chega.
 * Funciona com qualquer provedor de TTS que suporte streaming.
 */

import { EventsOn } from '@wailsjs/runtime/runtime';
import { base64ToBytes } from '../../lib/audioUtils';

// Evento de streaming do backend (formato padronizado)
interface TTSStreamEvent {
  sessionId: string;
  chunkBase64?: string;
  format?: string;
  done?: boolean;
  error?: string;
}

// Callbacks para o stream player
export interface StreamPlayerCallbacks {
  onStart?: () => void;
  onEnd?: () => void;
  onError?: (error: Error) => void;
}

// Estado do player
type PlayerState = 'idle' | 'buffering' | 'playing' | 'paused' | 'stopped' | 'error';

/**
 * TTSStreamPlayer
 * 
 * Usa MediaSource API para reproduzir áudio enquanto ainda está sendo baixado.
 * Para browsers que não suportam MediaSource, faz fallback para buffering completo.
 */
export class TTSStreamPlayer {
  private sessionId: string | null = null;
  private state: PlayerState = 'idle';
  private callbacks: StreamPlayerCallbacks = {};
  
  // MediaSource API
  private mediaSource: MediaSource | null = null;
  private sourceBuffer: SourceBuffer | null = null;
  private audioElement: HTMLAudioElement | null = null;
  private audioUrl: string | null = null;
  
  // Buffer de chunks pendentes (quando sourceBuffer está atualizando)
  private pendingChunks: Uint8Array[] = [];
  private isUpdating = false;
  
  // Fallback para browsers sem MediaSource
  private useFallback = false;
  private fallbackChunks: Uint8Array[] = [];
  
  // Timer de endStream (para cleanup)
  private endStreamTimer: number | null = null;
  
  // Event listeners do Wails
  private unsubscribeStart: (() => void) | null = null;
  private unsubscribeChunk: (() => void) | null = null;
  private unsubscribeDone: (() => void) | null = null;
  private unsubscribeError: (() => void) | null = null;

  // Timestamp para medir latência (apenas primeiro chunk)
  private firstChunkLogged = false;

  constructor() {
    // Verifica suporte a MediaSource
    this.useFallback = !('MediaSource' in window) || !MediaSource.isTypeSupported('audio/mpeg');
  }

  /**
   * Inicia a escuta de eventos de streaming para uma sessão
   */
  startListening(sessionId: string, callbacks: StreamPlayerCallbacks = {}): void {
    this.cleanup();
    
    this.sessionId = sessionId;
    this.callbacks = callbacks;
    this.state = 'buffering';
    this.firstChunkLogged = false;
    
    // Registra listeners de eventos Wails
    this.unsubscribeStart = EventsOn('tts:stream:start', (event: TTSStreamEvent) => {
      if (event.sessionId === this.sessionId) {
        this.handleStart();
      }
    });
    
    this.unsubscribeChunk = EventsOn('tts:stream:chunk', (event: TTSStreamEvent) => {
      if (event.sessionId === this.sessionId) {
        this.handleChunk(event);
      }
    });
    
    this.unsubscribeDone = EventsOn('tts:stream:done', (event: TTSStreamEvent) => {
      if (event.sessionId === this.sessionId) {
        this.handleDone();
      }
    });
    
    this.unsubscribeError = EventsOn('tts:stream:error', (event: TTSStreamEvent) => {
      if (event.sessionId === this.sessionId) {
        this.handleError(event);
      }
    });
  }

  /**
   * Handler para evento de início
   */
  private handleStart(): void {
    if (this.useFallback) {
      this.fallbackChunks = [];
    } else {
      this.setupMediaSource();
    }
  }

  /**
   * Handler para chunks de áudio
   */
  private handleChunk(event: TTSStreamEvent): void {
    if (!event.chunkBase64) return;
    
    // Decodifica base64 para Uint8Array
    const bytes = base64ToBytes(event.chunkBase64);
    
    // Marca primeiro chunk recebido
    if (!this.firstChunkLogged) {
      this.firstChunkLogged = true;
    }
    
    if (this.useFallback) {
      this.fallbackChunks.push(bytes);
    } else {
      this.appendChunk(bytes);
    }
  }

  /**
   * Handler para fim do streaming
   */
  private handleDone(): void {
    if (this.useFallback) {
      this.playFallback();
    } else {
      this.endStream();
    }
  }

  /**
   * Handler para erro
   */
  private handleError(event: TTSStreamEvent): void {
    console.error('[TTSStream] Error:', event.error);
    this.state = 'error';
    this.callbacks.onError?.(new Error(event.error || 'Unknown streaming error'));
    this.cleanup();
  }

  /**
   * Configura MediaSource API
   */
  private setupMediaSource(): void {
    this.mediaSource = new MediaSource();
    this.audioElement = new Audio();
    this.audioUrl = URL.createObjectURL(this.mediaSource);
    this.audioElement.src = this.audioUrl;
    
    this.mediaSource.addEventListener('sourceopen', () => {
      try {
        this.sourceBuffer = this.mediaSource!.addSourceBuffer('audio/mpeg');
        
        this.sourceBuffer.addEventListener('updateend', () => {
          this.isUpdating = false;
          this.tryStartPlayback();
          this.processPendingChunks();
        });
        
        this.sourceBuffer.addEventListener('error', (e) => {
          console.error('[TTSStream] SourceBuffer error:', e);
        });
        
        this.processPendingChunks();
        
      } catch (e) {
        console.error('[TTSStream] Error adding source buffer:', e);
        // Fallback para modo sem streaming
        this.useFallback = true;
        this.fallbackChunks = [...this.pendingChunks];
        this.pendingChunks = [];
      }
    });
    
    this.audioElement.addEventListener('playing', () => {
      if (this.state === 'buffering') {
        this.state = 'playing';
        this.callbacks.onStart?.();
      }
    });
    
    this.audioElement.addEventListener('ended', () => {
      this.state = 'idle';
      this.callbacks.onEnd?.();
      this.cleanup();
    });
    
    this.audioElement.addEventListener('error', () => {
      // Ignora erros após o stream terminar (são erros de cleanup)
      if (this.state === 'idle' || this.state === 'stopped') {
        return;
      }
      
      // Ignora erros se o MediaSource já foi fechado
      if (this.mediaSource?.readyState === 'ended') {
        return;
      }
      
      this.state = 'error';
      this.callbacks.onError?.(new Error('Audio playback error'));
    });
  }

  /**
   * Adiciona chunk ao SourceBuffer
   */
  private appendChunk(chunk: Uint8Array): void {
    this.pendingChunks.push(chunk);
    this.processPendingChunks();
  }

  /**
   * Tenta iniciar a reprodução se houver buffer suficiente
   */
  private tryStartPlayback(): void {
    if (!this.audioElement || !this.sourceBuffer) return;
    if (!this.audioElement.paused) return;
    if (this.state !== 'buffering') return;
    
    try {
      const buffered = this.sourceBuffer.buffered;
      if (buffered.length > 0) {
        const bufferedSeconds = buffered.end(0);
        // Inicia com 0.1s de buffer para menor latência
        if (bufferedSeconds > 0.1) {
          this.audioElement.play().catch(e => {
            console.error('[TTSStream] Play error:', e);
          });
        }
      }
    } catch {
      // Ignora erros ao verificar buffer
    }
  }

  /**
   * Processa chunks pendentes
   */
  private processPendingChunks(): void {
    if (this.isUpdating || !this.sourceBuffer || this.pendingChunks.length === 0) {
      return;
    }
    
    if (this.sourceBuffer.updating) {
      return;
    }
    
    const chunk = this.pendingChunks.shift()!;
    this.isUpdating = true;
    
    try {
      const buffer = new ArrayBuffer(chunk.byteLength);
      new Uint8Array(buffer).set(chunk);
      this.sourceBuffer.appendBuffer(buffer);
    } catch (e) {
      console.error('[TTSStream] Error appending chunk:', e);
      this.isUpdating = false;
    }
  }

  /**
   * Sinaliza fim do stream
   */
  private endStream(): void {
    const checkAndEnd = () => {
      if (this.pendingChunks.length > 0 || this.isUpdating) {
        this.endStreamTimer = window.setTimeout(checkAndEnd, 100);
        return;
      }
      this.endStreamTimer = null;
      
      if (this.mediaSource && this.mediaSource.readyState === 'open') {
        try {
          this.mediaSource.endOfStream();
        } catch {
          // Ignora erros ao finalizar stream
        }
      }
    };
    
    checkAndEnd();
  }

  /**
   * Modo fallback: junta chunks e reproduz
   */
  private playFallback(): void {
    if (this.fallbackChunks.length === 0) {
      this.callbacks.onEnd?.();
      return;
    }
    
    const totalLength = this.fallbackChunks.reduce((acc, chunk) => acc + chunk.length, 0);
    const combined = new Uint8Array(totalLength);
    let offset = 0;
    for (const chunk of this.fallbackChunks) {
      combined.set(chunk, offset);
      offset += chunk.length;
    }
    
    const blob = new Blob([combined], { type: 'audio/mpeg' });
    this.audioUrl = URL.createObjectURL(blob);
    this.audioElement = new Audio(this.audioUrl);
    
    this.audioElement.addEventListener('playing', () => {
      this.state = 'playing';
      this.callbacks.onStart?.();
    });
    
    this.audioElement.addEventListener('ended', () => {
      this.state = 'idle';
      this.callbacks.onEnd?.();
      this.cleanup();
    });
    
    this.audioElement.play().catch(e => {
      console.error('[TTSStream] Fallback play error:', e);
      this.callbacks.onError?.(e);
      this.cleanup();
    });
  }

  /**
   * Para a reprodução
   */
  stop(): void {
    this.state = 'stopped';
    
    if (this.audioElement) {
      this.audioElement.pause();
      this.audioElement.currentTime = 0;
    }
    
    this.cleanup();
  }

  /**
   * Pausa a reprodução
   */
  pause(): void {
    if (this.audioElement && this.state === 'playing') {
      this.audioElement.pause();
      this.state = 'paused';
    }
  }

  /**
   * Retoma a reprodução
   */
  resume(): void {
    if (this.audioElement && this.state === 'paused') {
      this.audioElement.play().catch(() => {
        // Ignora erros ao retomar
      });
      this.state = 'playing';
    }
  }

  /**
   * Retorna o estado atual
   */
  getState(): PlayerState {
    return this.state;
  }

  /**
   * Verifica se está reproduzindo
   */
  isPlaying(): boolean {
    return this.state === 'playing';
  }

  /**
   * Limpa recursos
   */
  private cleanup(): void {
    this.state = 'idle';

    // Cancela timer de endStream pendente
    if (this.endStreamTimer !== null) {
      clearTimeout(this.endStreamTimer);
      this.endStreamTimer = null;
    }

    this.unsubscribeStart?.();
    this.unsubscribeChunk?.();
    this.unsubscribeDone?.();
    this.unsubscribeError?.();
    
    this.unsubscribeStart = null;
    this.unsubscribeChunk = null;
    this.unsubscribeDone = null;
    this.unsubscribeError = null;
    
    if (this.sourceBuffer && this.mediaSource?.readyState === 'open') {
      try {
        this.mediaSource.removeSourceBuffer(this.sourceBuffer);
      } catch {
        // Ignora erros ao remover
      }
    }
    
    this.sourceBuffer = null;
    this.mediaSource = null;
    
    if (this.audioElement) {
      this.audioElement.pause();
      this.audioElement.src = '';
      this.audioElement = null;
    }
    
    if (this.audioUrl) {
      URL.revokeObjectURL(this.audioUrl);
      this.audioUrl = null;
    }
    
    this.pendingChunks = [];
    this.fallbackChunks = [];
    this.isUpdating = false;
    this.sessionId = null;
  }

  /**
   * Libera recursos (para quando o componente é desmontado)
   */
  dispose(): void {
    this.stop();
    this.cleanup();
    // Invalida o singleton para que getStreamPlayer() crie uma nova instância
    if (globalStreamPlayer === this) {
      globalStreamPlayer = null;
    }
  }
}

// Singleton global do stream player
let globalStreamPlayer: TTSStreamPlayer | null = null;

export function getStreamPlayer(): TTSStreamPlayer {
  if (!globalStreamPlayer) {
    globalStreamPlayer = new TTSStreamPlayer();
  }
  return globalStreamPlayer;
}
