/**
 * Voice Activity Detection (VAD) Service
 * 
 * Exporta todos os componentes do serviço VAD.
 * 
 * @example
 * ```typescript
 * import { VoiceActivityDetector, DEFAULT_VAD_CONFIG } from '@/services/vad';
 * 
 * const vad = new VoiceActivityDetector({
 *   silenceDuration: 2000,
 *   onActivityStart: () => console.log('Usuário começou a falar'),
 *   onActivityEnd: () => console.log('Usuário parou de falar'),
 * });
 * 
 * await vad.init();
 * vad.start();
 * ```
 */

export { VoiceActivityDetector } from './VoiceActivityDetector';
export { default } from './VoiceActivityDetector';

export type {
  VADConfig,
  VADCallbacks,
  VADOptions,
  VADState,
} from './types';

export { DEFAULT_VAD_CONFIG } from './types';
