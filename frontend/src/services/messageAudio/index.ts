/**
 * Message Audio Service - Reprodução de áudio de mensagens com cache hierárquico
 *
 * Dois níveis de cache:
 *   1. Memória (Blob) — replay instantâneo sem IPC, LRU com limite de entradas
 *   2. DB (backend SpeakMessage) — persistente, cache-aware, gera TTS se necessário
 *
 * Fallback quando backend não tem TTS: frontend usa speakAsRole (WebSpeech/SAPI5)
 */

import { logger } from '../../utils/logger';
import { SpeakMessage } from '@wailsjs/go/wailsapi/Speech';
import { base64ToBlob } from '../../lib/audioUtils';

// ---------------------------------------------------------------------------
// Cache em memória (Blob) — evita re-transferência via IPC
// ---------------------------------------------------------------------------
const MEMORY_CACHE_MAX = 20;
const MEMORY_CACHE_MAX_BYTES = 50 * 1024 * 1024; // 50 MB
const memoryCache = new Map<string, Blob>();
let memoryCacheTotalBytes = 0;

function memoryCacheGet(messageId: string): Blob | undefined {
  const blob = memoryCache.get(messageId);
  if (blob) {
    // Move para o final (LRU refresh)
    memoryCache.delete(messageId);
    memoryCache.set(messageId, blob);
  }
  return blob;
}

function memoryCacheEvict(): void {
  // Remove entradas mais antigas até caber nos limites
  while (
    (memoryCache.size >= MEMORY_CACHE_MAX || memoryCacheTotalBytes >= MEMORY_CACHE_MAX_BYTES) &&
    memoryCache.size > 0
  ) {
    const oldest = memoryCache.keys().next().value;
    if (oldest === undefined) break;
    const blob = memoryCache.get(oldest);
    if (blob) memoryCacheTotalBytes -= blob.size;
    memoryCache.delete(oldest);
  }
}

function memoryCacheSet(messageId: string, blob: Blob): void {
  // Se já existe, remove antes (atualiza posição e contagem)
  const existing = memoryCache.get(messageId);
  if (existing) {
    memoryCacheTotalBytes -= existing.size;
    memoryCache.delete(messageId);
  }
  memoryCacheEvict();
  memoryCache.set(messageId, blob);
  memoryCacheTotalBytes += blob.size;
}

// ---------------------------------------------------------------------------
// Player global — apenas um áudio por vez
// ---------------------------------------------------------------------------
let currentPlayer: HTMLAudioElement | null = null;
let currentUrl: string | null = null;
let currentAbort: AbortController | null = null;
let currentMessageId: string | null = null;

/** Para qualquer áudio em reprodução e resolve Promises pendentes */
function stopCurrentAudio(): void {
  currentMessageId = null;
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
async function playAudioBlob(audioBlob: Blob, volume: number = 1.0, messageId?: string): Promise<void> {
  stopCurrentAudio();
  // Setar messageId DEPOIS do stop para não ser zerado
  if (messageId != null) currentMessageId = messageId;

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

/** Config do provider TTS passada pelo chamador */
export interface TTSProviderParams {
  providerId: string;
  voiceId: string;
  model: string;
  rate: number;
  /** Idioma do perfil que pediu a fala; define os rótulos falados no backend. */
  language?: string;
}

function canUseBackendTTS(provider?: TTSProviderParams): boolean {
  const providerId = provider?.providerId ?? '';
  return providerId !== '' && providerId !== 'webspeech' && !providerId.startsWith('ref_');
}

/**
 * Reproduz o áudio de uma mensagem usando cache hierárquico:
 *   1. Memória (Blob) → replay instantâneo
 *   2. Backend SpeakMessage (DB cache → gera TTS) → armazena em memória → toca
 *
 * @returns true se reproduziu, false se falhou (chamador deve usar speakAsRole)
 */
async function speakMessage(messageId: string, volume: number = 1.0, provider?: TTSProviderParams): Promise<boolean> {
  // 1. Cache em memória — instantâneo, sem IPC
  const cached = memoryCacheGet(messageId);
  if (cached) {
    await playAudioBlob(cached, volume, messageId);
    return true;
  }
  if (!canUseBackendTTS(provider)) {
    return false;
  }

  // 2. Backend (DB cache ou TTS) → armazena em memória
  try {
    const result = await SpeakMessage(
      messageId,
      provider?.providerId ?? '',
      provider?.model ?? '',
      provider?.voiceId ?? '',
      provider?.rate ?? 1.0,
      provider?.language ?? '',
    );
    if (result && result.audio && result.audio.length > 0) {
      const blob = base64ToBlob(result.audio, result.mimeType);
      memoryCacheSet(messageId, blob);
      await playAudioBlob(blob, volume, messageId);
      return true;
    }
    return false;
  } catch (err) {
    logger.warn('[messageAudio] speakMessage failed:', err);
    return false;
  }
}

/**
 * Obtém o áudio de uma mensagem como Blob (cache hierárquico).
 * Útil para download. Retorna null se falhar.
 */
async function getMessageAudioBlob(messageId: string, provider?: TTSProviderParams): Promise<Blob | null> {
  // Checa memória primeiro
  const cached = memoryCacheGet(messageId);
  if (cached) return cached;
  if (!canUseBackendTTS(provider)) {
    return null;
  }

  try {
    const result = await SpeakMessage(
      messageId,
      provider?.providerId ?? '',
      provider?.model ?? '',
      provider?.voiceId ?? '',
      provider?.rate ?? 1.0,
      provider?.language ?? '',
    );
    if (result && result.audio && result.audio.length > 0) {
      const blob = base64ToBlob(result.audio, result.mimeType);
      memoryCacheSet(messageId, blob);
      return blob;
    }
    return null;
  } catch (err) {
    logger.warn('[messageAudio] getMessageAudioBlob failed:', err);
    return null;
  }
}

/** Verifica se está tocando */
function isCurrentlyPlaying(): boolean {
  return currentPlayer !== null && !currentPlayer.paused;
}

/** Retorna o messageId sendo reproduzido, ou null */
function getCurrentPlayingMessageId(): string | null {
  if (!isCurrentlyPlaying()) return null;
  return currentMessageId;
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

/** Limpa o cache em memória. Uso em testes. */
function clearMemoryCache(): void {
  memoryCache.clear();
  memoryCacheTotalBytes = 0;
}

export const messageAudioService = {
  // Reprodução
  playAudioBlob,
  playAudioBase64,
  stopCurrentAudio,
  isCurrentlyPlaying,
  getCurrentPlayingMessageId,
  downloadAudioBlob,

  // Backend-driven (cache hierárquico: memória → DB)
  speakMessage,
  getMessageAudioBlob,
  clearMemoryCache,
};
