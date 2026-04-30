import {
  GetConversationInfo,
  GetMessageChildren,
  GetMessagesBefore,
  GetRecentMessages,
} from '@wailsjs/go/app/App';
import {
  withOriginalIndex,
  type MessageNode,
} from '../lib/chatMessageTree';

export interface LoadedConversationSnapshot {
  title: string;
  channel?: string;
  contactId?: string;
  threadedMessages: MessageNode[];
  hasOlderMessages: boolean;
}

export interface LoadedOlderMessages {
  nodes: MessageNode[];
  hasOlderMessages: boolean;
}

export async function loadConversationSnapshot(
  conversationId: string,
  windowSize: number,
): Promise<LoadedConversationSnapshot> {
  const requestedLimit = windowSize + 1;
  const [conversationInfo, backendNodes] = await Promise.all([
    GetConversationInfo(conversationId),
    GetRecentMessages(conversationId, requestedLimit),
  ]);
  const fetchedNodes = backendNodes || [];
  const hasOlderMessages = fetchedNodes.length > windowSize;
  const visibleNodes = hasOlderMessages ? fetchedNodes.slice(1) : fetchedNodes;

  return {
    title: conversationInfo?.title || 'Conversa',
    channel: conversationInfo?.channel || undefined,
    contactId: conversationInfo?.contact_id || undefined,
    threadedMessages: visibleNodes.map(withOriginalIndex),
    hasOlderMessages,
  };
}

export async function loadOlderConversationMessages(
  conversationId: string,
  beforeMessageId: string,
  windowSize: number,
): Promise<LoadedOlderMessages> {
  const requestedLimit = windowSize + 1;
  const backendNodes = await GetMessagesBefore(conversationId, beforeMessageId, requestedLimit);
  const fetchedNodes = backendNodes || [];
  const hasOlderMessages = fetchedNodes.length > windowSize;
  const visibleNodes = hasOlderMessages ? fetchedNodes.slice(1) : fetchedNodes;

  return {
    nodes: visibleNodes.map(withOriginalIndex),
    hasOlderMessages,
  };
}

export async function reloadConversationSnapshot(
  conversationId: string,
  windowSize: number,
): Promise<Pick<LoadedConversationSnapshot, 'threadedMessages' | 'hasOlderMessages'>> {
  const requestedLimit = windowSize + 1;
  const backendNodes = await GetRecentMessages(conversationId, requestedLimit);
  const fetchedNodes = backendNodes || [];
  const hasOlderMessages = fetchedNodes.length > windowSize;
  const visibleNodes = hasOlderMessages ? fetchedNodes.slice(1) : fetchedNodes;

  return {
    threadedMessages: visibleNodes.map(withOriginalIndex),
    hasOlderMessages,
  };
}

export async function loadMessageChildrenNodes(messageId: string): Promise<MessageNode[]> {
  const backendNodes = await GetMessageChildren(messageId);
  return (backendNodes || []).map(withOriginalIndex);
}
