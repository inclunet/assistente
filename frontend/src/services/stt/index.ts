/**
 * Speech-to-Text (STT) Service
 * 
 * Serviço unificado de transcrição de voz com suporte a múltiplos providers
 * e modos de gravação.
 * 
 * @example
 * ```typescript
 * import { 
 *   STTService, 
 *   STT_PROVIDERS, 
 *   RECORDING_MODES, 
 *   STT_STATES 
 * } from '@/services/stt';
 * 
 * const stt = new STTService({
 *   provider: STT_PROVIDERS.WEBSPEECH,
 *   mode: RECORDING_MODES.VAD_SILENCE,
 *   silenceDuration: 1500,
 *   onTranscription: (text, provider) => {
 *     console.log(`[${provider}] ${text}`);
 *   },
 *   onStateChange: (state) => {
 *     console.log('Estado:', state);
 *   },
 * });
 * 
 * await stt.init();
 * await stt.startRecording();
 * ```
 */

// Service principal
export { STTService } from './STTService';
export { default } from './STTService';

// Providers
export { WebSpeechProvider } from './providers/WebSpeechProvider';
export { WhisperProvider } from './providers/WhisperProvider';

// Audio Recorder
export { AudioRecorder } from './AudioRecorder';

// Tipos e constantes
export {
  // Constantes
  STT_PROVIDERS,
  RECORDING_MODES,
  STT_STATES,
  DEFAULT_STT_CONFIG,
  
  // Tipos
  type STTProvider,
  type RecordingMode,
  type STTState,
  type STTConfig,
  type STTCallbacks,
  type STTOptions,
  type TranscriptionEvent,
  type ISTTProvider,
  type WebSpeechProviderOptions,
  type AudioRecorderOptions,
} from './types';
