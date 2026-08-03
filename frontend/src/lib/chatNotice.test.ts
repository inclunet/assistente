import { describe, it, expect } from 'vitest';
import type { TFunction } from 'i18next';

import {
  chatNoticeMessage,
  CHAT_NOTICE_ATTACHMENTS_NOT_SENT,
  CHAT_NOTICE_PERMISSION_NO_WATCHER,
  CHAT_NOTICE_PERMISSION_TIMEOUT,
  CHAT_NOTICE_PERMISSION_UNAVAILABLE,
} from './chatNotice';

// Mock de TFunction: ecoa a chave + os args interpolados.
const t = ((key: string, opts?: Record<string, unknown>) =>
  opts ? `${key}|${JSON.stringify(opts)}` : key) as unknown as TFunction;

describe('chatNoticeMessage', () => {
  it('traduz o motivo nomeado pelo backend, com a quantidade', () => {
    const message = chatNoticeMessage(t, {
      conversationId: 'conversa-1',
      kind: CHAT_NOTICE_ATTACHMENTS_NOT_SENT,
      count: 2,
    });

    expect(message).toContain('app.chatNotice.attachmentsNotSent|');
    expect(message).toContain('"count":2');
  });

  it('não inventa aviso para motivo desconhecido', () => {
    expect(chatNoticeMessage(t, { kind: 'motivo_novo', count: 1 })).toBeNull();
    expect(chatNoticeMessage(t, {})).toBeNull();
  });

  it('diz o que foi negado quando não havia quem respondesse', () => {
    const message = chatNoticeMessage(t, {
      conversationId: 'conversa-1',
      kind: CHAT_NOTICE_PERMISSION_NO_WATCHER,
      action: 'execute',
    });

    expect(message).toContain('app.chatNotice.permissionNoWatcher|');
    expect(message).toContain('"action":"app.chatNotice.action.execute"');
  });

  it('distingue prazo estourado de pedido que não pôde ser mostrado', () => {
    const prazo = chatNoticeMessage(t, { kind: CHAT_NOTICE_PERMISSION_TIMEOUT, action: 'edit' });
    const semTela = chatNoticeMessage(t, {
      kind: CHAT_NOTICE_PERMISSION_UNAVAILABLE,
      action: 'edit',
    });

    expect(prazo).toContain('app.chatNotice.permissionTimeout|');
    expect(semTela).toContain('app.chatNotice.permissionUnavailable|');
  });

  it('classe de ação desconhecida vira o termo genérico, e não código cru', () => {
    const message = chatNoticeMessage(t, {
      kind: CHAT_NOTICE_PERMISSION_TIMEOUT,
      action: 'invocar_o_kraken',
    });

    expect(message).toContain('"action":"app.chatNotice.action.unknown"');
    expect(message).not.toContain('invocar_o_kraken');
  });
});
