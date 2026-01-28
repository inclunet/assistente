/**
 * Audio Feedback Service - Sons de feedback para acessibilidade
 * 
 * Fornece sons distintivos para diferentes ações na interface,
 * ajudando usuários a entenderem o que está acontecendo.
 * 
 * Usa Web Audio API para gerar sons sintéticos sem arquivos externos.
 */

let audioContext = null;

/**
 * Obtém ou cria o AudioContext compartilhado
 */
function getAudioContext() {
  if (!audioContext) {
    audioContext = new (window.AudioContext || window.webkitAudioContext)();
  }
  return audioContext;
}

/**
 * Cria um oscilador com gain conectado
 */
function createTone(ctx, startTime = ctx.currentTime) {
  const oscillator = ctx.createOscillator();
  const gainNode = ctx.createGain();
  oscillator.connect(gainNode);
  gainNode.connect(ctx.destination);
  return { oscillator, gainNode };
}

/**
 * Tipos de sons disponíveis
 */
export const SOUND_TYPES = {
  // Chat
  SEND: 'send',           // Mensagem enviada: "tum di" (grave → agudo)
  RECEIVE: 'receive',     // Mensagem recebida: "ti dum" (agudo → grave)
  
  // Status
  SUCCESS: 'success',     // Ação concluída com sucesso
  ERROR: 'error',         // Erro
  CLEAR: 'clear',         // Limpar/nova conversa
  
  // Gravação de voz
  RECORD_START: 'start',  // Início de gravação
  RECORD_END: 'end',      // Fim de gravação
  LISTENING: 'listening', // Modo VAD ativo
  
  // Navegação
  FOCUS: 'focus',         // Foco em elemento
  BOUNDARY: 'boundary',   // Limite de navegação
};

/**
 * Reproduz um som de feedback
 * @param {string} type - Tipo de som (use SOUND_TYPES)
 */
export function playSound(type) {
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
      case 'start':
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
      case 'end':
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
      case 'listening':
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
          gainNode.gain.linearRampToValueAtTime(0, now + 0.03);
          oscillator.start(now);
          oscillator.stop(now + 0.03);
        }
        break;
        
      case SOUND_TYPES.BOUNDARY:
        // Dois tons curtos (limite)
        {
          const { oscillator, gainNode } = createTone(ctx);
          oscillator.frequency.setValueAtTime(300, now);
          gainNode.gain.setValueAtTime(0.15, now);
          gainNode.gain.linearRampToValueAtTime(0, now + 0.05);
          oscillator.start(now);
          oscillator.stop(now + 0.05);
          
          const { oscillator: osc2, gainNode: gain2 } = createTone(ctx);
          osc2.frequency.setValueAtTime(300, now + 0.08);
          gain2.gain.setValueAtTime(0, now);
          gain2.gain.setValueAtTime(0.15, now + 0.08);
          gain2.gain.linearRampToValueAtTime(0, now + 0.13);
          osc2.start(now + 0.08);
          osc2.stop(now + 0.13);
        }
        break;
        
      default:
        // Fallback: bip simples
        {
          const { oscillator, gainNode } = createTone(ctx);
          oscillator.frequency.setValueAtTime(440, now);
          gainNode.gain.setValueAtTime(0.15, now);
          gainNode.gain.linearRampToValueAtTime(0, now + 0.1);
          oscillator.start(now);
          oscillator.stop(now + 0.1);
        }
    }
  } catch (e) {
    // Ignora erros de áudio silenciosamente
    // Pode acontecer se o usuário não interagiu com a página ainda
  }
}

/**
 * Atalhos convenientes para os sons mais usados
 */
export const sounds = {
  send: () => playSound(SOUND_TYPES.SEND),
  receive: () => playSound(SOUND_TYPES.RECEIVE),
  success: () => playSound(SOUND_TYPES.SUCCESS),
  error: () => playSound(SOUND_TYPES.ERROR),
  clear: () => playSound(SOUND_TYPES.CLEAR),
  recordStart: () => playSound(SOUND_TYPES.RECORD_START),
  recordEnd: () => playSound(SOUND_TYPES.RECORD_END),
  listening: () => playSound(SOUND_TYPES.LISTENING),
};

/**
 * Fecha o AudioContext (libera recursos)
 * Chame quando o componente for destruído se necessário
 */
export function dispose() {
  if (audioContext) {
    audioContext.close();
    audioContext = null;
  }
}

/**
 * Verifica se Web Audio API é suportada
 */
export function isSupported() {
  return !!(window.AudioContext || window.webkitAudioContext);
}


