/**
 * Message Audio Service - Serviço de reprodução de áudio com cache
 * 
 * O cache evita re-sintetizar mensagens já reproduzidas, economizando
 * chamadas à API e melhorando a experiência do usuário.
 */

// Player global único
let currentPlayer: HTMLAudioElement | null = null;
let currentUrl: string | null = null;

// Cache de áudio por mensagem (messageId → Blob)
const audioCache = new Map<string, { blob: Blob; lastAccess: number }>();
const MAX_CACHE_SIZE = 50; // Máximo de mensagens em cache
const CACHE_TTL_MS = 30 * 60 * 1000; // 30 minutos de TTL

/**
 * Limpa entradas antigas do cache (LRU + TTL)
 */
function pruneCache(): void {
  const now = Date.now();
  
  // Remove entradas expiradas
  for (const [key, entry] of audioCache.entries()) {
    if (now - entry.lastAccess > CACHE_TTL_MS) {
      audioCache.delete(key);
    }
  }
  
  // Se ainda excede o limite, remove as mais antigas
  if (audioCache.size > MAX_CACHE_SIZE) {
    const entries = Array.from(audioCache.entries())
      .sort((a, b) => a[1].lastAccess - b[1].lastAccess);
    
    const toRemove = entries.slice(0, audioCache.size - MAX_CACHE_SIZE);
    for (const [key] of toRemove) {
      audioCache.delete(key);
    }
  }
}

/**
 * Obtém áudio do cache
 */
function getCachedAudio(messageId: string): Blob | null {
  const entry = audioCache.get(messageId);
  if (entry) {
    // Atualiza último acesso
    entry.lastAccess = Date.now();
    console.log('[MessageAudio] Cache hit para mensagem:', messageId);
    return entry.blob;
  }
  return null;
}

/**
 * Armazena áudio no cache
 */
function cacheAudio(messageId: string, blob: Blob): void {
  pruneCache();
  audioCache.set(messageId, { blob, lastAccess: Date.now() });
  console.log('[MessageAudio] Áudio cacheado para mensagem:', messageId, '- Cache size:', audioCache.size);
}

/**
 * Verifica se mensagem tem áudio em cache
 */
function hasCachedAudio(messageId: string): boolean {
  return audioCache.has(messageId);
}

/**
 * Limpa todo o cache
 */
function clearCache(): void {
  audioCache.clear();
  console.log('[MessageAudio] Cache limpo');
}

/**
 * Para qualquer áudio em reprodução
 */
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

/**
 * Reproduz um blob de áudio
 */
async function playAudioBlob(audioBlob: Blob, volume: number = 1.0): Promise<void> {
  // Para qualquer áudio anterior
  stopCurrentAudio();
  
  // Cria URL e player
  currentUrl = URL.createObjectURL(audioBlob);
  currentPlayer = new Audio(currentUrl);
  currentPlayer.volume = Math.max(0, Math.min(1, volume));
  
  return new Promise((resolve, reject) => {
    if (!currentPlayer) {
      reject(new Error('Player não criado'));
      return;
    }
    
    currentPlayer.onended = () => {
      stopCurrentAudio();
      resolve();
    };
    
    currentPlayer.onerror = () => {
      stopCurrentAudio();
      reject(new Error('Erro ao reproduzir áudio'));
    };
    
    currentPlayer.play().catch(reject);
  });
}

/**
 * Verifica se está tocando
 */
function isCurrentlyPlaying(): boolean {
  return currentPlayer !== null && !currentPlayer.paused;
}

/**
 * Baixa um blob de áudio como arquivo
 */
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
  // Reprodução
  playAudioBlob,
  stopCurrentAudio,
  isCurrentlyPlaying,
  downloadAudioBlob,
  
  // Cache
  getCachedAudio,
  cacheAudio,
  hasCachedAudio,
  clearCache,
  
  // Aliases
  stopAll: stopCurrentAudio,
  clearAll: stopCurrentAudio,
};
