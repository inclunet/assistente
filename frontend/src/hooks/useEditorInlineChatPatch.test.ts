/** @vitest-environment jsdom */
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { act, renderHook } from '@testing-library/react';

import { useEditorInlineChatPatch } from './useEditorInlineChatPatch';

import { canonicalTestFixture, canonicalTestItem } from '../services/chatTimelineTestFixture';
import type { ChatCompletion } from '../services/chatCompletion';

let fixture: ReturnType<typeof canonicalTestFixture>;
const completion = (conversationId: string): ChatCompletion => ({ conversationId, itemId: 'response', turnId: 'turn', assistantMessageId: 'assistant', reason: 'completed', surfaceOrigin: { executionId: 'request' } as ChatCompletion['surfaceOrigin'] });
vi.mock('../services/chatTimelineWails', () => ({ getChatTimelineClient: () => fixture.client }));

const conversationId = "01926b90-7a5a-7c4e-8d3f-000000000042";
let eventHandler: ((data: unknown) => void) | null = null;
let unsubscribeCount = 0;
const eventsOnMock = vi.fn((eventName: string, handler: (data: unknown) => void) => {
  if (eventName === 'chat:done') {
    eventHandler = handler;
  }
  return () => {
    unsubscribeCount += 1;
    if (eventHandler === handler) eventHandler = null;
  };
});

vi.mock('@wailsjs/runtime/runtime', () => ({
  EventsOn: (eventName: string, handler: (data: unknown) => void) => eventsOnMock(eventName, handler),
}));

describe('useEditorInlineChatPatch', () => {
  it('ignora outro envio na mesma conversa e não aceita terminal antes da correlação', async () => {
    const { result } = renderHook(() => useEditorInlineChatPatch());
    let execution: string | undefined;
    const pending = result.current.waitForChatDone(conversationId, () => execution, 1000);
    let settled = false; void pending.then(() => { settled = true; });
    eventHandler?.(completion(conversationId));
    execution = 'request';
    eventHandler?.({ ...completion(conversationId), surfaceOrigin: { executionId: 'other-request' } });
    await Promise.resolve();
    expect(settled).toBe(false);
    eventHandler?.(completion(conversationId));
    await expect(pending).resolves.toEqual(completion(conversationId));
    expect(unsubscribeCount).toBe(1);
  });
  beforeEach(() => {
    fixture = canonicalTestFixture();
    eventsOnMock.mockClear();
    eventHandler = null;
    unsubscribeCount = 0;
  });

  afterEach(() => {
    void fixture.client.dispose();
    vi.useRealTimers();
  });

  it('retorna erro quando não há patch no corpo', async () => {
    fixture.setItems([canonicalTestItem('response', 'Não vou alterar nada.', { conversationId })]);

    const { result } = renderHook(() => useEditorInlineChatPatch());

    const found = await result.current.waitForEditorPatch(completion(conversationId));

    expect(found.ok).toBe(false);
  });

  it('usa fallback do corpo quando permitido', async () => {
    fixture.setItems([canonicalTestItem('response', '```editor_patch\n{"v":1,"op":"replace_selection","format":"plain","replacement":"texto"}\n```', { conversationId })]);

    const { result } = renderHook(() => useEditorInlineChatPatch());

    const found = await result.current.waitForEditorPatch(completion(conversationId));

    expect(found.ok).toBe(true);
    if (found.ok) {
      expect(found.patch.replacement).toBe('texto');
      expect(fixture.port.close).toHaveBeenCalledTimes(1);
    }
  });

  it('resolve waitForChatDone quando evento chega', async () => {
    const { result } = renderHook(() => useEditorInlineChatPatch());

    const promise = result.current.waitForChatDone(conversationId, () => 'request', 1000);

    act(() => {
      eventHandler?.(completion(conversationId));
    });

    await expect(promise).resolves.toEqual(completion(conversationId));
  });

  it('cancela waitForChatDone e remove timer/listener com AbortSignal', async () => {
    vi.useFakeTimers();
    const { result } = renderHook(() => useEditorInlineChatPatch());
    const controller = new AbortController();
    const promise = result.current.waitForChatDone(conversationId, () => 'request', 5 * 60 * 1000, controller.signal);

    // O consumidor rejeitado trata a promise antes de qualquer await posterior.
    void promise.catch(() => undefined);
    act(() => controller.abort());

    await expect(promise).rejects.toMatchObject({ name: 'AbortError' });
    expect(unsubscribeCount).toBe(1);
    expect(eventHandler).toBeNull();

    act(() => vi.advanceTimersByTime(5 * 60 * 1000));
    expect(unsubscribeCount).toBe(1);
  });
});
