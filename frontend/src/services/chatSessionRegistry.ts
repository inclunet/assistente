import type { MediaFile } from './mediaService';

export interface ConversationInfo {
  id: string;
  title: string;
  channel?: string;
  contactId?: string;
}

export type ChatSessionKey = string;
export type ChatSurfaceType = 'page' | 'embedded' | 'modal' | 'external';

export interface ChatSurfaceOrigin {
  /** Identifica este envio/retry, sem criar uma mensagem local. */
  executionId?: string;
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
  sendFailureMessage: string | null;
  sendFailureAnnounced: boolean;
  sendFailureRetryable: boolean;
  sendFailureRetryContent: string | null;
  sendFailureRetryMediaFiles: MediaFile[];
  draftMessage: string;
  draftMediaFiles: MediaFile[];
  scrollTop: number;
  scrollAnchorMessageId: string | null;
  expandedThreads: Set<string>;
  expandedReasonings: Set<string>;
  editingMessageId: string | null;
  skipFocusRestore: boolean;
}

export interface ChatConversationSession extends ChatSurfaceSession {
  conversation: ConversationInfo | null;
}

export interface ChatSessionRegistryState {
  conversationInfoById: Record<string, ConversationInfo>;
  surfaceSessionsByKey: Record<string, ChatSurfaceSession>;
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
  sendFailureMessage: null,
  sendFailureAnnounced: false,
  sendFailureRetryable: false,
  sendFailureRetryContent: null,
  sendFailureRetryMediaFiles: [],
  draftMessage: '',
  draftMediaFiles: [],
  scrollTop: 0,
  scrollAnchorMessageId: null,
  expandedThreads: new Set<string>(),
  expandedReasonings: new Set<string>(),
  editingMessageId: null,
  skipFocusRestore: false,
});

export function getChatSession(
  state: ChatSessionRegistryState,
  conversationId: string,
  sessionKey = getDefaultChatSessionKey(conversationId),
): ChatConversationSession {
  return {
    ...(state.surfaceSessionsByKey[sessionKey] ?? createEmptyChatSurfaceSession(conversationId, sessionKey)),
    conversation: getConversationInfo(state, conversationId),
  };
}

export function getConversationInfo(
  state: ChatSessionRegistryState,
  conversationId: string,
): ConversationInfo | null {
  return state.conversationInfoById[conversationId] ?? null;
}

export function getChatSurfaceSessionsForConversation(
  state: ChatSessionRegistryState,
  conversationId: string,
): ChatSurfaceSession[] {
  return Object.values(state.surfaceSessionsByKey).filter(session => session.conversationId === conversationId);
}

export function patchChatSession(
  state: ChatSessionRegistryState,
  conversationId: string,
  patch: Partial<ChatSurfaceSession> | ((session: ChatSurfaceSession) => ChatSurfaceSession),
  sessionKey = getDefaultChatSessionKey(conversationId),
): Pick<ChatSessionRegistryState, 'surfaceSessionsByKey'> {
  const current = state.surfaceSessionsByKey[sessionKey] ?? createEmptyChatSurfaceSession(conversationId, sessionKey);
  const next = typeof patch === 'function' ? patch(current) : { ...current, ...patch };
  return {
    surfaceSessionsByKey: {
      ...state.surfaceSessionsByKey,
      [sessionKey]: { ...next, sessionKey, conversationId },
    },
  };
}

export function patchChatConversation(
  state: ChatSessionRegistryState,
  conversationId: string,
  updater: (conversation: ConversationInfo) => ConversationInfo,
): Pick<ChatSessionRegistryState, 'conversationInfoById'> {
  const conversation = getConversationInfo(state, conversationId);
  return {
    conversationInfoById: conversation
      ? { ...state.conversationInfoById, [conversationId]: updater(conversation) }
      : state.conversationInfoById,
  };
}

export function removeChatSession(state: ChatSessionRegistryState, conversationId: string): ChatSessionRegistryState {
  const conversationInfoById = { ...state.conversationInfoById };
  delete conversationInfoById[conversationId];
  const surfaceSessionsByKey = Object.fromEntries(
    Object.entries(state.surfaceSessionsByKey).filter(([, session]) => session.conversationId !== conversationId),
  );
  return { conversationInfoById, surfaceSessionsByKey };
}
