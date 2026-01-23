/**
 * Message Audio Service - REFATORADO
 * Sistema simplificado com lock global para evitar reproduções simultâneas
 */

export interface MessageAudio {
  messageId: string;
  audioUrl: string;
  player: HTMLAudioElement;
}

class MessageAudioService {
  private audioMap = new Map<string, MessageAudio>();
  private playbackLock = false;
  private currentPlayer: HTMLAudioElement | null = null;
  private currentlyPlaying: string | null = null;

  /**
   * Cria player para mensagem
   */
  createAudioForMessage(messageId: string, audioBlob: Blob): MessageAudio {
    console.log(`[MessageAudio] 🎨 Criando player: ${messageId}`);
    
    // Remove player antigo se existir
    const existing = this.audioMap.get(messageId);
    if (existing) {
      this.removeAudio(messageId);
    }

    const audioUrl = URL.createObjectURL(audioBlob);
    const player = new Audio(audioUrl);

    const messageAudio: MessageAudio = {
      messageId,
      audioUrl,
      player,
    };

    // Handlers para liberar lock
    player.onended = () => {
      console.log(`[MessageAudio] ✅ Finalizado: ${messageId}`);
      this.playbackLock = false;
      if (this.currentPlayer === player) {
        this.currentPlayer = null;
      }
      if (this.currentlyPlaying === messageId) {
        this.currentlyPlaying = null;
      }
    };

    player.onpause = () => {
      this.playbackLock = false;
      if (this.currentPlayer === player) {
        this.currentPlayer = null;
      }
      if (this.currentlyPlaying === messageId) {
        this.currentlyPlaying = null;
      }
    };

    player.onerror = (error) => {
      console.error(`[MessageAudio] ❌ Erro:`, error);
      this.playbackLock = false;
      if (this.currentPlayer === player) {
        this.currentPlayer = null;
      }
    };

    this.audioMap.set(messageId, messageAudio);
    console.log(`[MessageAudio] ✅ Criado. Total: ${this.audioMap.size}`);
    return messageAudio;
  }

  /**
   * Reproduz mensagem - COM LOCK GLOBAL
   */
  async playMessage(messageId: string, volume: number = 1.0): Promise<void> {
    // 🔒 LOCK - Impede chamadas simultâneas
    if (this.playbackLock) {
      console.log(`[MessageAudio] 🔒 BLOQUEADO - já está tocando`);
      return;
    }

    const messageAudio = this.audioMap.get(messageId);
    if (!messageAudio) {
      console.warn(`[MessageAudio] ⚠️  Sem áudio: ${messageId}`);
      return;
    }

    console.log(`[MessageAudio] ▶️  Tocando: ${messageId}`);

    // Ativa lock IMEDIATAMENTE
    this.playbackLock = true;
    this.currentlyPlaying = messageId;

    // Para player atual
    if (this.currentPlayer && this.currentPlayer !== messageAudio.player) {
      this.currentPlayer.pause();
      this.currentPlayer.currentTime = 0;
    }

    // Configura e toca
    this.currentPlayer = messageAudio.player;
    messageAudio.player.volume = volume;
    messageAudio.player.currentTime = 0;

    try {
      await messageAudio.player.play();
    } catch (error) {
      console.error(`[MessageAudio] ❌ Falha ao tocar:`, error);
      this.playbackLock = false;
      this.currentPlayer = null;
      this.currentlyPlaying = null;
    }
  }

  /**
   * Para tudo
   */
  stopAll(): void {
    console.log(`[MessageAudio] ⏹️  Parando tudo`);
    
    if (this.currentPlayer) {
      this.currentPlayer.pause();
      this.currentPlayer.currentTime = 0;
    }
    
    this.audioMap.forEach((messageAudio) => {
      messageAudio.player.pause();
      messageAudio.player.currentTime = 0;
    });
    
    this.playbackLock = false;
    this.currentPlayer = null;
    this.currentlyPlaying = null;
  }

  /**
   * Remove e limpa o áudio de uma mensagem
   */
  removeAudio(messageId: string): void {
    const messageAudio = this.audioMap.get(messageId);
    if (messageAudio) {
      messageAudio.player.pause();
      messageAudio.player.onplay = null;
      messageAudio.player.onended = null;
      messageAudio.player.onerror = null;
      URL.revokeObjectURL(messageAudio.audioUrl);
      this.audioMap.delete(messageId);
      
      if (this.currentlyPlaying === messageId) {
        this.currentlyPlaying = null;
      }
    }
  }

  /**
   * Verifica se uma mensagem tem áudio disponível
   */
  hasAudio(messageId: string): boolean {
    return this.audioMap.has(messageId);
  }

  /**
   * Verifica se uma mensagem específica está tocando
   */
  isMessagePlaying(messageId: string): boolean {
    return this.currentlyPlaying === messageId;
  }

  /**
   * Limpa todos os áudios (útil ao trocar de aba)
   */
  clearAll(): void {
    this.audioMap.forEach((messageAudio) => {
      messageAudio.player.pause();
      messageAudio.player.onplay = null;
      messageAudio.player.onended = null;
      messageAudio.player.onerror = null;
      URL.revokeObjectURL(messageAudio.audioUrl);
    });
    this.audioMap.clear();
    this.currentlyPlaying = null;
  }
}

// Singleton export
export const messageAudioService = new MessageAudioService();
