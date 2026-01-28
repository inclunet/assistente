/**
 * Voice Activity Detection (VAD) - Tipos
 * 
 * Tipos e interfaces para o serviço de detecção de atividade de voz.
 */

/**
 * Configuração do VAD
 */
export interface VADConfig {
  /** Limite de volume para considerar silêncio (0-1, default: 0.01) */
  silenceThreshold: number;
  
  /** Duração de silêncio para considerar fim de fala (ms, default: 1500) */
  silenceDuration: number;
  
  /** Limite de volume para considerar atividade (0-1, default: 0.02) */
  activityThreshold: number;
  
  /** Duração mínima de atividade para considerar início de fala (ms, default: 200) */
  activityDuration: number;
  
  /** Intervalo de verificação (ms, default: 100) */
  checkInterval: number;
}

/**
 * Callbacks do VAD
 */
export interface VADCallbacks {
  /** Chamado quando silêncio é detectado (início do silêncio) */
  onSilenceStart?: () => void;
  
  /** Chamado quando silêncio termina (usuário voltou a falar) */
  onSilenceEnd?: () => void;
  
  /** Chamado quando atividade de voz é detectada (usuário começou a falar) */
  onActivityStart?: () => void;
  
  /** Chamado quando atividade de voz termina (usuário parou de falar) */
  onActivityEnd?: () => void;
  
  /** Chamado a cada verificação com o volume atual (0-1) */
  onVolumeChange?: (volume: number) => void;
}

/**
 * Opções completas do VAD (config + callbacks)
 */
export type VADOptions = Partial<VADConfig> & VADCallbacks;

/**
 * Estado do VAD
 */
export interface VADState {
  /** Se o VAD está ativo (rodando) */
  isActive: boolean;
  
  /** Se o usuário está falando */
  isSpeaking: boolean;
  
  /** Volume atual (0-1) */
  currentVolume: number;
}

/**
 * Valores padrão da configuração
 */
export const DEFAULT_VAD_CONFIG: VADConfig = {
  silenceThreshold: 0.01,
  silenceDuration: 1500,
  activityThreshold: 0.02,
  activityDuration: 200,
  checkInterval: 100,
};
