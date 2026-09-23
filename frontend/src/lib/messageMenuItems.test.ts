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
import type { MenuItem } from '../components/menu';
import type { ChatSendToEditorPayload } from './editorSendMenu';
import { chat } from '../../wailsjs/go/models';

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
  it('captura origem integral de mensagem, código, tabela e link em ambos destinos', () => {
    const originalContent = '# Fonte\n\n```ts\nconst a = 1;\n```\n\n| A | B |\n| --- | --- |\n| 1 | 2 |\n\n[Site](https://example.com)';
    const message = new chat.EnrichedMessage({ id: 'source-a', role: 'assistant', content: originalContent });
    const onSendToEditor = vi.fn<(payload: ChatSendToEditorPayload & { kind: string }) => void>();
    const items = getMessageMenuItems(message, { onSendToEditor, editorTargets: [{ id: 'doc-a', title: 'A' }] });
    message.id = 'source-b';
    message.content = 'alterado após abertura';
    const visit = (entries: MenuItem[]) => entries.forEach(item => {
      if (item.submenu) visit(item.submenu);
      else if (item.id && /^send-(editor|code|table|link)-/.test(item.id)) item.action?.();
    });
    visit(items);
    const payloads = onSendToEditor.mock.calls.map(([payload]) => payload);
    expect(payloads.length).toBeGreaterThanOrEqual(12);
    expect(new Set(payloads.map(payload => payload.kind))).toEqual(new Set(['message', 'code', 'table', 'link']));
    expect(new Set(payloads.map(payload => payload.format))).toEqual(new Set(['markdown', 'plain', 'html']));
    expect(new Set(payloads.map(payload => payload.target))).toEqual(new Set(['document', 'new_document']));
    for (const payload of payloads) {
      expect(payload).toMatchObject({ messageId: 'source-a', originalContent });
      if (payload.target === 'document') expect(payload.targetDocumentId).toBe('doc-a');
      if (payload.kind === 'message' && payload.format === 'markdown') expect(payload.content).toBe(originalContent);
      if (payload.kind === 'code') expect(payload.content).toContain('const a = 1;');
      if (payload.kind === 'link') expect(payload.content).toBe('[Site](https://example.com)');
      if (payload.kind === 'table') expect(payload.content).toContain(payload.format === 'html' ? '<table' : '| A | B |');
    }
  });

  it('oferece fixar ou desafixar apenas para mensagens persistidas', () => {
    const onPin = vi.fn();
    const persisted = new chat.EnrichedMessage({
      id: '01926b90-7a5a-7c4e-8d3f-000000000002',
      conversationId: '01926b90-7a5a-7c4e-8d3f-000000000001',
      role: 'assistant',
      content: 'Resposta',
      pinned: true,
      createdAt: new Date().toISOString(),
    }) as Message;
    const pinItem = getMessageMenuItems(persisted, { onPin }).find((item) => item.id === 'pin');

    expect(pinItem?.label).toBe('chat.unpinMessage');
    pinItem?.action?.();
    expect(onPin).toHaveBeenCalledWith(persisted);

    const synthetic = new chat.EnrichedMessage({
      ...persisted,
      id: 'streaming-conversation',
      pinned: false,
    }) as Message;
    expect(getMessageMenuItems(synthetic, { onPin }).some((item) => item.id === 'pin')).toBe(false);
  });

  it('inclui itens basicos e markdown', () => {
    i18nTMock.mockClear();
    const assistantMessage = new chat.EnrichedMessage({
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
    const userMessage = new chat.EnrichedMessage({
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

  it('inclui "Continuar resposta" quando habilitado', () => {
    const assistantMessage = new chat.EnrichedMessage({
      id: 'a-sintetico',
      conversationId: '01926b90-7a5a-7c4e-8d3f-000000000001',
      role: 'assistant',
      content: 'parcial',
      turnId: 'user-1',
      createdAt: new Date().toISOString(),
      timestamp: Date.now(),
      isStreaming: false,
      internal: false,
    }) as Message;

    const items = getMessageMenuItems(assistantMessage, {
      isTTSDisabled: true,
      onContinue: vi.fn(),
      shouldShowContinue: () => true,
    });

    expect(items.some((item) => item.id === 'continue-response')).toBe(true);

    const hidden = getMessageMenuItems(assistantMessage, {
      isTTSDisabled: true,
      onContinue: vi.fn(),
      shouldShowContinue: () => false,
    });
    expect(hidden.some((item) => item.id === 'continue-response')).toBe(false);
  });

  it('inclui envio para editor quando configurado', () => {
    i18nTMock.mockClear();
    const assistantMessage = new chat.EnrichedMessage({
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
    const assistantMessage = new chat.EnrichedMessage({
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

  it('inclui cancelar geração quando mensagem está em streaming', () => {
    const onCancelStreaming = vi.fn();
    const streamingMessage = new chat.EnrichedMessage({
      id: 'streaming-1',
      conversationId: '01926b90-7a5a-7c4e-8d3f-000000000001',
      role: 'assistant',
      content: 'parcial',
      createdAt: new Date().toISOString(),
      timestamp: Date.now(),
      isStreaming: true,
      internal: false,
    }) as Message;

    const items = getMessageMenuItems(streamingMessage, {
      isTTSDisabled: true,
      onCancelStreaming,
    });

    const cancelItem = items.find((item) => item.id === 'cancel-generation');
    expect(cancelItem).toBeTruthy();
    cancelItem?.action?.();
    expect(onCancelStreaming).toHaveBeenCalledWith(streamingMessage);
  });
});
