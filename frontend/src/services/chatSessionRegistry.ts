import type { ToolCallStatus } from '../components/chat/ToolCallsSection';
import type { MessageNode, TurnSegment } from '../lib/chatMessageTree';

export interface ActiveConversation {
  id: string;
  title: string;
  threadedMessages: MessageNode[];
  channel?: string;
  contactId?: string;
}

export interface ChatConversationSession {
  conversation: ActiveConversation | null;
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

export interface ChatSessionRegistryState {
  sessionsByConversationId: Record<string, ChatConversationSession>;
}

export const createEmptyChatSession = (): ChatConversationSession => ({
  conversation: null,
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

export function getChatSession(
  state: ChatSessionRegistryState,
  conversationId: string,
): ChatConversationSession {
  return state.sessionsByConversationId[conversationId] ?? createEmptyChatSession();
}

export function patchChatSession<TState extends ChatSessionRegistryState>(
  state: TState,
  conversationId: string,
  patch: Partial<ChatConversationSession> | ((session: ChatConversationSession) => ChatConversationSession),
): Pick<TState, 'sessionsByConversationId'> {
  const currentSession = getChatSession(state, conversationId);
  const nextSession = typeof patch === 'function'
    ? patch(currentSession)
    : { ...currentSession, ...patch };

  return {
    sessionsByConversationId: {
      ...state.sessionsByConversationId,
      [conversationId]: nextSession,
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
): Pick<TState, 'sessionsByConversationId'> {
  const sessionsByConversationId = { ...state.sessionsByConversationId };
  delete sessionsByConversationId[conversationId];
  return { sessionsByConversationId };
}
