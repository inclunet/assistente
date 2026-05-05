import {
  GetConversationInfo,
  GetConversationMessageWindow,
  GetMessageChildren,
} from '@wailsjs/go/app/App';
import i18next from 'i18next';
import {
  withOriginalIndex,
  type MessageNode,
} from '../lib/chatMessageTree';
import type { MessageWindowState } from './chatSessionRegistry';

interface LoadedMessageWindow {
  nodes: MessageNode[];
  window: MessageWindowState;
}

export interface LoadedConversationSnapshot {
  title: string;
  channel?: string;
  contactId?: string;
  threadedMessages: MessageNode[];
  messageWindow: MessageWindowState;
  hasOlderMessages: boolean;
  hasNewerMessages: boolean;
}

export interface LoadedOlderMessages {
  nodes: MessageNode[];
  messageWindow: MessageWindowState;
  hasOlderMessages: boolean;
  hasNewerMessages: boolean;
}

export interface LoadedNewerMessages {
  nodes: MessageNode[];
  messageWindow: MessageWindowState;
  hasOlderMessages: boolean;
  hasNewerMessages: boolean;
}

function normalizeWindow(rawWindow: {
  scope?: string;
  conversationId?: string;
  threadParentId?: string;
  totalCount?: number;
  startIndex?: number;
  endIndex?: number;
  hasBefore?: boolean;
  hasAfter?: boolean;
}): MessageWindowState {
  return {
    scope: rawWindow.scope === 'thread' ? 'thread' : 'conversation',
    conversationId: String(rawWindow.conversationId ?? ''),
    threadParentId: rawWindow.threadParentId || undefined,
    totalCount: Number(rawWindow.totalCount ?? 0),
    startIndex: Number(rawWindow.startIndex ?? 0),
    endIndex: Number(rawWindow.endIndex ?? -1),
    hasBefore: Boolean(rawWindow.hasBefore),
    hasAfter: Boolean(rawWindow.hasAfter),
  };
}

async function loadConversationMessageWindow(request: {
  conversationId: string;
  anchor?: 'start' | 'end';
  anchorMessageId?: string;
  direction: 'before' | 'after' | 'around';
  limit: number;
}): Promise<LoadedMessageWindow> {
  const backendWindow = await GetConversationMessageWindow({
    scope: 'conversation',
    conversationId: request.conversationId,
    anchor: request.anchor,
    anchorMessageId: request.anchorMessageId,
    direction: request.direction,
    limit: request.limit,
  });
  const window = normalizeWindow(backendWindow ?? {});
  return {
    nodes: (backendWindow?.nodes || []).map((node, index) => withOriginalIndex(node, window.startIndex + index)),
    window,
  };
}

export async function loadConversationSnapshot(
  conversationId: string,
  windowSize: number,
): Promise<LoadedConversationSnapshot> {
  const [conversationInfo, loadedWindow] = await Promise.all([
    GetConversationInfo(conversationId),
    loadConversationMessageWindow({
      conversationId,
      anchor: 'end',
      direction: 'before',
      limit: windowSize,
    }),
  ]);

  return {
    title: conversationInfo?.title || i18next.t('chat.conversation'),
    channel: conversationInfo?.channel || undefined,
    contactId: conversationInfo?.contact_id || undefined,
    threadedMessages: loadedWindow.nodes,
    messageWindow: loadedWindow.window,
    hasOlderMessages: loadedWindow.window.hasBefore,
    hasNewerMessages: loadedWindow.window.hasAfter,
  };
}

export async function loadOlderConversationMessages(
  conversationId: string,
  beforeMessageId: string,
  windowSize: number,
): Promise<LoadedOlderMessages> {
  const loadedWindow = await loadConversationMessageWindow({
    conversationId,
    anchorMessageId: beforeMessageId,
    direction: 'before',
    limit: windowSize,
  });

  return {
    nodes: loadedWindow.nodes,
    messageWindow: loadedWindow.window,
    hasOlderMessages: loadedWindow.window.hasBefore,
    hasNewerMessages: loadedWindow.window.hasAfter,
  };
}

export async function loadNewerConversationMessages(
  conversationId: string,
  afterMessageId: string,
  windowSize: number,
): Promise<LoadedNewerMessages> {
  const loadedWindow = await loadConversationMessageWindow({
    conversationId,
    anchorMessageId: afterMessageId,
    direction: 'after',
    limit: windowSize,
  });

  return {
    nodes: loadedWindow.nodes,
    messageWindow: loadedWindow.window,
    hasOlderMessages: loadedWindow.window.hasBefore,
    hasNewerMessages: loadedWindow.window.hasAfter,
  };
}

export async function reloadConversationSnapshot(
  conversationId: string,
  windowSize: number,
): Promise<Pick<LoadedConversationSnapshot, 'threadedMessages' | 'messageWindow' | 'hasOlderMessages' | 'hasNewerMessages'>> {
  const loadedWindow = await loadConversationMessageWindow({
    conversationId,
    anchor: 'end',
    direction: 'before',
    limit: windowSize,
  });

  return {
    threadedMessages: loadedWindow.nodes,
    messageWindow: loadedWindow.window,
    hasOlderMessages: loadedWindow.window.hasBefore,
    hasNewerMessages: loadedWindow.window.hasAfter,
  };
}

export async function loadMessageChildrenNodes(messageId: string): Promise<MessageNode[]> {
  const backendNodes = await GetMessageChildren(messageId);
  return (backendNodes || []).map(withOriginalIndex);
}
