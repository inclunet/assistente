import type { TFunction } from 'i18next';

import { agentActionClassName, agentActionKey } from './agentAction';

/** Motivos de aviso de turno que a interface sabe traduzir (backend: ChatNoticeKind*). */
export const CHAT_NOTICE_ATTACHMENTS_NOT_SENT = 'attachments_not_sent';
export const CHAT_NOTICE_PERMISSION_NO_WATCHER = 'permission_denied_no_watcher';
export const CHAT_NOTICE_PERMISSION_TIMEOUT = 'permission_denied_timeout';
export const CHAT_NOTICE_PERMISSION_UNAVAILABLE = 'permission_denied_unavailable';
export const CHAT_NOTICE_PERMISSION_ALWAYS_ALLOWED = 'permission_always_allowed';
export const CHAT_NOTICE_PERMISSION_ALWAYS_NOT_SAVED = 'permission_always_not_saved';
export const CHAT_NOTICE_QUESTION_NO_WATCHER = 'question_skipped_no_watcher';
export const CHAT_NOTICE_QUESTION_TIMEOUT = 'question_skipped_timeout';
export const CHAT_NOTICE_QUESTION_UNAVAILABLE = 'question_skipped_unavailable';
export const CHAT_NOTICE_PLAN_NO_WATCHER = 'plan_rejected_no_watcher';
export const CHAT_NOTICE_PLAN_TIMEOUT = 'plan_rejected_timeout';
export const CHAT_NOTICE_PLAN_UNAVAILABLE = 'plan_rejected_unavailable';
export const CHAT_NOTICE_MODEL_NOT_OFFERED = 'model_not_offered';
export const CHAT_NOTICE_MODEL_NOT_APPLIED = 'model_not_applied';

const KIND_KEYS: Record<string, string> = {
  [CHAT_NOTICE_ATTACHMENTS_NOT_SENT]: 'app.chatNotice.attachmentsNotSent',
  [CHAT_NOTICE_PERMISSION_NO_WATCHER]: 'app.chatNotice.permissionNoWatcher',
  [CHAT_NOTICE_PERMISSION_TIMEOUT]: 'app.chatNotice.permissionTimeout',
  [CHAT_NOTICE_PERMISSION_UNAVAILABLE]: 'app.chatNotice.permissionUnavailable',
  [CHAT_NOTICE_PERMISSION_ALWAYS_ALLOWED]: 'app.chatNotice.permissionAlwaysAllowed',
  [CHAT_NOTICE_PERMISSION_ALWAYS_NOT_SAVED]: 'app.chatNotice.permissionAlwaysNotSaved',
  [CHAT_NOTICE_QUESTION_NO_WATCHER]: 'app.chatNotice.questionNoWatcher',
  [CHAT_NOTICE_QUESTION_TIMEOUT]: 'app.chatNotice.questionTimeout',
  [CHAT_NOTICE_QUESTION_UNAVAILABLE]: 'app.chatNotice.questionUnavailable',
  [CHAT_NOTICE_PLAN_NO_WATCHER]: 'app.chatNotice.planNoWatcher',
  [CHAT_NOTICE_PLAN_TIMEOUT]: 'app.chatNotice.planTimeout',
  [CHAT_NOTICE_PLAN_UNAVAILABLE]: 'app.chatNotice.planUnavailable',
  [CHAT_NOTICE_MODEL_NOT_OFFERED]: 'app.chatNotice.modelNotOffered',
  [CHAT_NOTICE_MODEL_NOT_APPLIED]: 'app.chatNotice.modelNotApplied',
};

/**
 * Avisos que contam algo que deu certo, e não um problema. O tom do toast é
 * lido em voz alta pelo leitor de telas antes da frase; anunciar "aviso" para
 * uma autorização que a própria pessoa acabou de conceder daria alarme onde
 * não houve nenhum.
 */
const INFORMATIVE_KINDS = new Set<string>([CHAT_NOTICE_PERMISSION_ALWAYS_ALLOWED]);

export interface ChatNoticeEvent {
  conversationId?: string;
  kind?: string;
  count?: number;
  action?: string;
  /** Modelo que atendeu ao turno, quando o escolhido não pôde valer. */
  model?: string;
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
  return t(key, {
    count: event.count ?? 0,
    model: event.model ?? '',
    // As duas formas de nomear a mesma classe: dentro da frase e como item de
    // lista. Cada aviso usa a que couber na sua frase, e nenhum precisa saber
    // qual foi a classe para escolher.
    action: actionName(t, event.action),
    actionClass: agentActionClassName(t, event.action),
  });
}

/** Tom do aviso na tela: problema no turno é alerta; o resto é informação. */
export function chatNoticeTone(kind?: string): 'warning' | 'info' {
  return kind && INFORMATIVE_KINDS.has(kind) ? 'info' : 'warning';
}

/**
 * Nome da ação envolvida, na forma que cabe dentro da frase do aviso ("pediu
 * permissão para executar um comando").
 */
function actionName(t: TFunction, action?: string): string {
  return t(`app.chatNotice.action.${agentActionKey(action)}`);
}
