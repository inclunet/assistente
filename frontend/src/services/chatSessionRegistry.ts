import type { ToolCallStatus } from '../types/chat';
import type { MessageNode, TurnSegment } from '../lib/chatMessageTree';
import type { MediaFile } from './mediaService';

export interface ActiveConversation {
  id: string;
  title: string;
  threadedMessages: MessageNode[];
  channel?: string;
  contactId?: string;
  messageWindow?: MessageWindowState;
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
    visibleThreadedMessages: session.conversation?.threadedMessages ?? surface.visibleThreadedMessages,
    messageWindow: session.conversation?.messageWindow ?? surface.messageWindow,
  };
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
        messageWindow: surfaceSession.messageWindow ?? timeline.messageWindow,
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
  const nextSurfaceSession = toSurfaceSession(nextSession, conversationId, sessionKey);
  const defaultSessionKey = getDefaultChatSessionKey(conversationId);
  const isDefaultSession = sessionKey === defaultSessionKey;

  const shouldPatchTimeline = typeof patch === 'function'
    || Object.prototype.hasOwnProperty.call(patch, 'conversation');
  const currentTimelinesByConversationId = state.timelinesByConversationId ?? {};
  let timelinesByConversationId = currentTimelinesByConversationId;
  if (shouldPatchTimeline) {
    timelinesByConversationId = { ...currentTimelinesByConversationId };
    if (nextSession.conversation) {
      timelinesByConversationId[conversationId] = nextSession.conversation;
    } else {
      delete timelinesByConversationId[conversationId];
    }
  }
  const defaultSession = state.sessionsByConversationId[conversationId] ?? createEmptyChatSession(conversationId);
  const nextDefaultSession = isDefaultSession
    ? nextSession
    : {
      ...defaultSession,
      conversation: nextSession.conversation,
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
    surfaceSessionsByKey: state.surfaceSessionsByKey ?? {},
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
