import { useMemo } from 'react';
import { EventsOn } from '@wailsjs/runtime/runtime';

import { useChatStore } from '../store/chatStore';
import { extractEditorPatch } from '../lib/editorPatch';

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
  afterMessageId?: number;
  timeoutMs?: number;
};

type MessageLike = {
  id?: number | string;
  role?: string;
  content?: string;
};

function getMaxNumericMessageId(messages: MessageLike[]): number {
  let maxId = 0;
  for (const m of messages) {
    const n = typeof m?.id === 'number' ? m.id : parseInt(String(m?.id || ''), 10);
    if (!isNaN(n) && n > maxId) maxId = n;
  }
  return maxId;
}

function findBodyPatch(opts?: Pick<FindLatestEditorPatchOptions, 'afterMessageId'>): FindPatchResult {
  const afterState = useChatStore.getState();
  const allMessages = afterState.getMessages() as MessageLike[];

  const afterMessageId = opts?.afterMessageId || 0;
  const messages = afterMessageId > 0
    ? allMessages.filter((m) => {
        const n = typeof m?.id === 'number' ? m.id : parseInt(String(m?.id || ''), 10);
        return !isNaN(n) && n > afterMessageId;
      })
    : allMessages;

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

function waitForChatDone(expectedConversationId?: number, timeoutMs = 5 * 60 * 1000): Promise<number> {
  return new Promise<number>((resolve, reject) => {
    let timer: number;
    const unsub = EventsOn('chat:done', (data: unknown) => {
      const eventData = data as { conversationId?: number };
      const convId = eventData?.conversationId;
      if (typeof convId !== 'number') return;
      if (expectedConversationId && expectedConversationId > 0 && convId !== expectedConversationId) return;
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
      getMaxNumericMessageId,
    };
  }, []);
}
