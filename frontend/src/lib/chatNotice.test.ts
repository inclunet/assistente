import { describe, it, expect } from 'vitest';
import type { TFunction } from 'i18next';

import { chatNoticeMessage, CHAT_NOTICE_ATTACHMENTS_NOT_SENT } from './chatNotice';

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

    expect(message).toBe('app.chatNotice.attachmentsNotSent|{"count":2}');
  });

  it('não inventa aviso para motivo desconhecido', () => {
    expect(chatNoticeMessage(t, { kind: 'motivo_novo', count: 1 })).toBeNull();
    expect(chatNoticeMessage(t, {})).toBeNull();
  });
});
