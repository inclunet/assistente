import { describe, expect, it, vi } from 'vitest';
const { i18nTMock } = vi.hoisted(() => ({
  i18nTMock: vi.fn((key: string, options?: Record<string, unknown>) => {
    if (key === 'editor.sendToEditor.format.markdown') return 'Markdown';
    if (key === 'editor.sendToEditor.format.plainText') return 'Text';
    if (key === 'editor.sendToEditor.format.html') return 'HTML';
    if (key === 'editor.sendToEditor.title.markdownTableIndexed') return `Table ${options?.index} (Markdown)`;
    if (key === 'editor.sendToEditor.title.htmlTableIndexed') return `Table ${options?.index} (HTML)`;
    return key;
  }),
}));
vi.mock('i18next', () => ({
  default: {
    t: i18nTMock,
  },
}));
import { getMessageMenuItems } from './messageMenuItems';
import type { Message } from '../store/chatStore';
import { main } from '../../wailsjs/go/models';

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
  it('inclui itens basicos e markdown', () => {
    i18nTMock.mockClear();
    const assistantMessage = new main.EnrichedMessage({
      id: '1',
      conversationId: '01926b90-7a5a-7c4e-8d3f-000000000001',
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
    expect(i18nTMock).not.toHaveBeenCalledWith('editor.sendToEditor.action');
  });

  it('inclui itens de usuario', () => {
    i18nTMock.mockClear();
    const userMessage = new main.EnrichedMessage({
      id: '2',
      conversationId: '01926b90-7a5a-7c4e-8d3f-000000000001',
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
    i18nTMock.mockClear();
    const assistantMessage = new main.EnrichedMessage({
      id: '3',
      conversationId: '01926b90-7a5a-7c4e-8d3f-000000000001',
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

  it('usa titulos distintos ao enviar multiplas tabelas para novo documento', () => {
    i18nTMock.mockClear();
    const onSendToEditor = vi.fn();
    const assistantMessage = new main.EnrichedMessage({
      id: '4',
      conversationId: '01926b90-7a5a-7c4e-8d3f-000000000001',
      role: 'assistant',
      content: '|A|B|\n|---|---|\n|1|2|\n\n|C|D|\n|---|---|\n|3|4|\n',
      createdAt: new Date().toISOString(),
      timestamp: Date.now(),
      isStreaming: false,
      internal: false,
    }) as Message;

    const items = getMessageMenuItems(assistantMessage, {
      isTTSDisabled: true,
      onSendToEditor,
    });

    const sendBlocks = items.find((item) => item.id === 'send-blocks-editor');
    const sendTables = sendBlocks?.submenu?.find((item) => item.id === 'send-tables');
    const firstTableNewDocMarkdown = sendTables?.submenu?.[0]?.submenu?.find((item) => item.id === 'send-table-0-new-document')
      ?.submenu?.find((item) => item.label === 'Markdown');
    const secondTableNewDocHtml = sendTables?.submenu?.[1]?.submenu?.find((item) => item.id === 'send-table-1-new-document')
      ?.submenu?.find((item) => item.label === 'HTML');

    firstTableNewDocMarkdown?.action?.();
    secondTableNewDocHtml?.action?.();

    expect(onSendToEditor).toHaveBeenNthCalledWith(1, expect.objectContaining({
      target: 'new_document',
      title: 'Table 1 (Markdown)',
    }));
    expect(onSendToEditor).toHaveBeenNthCalledWith(2, expect.objectContaining({
      target: 'new_document',
      title: 'Table 2 (HTML)',
    }));
  });
});
