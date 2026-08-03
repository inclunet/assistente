import type { TFunction } from 'i18next';

/** Motivos de aviso de turno que a interface sabe traduzir (backend: ChatNoticeKind*). */
export const CHAT_NOTICE_ATTACHMENTS_NOT_SENT = 'attachments_not_sent';
export const CHAT_NOTICE_PERMISSION_NO_WATCHER = 'permission_denied_no_watcher';
export const CHAT_NOTICE_PERMISSION_TIMEOUT = 'permission_denied_timeout';
export const CHAT_NOTICE_PERMISSION_UNAVAILABLE = 'permission_denied_unavailable';

const KIND_KEYS: Record<string, string> = {
  [CHAT_NOTICE_ATTACHMENTS_NOT_SENT]: 'app.chatNotice.attachmentsNotSent',
  [CHAT_NOTICE_PERMISSION_NO_WATCHER]: 'app.chatNotice.permissionNoWatcher',
  [CHAT_NOTICE_PERMISSION_TIMEOUT]: 'app.chatNotice.permissionTimeout',
  [CHAT_NOTICE_PERMISSION_UNAVAILABLE]: 'app.chatNotice.permissionUnavailable',
};

/** Classes de ação do ACP que a interface sabe nomear (backend: ToolCall.Kind). */
const ACTION_KEYS: Record<string, string> = {
  read: 'app.chatNotice.action.read',
  edit: 'app.chatNotice.action.edit',
  delete: 'app.chatNotice.action.delete',
  move: 'app.chatNotice.action.move',
  search: 'app.chatNotice.action.search',
  execute: 'app.chatNotice.action.execute',
  think: 'app.chatNotice.action.think',
  fetch: 'app.chatNotice.action.fetch',
  switch_mode: 'app.chatNotice.action.switchMode',
  other: 'app.chatNotice.action.other',
};

export interface ChatNoticeEvent {
  conversationId?: string;
  kind?: string;
  count?: number;
  action?: string;
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
  return t(key, { count: event.count ?? 0, action: actionName(t, event.action) });
}

/**
 * Nome da ação envolvida. Classe que a interface não conhece vira o termo
 * genérico: o aviso continua dizendo o que houve, e um código cru no meio da
 * frase só confundiria quem lê.
 */
function actionName(t: TFunction, action?: string): string {
  const key = action ? ACTION_KEYS[action] : undefined;
  return t(key ?? 'app.chatNotice.action.unknown');
}
