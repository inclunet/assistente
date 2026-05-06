/** @vitest-environment jsdom */
import { describe, it, expect, vi, beforeEach } from 'vitest';
import { act, renderHook } from '@testing-library/react';

import { useEditorInlineChatPatch } from './useEditorInlineChatPatch';

const conversationId = "01926b90-7a5a-7c4e-8d3f-000000000042";
let messages: Array<{ role?: string; toolCalls?: unknown; toolCallId?: string; content?: string }>;
const getConversationMessagesMock = vi.fn(() => messages);

let eventHandler: ((data: unknown) => void) | null = null;
const eventsOnMock = vi.fn((eventName: string, handler: (data: unknown) => void) => {
  if (eventName === 'chat:done') {
    eventHandler = handler;
  }
  return () => {};
});

vi.mock('../store/chatStore', () => ({
  useChatStore: {
    getState: () => ({ getConversationMessages: getConversationMessagesMock }),
  },
}));

vi.mock('@wailsjs/runtime/runtime', () => ({
  EventsOn: (eventName: string, handler: (data: unknown) => void) => eventsOnMock(eventName, handler),
}));

describe('useEditorInlineChatPatch', () => {
  beforeEach(() => {
    messages = [];
    getConversationMessagesMock.mockClear();
    eventsOnMock.mockClear();
    eventHandler = null;
  });

  it('retorna erro quando não há patch no corpo', async () => {
    messages = [
      {
        role: 'assistant',
        content: 'Não vou alterar nada.',
      },
    ];

    const { result } = renderHook(() => useEditorInlineChatPatch());

    const found = await result.current.waitForEditorPatch({ conversationId, timeoutMs: 200 });

    expect(found.ok).toBe(false);
  });

  it('usa fallback do corpo quando permitido', async () => {
    messages = [
      {
        role: 'assistant',
        content: '```editor_patch\n{"v":1,"op":"replace_selection","format":"plain","replacement":"texto"}\n```',
      },
    ];

    const { result } = renderHook(() => useEditorInlineChatPatch());

    const found = await result.current.waitForEditorPatch({ conversationId });

    expect(found.ok).toBe(true);
    if (found.ok) {
      expect(found.patch.replacement).toBe('texto');
      expect(found.source).toBe('body');
    }
  });

  it('resolve waitForChatDone quando evento chega', async () => {
    const { result } = renderHook(() => useEditorInlineChatPatch());

    const promise = result.current.waitForChatDone(conversationId, 1000);

    act(() => {
      eventHandler?.({ conversationId });
    });

    await expect(promise).resolves.toBe(conversationId);
  });
});
