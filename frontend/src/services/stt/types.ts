/**
 * Speech-to-Text (STT) Service - Tipos
 * 
 * Tipos e interfaces para o serviço unificado de transcrição de voz.
 */

/**
 * Providers de STT disponíveis
 */
export const STT_PROVIDERS = {
  WEBSPEECH: 'webspeech',
  WHISPER_API: 'whisper_api',
  VOSK: 'vosk',
} as const;

export type STTProvider = typeof STT_PROVIDERS[keyof typeof STT_PROVIDERS];

/**
 * Modos de gravação
 */
export const RECORDING_MODES = {
  /** Push-to-Talk: segura para gravar */
  PTT: 'ptt',
  /** Toggle: clique para iniciar/parar */
  TOGGLE: 'toggle',
  /** VAD Silence: clique + detecta silêncio para parar */
  VAD_SILENCE: 'vad_silence',
  /** VAD Activity: detecta início e fim automaticamente */
  VAD_ACTIVITY: 'vad_activity',
  /** Record Audio: grava áudio como arquivo (não transcreve) */
  RECORD_AUDIO: 'record_audio',
} as const;

export type RecordingMode = typeof RECORDING_MODES[keyof typeof RECORDING_MODES];

/**
 * Estados do STT
 */
export const STT_STATES = {
  /** Pronto para gravar */
  IDLE: 'idle',
  /** Esperando atividade de voz (VAD Activity mode) */
  LISTENING: 'listening',
  /** Gravando */
  RECORDING: 'recording',
  /** Processando transcrição */
  PROCESSING: 'processing',
  /** Erro */
  ERROR: 'error',
} as const;

export type STTState = typeof STT_STATES[keyof typeof STT_STATES];

/**
 * Configuração do serviço STT
 */
export interface STTConfig {
  /** Provider de STT */
  provider: STTProvider;
  
  /** Modo de gravação */
  mode: RecordingMode;
  
  /** Idioma (ex: 'pt-BR') */
  language: string;
  
  /** Duração de silêncio para VAD (ms) */
  silenceDuration: number;
  
  /** Threshold de silêncio para VAD (0-1) */
  silenceThreshold: number;
  
  /** Threshold de atividade para VAD (0-1) */
  activityThreshold: number;
  
  /** Duração de atividade para VAD (ms) */
  activityDuration: number;
}

/**
 * Configuração padrão
 */
export const DEFAULT_STT_CONFIG: STTConfig = {
  provider: STT_PROVIDERS.WEBSPEECH,
  mode: RECORDING_MODES.PTT,
  language: 'pt-BR',
  silenceDuration: 1500,
  silenceThreshold: 0.01,
  activityThreshold: 0.02,
  activityDuration: 200,
};

/**
 * Callbacks do serviço STT
 */
export interface STTCallbacks {
  /** Chamado quando a transcrição é finalizada */
  onTranscription?: (text: string, provider: STTProvider) => void;
  
  /** Chamado durante a transcrição (resultados parciais) */
  onInterimResult?: (text: string) => void;
  
  /** Chamado quando o estado muda */
  onStateChange?: (state: STTState, previousState: STTState, message?: string) => void;
  
  /** Chamado quando o volume muda (0-1) */
  onVolumeChange?: (volume: number) => void;
  
  /** Chamado quando atividade de voz é detectada */
  onActivityStart?: () => void;
  
  /** Chamado quando atividade de voz termina */
  onActivityEnd?: () => void;
  
  /** Chamado quando há erro */
  onError?: (message: string, code?: string) => void;
  
  /** Chamado quando nenhuma fala é detectada */
  onNoSpeechDetected?: () => void;
  
  /** Chamado quando um arquivo de áudio é gravado (modo RECORD_AUDIO) */
  onAudioFile?: (file: File, blob: Blob) => void;
  
  /** Chamado quando a gravação é cancelada */
  onRecordingCancelled?: () => void;
}

/**
 * Opções completas do STT (config + callbacks)
 */
export type STTOptions = Partial<STTConfig> & STTCallbacks;

/**
 * Evento de transcrição
 */
export interface TranscriptionEvent {
  text: string;
  isFinal: boolean;
  provider: STTProvider;
}

/**
 * Interface para providers de STT
 */
export interface ISTTProvider {
  /** Nome do provider */
  readonly name: STTProvider;
  
  /** Se está suportado no ambiente atual */
  readonly isSupported: boolean;
  
  /** Inicializa o provider */
  init(): Promise<boolean>;
  
  /** Inicia a transcrição */
  start(): boolean;
  
  /** Para a transcrição */
  stop(): void;
  
  /** Aborta sem processar */
  abort(): void;
  
  /** Define o idioma */
  setLanguage(language: string): void;
  
  /** Libera recursos */
  destroy(): void;
}

/**
 * Opções do WebSpeech Provider
 */
export interface WebSpeechProviderOptions {
  language?: string;
  continuous?: boolean;
  interimResults?: boolean;
  maxAlternatives?: number;
  onStart?: () => void;
  onEnd?: (transcript: string) => void;
  onResult?: (transcript: string) => void;
  onInterim?: (text: string) => void;
  onError?: (message: string, code: string) => void;
}

/**
 * Opções do Audio Recorder
 */
export interface AudioRecorderOptions {
  mimeType?: string;
  audioBitsPerSecond?: number;
  onStart?: () => void;
  onStop?: (blob: Blob) => void;
  onData?: (data: Blob) => void;
  onError?: (error: Error | string) => void;
}
