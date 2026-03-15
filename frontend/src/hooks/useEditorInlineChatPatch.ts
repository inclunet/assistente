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
  | { ok: true; patch: EditorPatch; source: 'tool' | 'body' }
  | { ok: false; error: string };

type FindLatestEditorPatchOptions = {
  afterMessageId?: number;
  preferToolCalling?: boolean;
  allowBodyFallback?: boolean;
};

type ToolCall = {
  function?: { name?: string };
  name?: string;
  id?: string;
  callId?: string;
};

type MessageLike = {
  id?: number | string;
  role?: string;
  toolCallId?: string;
  content?: string;
  toolCalls?: unknown;
};

function parseToolCalls(toolCallsJson: unknown): ToolCall[] {
  if (!toolCallsJson) return [];
  if (Array.isArray(toolCallsJson)) return toolCallsJson;
  if (typeof toolCallsJson === 'object') return [toolCallsJson];

  if (typeof toolCallsJson !== 'string') return [];
  const raw = toolCallsJson.trim();
  if (!raw) return [];

  try {
    const parsed = JSON.parse(raw);
    return Array.isArray(parsed) ? parsed : [parsed];
  } catch {
    return [];
  }
}

function extractTextEditToolCallIds(toolCallsJson: unknown): string[] {
  const calls = parseToolCalls(toolCallsJson);
  const ids: string[] = [];
  for (const call of calls) {
    const name = String(call?.function?.name || call?.name || '').trim();
    const id = String(call?.id || call?.callId || '').trim();
    if (name === 'text_edit' && id) ids.push(id);
  }
  return ids;
}

function parseEditorPatchFromToolResultContent(toolContent: string): EditorPatch | null {
  const raw = String(toolContent || '').trim();
  if (!raw) return null;

  try {
    const parsed = JSON.parse(raw);
    const candidate = parsed?.patch && typeof parsed?.patch === 'object' ? parsed.patch : parsed;

    if (candidate?.v !== 1 || candidate?.op !== 'replace_selection') return null;
    if (candidate?.format !== 'markdown' && candidate?.format !== 'plain') return null;
    if (typeof candidate?.replacement !== 'string') return null;

    return candidate as EditorPatch;
  } catch {
    return null;
  }
}

function getMaxNumericMessageId(messages: MessageLike[]): number {
  let maxId = 0;
  for (const m of messages) {
    const n = typeof m?.id === 'number' ? m.id : parseInt(String(m?.id || ''), 10);
    if (!isNaN(n) && n > maxId) maxId = n;
  }
  return maxId;
}

function findLatestEditorPatch(chatTabId: string, opts?: FindLatestEditorPatchOptions): FindPatchResult {
  const afterState = useChatStore.getState();
  const allMessages = afterState.getTabMessages(chatTabId) as MessageLike[];

  const afterMessageId = opts?.afterMessageId || 0;
  const preferToolCalling = opts?.preferToolCalling !== false;
  const allowBodyFallback = opts?.allowBodyFallback !== false;

  const messages = afterMessageId > 0
    ? allMessages.filter((m) => {
        const n = typeof m?.id === 'number' ? m.id : parseInt(String(m?.id || ''), 10);
        return !isNaN(n) && n > afterMessageId;
      })
    : allMessages;

  // Indexa resultados de tools por toolCallId
  const toolResultsByCallId = new Map<string, string>();
  for (const m of messages) {
    if (m?.role !== 'tool') continue;
    const callId = String(m?.toolCallId || '').trim();
    if (!callId) continue;
    toolResultsByCallId.set(callId, String(m?.content || ''));
  }

  // Passo 1 (preferido): assistant tool_calls(text_edit) + tool_result correspondente.
  for (let i = messages.length - 1; i >= 0; i--) {
    const msg = messages[i];
    if (msg?.role !== 'assistant') continue;

    const textEditCallIds = extractTextEditToolCallIds(msg?.toolCalls);
    if (textEditCallIds.length > 0) {
      for (let j = textEditCallIds.length - 1; j >= 0; j--) {
        const callId = textEditCallIds[j];
        const toolContent = toolResultsByCallId.get(callId);
        if (!toolContent) continue;

        const patch = parseEditorPatchFromToolResultContent(toolContent);
        if (patch) return { ok: true, patch, source: 'tool' };

        const toolText = String(toolContent || '').trim();
        if (toolText) return { ok: false, error: toolText };
      }

      return { ok: false, error: 'Aguardando resultado da ferramenta…' };
    }
  }

  // Passo 2: patch no corpo da resposta.
  if (!preferToolCalling || allowBodyFallback) {
    for (let i = messages.length - 1; i >= 0; i--) {
      const msg = messages[i];
      if (msg?.role !== 'assistant') continue;
      const content = String(msg?.content || '');
      const extracted = extractEditorPatch(content);
      if (extracted.ok) return { ok: true, patch: extracted.patch as EditorPatch, source: 'body' };
    }
  }

  return {
    ok: false,
    error: allowBodyFallback
      ? 'Nenhum patch encontrado'
      : 'Tool calling está ativo: aguardando um text_edit (nenhum tool_call foi recebido).',
  };
}

async function waitForEditorPatch(
  chatTabId: string,
  opts?: FindLatestEditorPatchOptions & { timeoutMs?: number }
): Promise<FindPatchResult> {
  const timeoutMs = typeof opts?.timeoutMs === 'number' ? opts.timeoutMs : 5000;
  const startedAt = Date.now();
  while (Date.now() - startedAt < timeoutMs) {
    const found = findLatestEditorPatch(chatTabId, opts);
    if (found.ok) return found;
    await new Promise((r) => setTimeout(r, 120));
  }

  return findLatestEditorPatch(chatTabId, opts);
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
