import { useMemo } from 'react';
import { EventsOn } from '@wailsjs/runtime/runtime';

import { extractEditorPatch } from '../lib/editorPatch';
import { readChatCompletion, type ChatCompletion } from '../services/chatCompletion';

async function waitForEditorPatch(completion: ChatCompletion, signal?: AbortSignal) {
  return extractEditorPatch(await readChatCompletion(completion, signal));
}

function waitForChatDone(
  expectedConversationId: string,
  executionId: () => string | undefined,
  timeoutMs = 5 * 60 * 1000,
  signal?: AbortSignal,
): Promise<ChatCompletion> {
  return new Promise<ChatCompletion>((resolve, reject) => {
    let timer: number | undefined;
    let unsub: (() => void) | null = null;
    let settled = false;
    const cleanup = () => {
      if (timer !== undefined) {
        window.clearTimeout(timer);
        timer = undefined;
      }
      unsub?.();
      unsub = null;
      signal?.removeEventListener('abort', onAbort);
    };
    const resolveOnce = (completion: ChatCompletion) => {
      if (settled) return;
      settled = true;
      cleanup();
      resolve(completion);
    };
    const rejectOnce = (error: Error) => {
      if (settled) return;
      settled = true;
      cleanup();
      reject(error);
    };
    const onAbort = () => {
      const error = new Error('Aguardando chat:done cancelado');
      error.name = 'AbortError';
      rejectOnce(error);
    };

    if (signal?.aborted) {
      onAbort();
      return;
    }

    signal?.addEventListener('abort', onAbort, { once: true });
    unsub = EventsOn('chat:done', (data: unknown) => {
      const eventData = data as ChatCompletion;
      const convId = eventData?.conversationId;
      if (typeof convId !== 'string') return;
      if (expectedConversationId && convId !== expectedConversationId) return;
      const expectedExecution = executionId();
      if (!expectedExecution || eventData.surfaceOrigin?.executionId !== expectedExecution) return;
      resolveOnce(eventData);
    });

    timer = window.setTimeout(() => {
      rejectOnce(new Error('Timeout aguardando chat:done'));
    }, timeoutMs);
  });
}

export function useEditorInlineChatPatch() {
  return useMemo(() => {
    return {
      waitForChatDone,
      waitForEditorPatch,
    };
  }, []);
}
