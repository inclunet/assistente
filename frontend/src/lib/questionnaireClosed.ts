import type { TFunction } from 'i18next';

/** Evento do backend que tira da tela uma pergunta que perdeu o dono. */
export const QUESTIONNAIRE_CLOSED_EVENT = 'tool:questionnaire:closed';

/** Motivos de fechamento que o backend nomeia (questionnaire.Closed*). */
export const QUESTIONNAIRE_CLOSED_CANCELLED = 'cancelled';
export const QUESTIONNAIRE_CLOSED_TIMEOUT = 'timeout';

const REASON_KEYS: Record<string, string> = {
  [QUESTIONNAIRE_CLOSED_CANCELLED]: 'app.questionnaire.closedCancelled',
  [QUESTIONNAIRE_CLOSED_TIMEOUT]: 'app.questionnaire.closedTimeout',
};

export interface QuestionnaireClosedEvent {
  id?: string;
  reason?: string;
}

/**
 * Texto que explica por que o diálogo sumiu. O diálogo fechar sozinho, sem
 * dizer nada, é pior do que ficar aberto para quem depende de leitor de telas:
 * o foco volta para a conversa sem explicação nenhuma. Motivo desconhecido cai
 * no aviso genérico, porque o sumiço em si sempre precisa ser dito.
 */
export function questionnaireClosedMessage(t: TFunction, event: QuestionnaireClosedEvent): string {
  const key = (event.reason && REASON_KEYS[event.reason]) || 'app.questionnaire.closed';
  return t(key);
}
