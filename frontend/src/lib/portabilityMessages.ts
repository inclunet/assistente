import type { TFunction } from 'i18next';

/**
 * Aviso, erro ou motivo de conflito vindo do backend de portabilidade
 * (`portability.LocalizedMessage`): o código diz qual é o caso, os parâmetros o
 * completam e `message` é o texto em português que o backend manda de reserva.
 */
export interface PortabilityMessage {
  code?: string;
  params?: Record<string, string>;
  message?: string;
}

const MESSAGE_KEY_PREFIX = 'portability.messages.';

// Código é identificador do backend, não texto livre: recusar o resto evita
// que um arquivo importado alcance uma chave de tradução qualquer.
const CODE_PATTERN = /^[a-zA-Z][a-zA-Z0-9]*(\.[a-zA-Z][a-zA-Z0-9]*)*$/;

/**
 * Texto do aviso ou erro de importação no idioma de quem lê.
 *
 * Código conhecido vira tradução com os parâmetros interpolados como texto
 * puro. Código que a tradução não conhece — app velho diante de backend novo —
 * cai no texto que o backend mandou junto, para a lista nunca aparecer vazia.
 */
export function formatPortabilityMessage(message: PortabilityMessage | string, t: TFunction): string {
  if (typeof message === 'string') return message;

  const fallback = (message.message ?? '').trim();
  const code = (message.code ?? '').trim();
  if (!code || !CODE_PATTERN.test(code)) return fallback;

  return t(`${MESSAGE_KEY_PREFIX}${code}`, {
    replace: message.params ?? {},
    defaultValue: fallback || code,
  });
}

/** Chave de lista estável para a mensagem, já que o texto pode se repetir. */
export function portabilityMessageKey(message: PortabilityMessage | string, index: number): string {
  if (typeof message === 'string') return `${index}:${message}`;
  return `${index}:${message.code ?? ''}:${message.message ?? ''}`;
}
