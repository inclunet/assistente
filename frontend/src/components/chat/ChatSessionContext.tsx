import React, { createContext, useCallback, useContext, useEffect, useMemo, useRef } from 'react';
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
  getConversationTimeline,
  normalizeChatSurfaceOrigin,
  type ChatSurfaceIdentity,
  type ChatSurfaceOrigin,
} from '../../services/chatSessionRegistry';
import { useChatTimelinePresentation } from './useChatTimelinePresentation';
import type { ChatTimelinePresentation } from '../../services/chatTimelinePresentation';
import type { MediaFile } from '../../services/mediaService';

const EMPTY_SEGMENTS: never[] = [];
const EMPTY_TOOL_CALLS: never[] = [];

export interface ChatSessionContextValue {
  presentation: ChatTimelinePresentation | null;
  surface: ChatSurfaceIdentity;
  origin: ChatSurfaceOrigin;
  conversationId: string | null;
  session: ChatConversationSession | null;
  conversation: ActiveConversation | null;
  threadedMessages: NonNullable<ActiveConversation['threadedMessages']>;
  isLoading: boolean;
  hasOlderMessages: boolean;
  isLoadingOlderMessages: boolean;
  isLoadingMessageWindow: boolean;
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
  loadConversationSession: ReturnType<typeof useChatStore.getState>['loadConversationSession'];
  loadMessageChildren: ReturnType<typeof useChatStore.getState>['loadMessageChildren'];
  retryMessageToConversation: ReturnType<typeof useChatStore.getState>['retryMessageToConversation'];
  updateConversationMessage: ReturnType<typeof useChatStore.getState>['updateConversationMessage'];
  updateConversationMessagePinned: ReturnType<typeof useChatStore.getState>['updateConversationMessagePinned'];
  clearConversationMessages: ReturnType<typeof useChatStore.getState>['clearConversationMessages'];
  startConversationEditing: (conversationId: string, id: string) => void;
  setConversationEditingMessageId: (conversationId: string, id: string | null) => void;
  toggleConversationThreadExpanded: (conversationId: string, messageId: string) => void;
  toggleConversationReasoningExpanded: (conversationId: string, messageId: string) => void;
  isConversationReasoningExpanded: (conversationId: string, messageId: string) => boolean;
}

const ChatTimelineContext = createContext<ChatTimelinePresentation | null>(null);
export const useChatTimelineSource = () => useContext(ChatTimelineContext);

const ChatSessionContext = createContext<ChatSessionContextValue | null>(null);
type ChatNodeContextValue = Pick<
  ChatSessionContextValue,
  | 'conversationId'
  | 'origin'
  | 'setConversationEditingMessageId'
  | 'toggleConversationThreadExpanded'
  | 'toggleConversationReasoningExpanded'
>;
const ChatNodeContext = createContext<ChatNodeContextValue | null>(null);
const getStoredNodeSession = (
  state: ReturnType<typeof useChatStore.getState>,
  conversationId: string,
  sessionKey: string,
) => state.surfaceSessionsByKey?.[sessionKey] ?? state.sessionsByConversationId[conversationId];

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

  const timeline = useChatStore((state) => (
    normalizedConversationId ? getConversationTimeline(state, normalizedConversationId) : null
  ));
  const surfaceSession = useChatStore((state) => (
    normalizedConversationId ? state.surfaceSessionsByKey?.[sessionKey] ?? null : null
  ));
  const { presentation, nodes: threadedMessages, revision } = useChatTimelinePresentation(normalizedConversationId, sessionKey);
  const window = presentation?.window;
  const session = useMemo(() => {
    if (!normalizedConversationId) return null;
    const state = surfaceSession ?? createEmptyChatSession(normalizedConversationId, sessionKey);
    return { ...state, conversation: window ? { id: normalizedConversationId, title: timeline?.title ?? '', threadedMessages } : null,
      messageWindow: window ? { scope: 'conversation' as const, conversationId: normalizedConversationId,
        totalCount: window.metadata.totalCount, startIndex: window.metadata.startIndex, endIndex: window.metadata.endIndex,
        hasBefore: window.metadata.hasBefore, hasAfter: window.metadata.hasAfter } : undefined };
  }, [normalizedConversationId, sessionKey, surfaceSession, threadedMessages, timeline?.title, window]);
  const conversation = session?.conversation ?? null;
  const isLoading = !!session?.isLoading;
  const hasOlderMessages = window?.metadata.hasBefore ?? false;
  const isLoadingOlderMessages = presentation?.loading ?? false;
  const isLoadingMessageWindow = presentation?.loading ?? false;
  const hasNewerMessages = window?.metadata.hasAfter ?? false;
  const draftMessage = session?.draftMessage ?? '';
  const draftMediaFiles = session?.draftMediaFiles ?? [];
  const scrollTop = session?.scrollTop ?? 0;
  const scrollAnchorMessageId = session?.scrollAnchorMessageId ?? null;

  const loadMessageChildren = useCallback(async (messageId: string) => {
    const target = presentation?.getMessage(messageId)?.timelineItem?.metadata.threadParentMessageId;
    if (!presentation || !target) return [];
    await presentation.openThread(target);
    return presentation.nodes(target);
  }, [presentation]);
  const loadConversationSession = useCallback(async (_id: string) => {
    if (presentation?.window) await presentation.refresh();
  }, [presentation]);
  const retryMessageToConversationBase = useChatStore((state) => state.retryMessageToConversation);
  const ensureConversationSurfaceSession = useChatStore((state) => state.ensureConversationSurfaceSession);
  const removeConversationSurfaceSession = useChatStore((state) => state.removeConversationSurfaceSession);
  const updateConversationMessage = useChatStore((state) => state.updateConversationMessage);
  const updateConversationMessagePinned = useChatStore((state) => state.updateConversationMessagePinned);
  const clearConversationMessages = useChatStore((state) => state.clearConversationMessages);
  const startConversationEditingBase = useChatStore((state) => state.startConversationEditing);
  const setConversationDraftMessage = useChatStore((state) => state.setConversationDraftMessage);
  const setConversationDraftMediaFiles = useChatStore((state) => state.setConversationDraftMediaFiles);
  const clearConversationDraft = useChatStore((state) => state.clearConversationDraft);
  const setConversationScrollState = useChatStore((state) => state.setConversationScrollState);
  const setConversationEditingMessageIdBase = useChatStore((state) => state.setConversationEditingMessageId);
  const toggleConversationThreadExpandedBase = useChatStore((state) => state.toggleConversationThreadExpanded);
  const toggleConversationReasoningExpandedBase = useChatStore((state) => state.toggleConversationReasoningExpanded);
  const isConversationReasoningExpandedBase = useChatStore((state) => state.isConversationReasoningExpanded);

  const loadOlderMessages = useCallback(async () => { await presentation?.navigate('older'); }, [presentation]);
  const loadNewerMessages = useCallback(async () => { await presentation?.navigate('newer'); }, [presentation]);
  const loadStartMessages = useCallback(async () => { await presentation?.navigate('start'); }, [presentation]);
  const loadEndMessages = useCallback(async () => { await presentation?.navigate('end'); }, [presentation]);

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

  const setConversationEditingMessageId = useCallback<ChatSessionContextValue['setConversationEditingMessageId']>(
    (targetConversationId, id) => setConversationEditingMessageIdBase(targetConversationId, id, sessionKey),
    [sessionKey, setConversationEditingMessageIdBase],
  );

  const toggleConversationThreadExpanded = useCallback<ChatSessionContextValue['toggleConversationThreadExpanded']>(
    (targetConversationId, messageId) => {
      const closing = useChatStore.getState().isConversationThreadExpanded(targetConversationId, messageId, sessionKey);
      toggleConversationThreadExpandedBase(targetConversationId, messageId, sessionKey);
      const target = presentation?.getMessage(messageId)?.timelineItem?.metadata.threadParentMessageId;
      if (closing && target) void presentation?.closeThread(target).catch(() => { /* Recuperação permanece visível na superfície. */ });
    },
    [sessionKey, toggleConversationThreadExpandedBase, presentation],
  );

  const toggleConversationReasoningExpanded = useCallback<ChatSessionContextValue['toggleConversationReasoningExpanded']>(
    (targetConversationId, messageId) => toggleConversationReasoningExpandedBase(targetConversationId, messageId, sessionKey),
    [sessionKey, toggleConversationReasoningExpandedBase],
  );

  const isConversationReasoningExpanded = useCallback<ChatSessionContextValue['isConversationReasoningExpanded']>(
    (targetConversationId, messageId) => isConversationReasoningExpandedBase(targetConversationId, messageId, sessionKey),
    [isConversationReasoningExpandedBase, sessionKey],
  );

  const origin = useMemo(() => createChatSurfaceOrigin({
    conversationId: normalizedConversationId,
    sessionKey,
    surfaceId,
    surfaceType: surfaceIdentity.surfaceType,
    tabId: surfaceIdentity.tabId,
  }), [
    normalizedConversationId,
    sessionKey,
    surfaceId,
    surfaceIdentity.surfaceType,
    surfaceIdentity.tabId,
  ]);

  const value = useMemo<ChatSessionContextValue>(() => ({
    presentation,
    surface: surfaceIdentity,
    origin,
    conversationId: normalizedConversationId,
    session,
    conversation,
    threadedMessages,
    isLoading,
    hasOlderMessages,
    isLoadingOlderMessages,
    isLoadingMessageWindow,
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
    updateConversationMessagePinned,
    clearConversationMessages,
    startConversationEditing,
    setConversationEditingMessageId,
    toggleConversationThreadExpanded,
    toggleConversationReasoningExpanded,
    isConversationReasoningExpanded,
  }), [
    presentation, revision,
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
    isLoadingMessageWindow,
    loadConversationSession,
    loadEndMessages,
    loadMessageChildren,
    loadNewerMessages,
    loadOlderMessages,
    loadStartMessages,
    normalizedConversationId,
    origin,
    retryMessageToConversation,
    session,
    sessionKey,
    scrollAnchorMessageId,
    scrollTop,
    setDraftMediaFiles,
    setDraftMessage,
    setScrollState,
    setConversationEditingMessageId,
    startConversationEditing,
    surfaceId,
    surfaceIdentity,
    threadedMessages,
    toggleConversationReasoningExpanded,
    toggleConversationThreadExpanded,
    updateConversationMessage,
    updateConversationMessagePinned,
  ]);
  const nodeValue = useMemo<ChatNodeContextValue>(() => ({
    conversationId: normalizedConversationId,
    origin,
    setConversationEditingMessageId,
    toggleConversationThreadExpanded,
    toggleConversationReasoningExpanded,
  }), [
    normalizedConversationId,
    origin,
    setConversationEditingMessageId,
    toggleConversationReasoningExpanded,
    toggleConversationThreadExpanded,
  ]);

  return (
    <ChatSessionContext.Provider value={value}>
      <ChatNodeContext.Provider value={nodeValue}>
        <ChatTimelineContext.Provider value={presentation}>{children}</ChatTimelineContext.Provider>
      </ChatNodeContext.Provider>
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
  const context = useContext(ChatNodeContext);
  if (!context) {
    throw new Error('useChatNodeSessionState must be used within ChatSessionProvider');
  }
  const conversationId = context.conversationId;
  const sessionKey = context.origin.sessionKey;
  const editingMessageId = useChatStore((state) => (
    conversationId ? getStoredNodeSession(state, conversationId, sessionKey)?.editingMessageId ?? null : null
  ));
  const isExpanded = useChatStore((state) => (
    !!conversationId && !!getStoredNodeSession(state, conversationId, sessionKey)?.expandedThreads?.has(messageId)
  ));
  const reasoningExpanded = useChatStore((state) => (
    !!conversationId && !!getStoredNodeSession(state, conversationId, sessionKey)?.expandedReasonings?.has(messageId)
  ));

  return {
    conversationId,
    editingMessageId,
    sessionKey,
    isExpanded,
    reasoningExpanded,
    setConversationEditingMessageId: context.setConversationEditingMessageId,
    toggleConversationThreadExpanded: context.toggleConversationThreadExpanded,
    toggleConversationReasoningExpanded: context.toggleConversationReasoningExpanded,
  };
}

export function useChatMessageLiveState(message: Message) {
  const nodeContext = useContext(ChatNodeContext);
  const messageId = message.id;
  const messageConversationId = nodeContext?.conversationId
    || String(message.conversationId || '');
  const liveContent = useChatStore((state) => (
    messageConversationId
      ? state.liveMessageContentByConversationId?.[messageConversationId]?.[messageId] ?? null
      : null
  ));

  return {
    liveContent,
    liveIsStreaming: null,
    liveReasoning: null,
    liveToolCallsRaw: null,
    liveSegments: EMPTY_SEGMENTS as TurnSegment[],
    liveToolCalls: EMPTY_TOOL_CALLS,
  };
}
