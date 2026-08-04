import type { TFunction } from 'i18next';

import { agentActionKey } from './agentAction';

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
 * Nome da ação envolvida, na forma que cabe dentro da frase do aviso ("pediu
 * permissão para executar um comando").
 */
function actionName(t: TFunction, action?: string): string {
  return t(`app.chatNotice.action.${agentActionKey(action)}`);
}
