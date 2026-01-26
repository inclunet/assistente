/**
 * Message Audio Service - Serviço simples de reprodução de áudio
 */

// Player global único
let currentPlayer: HTMLAudioElement | null = null;
let currentUrl: string | null = null;

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
    
    currentPlayer.onerror = (e) => {
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
  playAudioBlob,
  stopCurrentAudio,
  isCurrentlyPlaying,
  downloadAudioBlob,
  
  // Aliases
  stopAll: stopCurrentAudio,
  clearAll: stopCurrentAudio,
};
