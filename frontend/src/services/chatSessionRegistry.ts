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

export interface ChatSurfaceSession {
  sessionKey?: string;
  conversationId?: string | null;
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
  const surfaceSession = state.surfaceSessionsByKey?.[sessionKey]
    ?? (compatibilitySession ? toSurfaceSession(compatibilitySession, conversationId, sessionKey) : null)
    ?? createEmptyChatSurfaceSession(conversationId, sessionKey);

  return {
    ...surfaceSession,
    conversation: timeline,
  };
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

  const timelinesByConversationId = { ...(state.timelinesByConversationId ?? {}) };
  if (nextSession.conversation) {
    timelinesByConversationId[conversationId] = nextSession.conversation;
  } else {
    delete timelinesByConversationId[conversationId];
  }

  return {
    sessionsByConversationId: {
      ...state.sessionsByConversationId,
      [conversationId]: nextSession,
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
  const session = getChatSession(state, conversationId);
  if (!session.conversation) return state;
  return patchChatSession(state, conversationId, {
    conversation: updater(session.conversation),
  }) as Partial<TState>;
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
