/** @vitest-environment jsdom */
import { describe, it, expect, vi, beforeEach } from 'vitest';
import { act, renderHook } from '@testing-library/react';

import { useEditorInlineChatPatch } from './useEditorInlineChatPatch';

let messages: Array<{ role?: string; toolCalls?: unknown; toolCallId?: string; content?: string }>; 
const getMessagesMock = vi.fn(() => messages);

let eventHandler: ((data: unknown) => void) | null = null;
const eventsOnMock = vi.fn((eventName: string, handler: (data: unknown) => void) => {
  if (eventName === 'chat:done') {
    eventHandler = handler;
  }
  return () => {};
});

vi.mock('../store/chatStore', () => ({
  useChatStore: {
    getState: () => ({ getMessages: getMessagesMock }),
  },
}));

vi.mock('@wailsjs/runtime/runtime', () => ({
  EventsOn: (eventName: string, handler: (data: unknown) => void) => eventsOnMock(eventName, handler),
}));

describe('useEditorInlineChatPatch', () => {
  beforeEach(() => {
    messages = [];
    getMessagesMock.mockClear();
    eventsOnMock.mockClear();
    eventHandler = null;
  });

  it('encontra patch via tool calling', async () => {
    messages = [
      {
        role: 'assistant',
        toolCalls: JSON.stringify([{ function: { name: 'text_edit' }, id: 'call-1' }]),
      },
      {
        role: 'tool',
        toolCallId: 'call-1',
        content: JSON.stringify({ patch: { v: 1, op: 'replace_selection', format: 'markdown', replacement: 'ok' } }),
      },
    ];

    const { result } = renderHook(() => useEditorInlineChatPatch());

    const found = await result.current.waitForEditorPatch();

    expect(found.ok).toBe(true);
    if (found.ok) {
      expect(found.patch.replacement).toBe('ok');
      expect(found.source).toBe('tool');
    }
  });

  it('usa fallback do corpo quando permitido', async () => {
    messages = [
      {
        role: 'assistant',
        content: '```editor_patch\n{"v":1,"op":"replace_selection","format":"plain","replacement":"texto"}\n```',
      },
    ];

    const { result } = renderHook(() => useEditorInlineChatPatch());

    const found = await result.current.waitForEditorPatch({ preferToolCalling: false });

    expect(found.ok).toBe(true);
    if (found.ok) {
      expect(found.patch.replacement).toBe('texto');
      expect(found.source).toBe('body');
    }
  });

  it('resolve waitForChatDone quando evento chega', async () => {
    const { result } = renderHook(() => useEditorInlineChatPatch());

    const promise = result.current.waitForChatDone(42, 1000);

    act(() => {
      eventHandler?.({ conversationId: 42 });
    });

    await expect(promise).resolves.toBe(42);
  });
});
