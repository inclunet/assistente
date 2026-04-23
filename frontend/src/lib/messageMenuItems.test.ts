import { afterAll, beforeAll, describe, expect, it, vi } from 'vitest';
import { getMessageMenuItems } from './messageMenuItems';
import type { Message } from '../store/chatStore';
import { main } from '../../wailsjs/go/models';
import i18n from 'i18next';

vi.mock('../services/messageAudio', () => ({
  messageAudioService: {
    getMessageAudioBlob: vi.fn(),
    downloadAudioBlob: vi.fn(),
    base64ToBlob: vi.fn(),
  },
}));

vi.mock('../services/tts', () => ({
  ttsService: {
    getVoiceContext: vi.fn(() => ({
      providerId: 'test-provider',
      voiceId: 'test-voice',
      model: 'tts-1',
      rate: 1.0,
    })),
  },
}));

describe('messageMenuItems', () => {
  const originalTranslationBundle = structuredClone(i18n.getResourceBundle('en', 'translation') || {});

  beforeAll(() => {
    i18n.addResourceBundle('en', 'translation', {
      editor: {
        sendToEditor: {
          format: {
            markdown: 'Markdown',
            plainText: 'Text',
          },
        },
      },
    }, true, true);
  });

  afterAll(() => {
    i18n.removeResourceBundle('en', 'translation');
    i18n.addResourceBundle('en', 'translation', originalTranslationBundle, true, true);
  });

  it('inclui itens basicos e markdown', () => {
    const assistantMessage = new main.EnrichedMessage({
      id: '1',
      conversationId: 1,
      role: 'assistant',
      content:
        'Texto\n\n```js\nconsole.log(1)\n```\n\n[Link](http://x)\n\n|A|B|\n|---|---|\n|1|2|\n',
      createdAt: new Date().toISOString(),
      timestamp: Date.now(),
      isStreaming: false,
      internal: false,
    }) as Message;

    const items = getMessageMenuItems(
      assistantMessage,
      {
        isTTSDisabled: true,
        onCopy: vi.fn(),
      }
    );

    expect(items.some((item) => item.id === 'copy')).toBe(true);
    expect(items.some((item) => item.id === 'copy-md')).toBe(true);
    expect(items.some((item) => item.id === 'code')).toBe(true);
    expect(items.some((item) => item.id === 'links')).toBe(true);
    expect(items.some((item) => item.id === 'table-copy')).toBe(true);
  });

  it('inclui itens de usuario', () => {
    const userMessage = new main.EnrichedMessage({
      id: '2',
      conversationId: 1,
      role: 'user',
      content: 'Oi',
      createdAt: new Date().toISOString(),
      timestamp: Date.now(),
      isStreaming: false,
      internal: false,
    }) as Message;

    const items = getMessageMenuItems(
      userMessage,
      {
        isUser: true,
        isTTSDisabled: true,
        onEdit: vi.fn(),
        onResend: vi.fn(),
        onDelete: vi.fn(),
      }
    );

    expect(items.some((item) => item.id === 'edit')).toBe(true);
    expect(items.some((item) => item.id === 'resend')).toBe(true);
    expect(items.some((item) => item.id === 'delete')).toBe(true);
  });

  it('inclui envio para editor quando configurado', () => {
    const assistantMessage = new main.EnrichedMessage({
      id: '3',
      conversationId: 1,
      role: 'assistant',
      content: 'Oi',
      createdAt: new Date().toISOString(),
      timestamp: Date.now(),
      isStreaming: false,
      internal: false,
    }) as Message;

    const items = getMessageMenuItems(
      assistantMessage,
      {
        isTTSDisabled: true,
        onSendToEditor: vi.fn(),
        editorTargets: [
          { id: 'tab-1', title: 'README.md' },
          { id: 'tab-2', title: 'Notas' },
        ],
      }
    );

    const sendEditor = items.find((item) => item.id === 'send-editor');
    expect(sendEditor).toBeTruthy();
    expect(sendEditor?.submenu?.map((item) => item.label)).toContain('README.md');
    expect(sendEditor?.submenu?.map((item) => item.label)).toContain('Notas');
    expect(sendEditor?.submenu?.[0]?.submenu?.map((item) => item.label)).toContain('Markdown');
    expect(sendEditor?.submenu?.[0]?.submenu?.map((item) => item.label)).toContain('Text');
  });
});
