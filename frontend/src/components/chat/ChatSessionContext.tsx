import type { TimelineWindowMetadata } from '../../types/chatTimelineEvents';
import React, { createContext, useCallback, useContext, useEffect, useMemo, useRef } from 'react';
import {
  type ConversationInfo,
  type ChatSurfaceSession,
  useChatStore,
} from '../../store/chatStore';
import {
  buildChatSessionKey,
  createChatSurfaceOrigin,
  createEmptyChatSurfaceSession,
  getConversationInfo,
  normalizeChatSurfaceOrigin,
  type ChatSurfaceIdentity,
  type ChatSurfaceOrigin,
} from '../../services/chatSessionRegistry';
import { useChatTimelinePresentation } from './useChatTimelinePresentation';
import type { ChatTimelinePresentation } from '../../services/chatTimelinePresentation';
import type { MediaFile } from '../../services/mediaService';

type ActiveConversation = ConversationInfo & { threadedMessages: import('../../lib/chatMessageTree').MessageNode[] };
type ChatConversationSession = ChatSurfaceSession & { conversation: ActiveConversation | null };

export interface ChatSessionContextValue {
  presentation: ChatTimelinePresentation | null;
  surface: ChatSurfaceIdentity;
  origin: ChatSurfaceOrigin;
  conversationId: string | null;
  session: ChatConversationSession | null;
  conversation: ActiveConversation | null;
  threadedMessages: NonNullable<ActiveConversation['threadedMessages']>;
  isLoading: boolean;
  messageWindow: TimelineWindowMetadata | undefined;
  isLoadingMessageWindow: boolean;
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
  loadMessageChildren: (messageId: string) => Promise<ActiveConversation['threadedMessages']>;
  retryMessageToConversation: ReturnType<typeof useChatStore.getState>['retryMessageToConversation'];
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
  sessionKey: string,
) => state.surfaceSessionsByKey[sessionKey];

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
    normalizedConversationId ? getConversationInfo(state, normalizedConversationId) : null
  ));
  const surfaceSession = useChatStore((state) => (
    normalizedConversationId ? state.surfaceSessionsByKey?.[sessionKey] ?? null : null
  ));
  const { presentation, nodes: threadedMessages, revision } = useChatTimelinePresentation(normalizedConversationId, sessionKey);
  const window = presentation?.window;
  const session = useMemo(() => {
    if (!normalizedConversationId) return null;
    const state = surfaceSession ?? createEmptyChatSurfaceSession(normalizedConversationId, sessionKey);
    return { ...state, conversation: window ? { id: normalizedConversationId, title: timeline?.title ?? '', threadedMessages } : null };
  }, [normalizedConversationId, sessionKey, surfaceSession, threadedMessages, timeline?.title, window]);
  const conversation = session?.conversation ?? null;
  const isLoading = !!session?.isLoading;
  const messageWindow = window?.metadata;
  const isLoadingMessageWindow = presentation?.loading ?? false;
  const draftMessage = session?.draftMessage ?? '';
  const draftMediaFiles = session?.draftMediaFiles ?? [];
  const scrollTop = session?.scrollTop ?? 0;
  const scrollAnchorMessageId = session?.scrollAnchorMessageId ?? null;

  const loadMessageChildren = useCallback(async (messageId: string) => {
    const target = presentation?.getMessage(messageId)?.timelineItem?.representativeMessageId;
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
      const target = presentation?.getMessage(messageId)?.timelineItem?.representativeMessageId;
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
    messageWindow,
    isLoadingMessageWindow,
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
    startConversationEditing,
    setConversationEditingMessageId,
    toggleConversationThreadExpanded,
    toggleConversationReasoningExpanded,
    isConversationReasoningExpanded,
  }), [
    presentation, revision,
    clearDraft,
    conversation,
    draftMediaFiles,
    draftMessage,
    isConversationReasoningExpanded,
    isLoading,
    messageWindow,
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
    conversationId ? getStoredNodeSession(state, sessionKey)?.editingMessageId ?? null : null
  ));
  const isExpanded = useChatStore((state) => (
    !!conversationId && !!getStoredNodeSession(state, sessionKey)?.expandedThreads?.has(messageId)
  ));
  const reasoningExpanded = useChatStore((state) => (
    !!conversationId && !!getStoredNodeSession(state, sessionKey)?.expandedReasonings?.has(messageId)
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
