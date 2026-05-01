import React, { createContext, useCallback, useContext, useEffect, useMemo, useState } from 'react';
import { useWorkspacePanel } from '../workspace/WorkspacePanelContext';
import {
  type ActiveConversation,
  type ChatConversationSession,
  type Message,
  type TurnSegment,
  useChatStore,
} from '../../store/chatStore';
import { getChatSession } from '../../services/chatSessionRegistry';
import type { ToolCallStatus } from './ToolCallsSection';

const EMPTY_MESSAGES: never[] = [];

export type ChatSurfaceType = 'page' | 'embedded' | 'modal' | 'external';
export type ChatSessionKey = string;

export interface ChatSurfaceOrigin {
  sessionKey: ChatSessionKey;
  conversationId: string | null;
  tabId?: string;
  surfaceId: string;
  surfaceType: ChatSurfaceType;
}

export interface ChatSessionContextValue {
  origin: ChatSurfaceOrigin;
  conversationId: string | null;
  session: ChatConversationSession | null;
  conversation: ActiveConversation | null;
  threadedMessages: NonNullable<ActiveConversation['threadedMessages']>;
  isLoading: boolean;
  hasOlderMessages: boolean;
  isLoadingOlderMessages: boolean;
  loadOlderMessages: () => Promise<void>;
  loadConversationSession: (conversationId: string, options?: { activate?: boolean }) => Promise<void>;
  loadMessageChildren: ReturnType<typeof useChatStore.getState>['loadMessageChildren'];
  retryMessageToConversation: ReturnType<typeof useChatStore.getState>['retryMessageToConversation'];
  updateConversationMessage: ReturnType<typeof useChatStore.getState>['updateConversationMessage'];
  clearConversationMessages: ReturnType<typeof useChatStore.getState>['clearConversationMessages'];
  startConversationEditing: ReturnType<typeof useChatStore.getState>['startConversationEditing'];
  startConversationReading: ReturnType<typeof useChatStore.getState>['startConversationReading'];
  setConversationEditingMessageId: ReturnType<typeof useChatStore.getState>['setConversationEditingMessageId'];
  setConversationReadingMessageId: ReturnType<typeof useChatStore.getState>['setConversationReadingMessageId'];
  toggleConversationThreadExpanded: ReturnType<typeof useChatStore.getState>['toggleConversationThreadExpanded'];
  toggleConversationReasoningExpanded: ReturnType<typeof useChatStore.getState>['toggleConversationReasoningExpanded'];
  isConversationReasoningExpanded: ReturnType<typeof useChatStore.getState>['isConversationReasoningExpanded'];
}

const ChatSessionContext = createContext<ChatSessionContextValue | null>(null);

const buildSessionKey = (surfaceId: string, conversationId: string | null): ChatSessionKey => (
  `${surfaceId}:${conversationId || 'none'}`
);

export interface ChatSessionProviderProps {
  conversationId?: string | null;
  surfaceType?: ChatSurfaceType;
  children: React.ReactNode;
}

export function ChatSessionProvider({
  conversationId = null,
  surfaceType = 'page',
  children,
}: ChatSessionProviderProps) {
  const { tab } = useWorkspacePanel();
  const normalizedConversationId = conversationId || null;
  const tabId = tab?.id;
  const surfaceId = tabId || normalizedConversationId || `${surfaceType}:standalone`;
  const sessionKey = buildSessionKey(surfaceId, normalizedConversationId);

  const session = useChatStore((state) => (
    normalizedConversationId ? getChatSession(state, normalizedConversationId, sessionKey) : null
  ));
  const conversation = session?.conversation ?? null;
  const threadedMessages = conversation?.threadedMessages ?? EMPTY_MESSAGES;
  const isLoading = session?.isLoading ?? false;
  const hasOlderMessages = session?.hasOlderMessages ?? false;
  const isLoadingOlderMessages = session?.isLoadingOlderMessages ?? false;

  const loadOlderMessagesForConversation = useChatStore((state) => state.loadOlderMessagesForConversation);
  const loadMessageChildren = useChatStore((state) => state.loadMessageChildren);
  const loadConversationSession = useChatStore((state) => state.loadConversationSession);
  const retryMessageToConversation = useChatStore((state) => state.retryMessageToConversation);
  const updateConversationMessage = useChatStore((state) => state.updateConversationMessage);
  const clearConversationMessages = useChatStore((state) => state.clearConversationMessages);
  const startConversationEditing = useChatStore((state) => state.startConversationEditing);
  const startConversationReading = useChatStore((state) => state.startConversationReading);
  const setConversationEditingMessageId = useChatStore((state) => state.setConversationEditingMessageId);
  const setConversationReadingMessageId = useChatStore((state) => state.setConversationReadingMessageId);
  const toggleConversationThreadExpanded = useChatStore((state) => state.toggleConversationThreadExpanded);
  const toggleConversationReasoningExpanded = useChatStore((state) => state.toggleConversationReasoningExpanded);
  const isConversationReasoningExpanded = useChatStore((state) => state.isConversationReasoningExpanded);

  const loadOlderMessages = useCallback(async () => {
    if (normalizedConversationId) {
      await loadOlderMessagesForConversation(normalizedConversationId);
    }
  }, [loadOlderMessagesForConversation, normalizedConversationId]);

  const value = useMemo<ChatSessionContextValue>(() => ({
    origin: {
      sessionKey,
      conversationId: normalizedConversationId,
      tabId,
      surfaceId,
      surfaceType,
    },
    conversationId: normalizedConversationId,
    session,
    conversation,
    threadedMessages,
    isLoading,
    hasOlderMessages,
    isLoadingOlderMessages,
    loadOlderMessages,
    loadConversationSession,
    loadMessageChildren,
    retryMessageToConversation,
    updateConversationMessage,
    clearConversationMessages,
    startConversationEditing,
    startConversationReading,
    setConversationEditingMessageId,
    setConversationReadingMessageId,
    toggleConversationThreadExpanded,
    toggleConversationReasoningExpanded,
    isConversationReasoningExpanded,
  }), [
    clearConversationMessages,
    conversation,
    hasOlderMessages,
    isConversationReasoningExpanded,
    isLoading,
    isLoadingOlderMessages,
    loadConversationSession,
    loadMessageChildren,
    loadOlderMessages,
    normalizedConversationId,
    retryMessageToConversation,
    session,
    sessionKey,
    setConversationEditingMessageId,
    setConversationReadingMessageId,
    startConversationEditing,
    startConversationReading,
    surfaceId,
    surfaceType,
    tabId,
    threadedMessages,
    toggleConversationReasoningExpanded,
    toggleConversationThreadExpanded,
    updateConversationMessage,
  ]);

  return (
    <ChatSessionContext.Provider value={value}>
      {children}
    </ChatSessionContext.Provider>
  );
}

export function useChatSession(): ChatSessionContextValue {
  const context = useContext(ChatSessionContext);
  if (!context) {
    throw new Error('useChatSession must be used within ChatSessionProvider');
  }
  return context;
}

export function useOptionalChatSession(): ChatSessionContextValue | null {
  return useContext(ChatSessionContext);
}

export function useChatNodeSessionState(fallbackConversationId: string, messageId: string) {
  const context = useOptionalChatSession();
  const conversationId = context?.conversationId || fallbackConversationId;
  const session = useChatStore((state) => (
    conversationId ? getChatSession(state, conversationId) : null
  ));
  const isExpanded = useChatStore(
    useCallback((state) => {
      const scopedSession = conversationId ? getChatSession(state, conversationId) : null;
      return scopedSession?.expandedThreads.has(messageId) ?? false;
    }, [conversationId, messageId]),
  );
  const reasoningExpanded = useChatStore(
    useCallback((state) => {
      const scopedSession = conversationId ? getChatSession(state, conversationId) : null;
      return scopedSession?.expandedReasonings.has(messageId) ?? false;
    }, [conversationId, messageId]),
  );
  const setConversationEditingMessageId = useChatStore((state) => state.setConversationEditingMessageId);
  const setConversationReadingMessageId = useChatStore((state) => state.setConversationReadingMessageId);
  const toggleConversationThreadExpanded = useChatStore((state) => state.toggleConversationThreadExpanded);
  const toggleConversationReasoningExpanded = useChatStore((state) => state.toggleConversationReasoningExpanded);

  return {
    conversationId,
    editingMessageId: session?.editingMessageId ?? null,
    readingMessageId: session?.readingMessageId ?? null,
    streamingMessageId: session?.streamingMessageId ?? null,
    streamingReasoning: session?.streamingReasoning ?? null,
    isThinking: session?.isThinking ?? false,
    activeToolCalls: session?.activeToolCalls ?? [],
    completedSegments: session?.completedSegments ?? [],
    isExpanded,
    reasoningExpanded,
    setConversationEditingMessageId: context?.setConversationEditingMessageId ?? setConversationEditingMessageId,
    setConversationReadingMessageId: context?.setConversationReadingMessageId ?? setConversationReadingMessageId,
    toggleConversationThreadExpanded: context?.toggleConversationThreadExpanded ?? toggleConversationThreadExpanded,
    toggleConversationReasoningExpanded: context?.toggleConversationReasoningExpanded ?? toggleConversationReasoningExpanded,
  };
}

export function useChatMessageLiveState(message: Message) {
  const context = useOptionalChatSession();
  const messageId = message.id;
  const messageConversationId = context?.conversationId || String(message.conversationId || '');
  const { isStreaming } = message;
  const [liveContent, setLiveContent] = useState<string | null>(null);
  const [liveIsStreaming, setLiveIsStreaming] = useState<boolean | null>(null);
  const [liveReasoning, setLiveReasoning] = useState<string | null>(null);
  const [liveToolCallsRaw, setLiveToolCallsRaw] = useState<string | null>(null);
  const [liveSegments, setLiveSegments] = useState<TurnSegment[]>([]);
  const [liveToolCalls, setLiveToolCalls] = useState<ToolCallStatus[]>([]);

  useEffect(() => {
    const unsub = useChatStore.subscribe((state) => {
      const session = messageConversationId ? getChatSession(state, messageConversationId) : null;
      const streamingMessageId = session?.streamingMessageId ?? null;
      const completedSegments = session?.completedSegments ?? [];
      const activeToolCalls = session?.activeToolCalls ?? [];

      if (streamingMessageId === messageId) {
        setLiveSegments((prev) => (prev !== completedSegments ? completedSegments : prev));
        setLiveToolCalls((prev) => (prev !== activeToolCalls ? activeToolCalls : prev));
      } else {
        setLiveSegments((prev) => (prev.length > 0 ? [] : prev));
        setLiveToolCalls((prev) => (prev.length > 0 ? [] : prev));
      }
    });

    const initial = useChatStore.getState();
    const initialSession = messageConversationId ? getChatSession(initial, messageConversationId) : null;
    const initialStreamingMessageId = initialSession?.streamingMessageId ?? null;
    if (initialStreamingMessageId === messageId) {
      setLiveSegments(initialSession?.completedSegments ?? []);
      setLiveToolCalls(initialSession?.activeToolCalls ?? []);
    }

    return unsub;
  }, [messageConversationId, messageId]);

  useEffect(() => {
    const trackingRef = { current: !!isStreaming };

    const findMessageInState = (state: ReturnType<typeof useChatStore.getState>) => {
      const targetId = String(messageId);
      type ThreadedNode = {
        message?: { id?: string | number; content?: string; isStreaming?: boolean; reasoning?: string; toolCalls?: string | null };
        children?: ThreadedNode[];
      };
      const visit = (nodes: ThreadedNode[]): ThreadedNode['message'] | null => {
        for (const node of nodes || []) {
          const msg = node?.message;
          if (msg && String(msg.id) === targetId) return msg;
          if (node?.children?.length) {
            const hit = visit(node.children);
            if (hit) return hit;
          }
        }
        return null;
      };

      const conv = messageConversationId
        ? getChatSession(state, messageConversationId).conversation
        : null;
      if (conv?.threadedMessages) {
        const hit = visit(conv.threadedMessages as ThreadedNode[]);
        if (hit) return hit;
      }
      return null;
    };

    const sync = (state: ReturnType<typeof useChatStore.getState>) => {
      const msg = findMessageInState(state);
      if (!msg) return;

      const nextContent = typeof msg.content === 'string' ? msg.content : '';
      const nextIsStreaming = !!msg.isStreaming;
      const nextReasoning = typeof msg.reasoning === 'string' ? msg.reasoning : '';
      const nextToolCalls = typeof msg.toolCalls === 'string' ? msg.toolCalls : null;

      setLiveContent((prev) => (prev === nextContent ? prev : nextContent));
      setLiveIsStreaming((prev) => (prev === nextIsStreaming ? prev : nextIsStreaming));
      setLiveReasoning((prev) => (prev === nextReasoning ? prev : nextReasoning));
      setLiveToolCallsRaw((prev) => (prev === nextToolCalls ? prev : nextToolCalls));

      const session = messageConversationId ? getChatSession(state, messageConversationId) : null;
      const streamingMessageId = session?.streamingMessageId ?? null;
      if (!nextIsStreaming && streamingMessageId !== messageId) {
        trackingRef.current = false;
      }
    };

    sync(useChatStore.getState());

    const unsub = useChatStore.subscribe((state) => {
      const session = messageConversationId ? getChatSession(state, messageConversationId) : null;
      const streamingMessageId = session?.streamingMessageId ?? null;
      if (streamingMessageId === messageId) trackingRef.current = true;
      if (!trackingRef.current) return;
      sync(state);
    });

    return unsub;
  }, [messageConversationId, messageId, isStreaming]);

  return {
    liveContent,
    liveIsStreaming,
    liveReasoning,
    liveToolCallsRaw,
    liveSegments,
    liveToolCalls,
  };
}
