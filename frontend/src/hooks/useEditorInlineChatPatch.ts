import { useMemo } from 'react';
import { EventsOn } from '@wailsjs/runtime/runtime';

import { useChatStore } from '../store/chatStore';
import { extractEditorPatch } from '../lib/editorPatch';
import { isBackendId } from '../lib/idUtils';

type EditorPatch = {
  v: 1;
  op: 'replace_selection';
  format: 'markdown' | 'plain';
  replacement: string;
  notes?: string;
};

type FindPatchResult =
  | { ok: true; patch: EditorPatch; source: 'body' }
  | { ok: false; error: string };

type FindLatestEditorPatchOptions = {
  conversationId: string;
  afterMessageId?: string;
  timeoutMs?: number;
};

type MessageLike = {
  id?: number | string;
  role?: string;
  content?: string;
};

/**
 * Returns the last backend message ID from a list.
 * Iterates from the end to find the most recent persisted message,
 * avoiding lexicographic comparison of UUIDs.
 */
function getMaxMessageId(messages: MessageLike[]): string {
  for (let i = messages.length - 1; i >= 0; i--) {
    const id = String(messages[i]?.id || '');
    if (isBackendId(id)) return id;
  }
  return '';
}

function findBodyPatch(opts: Pick<FindLatestEditorPatchOptions, 'conversationId' | 'afterMessageId'>): FindPatchResult {
  const afterState = useChatStore.getState();
  const allMessages = afterState.getConversationMessages(opts.conversationId) as MessageLike[];

  const afterMessageId = opts?.afterMessageId || '';
  let messages = allMessages;
  if (afterMessageId) {
    const idx = allMessages.findIndex((m) => String(m?.id || '') === afterMessageId);
    if (idx >= 0) {
      messages = allMessages.slice(idx + 1);
    } else {
      // ID not found (list reloaded/compacted) — scan all messages as fallback.
      // We intentionally avoid lexicographic ID comparison because UUIDv7
      // ordering within the same millisecond is not guaranteed.
      messages = allMessages;
    }
  }

  for (let i = messages.length - 1; i >= 0; i--) {
    const msg = messages[i];
    if (msg?.role !== 'assistant') continue;
    const content = String(msg?.content || '');
    const extracted = extractEditorPatch(content);
    if (extracted.ok) return { ok: true, patch: extracted.patch as EditorPatch, source: 'body' };
  }

  return { ok: false, error: 'Nenhum patch encontrado' };
}

async function waitForEditorPatch(opts: FindLatestEditorPatchOptions): Promise<FindPatchResult> {
  const timeoutMs = typeof opts?.timeoutMs === 'number' ? opts.timeoutMs : 5000;
  const startedAt = Date.now();
  while (Date.now() - startedAt < timeoutMs) {
    const found = findBodyPatch(opts);
    if (found.ok) return found;
    await new Promise((r) => setTimeout(r, 120));
  }
  return findBodyPatch(opts);
}

function waitForChatDone(
  expectedConversationId?: string,
  timeoutMs = 5 * 60 * 1000,
  signal?: AbortSignal,
): Promise<string> {
  return new Promise<string>((resolve, reject) => {
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
    const resolveOnce = (conversationId: string) => {
      if (settled) return;
      settled = true;
      cleanup();
      resolve(conversationId);
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
      const eventData = data as { conversationId?: string };
      const convId = eventData?.conversationId;
      if (typeof convId !== 'string') return;
      if (expectedConversationId && convId !== expectedConversationId) return;
      resolveOnce(convId);
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
      getMaxMessageId,
    };
  }, []);
}
