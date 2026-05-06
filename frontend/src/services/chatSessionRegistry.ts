import type { ToolCallStatus } from '../types/chat';
import type { MessageNode, TurnSegment } from '../lib/chatMessageTree';
import type { MediaFile } from './mediaService';
import {
  MAX_MESSAGE_WINDOW_NODES,
  MAX_MESSAGE_WINDOW_TURN_BOUNDARY_OVERFLOW,
} from './messageWindowLimits';

export interface ActiveConversation {
  id: string;
  title: string;
  threadedMessages: MessageNode[];
  channel?: string;
  contactId?: string;
}

export type ConversationTimeline = ActiveConversation;
export type ChatSessionKey = string;
export type ChatSurfaceType = 'page' | 'embedded' | 'modal' | 'external';

export interface ChatSurfaceOrigin {
  sessionKey: ChatSessionKey;
  conversationId: string | null;
  tabId?: string;
  surfaceId: string;
  surfaceType: ChatSurfaceType;
}

export interface ChatSurfaceIdentity {
  surfaceId: string;
  surfaceType: ChatSurfaceType;
  sessionKey: ChatSessionKey;
  conversationId: string | null;
  tabId?: string;
}

export interface MessageWindowState {
  scope: 'conversation' | 'thread';
  conversationId: string;
  threadParentId?: string;
  totalCount: number;
  startIndex: number;
  endIndex: number;
  hasBefore: boolean;
  hasAfter: boolean;
}

export interface ChatSurfaceDescriptor {
  identity: ChatSurfaceIdentity;
  isActive: boolean;
}

export const buildChatSessionKey = (surfaceId: string, conversationId: string | null): ChatSessionKey => (
  `${surfaceId}:${conversationId || 'none'}`
);

export const buildTabChatSurfaceId = (
  tabId: string,
  surfaceType: ChatSurfaceType = 'page',
): string => `${surfaceType}:tab:${tabId}`;

export const buildWorkspaceModalChatSurfaceId = (tabId: string): string => (
  `modal:workspace-chat:${tabId}`
);

export function createChatSurfaceIdentity({
  conversationId = null,
  sessionKey,
  surfaceId,
  surfaceType = 'page',
  tabId,
}: {
  conversationId?: string | null;
  sessionKey?: ChatSessionKey;
  surfaceId?: string;
  surfaceType?: ChatSurfaceType;
  tabId?: string;
}): ChatSurfaceIdentity {
  const resolvedSurfaceId = surfaceId
    ?? (tabId ? buildTabChatSurfaceId(tabId, surfaceType) : `${surfaceType}:standalone`);
  return {
    conversationId,
    sessionKey: sessionKey ?? buildChatSessionKey(resolvedSurfaceId, conversationId),
    surfaceId: resolvedSurfaceId,
    surfaceType,
    ...(tabId ? { tabId } : {}),
  };
}

export const createChatSurfaceOrigin = (identity: ChatSurfaceIdentity): ChatSurfaceOrigin => ({
  conversationId: identity.conversationId,
  sessionKey: identity.sessionKey,
  surfaceId: identity.surfaceId,
  surfaceType: identity.surfaceType,
  ...(identity.tabId ? { tabId: identity.tabId } : {}),
});

export function normalizeChatSurfaceOrigin(
  origin: ChatSurfaceOrigin,
  conversationId: string,
): ChatSurfaceOrigin;
export function normalizeChatSurfaceOrigin(
  origin: undefined,
  conversationId: string,
): undefined;
export function normalizeChatSurfaceOrigin(
  origin: ChatSurfaceOrigin | undefined,
  conversationId: string,
): ChatSurfaceOrigin | undefined {
  return origin
    ? {
      ...origin,
      conversationId,
      sessionKey: origin.conversationId === conversationId
        ? origin.sessionKey
        : buildChatSessionKey(origin.surfaceId, conversationId),
    }
    : undefined;
}

export interface ChatSurfaceSession {
  sessionKey: string;
  conversationId: string | null;
  surfaceOrigin?: ChatSurfaceOrigin;
  queuedTurnCount?: number;
  isLoading: boolean;
  hasOlderMessages: boolean;
  isLoadingOlderMessages: boolean;
  isLoadingMessageWindow: boolean;
  visibleThreadedMessages?: MessageNode[];
  messageWindow?: MessageWindowState;
  streamingMessageId: string | null;
  streamingReasoning: string | null;
  isThinking: boolean;
  activeToolCalls: ToolCallStatus[];
  completedSegments: TurnSegment[];
  draftMessage: string;
  draftMediaFiles: MediaFile[];
  scrollTop: number;
  scrollAnchorMessageId: string | null;
  expandedThreads: Set<string>;
  expandedReasonings: Set<string>;
  editingMessageId: string | null;
  readingMessageId: string | null;
  skipFocusRestore: boolean;
}

export interface ChatConversationSession extends ChatSurfaceSession {
  conversation: ConversationTimeline | null;
}

export interface ChatSessionRegistryState {
  sessionsByConversationId: Record<string, ChatConversationSession>;
  timelinesByConversationId?: Record<string, ConversationTimeline>;
  surfaceSessionsByKey?: Record<string, ChatSurfaceSession>;
}

export const getDefaultChatSessionKey = (conversationId: string): string => `conversation:${conversationId}`;

export const createEmptyChatSurfaceSession = (
  conversationId: string | null = null,
  sessionKey = conversationId ? getDefaultChatSessionKey(conversationId) : 'chat:none',
): ChatSurfaceSession => ({
  sessionKey,
  conversationId,
  queuedTurnCount: 0,
  isLoading: false,
  hasOlderMessages: false,
  isLoadingOlderMessages: false,
  isLoadingMessageWindow: false,
  streamingMessageId: null,
  streamingReasoning: null,
  isThinking: false,
  activeToolCalls: [],
  completedSegments: [],
  draftMessage: '',
  draftMediaFiles: [],
  scrollTop: 0,
  scrollAnchorMessageId: null,
  expandedThreads: new Set<string>(),
  expandedReasonings: new Set<string>(),
  editingMessageId: null,
  readingMessageId: null,
  skipFocusRestore: false,
});

export const createEmptyChatSession = (
  conversationId: string | null = null,
  sessionKey = conversationId ? getDefaultChatSessionKey(conversationId) : 'chat:none',
): ChatConversationSession => ({
  ...createEmptyChatSurfaceSession(conversationId, sessionKey),
  conversation: null,
});

const toSurfaceSession = (
  session: ChatConversationSession,
  conversationId: string | null,
  sessionKey: string,
): ChatSurfaceSession => {
  const {
    conversation: _conversation,
    ...surface
  } = session;
  void _conversation;
  return {
    ...surface,
    sessionKey,
    conversationId,
    visibleThreadedMessages: surface.visibleThreadedMessages ?? (session.conversation
      ? capVisibleSurfaceMessages(
        session.conversation.threadedMessages,
        surface.messageWindow?.startIndex === 0 && surface.messageWindow?.hasBefore === false ? 'start' : 'end',
      )
      : undefined),
    messageWindow: surface.messageWindow,
  };
};

export const getMessageNodeOrder = (node: MessageNode): number => {
  const timestamp = Number(node.message.timestamp ?? Date.parse(String(node.message.createdAt ?? '')));
  return Number.isFinite(timestamp) ? timestamp : Number.MAX_SAFE_INTEGER;
};

export const mergeMessageNode = (existing: MessageNode, incoming: MessageNode): MessageNode => {
  const incomingChildCount = incoming.childCount ?? 0;
  const existingChildCount = existing.childCount ?? 0;
  const shouldPreserveLoadedChildren = existing.children?.length
    && !incoming.children?.length
    && incomingChildCount >= existingChildCount;

  return {
    ...existing,
    ...incoming,
    children: incoming.children?.length
      ? incoming.children
      : shouldPreserveLoadedChildren
        ? existing.children
        : incoming.children,
    childCount: incoming.childCount ?? existing.childCount,
    originalIndex: incoming.originalIndex ?? existing.originalIndex,
    isExpanded: existing.isExpanded ?? incoming.isExpanded,
  } as MessageNode;
};

export const sortMessageNodes = (nodes: MessageNode[]): MessageNode[] => (
  [...nodes].sort((a, b) => {
    const order = getMessageNodeOrder(a) - getMessageNodeOrder(b);
    return order !== 0 ? order : String(a.message.id).localeCompare(String(b.message.id));
  })
);

const reconcileWindowForVisibleMessages = (
  window: MessageWindowState | undefined,
  nodes: MessageNode[],
  totalCountHint: number,
): MessageWindowState | undefined => {
  if (!window) return window;
  if (nodes.length === 0) {
    return {
      ...window,
      totalCount: 0,
      startIndex: 0,
      endIndex: -1,
      hasBefore: false,
      hasAfter: false,
    };
  }
  const explicitIndexes = nodes
    .map((node) => node.originalIndex)
    .filter((index): index is number => index !== undefined);
  const startIndex = explicitIndexes.length ? Math.min(...explicitIndexes) : Math.min(window.startIndex, totalCountHint - 1);
  const endIndex = explicitIndexes.length ? Math.max(...explicitIndexes) : startIndex + nodes.length - 1;
  const totalCount = Math.max(window.totalCount, totalCountHint, endIndex + 1);

  return {
    ...window,
    totalCount,
    startIndex,
    endIndex,
    hasBefore: startIndex > 0,
    hasAfter: totalCount > 0 && endIndex < totalCount - 1,
  };
};

const mergeTimelineConversation = (
  current: ConversationTimeline | null,
  incoming: ConversationTimeline,
): ConversationTimeline => {
  if (!current) return incoming;
  const byId = new Map<string, MessageNode>();
  for (const node of current.threadedMessages) {
    byId.set(String(node.message.id), node);
  }
  for (const node of incoming.threadedMessages) {
    const existing = byId.get(String(node.message.id));
    byId.set(String(node.message.id), existing
      ? mergeMessageNode(existing, node)
      : node);
  }
  return {
    ...current,
    ...incoming,
    threadedMessages: sortMessageNodes(Array.from(byId.values())),
  };
};

const capVisibleSurfaceMessages = (nodes: MessageNode[], keep: 'start' | 'end' = 'end'): MessageNode[] => {
  if (nodes.length <= MAX_MESSAGE_WINDOW_NODES) return nodes;
  if (keep === 'start') {
    let endIndex = MAX_MESSAGE_WINDOW_NODES;
    const boundaryTurnId = nodes[endIndex - 1]?.message.turnId;
    while (boundaryTurnId && endIndex < nodes.length && nodes[endIndex]?.message.turnId === boundaryTurnId) {
      endIndex += 1;
    }
    if (endIndex > MAX_MESSAGE_WINDOW_NODES + MAX_MESSAGE_WINDOW_TURN_BOUNDARY_OVERFLOW) {
      endIndex = MAX_MESSAGE_WINDOW_NODES;
    }
    return nodes.slice(0, endIndex);
  }
  const minStartIndex = nodes.length - MAX_MESSAGE_WINDOW_NODES;
  let startIndex = minStartIndex;
  const boundaryTurnId = nodes[startIndex]?.message.turnId;
  while (boundaryTurnId && startIndex > 0 && nodes[startIndex - 1]?.message.turnId === boundaryTurnId) {
    startIndex -= 1;
  }
  if (nodes.length - startIndex > MAX_MESSAGE_WINDOW_NODES + MAX_MESSAGE_WINDOW_TURN_BOUNDARY_OVERFLOW) {
    startIndex = minStartIndex;
  }
  return nodes.slice(startIndex);
};

export function getChatSession(
  state: ChatSessionRegistryState,
  conversationId: string,
  sessionKey = getDefaultChatSessionKey(conversationId),
): ChatConversationSession {
  const defaultSession = state.sessionsByConversationId[conversationId];
  const timeline = state.timelinesByConversationId?.[conversationId] ?? defaultSession?.conversation ?? null;
  const defaultSessionKey = getDefaultChatSessionKey(conversationId);
  const shouldUseDefaultSessionFallback = sessionKey === defaultSessionKey;
  const surfaceSession = state.surfaceSessionsByKey?.[sessionKey]
    ?? (shouldUseDefaultSessionFallback && defaultSession
      ? toSurfaceSession(defaultSession, conversationId, sessionKey)
      : null)
    ?? createEmptyChatSurfaceSession(conversationId, sessionKey);

  return {
    ...surfaceSession,
    conversation: timeline
      ? {
        ...timeline,
        threadedMessages: surfaceSession.visibleThreadedMessages ?? timeline.threadedMessages,
      }
      : null,
  };
}

export function getConversationTimeline(
  state: ChatSessionRegistryState,
  conversationId: string,
): ConversationTimeline | null {
  return state.timelinesByConversationId?.[conversationId]
    ?? state.sessionsByConversationId[conversationId]?.conversation
    ?? null;
}

export function getDefaultChatConversationSession(
  state: ChatSessionRegistryState,
  conversationId: string,
): ChatConversationSession | null {
  return state.sessionsByConversationId[conversationId] ?? null;
}

export function getChatSurfaceSessionsForConversation(
  state: ChatSessionRegistryState,
  conversationId: string,
): ChatSurfaceSession[] {
  return Object.values(state.surfaceSessionsByKey ?? {})
    .filter((session) => session.conversationId === conversationId);
}

export function patchChatSession<TState extends ChatSessionRegistryState>(
  state: TState,
  conversationId: string,
  patch: Partial<ChatConversationSession> | ((session: ChatConversationSession) => ChatConversationSession),
  sessionKey = getDefaultChatSessionKey(conversationId),
): Pick<TState, 'sessionsByConversationId'> & Pick<ChatSessionRegistryState, 'timelinesByConversationId' | 'surfaceSessionsByKey'> {
  const currentSession = getChatSession(state, conversationId, sessionKey);
  const nextSession = typeof patch === 'function'
    ? patch(currentSession)
    : { ...currentSession, ...patch };
  let nextSurfaceSession = toSurfaceSession(nextSession, conversationId, sessionKey);
  const defaultSessionKey = getDefaultChatSessionKey(conversationId);
  const isDefaultSession = sessionKey === defaultSessionKey;

  const patchCarriesConversation = typeof patch === 'function'
    || Object.prototype.hasOwnProperty.call(patch, 'conversation');
  const shouldPatchTimeline = patchCarriesConversation;
  const shouldMirrorConversationIntoSurface = patchCarriesConversation
    && nextSession.conversation
    && (typeof patch === 'function' || !Object.prototype.hasOwnProperty.call(patch, 'visibleThreadedMessages'));
  if (shouldMirrorConversationIntoSurface) {
    const nextConversation = nextSession.conversation as ConversationTimeline;
    nextSurfaceSession = {
      ...nextSurfaceSession,
      visibleThreadedMessages: capVisibleSurfaceMessages(nextConversation.threadedMessages),
    };
  }
  const defaultSession = state.sessionsByConversationId[conversationId] ?? createEmptyChatSession(conversationId);
  const currentTimelinesByConversationId = state.timelinesByConversationId ?? {};
  let timelinesByConversationId = currentTimelinesByConversationId;
  if (shouldPatchTimeline) {
    timelinesByConversationId = { ...currentTimelinesByConversationId };
    if (nextSession.conversation) {
      timelinesByConversationId[conversationId] = isDefaultSession
        ? nextSession.conversation
        : mergeTimelineConversation(
          currentTimelinesByConversationId[conversationId] ?? defaultSession?.conversation ?? null,
          nextSession.conversation,
        );
    } else {
      delete timelinesByConversationId[conversationId];
    }
  }
  const nextDefaultSession = isDefaultSession
    ? nextSession
    : {
      ...defaultSession,
      conversation: timelinesByConversationId[conversationId] ?? defaultSession.conversation,
    };

  return {
    sessionsByConversationId: {
      ...state.sessionsByConversationId,
      [conversationId]: nextDefaultSession,
    },
    timelinesByConversationId,
    surfaceSessionsByKey: {
      ...(state.surfaceSessionsByKey ?? {}),
      [sessionKey]: nextSurfaceSession,
    },
  };
}

export function patchChatConversation<TState extends ChatSessionRegistryState>(
  state: TState,
  conversationId: string,
  updater: (conversation: ActiveConversation) => ActiveConversation,
): Partial<TState> {
  const timeline = getConversationTimeline(state, conversationId);
  if (!timeline) return state;
  const conversation = updater(timeline);
  const currentSession = getChatSession(state, conversationId);
  const surfaceSessionsByKey = { ...(state.surfaceSessionsByKey ?? {}) };
  for (const [sessionKey, surfaceSession] of Object.entries(surfaceSessionsByKey)) {
    if (surfaceSession.conversationId !== conversationId || !surfaceSession.visibleThreadedMessages) {
      continue;
    }
    const surfaceConversation = updater({
      ...timeline,
      threadedMessages: surfaceSession.visibleThreadedMessages,
    });
    const keepVisibleBoundary = surfaceSession.messageWindow?.startIndex === 0
      && surfaceSession.messageWindow?.hasBefore === false
      ? 'start'
      : 'end';
    surfaceSessionsByKey[sessionKey] = {
      ...surfaceSession,
      visibleThreadedMessages: capVisibleSurfaceMessages(surfaceConversation.threadedMessages, keepVisibleBoundary),
    };
    const messageWindow = reconcileWindowForVisibleMessages(
      surfaceSession.messageWindow,
      surfaceSessionsByKey[sessionKey].visibleThreadedMessages ?? [],
      conversation.threadedMessages.length,
    );
    surfaceSessionsByKey[sessionKey] = {
      ...surfaceSessionsByKey[sessionKey],
      messageWindow,
      hasOlderMessages: messageWindow?.hasBefore ?? surfaceSession.hasOlderMessages,
    };
  }
  return {
    sessionsByConversationId: {
      ...state.sessionsByConversationId,
      [conversationId]: {
        ...currentSession,
        conversation,
      },
    },
    timelinesByConversationId: {
      ...(state.timelinesByConversationId ?? {}),
      [conversationId]: conversation,
    },
    surfaceSessionsByKey,
  } as Partial<TState>;
}

export function removeChatSession<TState extends ChatSessionRegistryState>(
  state: TState,
  conversationId: string,
): Pick<TState, 'sessionsByConversationId'> & Pick<ChatSessionRegistryState, 'timelinesByConversationId' | 'surfaceSessionsByKey'> {
  const sessionsByConversationId = { ...state.sessionsByConversationId };
  delete sessionsByConversationId[conversationId];
  const timelinesByConversationId = { ...(state.timelinesByConversationId ?? {}) };
  delete timelinesByConversationId[conversationId];
  const surfaceSessionsByKey = Object.fromEntries(
    Object.entries(state.surfaceSessionsByKey ?? {}).filter(([, session]) => session.conversationId !== conversationId),
  );
  return { sessionsByConversationId, timelinesByConversationId, surfaceSessionsByKey };
}
