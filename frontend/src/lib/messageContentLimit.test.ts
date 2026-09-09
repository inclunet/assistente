import { describe, expect, it } from 'vitest';

import {
  getUtf8ByteLength,
  MAX_MESSAGE_CONTENT_BYTES,
  MAX_MESSAGE_CONTENT_KIB,
} from './messageContentLimit';

describe('limite de conteúdo de mensagem em UTF-8', () => {
  it('mantém o espelho frontend em 512 KiB', () => {
    expect(MAX_MESSAGE_CONTENT_BYTES).toBe(512 * 1024);
    expect(MAX_MESSAGE_CONTENT_KIB).toBe(512);
  });

  it.each([
    ['vazia', '', 0],
    ['ASCII', 'abc\r\n', 5],
    ['emoji em surrogate pair', '😀', 4],
    ['caractere combinante', 'e\u0301', 3],
    ['emoji composto', '👩‍💻', 11],
    ['surrogate isolado', '\ud800', 3],
  ])('conta %s pelos bytes codificados', (_name, content, expected) => {
    expect(getUtf8ByteLength(content)).toBe(expected);
  });

  it('distingue emojis abaixo e acima do limite', () => {
    expect(getUtf8ByteLength('😀'.repeat(MAX_MESSAGE_CONTENT_BYTES / 4))).toBe(MAX_MESSAGE_CONTENT_BYTES);
    expect(getUtf8ByteLength(`${'😀'.repeat(MAX_MESSAGE_CONTENT_BYTES / 4)}a`)).toBe(MAX_MESSAGE_CONTENT_BYTES + 1);
  });
});
