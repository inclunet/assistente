/**
 * Message Audio Service - Audio com persistencia no DB
 *
 * Audio persistido no banco de dados (campo audio da mensagem),
 * eliminando cache em memoria. Fonte de verdade e o DB.
 *
 * Hierarquia de reproducao:
 *   1. Audio no DB -> reproduz direto
 *   2. TTS OpenAI (gera arquivo) -> gera, salva no DB, reproduz
 *   3. TTS WebSpeech -> reproduz via browser (sem salvar)
 *   4. Sem TTS -> aviso ao usuario
 */

import { GetMessageAudio, GenerateAndSaveMessageAudio, SaveMessageAudio } from '@wailsjs/go/main/App';
import { base64ToBlob } from '../../lib/audioUtils';

// Player global
let currentPlayer: HTMLAudioElement | null = null;
let currentUrl: string | null = null;
let currentAbort: AbortController | null = null;

/** Para qualquer audio em reproducao e resolve Promises pendentes */
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

/** Reproduz um blob de audio */
async function playAudioBlob(audioBlob: Blob, volume: number = 1.0): Promise<void> {
  stopCurrentAudio();

  const abort = new AbortController();
  currentAbort = abort;

  currentUrl = URL.createObjectURL(audioBlob);
  currentPlayer = new Audio(currentUrl);
  currentPlayer.volume = Math.max(0, Math.min(1, volume));

  return new Promise<void>((resolve, reject) => {
    if (!currentPlayer) { reject(new Error('Player nao criado')); return; }

    // Se stopCurrentAudio() for chamado externamente, resolve a Promise
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
      reject(new Error('Erro ao reproduzir audio'));
    };
    currentPlayer.play().catch((err) => {
      abort.signal.removeEventListener('abort', onAbort);
      stopCurrentAudio();
      reject(err);
    });
  });
}

/** Reproduz audio a partir de base64 */
async function playAudioBase64(audioBase64: string, mimeType: string, volume: number = 1.0): Promise<void> {
  const blob = base64ToBlob(audioBase64, mimeType);
  return playAudioBlob(blob, volume);
}

/** Busca audio do DB. Retorna { audio, mimeType } ou null. */
async function getAudioFromDB(messageId: number): Promise<{ audio: string; mimeType: string } | null> {
  try {
    const result = await GetMessageAudio(messageId);
    if (result && result.audio && result.audio.length > 0) {
      return { audio: result.audio, mimeType: result.mimeType };
    }
    return null;
  } catch {
    return null;
  }
}

/** Gera TTS via backend, salva no DB e retorna. */
async function generateAndSaveAudio(
  messageId: number,
  text: string,
): Promise<{ audio: string; mimeType: string } | null> {
  try {
    const result = await GenerateAndSaveMessageAudio(messageId, text);
    if (result && result.audio && result.audio.length > 0) {
      return { audio: result.audio, mimeType: result.mimeType };
    }
    return null;
  } catch {
    return null;
  }
}

/** Salva audio blob no DB de uma mensagem. */
async function saveAudioToDB(messageId: number, audioBlob: Blob): Promise<void> {
  try {
    const reader = new FileReader();
    const base64 = await new Promise<string>((resolve, reject) => {
      reader.onload = () => {
        const result = reader.result as string;
        const base64Data = result.split(',')[1] || result;
        resolve(base64Data);
      };
      reader.onerror = reject;
      reader.readAsDataURL(audioBlob);
    });
    await SaveMessageAudio(messageId, base64, audioBlob.type || 'audio/mpeg');
  } catch (err) {
    console.warn('[messageAudio] Falha ao salvar áudio no DB:', err);
  }
}

/** Verifica se esta tocando */
function isCurrentlyPlaying(): boolean {
  return currentPlayer !== null && !currentPlayer.paused;
}

/** Baixa audio como arquivo */
function downloadAudioBlob(blob: Blob, filename: string): void {
  const url = URL.createObjectURL(blob);
  const a = document.createElement('a');
  a.href = url;
  a.download = filename;
  a.click();
  URL.revokeObjectURL(url);
}

// Export
export const messageAudioService = {
  // Reproducao
  playAudioBlob,
  playAudioBase64,
  stopCurrentAudio,
  isCurrentlyPlaying,
  downloadAudioBlob,

  // DB persistence
  getAudioFromDB,
  generateAndSaveAudio,
  saveAudioToDB,
  base64ToBlob,

  // Aliases
  stopAll: stopCurrentAudio,
  clearAll: stopCurrentAudio,
};
