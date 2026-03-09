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

// Player global
let currentPlayer: HTMLAudioElement | null = null;
let currentUrl: string | null = null;

/** Para qualquer audio em reproducao */
function stopCurrentAudio(): void {
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
  currentUrl = URL.createObjectURL(audioBlob);
  currentPlayer = new Audio(currentUrl);
  currentPlayer.volume = Math.max(0, Math.min(1, volume));

  return new Promise((resolve, reject) => {
    if (!currentPlayer) { reject(new Error('Player nao criado')); return; }
    currentPlayer.onended = () => { stopCurrentAudio(); resolve(); };
    currentPlayer.onerror = () => { stopCurrentAudio(); reject(new Error('Erro ao reproduzir audio')); };
    currentPlayer.play().catch(reject);
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
      console.log('[MessageAudio] Audio no DB para msg:', messageId);
      return { audio: result.audio, mimeType: result.mimeType };
    }
    return null;
  } catch (err) {
    console.warn('[MessageAudio] Erro ao buscar audio do DB:', err);
    return null;
  }
}

/** Gera TTS via backend, salva no DB e retorna. */
async function generateAndSaveAudio(
  messageId: number,
  text: string,
): Promise<{ audio: string; mimeType: string } | null> {
  try {
    console.log('[MessageAudio] Gerando TTS para msg:', messageId);
    const result = await GenerateAndSaveMessageAudio(messageId, text);
    if (result && result.audio && result.audio.length > 0) {
      console.log('[MessageAudio] TTS gerado e salvo:', messageId);
      return { audio: result.audio, mimeType: result.mimeType };
    }
    return null;
  } catch (err) {
    console.warn('[MessageAudio] Erro ao gerar TTS:', err);
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
    console.log('[MessageAudio] Audio salvo no DB:', messageId);
  } catch (err) {
    console.warn('[MessageAudio] Erro ao salvar audio no DB:', err);
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

/** Converte base64 para Blob */
function base64ToBlob(base64: string, mimeType: string): Blob {
  const binaryString = atob(base64);
  const bytes = new Uint8Array(binaryString.length);
  for (let i = 0; i < binaryString.length; i++) {
    bytes[i] = binaryString.charCodeAt(i);
  }
  return new Blob([bytes], { type: mimeType });
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
