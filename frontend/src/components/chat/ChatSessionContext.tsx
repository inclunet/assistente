import React, { createContext, useCallback, useContext, useEffect, useMemo, useRef, useState } from 'react';
import {
  type ActiveConversation,
  type ChatConversationSession,
  type Message,
  type TurnSegment,
  useChatStore,
} from '../../store/chatStore';
import {
  buildChatSessionKey,
  createChatSurfaceOrigin,
  createEmptyChatSession,
  getDefaultChatConversationSession,
  getConversationTimeline,
  getChatSession,
  normalizeChatSurfaceOrigin,
  type ChatSurfaceIdentity,
  type ChatSurfaceSession,
  type ChatSurfaceOrigin,
  type ConversationTimeline,
} from '../../services/chatSessionRegistry';
import type { ToolCallStatus } from '../../types/chat';
import type { MediaFile } from '../../services/mediaService';

const EMPTY_MESSAGES: never[] = [];

const composeChatSession = (
  conversationId: string,
  sessionKey: string,
  defaultSession: ChatConversationSession | null,
  timeline: ConversationTimeline | null,
  surfaceSession: ChatSurfaceSession | null,
): ChatConversationSession => {
  const session = surfaceSession ?? createEmptyChatSession(conversationId, sessionKey);
  const baseConversation = timeline ?? defaultSession?.conversation ?? null;
  return {
    ...session,
    sessionKey: session.sessionKey ?? sessionKey,
    conversationId: session.conversationId ?? conversationId,
    conversation: baseConversation
      ? {
        ...baseConversation,
        threadedMessages: session.visibleThreadedMessages ?? baseConversation.threadedMessages,
      }
      : null,
  };
};

export interface ChatSessionContextValue {
  surface: ChatSurfaceIdentity;
  origin: ChatSurfaceOrigin;
  conversationId: string | null;
  session: ChatConversationSession | null;
  conversation: ActiveConversation | null;
  threadedMessages: NonNullable<ActiveConversation['threadedMessages']>;
  isLoading: boolean;
  hasOlderMessages: boolean;
  isLoadingOlderMessages: boolean;
  hasNewerMessages: boolean;
  draftMessage: string;
  draftMediaFiles: MediaFile[];
  scrollTop: number;
  scrollAnchorMessageId: string | null;
  setDraftMessage: (message: string) => void;
  setDraftMediaFiles: (mediaFiles: MediaFile[]) => void;
  clearDraft: () => void;
  setScrollState: (scrollState: { scrollTop: number; scrollAnchorMessageId: string | null }) => void;
  loadOlderMessages: () => Promise<void>;
  loadNewerMessages: () => Promise<void>;
  loadStartMessages: () => Promise<void>;
  loadEndMessages: () => Promise<void>;
  loadConversationSession: (conversationId: string) => Promise<void>;
  loadMessageChildren: ReturnType<typeof useChatStore.getState>['loadMessageChildren'];
  retryMessageToConversation: ReturnType<typeof useChatStore.getState>['retryMessageToConversation'];
  updateConversationMessage: ReturnType<typeof useChatStore.getState>['updateConversationMessage'];
  clearConversationMessages: ReturnType<typeof useChatStore.getState>['clearConversationMessages'];
  startConversationEditing: (conversationId: string, id: string) => void;
  startConversationReading: (conversationId: string, id: string) => void;
  setConversationEditingMessageId: (conversationId: string, id: string | null) => void;
  setConversationReadingMessageId: (conversationId: string, id: string | null) => void;
  toggleConversationThreadExpanded: (conversationId: string, messageId: string) => void;
  toggleConversationReasoningExpanded: (conversationId: string, messageId: string) => void;
  isConversationReasoningExpanded: (conversationId: string, messageId: string) => boolean;
}

const ChatSessionContext = createContext<ChatSessionContextValue | null>(null);

export interface ChatSessionProviderProps {
  surface: ChatSurfaceIdentity;
  children: React.ReactNode;
}

export function ChatSessionProvider({
  surface,
  children,
}: ChatSessionProviderProps) {
  const surfaceIdentity = surface;
  const normalizedConversationId = surfaceIdentity.conversationId;
  const sessionKey = surfaceIdentity.sessionKey;
  const surfaceId = surfaceIdentity.surfaceId;

  const defaultSession = useChatStore((state) => (
    normalizedConversationId ? getDefaultChatConversationSession(state, normalizedConversationId) : null
  ));
  const timeline = useChatStore((state) => (
    normalizedConversationId ? getConversationTimeline(state, normalizedConversationId) : null
  ));
  const surfaceSession = useChatStore((state) => (
    normalizedConversationId ? state.surfaceSessionsByKey?.[sessionKey] ?? null : null
  ));
  const session = useMemo(() => (
    normalizedConversationId
      ? composeChatSession(normalizedConversationId, sessionKey, defaultSession, timeline, surfaceSession)
      : null
  ), [defaultSession, normalizedConversationId, sessionKey, surfaceSession, timeline]);
  const conversation = session?.conversation ?? null;
  const threadedMessages = conversation?.threadedMessages ?? EMPTY_MESSAGES;
  const isLoading = session?.isLoading ?? false;
  const hasOlderMessages = session?.hasOlderMessages ?? false;
  const isLoadingOlderMessages = session?.isLoadingOlderMessages ?? false;
  const hasNewerMessages = session?.messageWindow?.hasAfter ?? false;
  const draftMessage = session?.draftMessage ?? '';
  const draftMediaFiles = session?.draftMediaFiles ?? [];
  const scrollTop = session?.scrollTop ?? 0;
  const scrollAnchorMessageId = session?.scrollAnchorMessageId ?? null;

  const loadOlderMessagesForConversation = useChatStore((state) => state.loadOlderMessagesForConversation);
  const loadNewerMessagesForConversation = useChatStore((state) => state.loadNewerMessagesForConversation);
  const loadBoundaryMessagesForConversation = useChatStore((state) => state.loadBoundaryMessagesForConversation);
  const loadMessageChildren = useChatStore((state) => state.loadMessageChildren);
  const loadConversationSession = useChatStore((state) => state.loadConversationSession);
  const retryMessageToConversationBase = useChatStore((state) => state.retryMessageToConversation);
  const ensureConversationSurfaceSession = useChatStore((state) => state.ensureConversationSurfaceSession);
  const removeConversationSurfaceSession = useChatStore((state) => state.removeConversationSurfaceSession);
  const updateConversationMessage = useChatStore((state) => state.updateConversationMessage);
  const clearConversationMessages = useChatStore((state) => state.clearConversationMessages);
  const startConversationEditingBase = useChatStore((state) => state.startConversationEditing);
  const startConversationReadingBase = useChatStore((state) => state.startConversationReading);
  const setConversationDraftMessage = useChatStore((state) => state.setConversationDraftMessage);
  const setConversationDraftMediaFiles = useChatStore((state) => state.setConversationDraftMediaFiles);
  const clearConversationDraft = useChatStore((state) => state.clearConversationDraft);
  const setConversationScrollState = useChatStore((state) => state.setConversationScrollState);
  const setConversationEditingMessageIdBase = useChatStore((state) => state.setConversationEditingMessageId);
  const setConversationReadingMessageIdBase = useChatStore((state) => state.setConversationReadingMessageId);
  const toggleConversationThreadExpandedBase = useChatStore((state) => state.toggleConversationThreadExpanded);
  const toggleConversationReasoningExpandedBase = useChatStore((state) => state.toggleConversationReasoningExpanded);
  const isConversationReasoningExpandedBase = useChatStore((state) => state.isConversationReasoningExpanded);

  const loadOlderMessages = useCallback(async () => {
    if (normalizedConversationId) {
      await loadOlderMessagesForConversation(normalizedConversationId, sessionKey);
    }
  }, [loadOlderMessagesForConversation, normalizedConversationId, sessionKey]);

  const loadNewerMessages = useCallback(async () => {
    if (normalizedConversationId) {
      await loadNewerMessagesForConversation(normalizedConversationId, sessionKey);
    }
  }, [loadNewerMessagesForConversation, normalizedConversationId, sessionKey]);

  const loadStartMessages = useCallback(async () => {
    if (normalizedConversationId) {
      await loadBoundaryMessagesForConversation(normalizedConversationId, sessionKey, 'start');
    }
  }, [loadBoundaryMessagesForConversation, normalizedConversationId, sessionKey]);

  const loadEndMessages = useCallback(async () => {
    if (normalizedConversationId) {
      await loadBoundaryMessagesForConversation(normalizedConversationId, sessionKey, 'end');
    }
  }, [loadBoundaryMessagesForConversation, normalizedConversationId, sessionKey]);

  const setDraftMessage = useCallback((message: string) => {
    if (!normalizedConversationId) return;
    setConversationDraftMessage(normalizedConversationId, message, sessionKey);
  }, [normalizedConversationId, sessionKey, setConversationDraftMessage]);

  const setDraftMediaFiles = useCallback((mediaFiles: MediaFile[]) => {
    if (!normalizedConversationId) return;
    setConversationDraftMediaFiles(normalizedConversationId, mediaFiles, sessionKey);
  }, [normalizedConversationId, sessionKey, setConversationDraftMediaFiles]);

  const clearDraft = useCallback(() => {
    if (!normalizedConversationId) return;
    clearConversationDraft(normalizedConversationId, sessionKey);
  }, [clearConversationDraft, normalizedConversationId, sessionKey]);

  const setScrollState = useCallback((scrollState: { scrollTop: number; scrollAnchorMessageId: string | null }) => {
    if (!normalizedConversationId) return;
    setConversationScrollState(normalizedConversationId, scrollState, sessionKey);
  }, [normalizedConversationId, sessionKey, setConversationScrollState]);

  const materializedSurfaceSessionKeysRef = useRef(new Set<string>());

  useEffect(() => {
    if (!normalizedConversationId || surfaceSession) return;
    ensureConversationSurfaceSession(normalizedConversationId, sessionKey, {
      ...createChatSurfaceOrigin(surfaceIdentity),
    });
    materializedSurfaceSessionKeysRef.current.add(sessionKey);
  }, [
    ensureConversationSurfaceSession,
    normalizedConversationId,
    sessionKey,
    surfaceId,
    surfaceSession,
    surfaceIdentity,
  ]);

  useEffect(() => (
    () => {
      if (!materializedSurfaceSessionKeysRef.current.has(sessionKey)) return;
      materializedSurfaceSessionKeysRef.current.delete(sessionKey);
      removeConversationSurfaceSession(sessionKey);
    }
  ), [removeConversationSurfaceSession, sessionKey]);

  const buildOriginForConversation = useCallback((
    targetConversationId: string,
    providedOrigin?: ChatSurfaceOrigin,
  ): ChatSurfaceOrigin => {
    const fallbackOrigin: ChatSurfaceOrigin = {
      sessionKey: targetConversationId === normalizedConversationId
        ? sessionKey
        : buildChatSessionKey(surfaceId, targetConversationId),
      conversationId: targetConversationId,
      tabId: surfaceIdentity.tabId,
      surfaceId,
      surfaceType: surfaceIdentity.surfaceType,
    };

    return normalizeChatSurfaceOrigin(providedOrigin ?? fallbackOrigin, targetConversationId) ?? fallbackOrigin;
  }, [normalizedConversationId, sessionKey, surfaceId, surfaceIdentity.surfaceType, surfaceIdentity.tabId]);

  const retryMessageToConversation = useCallback<ChatSessionContextValue['retryMessageToConversation']>(
    (targetConversationId, messageId, paramsOverride, options) => (
      retryMessageToConversationBase(targetConversationId, messageId, paramsOverride, {
        ...options,
        origin: buildOriginForConversation(targetConversationId, options?.origin),
      })
    ),
    [buildOriginForConversation, retryMessageToConversationBase],
  );

  const startConversationEditing = useCallback<ChatSessionContextValue['startConversationEditing']>(
    (targetConversationId, id) => startConversationEditingBase(targetConversationId, id, sessionKey),
    [sessionKey, startConversationEditingBase],
  );

  const startConversationReading = useCallback<ChatSessionContextValue['startConversationReading']>(
    (targetConversationId, id) => startConversationReadingBase(targetConversationId, id, sessionKey),
    [sessionKey, startConversationReadingBase],
  );

  const setConversationEditingMessageId = useCallback<ChatSessionContextValue['setConversationEditingMessageId']>(
    (targetConversationId, id) => setConversationEditingMessageIdBase(targetConversationId, id, sessionKey),
    [sessionKey, setConversationEditingMessageIdBase],
  );

  const setConversationReadingMessageId = useCallback<ChatSessionContextValue['setConversationReadingMessageId']>(
    (targetConversationId, id) => setConversationReadingMessageIdBase(targetConversationId, id, sessionKey),
    [sessionKey, setConversationReadingMessageIdBase],
  );

  const toggleConversationThreadExpanded = useCallback<ChatSessionContextValue['toggleConversationThreadExpanded']>(
    (targetConversationId, messageId) => toggleConversationThreadExpandedBase(targetConversationId, messageId, sessionKey),
    [sessionKey, toggleConversationThreadExpandedBase],
  );

  const toggleConversationReasoningExpanded = useCallback<ChatSessionContextValue['toggleConversationReasoningExpanded']>(
    (targetConversationId, messageId) => toggleConversationReasoningExpandedBase(targetConversationId, messageId, sessionKey),
    [sessionKey, toggleConversationReasoningExpandedBase],
  );

  const isConversationReasoningExpanded = useCallback<ChatSessionContextValue['isConversationReasoningExpanded']>(
    (targetConversationId, messageId) => isConversationReasoningExpandedBase(targetConversationId, messageId, sessionKey),
    [isConversationReasoningExpandedBase, sessionKey],
  );

  const value = useMemo<ChatSessionContextValue>(() => ({
    surface: surfaceIdentity,
    origin: createChatSurfaceOrigin(surfaceIdentity),
    conversationId: normalizedConversationId,
    session,
    conversation,
    threadedMessages,
    isLoading,
    hasOlderMessages,
    isLoadingOlderMessages,
    hasNewerMessages,
    draftMessage,
    draftMediaFiles,
    scrollTop,
    scrollAnchorMessageId,
    setDraftMessage,
    setDraftMediaFiles,
    clearDraft,
    setScrollState,
    loadOlderMessages,
    loadNewerMessages,
    loadStartMessages,
    loadEndMessages,
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
    clearDraft,
    conversation,
    draftMediaFiles,
    draftMessage,
    hasOlderMessages,
    hasNewerMessages,
    isConversationReasoningExpanded,
    isLoading,
    isLoadingOlderMessages,
    loadConversationSession,
    loadEndMessages,
    loadMessageChildren,
    loadNewerMessages,
    loadOlderMessages,
    loadStartMessages,
    normalizedConversationId,
    retryMessageToConversation,
    session,
    sessionKey,
    scrollAnchorMessageId,
    scrollTop,
    setDraftMediaFiles,
    setDraftMessage,
    setScrollState,
    setConversationEditingMessageId,
    setConversationReadingMessageId,
    startConversationEditing,
    startConversationReading,
    surfaceId,
    surfaceIdentity,
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

export function useChatNodeSessionState(messageId: string) {
  const context = useChatSession();
  const conversationId = context.conversationId;
  const defaultSession = useChatStore((state) => (
    conversationId ? getDefaultChatConversationSession(state, conversationId) : null
  ));
  const timeline = useChatStore((state) => (
    conversationId ? getConversationTimeline(state, conversationId) : null
  ));
  const surfaceSession = useChatStore((state) => {
    const sessionKey = context.origin.sessionKey;
    return state.surfaceSessionsByKey?.[sessionKey] ?? null;
  });
  const session = useMemo(() => (
    conversationId
      ? composeChatSession(
        conversationId,
        context.origin.sessionKey,
        defaultSession,
        timeline,
        surfaceSession,
      )
      : null
  ), [defaultSession, context.origin.sessionKey, conversationId, surfaceSession, timeline]);
  const isExpanded = session?.expandedThreads.has(messageId) ?? false;
  const reasoningExpanded = session?.expandedReasonings.has(messageId) ?? false;

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
    setConversationEditingMessageId: context.setConversationEditingMessageId,
    setConversationReadingMessageId: context.setConversationReadingMessageId,
    toggleConversationThreadExpanded: context.toggleConversationThreadExpanded,
    toggleConversationReasoningExpanded: context.toggleConversationReasoningExpanded,
  };
}

export function useChatMessageLiveState(message: Message) {
  const context = useOptionalChatSession();
  const messageId = message.id;
  const messageConversationId = context?.conversationId || String(message.conversationId || '');
  const sessionKey = context?.origin.sessionKey;
  const { isStreaming } = message;
  const [liveContent, setLiveContent] = useState<string | null>(null);
  const [liveIsStreaming, setLiveIsStreaming] = useState<boolean | null>(null);
  const [liveReasoning, setLiveReasoning] = useState<string | null>(null);
  const [liveToolCallsRaw, setLiveToolCallsRaw] = useState<string | null>(null);
  const [liveSegments, setLiveSegments] = useState<TurnSegment[]>([]);
  const [liveToolCalls, setLiveToolCalls] = useState<ToolCallStatus[]>([]);

  useEffect(() => {
    const unsub = useChatStore.subscribe((state) => {
      const session = messageConversationId ? getChatSession(state, messageConversationId, sessionKey) : null;
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
    const initialSession = messageConversationId ? getChatSession(initial, messageConversationId, sessionKey) : null;
    const initialStreamingMessageId = initialSession?.streamingMessageId ?? null;
    if (initialStreamingMessageId === messageId) {
      setLiveSegments(initialSession?.completedSegments ?? []);
      setLiveToolCalls(initialSession?.activeToolCalls ?? []);
    }

    return unsub;
  }, [messageConversationId, messageId, sessionKey]);

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
        ? getChatSession(state, messageConversationId, sessionKey).conversation
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

      const session = messageConversationId ? getChatSession(state, messageConversationId, sessionKey) : null;
      const streamingMessageId = session?.streamingMessageId ?? null;
      if (!nextIsStreaming && streamingMessageId !== messageId) {
        trackingRef.current = false;
      }
    };

    sync(useChatStore.getState());

    const unsub = useChatStore.subscribe((state) => {
      const session = messageConversationId ? getChatSession(state, messageConversationId, sessionKey) : null;
      const streamingMessageId = session?.streamingMessageId ?? null;
      if (streamingMessageId === messageId) trackingRef.current = true;
      if (!trackingRef.current) return;
      sync(state);
    });

    return unsub;
  }, [messageConversationId, messageId, isStreaming, sessionKey]);

  return {
    liveContent,
    liveIsStreaming,
    liveReasoning,
    liveToolCallsRaw,
    liveSegments,
    liveToolCalls,
  };
}
