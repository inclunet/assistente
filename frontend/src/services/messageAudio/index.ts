/**
 * Message Audio Service - Reprodução de áudio de mensagens com cache no DB
 *
 * Toda a lógica de cache é backend-driven: o backend decide se retorna do cache
 * ou gera TTS novo, salva e retorna. O frontend apenas toca o resultado.
 *
 * Hierarquia (implementada no backend SpeakMessage):
 *   1. Áudio em cache no DB → retorna direto
 *   2. TTS OpenAI (gera) → salva no DB → retorna
 *   3. Falha → frontend usa speakAsRole (WebSpeech/SAPI5) como fallback
 */

import { SpeakMessage } from '@wailsjs/go/main/App';
import { base64ToBlob } from '../../lib/audioUtils';

// Player global — apenas um áudio por vez
let currentPlayer: HTMLAudioElement | null = null;
let currentUrl: string | null = null;
let currentAbort: AbortController | null = null;

/** Para qualquer áudio em reprodução e resolve Promises pendentes */
function stopCurrentAudio(): void {
  if (currentAbort) {
    currentAbort.abort();
    currentAbort = null;
  }
  if (currentPlayer) {
    currentPlayer.pause();
    currentPlayer.onended = null;
    currentPlayer.onerror = null;
    currentPlayer = null;
  }
  if (currentUrl) {
    URL.revokeObjectURL(currentUrl);
    currentUrl = null;
  }
}

/** Reproduz um blob de áudio */
async function playAudioBlob(audioBlob: Blob, volume: number = 1.0): Promise<void> {
  stopCurrentAudio();

  const abort = new AbortController();
  currentAbort = abort;

  currentUrl = URL.createObjectURL(audioBlob);
  currentPlayer = new Audio(currentUrl);
  currentPlayer.volume = Math.max(0, Math.min(1, volume));

  return new Promise<void>((resolve, reject) => {
    if (!currentPlayer) { reject(new Error('Player não criado')); return; }

    const onAbort = () => { resolve(); };
    abort.signal.addEventListener('abort', onAbort, { once: true });

    currentPlayer.onended = () => {
      abort.signal.removeEventListener('abort', onAbort);
      stopCurrentAudio();
      resolve();
    };
    currentPlayer.onerror = () => {
      abort.signal.removeEventListener('abort', onAbort);
      stopCurrentAudio();
      reject(new Error('Erro ao reproduzir áudio'));
    };
    currentPlayer.play().catch((err) => {
      abort.signal.removeEventListener('abort', onAbort);
      stopCurrentAudio();
      reject(err);
    });
  });
}

/** Reproduz áudio a partir de base64 */
async function playAudioBase64(audioBase64: string, mimeType: string, volume: number = 1.0): Promise<void> {
  const blob = base64ToBlob(audioBase64, mimeType);
  return playAudioBlob(blob, volume);
}

/**
 * Reproduz o áudio de uma mensagem (backend-driven, cache-aware).
 *
 * O backend (SpeakMessage) faz tudo: checa cache no DB, gera TTS se necessário,
 * salva no DB e retorna o áudio. O frontend apenas toca o resultado.
 *
 * @returns true se o áudio foi reproduzido com sucesso, false se falhou
 *          (ex: provider TTS indisponível). Quando retorna false o chamador
 *          deve usar ttsService.speakAsRole como fallback.
 */
async function speakMessage(messageId: number, volume: number = 1.0): Promise<boolean> {
  try {
    const result = await SpeakMessage(messageId);
    if (result && result.audio && result.audio.length > 0) {
      await playAudioBase64(result.audio, result.mimeType, volume);
      return true;
    }
    return false;
  } catch {
    return false;
  }
}

/**
 * Obtém o áudio de uma mensagem como Blob (backend-driven, cache-aware).
 * Útil para download. Retorna null se falhar.
 */
async function getMessageAudioBlob(messageId: number): Promise<Blob | null> {
  try {
    const result = await SpeakMessage(messageId);
    if (result && result.audio && result.audio.length > 0) {
      return base64ToBlob(result.audio, result.mimeType);
    }
    return null;
  } catch {
    return null;
  }
}

/** Verifica se está tocando */
function isCurrentlyPlaying(): boolean {
  return currentPlayer !== null && !currentPlayer.paused;
}

/** Baixa áudio como arquivo */
function downloadAudioBlob(blob: Blob, filename: string): void {
  const url = URL.createObjectURL(blob);
  const a = document.createElement('a');
  a.href = url;
  a.download = filename;
  a.click();
  URL.revokeObjectURL(url);
}

export const messageAudioService = {
  // Reprodução
  playAudioBlob,
  playAudioBase64,
  stopCurrentAudio,
  isCurrentlyPlaying,
  downloadAudioBlob,

  // Backend-driven (cache-aware)
  speakMessage,
  getMessageAudioBlob,
  base64ToBlob,

  // Aliases
  stopAll: stopCurrentAudio,
  clearAll: stopCurrentAudio,
};
