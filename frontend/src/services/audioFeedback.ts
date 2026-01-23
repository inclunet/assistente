/**
 * Audio Feedback Service
 * Gera sons sintéticos usando Web Audio API para feedback de interações
 * Não requer arquivos externos de áudio
 */

// Tipos de sons disponíveis
export const SOUND_TYPES = {
  // Chat
  SEND: 'send',           // Mensagem enviada: "tum di" (grave → agudo)
  RECEIVE: 'receive',     // Mensagem recebida: "ti dum" (agudo → grave)
  
  // Status
  SUCCESS: 'success',     // Operação bem-sucedida
  ERROR: 'error',         // Erro
  CLEAR: 'clear',         // Limpeza
  
  // Gravação
  RECORD_START: 'record_start', // Início de gravação
  RECORD_END: 'record_end',     // Fim de gravação
  LISTENING: 'listening',       // Ouvindo (VAD)
  
  // Navegação
  FOCUS: 'focus',         // Foco em elemento
  BOUNDARY: 'boundary',   // Limite de navegação
  BUMP: 'bump',           // Bateu no limite (som de tambor)
} as const;

export type SoundType = typeof SOUND_TYPES[keyof typeof SOUND_TYPES];

// Singleton AudioContext
let audioContext: AudioContext | null = null;

/**
 * Obtém ou cria o AudioContext
 */
function getAudioContext(): AudioContext {
  if (!audioContext) {
    audioContext = new (window.AudioContext || (window as any).webkitAudioContext)();
  }
  return audioContext;
}

/**
 * Cria um tom sintético
 */
function createTone(ctx: AudioContext): {
  oscillator: OscillatorNode;
  gainNode: GainNode;
} {
  const oscillator = ctx.createOscillator();
  const gainNode = ctx.createGain();
  
  oscillator.type = 'sine';
  oscillator.connect(gainNode);
  gainNode.connect(ctx.destination);
  gainNode.gain.value = 0;
  
  return { oscillator, gainNode };
}

/**
 * Reproduz um som de feedback
 * @param type - Tipo de som (use SOUND_TYPES)
 */
export function playSound(type: SoundType): void {
  try {
    const ctx = getAudioContext();
    const now = ctx.currentTime;
    
    switch (type) {
      // === CHAT ===
      
      case SOUND_TYPES.SEND:
        // "tum di" - grave depois agudo
        {
          const { oscillator: osc1, gainNode: gain1 } = createTone(ctx);
          osc1.frequency.setValueAtTime(330, now);
          gain1.gain.setValueAtTime(0.25, now);
          gain1.gain.linearRampToValueAtTime(0, now + 0.06);
          osc1.start(now);
          osc1.stop(now + 0.06);
          
          const { oscillator: osc2, gainNode: gain2 } = createTone(ctx);
          osc2.frequency.setValueAtTime(660, now + 0.07);
          gain2.gain.setValueAtTime(0, now);
          gain2.gain.setValueAtTime(0.25, now + 0.07);
          gain2.gain.linearRampToValueAtTime(0, now + 0.13);
          osc2.start(now + 0.07);
          osc2.stop(now + 0.13);
        }
        break;
        
      case SOUND_TYPES.RECEIVE:
        // "ti dum" - agudo depois grave
        {
          const { oscillator: osc1, gainNode: gain1 } = createTone(ctx);
          osc1.frequency.setValueAtTime(660, now);
          gain1.gain.setValueAtTime(0.25, now);
          gain1.gain.linearRampToValueAtTime(0, now + 0.06);
          osc1.start(now);
          osc1.stop(now + 0.06);
          
          const { oscillator: osc2, gainNode: gain2 } = createTone(ctx);
          osc2.frequency.setValueAtTime(330, now + 0.07);
          gain2.gain.setValueAtTime(0, now);
          gain2.gain.setValueAtTime(0.25, now + 0.07);
          gain2.gain.linearRampToValueAtTime(0, now + 0.13);
          osc2.start(now + 0.07);
          osc2.stop(now + 0.13);
        }
        break;
        
      // === STATUS ===
      
      case SOUND_TYPES.SUCCESS:
        // Tom ascendente suave
        {
          const { oscillator, gainNode } = createTone(ctx);
          oscillator.frequency.setValueAtTime(440, now);
          oscillator.frequency.setValueAtTime(880, now + 0.1);
          gainNode.gain.setValueAtTime(0.2, now);
          gainNode.gain.linearRampToValueAtTime(0, now + 0.15);
          oscillator.start(now);
          oscillator.stop(now + 0.15);
        }
        break;
        
      case SOUND_TYPES.ERROR:
        // Tom grave longo
        {
          const { oscillator, gainNode } = createTone(ctx);
          oscillator.frequency.setValueAtTime(200, now);
          gainNode.gain.setValueAtTime(0.3, now);
          gainNode.gain.linearRampToValueAtTime(0, now + 0.3);
          oscillator.start(now);
          oscillator.stop(now + 0.3);
        }
        break;
        
      case SOUND_TYPES.CLEAR:
        // Tom suave descendente
        {
          const { oscillator, gainNode } = createTone(ctx);
          oscillator.frequency.setValueAtTime(520, now);
          oscillator.frequency.linearRampToValueAtTime(400, now + 0.1);
          gainNode.gain.setValueAtTime(0.15, now);
          gainNode.gain.linearRampToValueAtTime(0, now + 0.12);
          oscillator.start(now);
          oscillator.stop(now + 0.12);
        }
        break;
        
      // === GRAVAÇÃO ===
      
      case SOUND_TYPES.RECORD_START:
        // Bip ascendente
        {
          const { oscillator, gainNode } = createTone(ctx);
          oscillator.frequency.setValueAtTime(440, now);
          oscillator.frequency.setValueAtTime(880, now + 0.1);
          gainNode.gain.setValueAtTime(0.2, now);
          gainNode.gain.linearRampToValueAtTime(0, now + 0.15);
          oscillator.start(now);
          oscillator.stop(now + 0.15);
        }
        break;
        
      case SOUND_TYPES.RECORD_END:
        // Bip descendente
        {
          const { oscillator, gainNode } = createTone(ctx);
          oscillator.frequency.setValueAtTime(660, now);
          oscillator.frequency.linearRampToValueAtTime(440, now + 0.1);
          gainNode.gain.setValueAtTime(0.2, now);
          gainNode.gain.linearRampToValueAtTime(0, now + 0.15);
          oscillator.start(now);
          oscillator.stop(now + 0.15);
        }
        break;
        
      case SOUND_TYPES.LISTENING:
        // Três tons ascendentes rápidos
        {
          const { oscillator, gainNode } = createTone(ctx);
          oscillator.frequency.setValueAtTime(330, now);
          oscillator.frequency.setValueAtTime(440, now + 0.05);
          oscillator.frequency.setValueAtTime(550, now + 0.1);
          gainNode.gain.setValueAtTime(0.15, now);
          gainNode.gain.linearRampToValueAtTime(0, now + 0.2);
          oscillator.start(now);
          oscillator.stop(now + 0.2);
        }
        break;
        
      // === NAVEGAÇÃO ===
      
      case SOUND_TYPES.FOCUS:
        // Clique suave
        {
          const { oscillator, gainNode } = createTone(ctx);
          oscillator.frequency.setValueAtTime(800, now);
          gainNode.gain.setValueAtTime(0.1, now);
          gainNode.gain.linearRampToValueAtTime(0, now + 0.05);
          oscillator.start(now);
          oscillator.stop(now + 0.05);
        }
        break;
        
      case SOUND_TYPES.BOUNDARY:
        // Tom de limite
        {
          const { oscillator, gainNode } = createTone(ctx);
          oscillator.frequency.setValueAtTime(300, now);
          gainNode.gain.setValueAtTime(0.15, now);
          gainNode.gain.linearRampToValueAtTime(0, now + 0.1);
          oscillator.start(now);
          oscillator.stop(now + 0.1);
        }
        break;
        
      case SOUND_TYPES.BUMP:
        // Som de marimba (dual sine oscillators)
        {
          const duration = 0.12; // 120ms para ressonância de marimba

          // Frequência fundamental (400Hz)
          const { oscillator: osc1, gainNode: gain1 } = createTone(ctx);
          osc1.type = 'sine';
          osc1.frequency.setValueAtTime(400, now);
          gain1.gain.setValueAtTime(0, now);
          gain1.gain.linearRampToValueAtTime(0.3, now + 0.01); // Attack de 10ms
          gain1.gain.exponentialRampToValueAtTime(0.01, now + duration);
          osc1.start(now);
          osc1.stop(now + duration);

          // Harmônico (oitava acima - 800Hz)
          const { oscillator: osc2, gainNode: gain2 } = createTone(ctx);
          osc2.type = 'sine';
          osc2.frequency.setValueAtTime(800, now);
          gain2.gain.setValueAtTime(0, now);
          gain2.gain.linearRampToValueAtTime(0.15, now + 0.01); // Harmônico mais suave
          gain2.gain.exponentialRampToValueAtTime(0.01, now + duration);
          osc2.start(now);
          osc2.stop(now + duration);
        }
        break;
        
      default:
        console.warn(`Unknown sound type: ${type}`);
    }
  } catch (error) {
    console.error('Error playing sound:', error);
  }
}

/**
 * Resume o AudioContext se estiver suspenso
 * Necessário em alguns navegadores após interação do usuário
 */
export async function resumeAudioContext(): Promise<void> {
  const ctx = getAudioContext();
  if (ctx.state === 'suspended') {
    await ctx.resume();
  }
}

/**
 * Funções auxiliares para sons específicos
 */
export const playSendSound = () => playSound(SOUND_TYPES.SEND);
export const playReceiveSound = () => playSound(SOUND_TYPES.RECEIVE);
export const playSuccessSound = () => playSound(SOUND_TYPES.SUCCESS);
export const playErrorSound = () => playSound(SOUND_TYPES.ERROR);
export const playClearSound = () => playSound(SOUND_TYPES.CLEAR);
export const playRecordStartSound = () => playSound(SOUND_TYPES.RECORD_START);
export const playRecordEndSound = () => playSound(SOUND_TYPES.RECORD_END);
export const playListeningSound = () => playSound(SOUND_TYPES.LISTENING);
export const playFocusSound = () => playSound(SOUND_TYPES.FOCUS);
export const playBoundarySound = () => playSound(SOUND_TYPES.BOUNDARY);
export const playBumpSound = () => playSound(SOUND_TYPES.BUMP);
