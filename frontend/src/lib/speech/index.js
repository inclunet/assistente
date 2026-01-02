/**
 * Speech Module - Centraliza funcionalidades de voz
 * 
 * Serviços de alto nível (recomendados):
 *   - ttsService: Serviço unificado de TTS (WebSpeech, SAPI5, OpenAI)
 *   - sttService: Serviço unificado de STT (WebSpeech, Whisper)
 * 
 * Classes de baixo nível (para casos específicos):
 *   - SpeechRecognitionManager: WebSpeech STT
 *   - SpeechSynthesisManager: WebSpeech TTS
 *   - AudioRecorder: Gravação de áudio
 *   - VoiceActivityDetector: Detecção de atividade de voz
 */

// Serviços de alto nível (singleton)
export { ttsService, TTSService, TTS_PROVIDERS } from './tts-service.js';
export { sttService, STTService, STT_PROVIDERS, RECORDING_MODES, STT_STATES } from './stt-service.js';

// Classes de baixo nível
export { SpeechRecognitionManager } from './stt-webspeech.js';
export { SpeechSynthesisManager } from './tts-webspeech.js';
export { AudioRecorder } from './audio-recorder.js';
export { VoiceActivityDetector } from './vad.js';

