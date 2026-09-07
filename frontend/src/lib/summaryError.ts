import type { TFunction } from 'i18next';

/** Motivos de recusa que a interface sabe traduzir (backend: SummaryErrorCode*). */
export const SUMMARY_ERROR_AGENT_PROVIDER = 'agent_provider';

const CODE_KEYS: Record<string, string> = {
  [SUMMARY_ERROR_AGENT_PROVIDER]: 'app.summary.errors.agentProvider',
};

export interface SummaryErrorEvent {
  error?: string;
  code?: string;
}

/**
 * Texto do aviso de resumo não gerado. Quando o backend nomeia o motivo, a
 * mensagem sai no idioma de quem lê; sem código, resta o texto cru do evento
 * (mensagem de erro do provedor, por exemplo).
 */
export function summaryErrorMessage(t: TFunction, event: SummaryErrorEvent): string {
  const key = event.code ? CODE_KEYS[event.code] : undefined;
  if (key) {
    return t(key);
  }
  return t('app.summary.error', { error: event.error || '' });
}
