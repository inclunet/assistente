import type { TFunction } from 'i18next';

/** Motivos de aviso de turno que a interface sabe traduzir (backend: ChatNoticeKind*). */
export const CHAT_NOTICE_ATTACHMENTS_NOT_SENT = 'attachments_not_sent';

const KIND_KEYS: Record<string, string> = {
  [CHAT_NOTICE_ATTACHMENTS_NOT_SENT]: 'app.chatNotice.attachmentsNotSent',
};

export interface ChatNoticeEvent {
  conversationId?: string;
  kind?: string;
  count?: number;
}

/**
 * Texto do aviso sobre o turno. O backend manda o motivo como código, então a
 * frase sai no idioma de quem lê. Motivo que a interface não conhece não vira
 * aviso nenhum: um código cru na tela não diz nada a ninguém, e é melhor
 * silêncio aqui do que ruído sem sentido.
 */
export function chatNoticeMessage(t: TFunction, event: ChatNoticeEvent): string | null {
  const key = event.kind ? KIND_KEYS[event.kind] : undefined;
  if (!key) {
    return null;
  }
  return t(key, { count: event.count ?? 0 });
}
