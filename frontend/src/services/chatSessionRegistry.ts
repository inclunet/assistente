import type { ToolCallStatus } from '../components/chat/ToolCallsSection';
import type { MessageNode, TurnSegment } from '../lib/chatMessageTree';

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

export const buildChatSessionKey = (surfaceId: string, conversationId: string | null): ChatSessionKey => (
  `${surfaceId}:${conversationId || 'none'}`
);

export const normalizeChatSurfaceOrigin = (
  origin: ChatSurfaceOrigin | undefined,
  conversationId: string,
): ChatSurfaceOrigin | undefined => (
  origin
    ? {
      ...origin,
      conversationId,
      sessionKey: origin.conversationId === conversationId
        ? origin.sessionKey
        : buildChatSessionKey(origin.surfaceId, conversationId),
    }
    : undefined
);

export interface ChatSurfaceSession {
  sessionKey?: string;
  conversationId?: string | null;
  surfaceOrigin?: ChatSurfaceOrigin;
  queuedTurnCount?: number;
  isLoading: boolean;
  hasOlderMessages: boolean;
  isLoadingOlderMessages: boolean;
  streamingMessageId: string | null;
  streamingReasoning: string | null;
  isThinking: boolean;
  activeToolCalls: ToolCallStatus[];
  completedSegments: TurnSegment[];
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
  return {
    ...surface,
    sessionKey,
    conversationId,
  };
};

export function getChatSession(
  state: ChatSessionRegistryState,
  conversationId: string,
  sessionKey = getDefaultChatSessionKey(conversationId),
): ChatConversationSession {
  const compatibilitySession = state.sessionsByConversationId[conversationId];
  const timeline = state.timelinesByConversationId?.[conversationId] ?? compatibilitySession?.conversation ?? null;
  const defaultSessionKey = getDefaultChatSessionKey(conversationId);
  const shouldUseCompatibilityFallback = sessionKey === defaultSessionKey;
  const surfaceSession = state.surfaceSessionsByKey?.[sessionKey]
    ?? (shouldUseCompatibilityFallback && compatibilitySession
      ? toSurfaceSession(compatibilitySession, conversationId, sessionKey)
      : null)
    ?? createEmptyChatSurfaceSession(conversationId, sessionKey);

  return {
    ...surfaceSession,
    conversation: timeline,
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
  const compatibilitySession = state.sessionsByConversationId[conversationId] ?? createEmptyChatSession(conversationId);
  const nextCompatibilitySession = isDefaultSession
    ? nextSession
    : {
      ...compatibilitySession,
      conversation: nextSession.conversation,
    };

  return {
    sessionsByConversationId: {
      ...state.sessionsByConversationId,
      [conversationId]: nextCompatibilitySession,
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
