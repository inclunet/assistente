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

function findBodyPatch(opts?: Pick<FindLatestEditorPatchOptions, 'afterMessageId'>): FindPatchResult {
  const afterState = useChatStore.getState();
  const allMessages = afterState.getMessages() as MessageLike[];

  const afterMessageId = opts?.afterMessageId || '';
  let messages = allMessages;
  if (afterMessageId) {
    const idx = allMessages.findIndex((m) => String(m?.id || '') === afterMessageId);
    if (idx >= 0) {
      messages = allMessages.slice(idx + 1);
    } else {
      // ID not found (list reloaded/compacted) — filter by lexicographic order (UUIDv7 is sortable)
      messages = allMessages.filter((m) => {
        const id = String(m?.id || '');
        return isBackendId(id) && id > afterMessageId;
      });
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

async function waitForEditorPatch(opts?: FindLatestEditorPatchOptions): Promise<FindPatchResult> {
  const timeoutMs = typeof opts?.timeoutMs === 'number' ? opts.timeoutMs : 5000;
  const startedAt = Date.now();
  while (Date.now() - startedAt < timeoutMs) {
    const found = findBodyPatch(opts);
    if (found.ok) return found;
    await new Promise((r) => setTimeout(r, 120));
  }
  return findBodyPatch(opts);
}

function waitForChatDone(expectedConversationId?: string, timeoutMs = 5 * 60 * 1000): Promise<string> {
  return new Promise<string>((resolve, reject) => {
    let timer: number;
    const unsub = EventsOn('chat:done', (data: unknown) => {
      const eventData = data as { conversationId?: string };
      const convId = eventData?.conversationId;
      if (typeof convId !== 'string') return;
      if (expectedConversationId && convId !== expectedConversationId) return;
      window.clearTimeout(timer);
      unsub();
      resolve(convId);
    });

    timer = window.setTimeout(() => {
      unsub();
      reject(new Error('Timeout aguardando chat:done'));
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
