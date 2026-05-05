import { create } from 'zustand';
import {
  SendMessage,
  RetryMessage,
  EnsureConversation,
  AssignConversationToChannel,
  UnassignConversationFromChannel,
} from '@wailsjs/go/app/App';
import { MediaFile } from '../services/mediaService';
import { llm } from '../../wailsjs/go/models';
import { announce } from '../hooks/useAnnouncer';
import i18next from 'i18next';
import { playSendSound } from '../services/audioFeedback';
import { isChatConversationActive } from '../services/chatArbitration';
import { startChatEventController, stopAllChatEventControllers, stopChatEventController } from '../services/chatEventController';
import { handleExternalChatIncoming } from '../services/externalChatController';
import {
  createConversationTurnQueue,
  isConversationTurnQueueClearedError,
} from '../services/chatTurnQueue';
import {
  loadConversationBoundaryWindow,
  loadConversationSnapshot,
  loadMessageChildrenNodes,
  loadNewerConversationMessages,
  loadOlderConversationMessages,
  reloadConversationSnapshot,
} from '../services/chatSessionLoader';
import {
  createEmptyChatSurfaceSession,
  getChatSession,
  getConversationTimeline,
  patchChatConversation,
  patchChatSession,
  removeChatSession,
  type ActiveConversation,
  type ChatConversationSession,
  type ChatSurfaceSession,
  type ChatSurfaceOrigin,
  type ConversationTimeline,
  type MessageWindowState,
} from '../services/chatSessionRegistry';
import {
  appendInternalMessageToTree,
  attachChildrenToMessage,
  flattenThreadedMessages,
  hasMessageId,
  updateMessageContentInTree,
  updateMessageReasoningInTree,
  type Message,
  type MessageNode,
} from '../lib/chatMessageTree';

const MAX_MESSAGE_CONTENT_SIZE = 512 * 1024;       // must match backend MaxMessageContentSize
const MAX_MEDIA_SIZE = 20 * 1024 * 1024;            // must match backend MaxMediaSize
const INITIAL_MESSAGE_WINDOW_SIZE = 120;
const MAX_RENDERED_MESSAGE_WINDOW_SIZE = 240;

interface MediaData {
  name: string;
  type: string;
  data: string;
  size: number;
}

export type { Message, MessageNode, TurnSegment } from '../lib/chatMessageTree';
export type {
  ActiveConversation,
  ChatConversationSession,
  ChatSurfaceOrigin,
  ChatSurfaceSession,
  ConversationTimeline,
} from '../services/chatSessionRegistry';

const getErrorMessage = (error: unknown): string => {
  if (error instanceof Error) return error.message;
  if (typeof error === 'string') return error;
  try {
    return JSON.stringify(error);
  } catch {
    return String(error);
  }
};

const fileToBase64 = (file: File): Promise<string> => {
  return new Promise((resolve, reject) => {
    const reader = new FileReader();
    reader.onload = () => {
      const result = reader.result as string;
      const base64 = result.split(',')[1];
      resolve(base64);
    };
    reader.onerror = reject;
    reader.readAsDataURL(file);
  });
};

interface ChatStore {
  sessionsByConversationId: Record<string, ChatConversationSession>;
  timelinesByConversationId: Record<string, ConversationTimeline>;
  surfaceSessionsByKey: Record<string, ChatSurfaceSession>;
  loadingConversationIds: Set<string>;
  isInitialized: boolean;

  setConversationEditingMessageId: (conversationId: string, id: string | null, sessionKey: string) => void;
  startConversationEditing: (conversationId: string, id: string, sessionKey: string) => void;
  consumeSkipFocusRestore: (conversationId: string, sessionKey: string) => boolean;
  setConversationReadingMessageId: (conversationId: string, id: string | null, sessionKey: string) => void;
  startConversationReading: (conversationId: string, id: string, sessionKey: string) => void;
  setConversationDraftMessage: (conversationId: string, message: string, sessionKey: string) => void;
  setConversationDraftMediaFiles: (conversationId: string, mediaFiles: MediaFile[], sessionKey: string) => void;
  clearConversationDraft: (conversationId: string, sessionKey: string) => void;
  setConversationScrollState: (
    conversationId: string,
    scrollState: { scrollTop: number; scrollAnchorMessageId: string | null },
    sessionKey: string,
  ) => void;
  ensureConversationSurfaceSession: (conversationId: string, sessionKey: string, origin?: ChatSurfaceOrigin) => void;
  removeConversationSurfaceSession: (sessionKey: string) => void;

  createConversation: (title?: string) => Promise<string>;
  loadConversationSession: (id: string) => Promise<void>;
  getConversationSession: (conversationId: string | null | undefined) => ChatConversationSession | null;
  loadOlderMessagesForConversation: (conversationId: string, sessionKey: string) => Promise<void>;
  loadNewerMessagesForConversation: (conversationId: string, sessionKey: string) => Promise<void>;
  loadBoundaryMessagesForConversation: (conversationId: string, sessionKey: string, anchor: 'start' | 'end') => Promise<void>;

  updateConversationMessage: (conversationId: string, messageId: string, content: string) => void;
  updateConversationMessageReasoning: (conversationId: string, messageId: string, reasoning: string) => void;
  addInternalMessage: (message: Message) => void;
  clearConversationMessages: (conversationId: string) => void;

  toggleConversationThreadExpanded: (conversationId: string, messageId: string, sessionKey: string) => void;
  isConversationThreadExpanded: (conversationId: string, messageId: string, sessionKey: string) => boolean;
  toggleConversationReasoningExpanded: (conversationId: string, messageId: string, sessionKey: string) => void;
  isConversationReasoningExpanded: (conversationId: string, messageId: string, sessionKey: string) => boolean;

  sendMessageToConversation: (
    conversationId: string,
    content: string,
    mediaFiles?: MediaFile[],
    paramsOverride?: Partial<llm.ChatParams>,
    options?: { origin?: ChatSurfaceOrigin },
  ) => Promise<void>;
  retryMessageToConversation: (
    conversationId: string,
    messageId: string,
    paramsOverride?: Partial<llm.ChatParams>,
    options?: { origin?: ChatSurfaceOrigin },
  ) => Promise<void>;
  cancelConversationTurn: (conversationId: string) => void;

  getConversationMessages: (conversationId: string) => Message[];
  getConversationThreadedMessages: (conversationId: string) => MessageNode[] | undefined;
  loadMessageChildren: (messageId: string) => Promise<MessageNode[]>;

  handleConversationDeleted: (conversationId: string) => void;
  handleConversationCleared: (conversationId: string) => void;
  handleConversationRenamed: (conversationId: string, newTitle: string) => void;
  handleDatabaseReset: () => void;

  assignConversationChannel: (conversationId: string, channel: string, contactId: string) => Promise<void>;
  unassignConversationChannel: (conversationId: string) => Promise<void>;
  reloadConversationMessages: (conversationId: string) => Promise<void>;
  handleExternalIncoming: (data: {
    channel: string; from: string; fromId?: string; text: string; conversationId: string;
    newConversation?: boolean;
  }) => void;
}

export const useChatStore = create<ChatStore>()((set, get) => {
  const turnQueue = createConversationTurnQueue();

  const getSession = (state: ChatStore, conversationId: string, sessionKey?: string): ChatConversationSession => (
    getChatSession(state, conversationId, sessionKey)
  );

  const patchSession = (
    state: ChatStore,
    conversationId: string,
    patch: Partial<ChatConversationSession> | ((session: ChatConversationSession) => ChatConversationSession),
    sessionKey?: string,
  ): Partial<ChatStore> => {
    return patchChatSession(state, conversationId, patch, sessionKey);
  };

  const patchSurfaceSession = (
    state: ChatStore,
    conversationId: string,
    patch: Partial<ChatSurfaceSession>,
    sessionKey: string,
  ): Partial<ChatStore> => {
    const defaultSession = state.sessionsByConversationId[conversationId];
    const currentSession = state.surfaceSessionsByKey[sessionKey] ?? {
      ...createEmptyChatSurfaceSession(conversationId, sessionKey),
      isLoading: defaultSession?.isLoading ?? false,
      hasOlderMessages: defaultSession?.hasOlderMessages ?? false,
      isLoadingOlderMessages: defaultSession?.isLoadingOlderMessages ?? false,
      visibleThreadedMessages: defaultSession?.conversation?.threadedMessages,
      messageWindow: defaultSession?.conversation?.messageWindow,
      queuedTurnCount: defaultSession?.queuedTurnCount ?? 0,
    };
    const patchKeys = Object.keys(patch) as Array<keyof ChatSurfaceSession>;
    const hasChanges = patchKeys.some((key) => currentSession[key] !== patch[key]);
    if (!hasChanges) return state;

    return {
      surfaceSessionsByKey: {
        ...state.surfaceSessionsByKey,
        [sessionKey]: {
          ...currentSession,
          ...patch,
        },
      },
    };
  };

  const patchConversation = (
    state: ChatStore,
    conversationId: string,
    updater: (conversation: ActiveConversation) => ActiveConversation,
  ): Partial<ChatStore> => {
    return patchChatConversation(state, conversationId, updater);
  };

  const setConversationLoading = (conversationId: string, isLoading: boolean, sessionKey?: string) => {
    set((state) => {
      const loadingConversationIds = new Set(state.loadingConversationIds);
      if (isLoading) {
        loadingConversationIds.add(conversationId);
      } else {
        loadingConversationIds.delete(conversationId);
      }
      const patches = patchSession(state, conversationId, { isLoading }, sessionKey);
      if (sessionKey) {
        return {
          loadingConversationIds,
          ...patches,
        };
      }

      const surfaceSessionsByKey = { ...(patches.surfaceSessionsByKey ?? state.surfaceSessionsByKey) };
      for (const [sessionKey, session] of Object.entries(surfaceSessionsByKey)) {
        if (session.conversationId === conversationId) {
          surfaceSessionsByKey[sessionKey] = { ...session, isLoading };
        }
      }
      return {
        loadingConversationIds,
        ...patches,
        surfaceSessionsByKey,
      };
    });
  };

  const adjustQueuedTurnCount = (conversationId: string, delta: number, sessionKey?: string) => {
    set((state) => {
      const session = getSession(state, conversationId, sessionKey);
      const queuedTurnCount = Math.max(0, (session.queuedTurnCount ?? 0) + delta);
      return patchSession(state, conversationId, { queuedTurnCount }, sessionKey);
    });
  };

  const mergeConversationNodes = (existing: MessageNode[], incoming: MessageNode[]): MessageNode[] => {
    if (incoming.length === 0) return existing;
    const byId = new Map<string, MessageNode>();
    for (const node of existing) {
      byId.set(String(node.message.id), node);
    }
    for (const node of incoming) {
      byId.set(String(node.message.id), node);
    }
    return Array.from(byId.values()).sort((a, b) => {
      const aIndex = a.originalIndex ?? 0;
      const bIndex = b.originalIndex ?? 0;
      if (aIndex !== bIndex) return aIndex - bIndex;
      return String(a.message.id).localeCompare(String(b.message.id));
    });
  };

  const trimRenderedWindow = (
    nodes: MessageNode[],
    window: MessageWindowState,
    keep: 'start' | 'end',
  ): { nodes: MessageNode[]; window: MessageWindowState } => {
    if (nodes.length <= MAX_RENDERED_MESSAGE_WINDOW_SIZE) {
      return { nodes, window };
    }

    const trimmedNodes = keep === 'start'
      ? nodes.slice(0, MAX_RENDERED_MESSAGE_WINDOW_SIZE)
      : nodes.slice(nodes.length - MAX_RENDERED_MESSAGE_WINDOW_SIZE);
    const startIndex = trimmedNodes[0]?.originalIndex ?? window.startIndex;
    const endIndex = trimmedNodes[trimmedNodes.length - 1]?.originalIndex ?? window.endIndex;

    return {
      nodes: trimmedNodes,
      window: {
        ...window,
        startIndex,
        endIndex,
        hasBefore: startIndex > 0,
        hasAfter: window.totalCount > 0 && endIndex < window.totalCount - 1,
      },
    };
  };

  const resetQueuedTurnCount = (conversationId: string) => {
    set((state) => {
      const patches = patchSession(state, conversationId, { queuedTurnCount: 0 });
      const surfaceSessionsByKey = { ...(patches.surfaceSessionsByKey ?? state.surfaceSessionsByKey) };
      for (const [sessionKey, session] of Object.entries(surfaceSessionsByKey)) {
        if (session.conversationId === conversationId) {
          surfaceSessionsByKey[sessionKey] = { ...session, queuedTurnCount: 0 };
        }
      }
      return {
        ...patches,
        surfaceSessionsByKey,
      };
    });
  };

  const chatEventAdapter = {
    getSession: (conversationId: string, sessionKey?: string) => getSession(get(), conversationId, sessionKey),
    patchSession: (conversationId: string, patch: Partial<ChatConversationSession>) => {
      set((state) => {
        const targetSessionKey = patch.surfaceOrigin?.sessionKey;
        if (targetSessionKey) {
          return patchSession(state, conversationId, patch, targetSessionKey);
        }

        const basePatches = patchSession(state, conversationId, patch);
        const existingSurfaceSessionsByKey = basePatches.surfaceSessionsByKey ?? state.surfaceSessionsByKey;
        const surfaceSessionsByKey = { ...existingSurfaceSessionsByKey };
        const surfacePatch: Partial<ChatSurfaceSession> = { ...patch };
        delete (surfacePatch as Partial<ChatConversationSession>).conversation;
        let hasMatchingSurfaceSession = false;

        for (const [sessionKey, session] of Object.entries(existingSurfaceSessionsByKey)) {
          if (session.conversationId !== conversationId) {
            continue;
          }

          hasMatchingSurfaceSession = true;
          surfaceSessionsByKey[sessionKey] = {
            ...session,
            ...surfacePatch,
            surfaceOrigin: patch.surfaceOrigin ?? session.surfaceOrigin,
          };
        }

        if (!hasMatchingSurfaceSession) {
          return basePatches;
        }

        return {
          ...basePatches,
          surfaceSessionsByKey,
        };
      });
    },
    patchConversation: (
      conversationId: string,
      updater: (conversation: ActiveConversation) => ActiveConversation,
    ) => {
      set((state) => patchConversation(state, conversationId, updater));
    },
    updateMessage: (conversationId: string, messageId: string, content: string) => {
      get().updateConversationMessage(conversationId, messageId, content);
    },
    updateReasoning: (conversationId: string, messageId: string, reasoning: string) => {
      get().updateConversationMessageReasoning(conversationId, messageId, reasoning);
    },
    setConversationLoading,
  };

  const sendMessageInternal = async (
    conversationId: string,
    content: string,
    mediaFiles?: MediaFile[],
    paramsOverride?: Partial<llm.ChatParams>,
    retryMessageId?: string,
    options?: { origin?: ChatSurfaceOrigin },
  ) => {
    if (content.length > MAX_MESSAGE_CONTENT_SIZE) {
      announce(i18next.t('chat.validation.messageTooLarge', {
        defaultValue: 'Mensagem muito grande ({{size}} bytes). Máximo permitido: {{max}} bytes',
        size: content.length,
        max: MAX_MESSAGE_CONTENT_SIZE,
      }));
      return;
    }

    if (mediaFiles && mediaFiles.length > 0) {
      const totalSize = mediaFiles.reduce((acc, f) => acc + f.file.size, 0);
      const estimatedBase64Size = Math.ceil(totalSize * 1.37);
      if (estimatedBase64Size > MAX_MEDIA_SIZE) {
        announce(i18next.t('chat.validation.mediaTooLarge', {
          defaultValue: 'Arquivos de mídia muito grandes (~{{size}}MB). Máximo permitido: {{max}}MB',
          size: Math.round(estimatedBase64Size / 1024 / 1024),
          max: Math.round(MAX_MEDIA_SIZE / 1024 / 1024),
        }));
        return;
      }
    }

    playSendSound();
    const controller = startChatEventController({
      conversationId,
      initialUserContent: content,
      origin: options?.origin,
      adapter: chatEventAdapter,
    });

    try {
      let mediaJson = '';
      if (mediaFiles && mediaFiles.length > 0) {
        const mediaDataArray: MediaData[] = [];
        for (const mediaFile of mediaFiles) {
          const base64Data = await fileToBase64(mediaFile.file);
          mediaDataArray.push({
            name: mediaFile.file.name,
            type: mediaFile.file.type,
            data: base64Data,
            size: mediaFile.file.size,
          });
        }
        mediaJson = JSON.stringify(mediaDataArray);
      }

      const mergedParams: llm.ChatParams = {
        model: paramsOverride?.model ?? '',
        temperature: paramsOverride?.temperature ?? 0,
        maxTokens: paramsOverride?.maxTokens ?? 0,
        maxTokensMode: paramsOverride?.maxTokensMode,
        topP: paramsOverride?.topP,
        reasoningEffort: paramsOverride?.reasoningEffort,
        profileSlug: paramsOverride?.profileSlug,
        tabType: paramsOverride?.tabType,
        activeFilePath: paramsOverride?.activeFilePath,
        surfaceStateJson: paramsOverride?.surfaceStateJson,
        surfaceContextJson: paramsOverride?.surfaceContextJson,
        surfaceSessionKey: options?.origin?.sessionKey,
        surfaceId: options?.origin?.surfaceId,
        surfaceType: options?.origin?.surfaceType,
        surfaceTabId: options?.origin?.tabId,
      };
      if (retryMessageId) {
        await RetryMessage(conversationId, retryMessageId, mergedParams);
      } else {
        await SendMessage(conversationId, content, mediaJson, mergedParams);
      }

    } catch (error: unknown) {
      const errorMsg = getErrorMessage(error);
      controller.handleSendFailure(errorMsg);
    }
  };

  return {
    sessionsByConversationId: {},
    timelinesByConversationId: {},
    surfaceSessionsByKey: {},
    loadingConversationIds: new Set(),
    isInitialized: false,

    setConversationEditingMessageId: (conversationId, id, sessionKey) => {
      set((state) => patchSession(state, conversationId, { editingMessageId: id }, sessionKey));
    },

    startConversationEditing: (conversationId, id, sessionKey) => {
      set((state) => patchSession(state, conversationId, { editingMessageId: id, skipFocusRestore: true }, sessionKey));
    },

    setConversationReadingMessageId: (conversationId, id, sessionKey) => {
      set((state) => patchSession(state, conversationId, { readingMessageId: id }, sessionKey));
    },

    startConversationReading: (conversationId, id, sessionKey) => {
      set((state) => patchSession(state, conversationId, { readingMessageId: id, skipFocusRestore: true }, sessionKey));
    },

    setConversationDraftMessage: (conversationId, message, sessionKey) => {
      set((state) => patchSurfaceSession(state, conversationId, { draftMessage: message }, sessionKey));
    },

    setConversationDraftMediaFiles: (conversationId, mediaFiles, sessionKey) => {
      set((state) => patchSurfaceSession(state, conversationId, { draftMediaFiles: mediaFiles }, sessionKey));
    },

    clearConversationDraft: (conversationId, sessionKey) => {
      set((state) => {
        const session = getSession(state, conversationId, sessionKey);
        if (session.draftMessage === '' && session.draftMediaFiles.length === 0) {
          return state;
        }
        return patchSurfaceSession(state, conversationId, {
          draftMessage: '',
          draftMediaFiles: session.draftMediaFiles.length === 0 ? session.draftMediaFiles : [],
        }, sessionKey);
      });
    },

    setConversationScrollState: (conversationId, scrollState, sessionKey) => {
      set((state) => patchSurfaceSession(state, conversationId, scrollState, sessionKey));
    },

    ensureConversationSurfaceSession: (conversationId, sessionKey, origin) => {
      set((state) => {
        const existingSurfaceSession = state.surfaceSessionsByKey[sessionKey];
        if (existingSurfaceSession) {
          if (!origin || existingSurfaceSession.surfaceOrigin) return state;
          return {
            surfaceSessionsByKey: {
              ...state.surfaceSessionsByKey,
              [sessionKey]: {
                ...existingSurfaceSession,
                surfaceOrigin: origin,
              },
            },
          };
        }
        const defaultSession = state.sessionsByConversationId[conversationId];
        const surfaceSession = {
          ...createEmptyChatSurfaceSession(conversationId, sessionKey),
          surfaceOrigin: origin,
          isLoading: false,
          hasOlderMessages: defaultSession?.hasOlderMessages ?? false,
          isLoadingOlderMessages: defaultSession?.isLoadingOlderMessages ?? false,
          visibleThreadedMessages: defaultSession?.conversation?.threadedMessages,
          messageWindow: defaultSession?.conversation?.messageWindow,
          queuedTurnCount: defaultSession?.queuedTurnCount ?? 0,
        };
        return {
          surfaceSessionsByKey: {
            ...state.surfaceSessionsByKey,
            [sessionKey]: surfaceSession,
          },
        };
      });
    },

    removeConversationSurfaceSession: (sessionKey) => {
      set((state) => {
        if (!state.surfaceSessionsByKey[sessionKey]) return state;
        const surfaceSessionsByKey = { ...state.surfaceSessionsByKey };
        delete surfaceSessionsByKey[sessionKey];
        return { surfaceSessionsByKey };
      });
    },

    consumeSkipFocusRestore: (conversationId, sessionKey) => {
      const session = getSession(get(), conversationId, sessionKey);
      const shouldSkip = !!session?.skipFocusRestore;
      if (shouldSkip) {
        set((state) => patchSession(state, conversationId, { skipFocusRestore: false }, sessionKey));
      }
      return shouldSkip;
    },

    createConversation: async (title) => {
      const defaultTitle = i18next.t('chat.newConversation');
      const conv = await EnsureConversation(title || defaultTitle);
      set((state) => {
        const conversation = {
          id: conv.id,
          title: conv.title || title || defaultTitle,
          threadedMessages: [],
        };
        return {
          ...patchSession(state, conv.id, { conversation }),
          isInitialized: true,
        };
      });
      return conv.id;
    },

    loadConversationSession: async (id) => {
      try {
        const snapshot = await loadConversationSnapshot(id, INITIAL_MESSAGE_WINDOW_SIZE);
        set((state) => {
          const conversation = {
            id,
            title: snapshot.title,
            threadedMessages: snapshot.threadedMessages,
            channel: snapshot.channel,
            contactId: snapshot.contactId,
            messageWindow: snapshot.messageWindow,
          };
          const patches = patchSession(state, id, {
            conversation,
            isLoading: state.loadingConversationIds.has(id),
            hasOlderMessages: snapshot.hasOlderMessages,
            isLoadingOlderMessages: false,
            visibleThreadedMessages: snapshot.threadedMessages,
            messageWindow: snapshot.messageWindow,
          });
          return {
            ...patches,
            isInitialized: true,
          };
        });
      } catch (error) {
        console.error('[Chat] Erro ao carregar conversa:', error);
        set((state) => {
          const patches = patchSession(state, id, {
            conversation: { id, title: i18next.t('chat.conversation'), threadedMessages: [] },
            isLoading: state.loadingConversationIds.has(id),
            hasOlderMessages: false,
            isLoadingOlderMessages: false,
            visibleThreadedMessages: [],
            messageWindow: {
              scope: 'conversation',
              conversationId: id,
              totalCount: 0,
              startIndex: 0,
              endIndex: -1,
              hasBefore: false,
              hasAfter: false,
            },
          });
          return {
            ...patches,
            isInitialized: true,
          };
        });
      }
    },

    getConversationSession: (conversationId) => {
      if (!conversationId) return null;
      return get().sessionsByConversationId[conversationId] ?? null;
    },

    loadOlderMessagesForConversation: async (conversationId, sessionKey) => {
      const state = get();
      const session = getSession(state, conversationId, sessionKey);
      const conversation = session.conversation;
      if (!conversation || session.isLoadingOlderMessages || !session.hasOlderMessages) return;

      const firstMessageId = conversation.threadedMessages[0]?.message.id;
      if (!firstMessageId) return;

      set((current) => {
        const currentSession = getSession(current, conversationId, sessionKey);
        const patches = patchSession(current, conversationId, {
          hasOlderMessages: currentSession.hasOlderMessages,
          isLoadingOlderMessages: true,
        }, sessionKey);
        return patches;
      });
      try {
        const olderMessages = await loadOlderConversationMessages(
          conversation.id,
          firstMessageId,
          INITIAL_MESSAGE_WINDOW_SIZE,
        );

        set((current) => {
          const currentSession = getSession(current, conversation.id, sessionKey);
          if (!currentSession.conversation) {
            const fallbackSession = getSession(current, conversationId, sessionKey);
            const patches = patchSession(current, conversationId, {
              hasOlderMessages: fallbackSession.hasOlderMessages,
              isLoadingOlderMessages: false,
            }, sessionKey);
            return patches;
          }
          const existingIds = new Set(currentSession.conversation.threadedMessages.map((node) => node.message.id));
          const dedupedOlderNodes = olderMessages.nodes.filter((node) => !existingIds.has(node.message.id));
          const expandedVisibleThreadedMessages = [...dedupedOlderNodes, ...currentSession.conversation.threadedMessages];
          const cachedThreadedMessages = mergeConversationNodes(
            getConversationTimeline(current, conversation.id)?.threadedMessages ?? [],
            expandedVisibleThreadedMessages,
          );
          const expandedMessageWindow = {
            ...olderMessages.messageWindow,
            endIndex: currentSession.conversation.messageWindow?.endIndex ?? olderMessages.messageWindow.endIndex,
            hasAfter: currentSession.conversation.messageWindow?.hasAfter ?? olderMessages.hasNewerMessages,
          };
          const { nodes: visibleThreadedMessages, window: messageWindow } = trimRenderedWindow(
            expandedVisibleThreadedMessages,
            expandedMessageWindow,
            'start',
          );
          const patches = patchSession(current, conversation.id, {
            conversation: {
              ...currentSession.conversation,
              threadedMessages: cachedThreadedMessages,
              messageWindow,
            },
            hasOlderMessages: olderMessages.hasOlderMessages,
            isLoadingOlderMessages: false,
            visibleThreadedMessages,
            messageWindow,
          }, sessionKey);
          return patches;
        });
      } catch (error) {
        console.error('[Chat] Erro ao carregar mensagens anteriores:', error);
        set((current) => {
          const currentSession = getSession(current, conversationId, sessionKey);
          const patches = patchSession(current, conversationId, {
            hasOlderMessages: currentSession.hasOlderMessages,
            isLoadingOlderMessages: false,
          }, sessionKey);
          return patches;
        });
      }
    },

    loadNewerMessagesForConversation: async (conversationId, sessionKey) => {
      const state = get();
      const session = getSession(state, conversationId, sessionKey);
      const conversation = session.conversation;
      if (!conversation || session.isLoadingOlderMessages || !conversation.messageWindow?.hasAfter) return;

      const lastMessageId = conversation.threadedMessages[conversation.threadedMessages.length - 1]?.message.id;
      if (!lastMessageId) return;

      set((current) => {
        const currentSession = getSession(current, conversationId, sessionKey);
        return patchSession(current, conversationId, {
          hasOlderMessages: currentSession.hasOlderMessages,
          isLoadingOlderMessages: true,
        }, sessionKey);
      });

      try {
        const newerMessages = await loadNewerConversationMessages(
          conversation.id,
          lastMessageId,
          INITIAL_MESSAGE_WINDOW_SIZE,
        );
        set((current) => {
          const currentSession = getSession(current, conversation.id, sessionKey);
          if (!currentSession.conversation) {
            return patchSession(current, conversationId, { isLoadingOlderMessages: false }, sessionKey);
          }
          const existingIds = new Set(currentSession.conversation.threadedMessages.map((node) => node.message.id));
          const dedupedNewerNodes = newerMessages.nodes.filter((node) => !existingIds.has(node.message.id));
          const expandedVisibleThreadedMessages = [...currentSession.conversation.threadedMessages, ...dedupedNewerNodes];
          const cachedThreadedMessages = mergeConversationNodes(
            getConversationTimeline(current, conversation.id)?.threadedMessages ?? [],
            expandedVisibleThreadedMessages,
          );
          const expandedMessageWindow = {
            ...newerMessages.messageWindow,
            startIndex: currentSession.conversation.messageWindow?.startIndex ?? newerMessages.messageWindow.startIndex,
            hasBefore: currentSession.conversation.messageWindow?.hasBefore ?? newerMessages.hasOlderMessages,
          };
          const { nodes: visibleThreadedMessages, window: messageWindow } = trimRenderedWindow(
            expandedVisibleThreadedMessages,
            expandedMessageWindow,
            'end',
          );
          return patchSession(current, conversation.id, {
            conversation: {
              ...currentSession.conversation,
              threadedMessages: cachedThreadedMessages,
              messageWindow,
            },
            hasOlderMessages: messageWindow.hasBefore,
            isLoadingOlderMessages: false,
            visibleThreadedMessages,
            messageWindow,
          }, sessionKey);
        });
      } catch (error) {
        console.error('[Chat] Erro ao carregar mensagens posteriores:', error);
        set((current) => {
          const currentSession = getSession(current, conversationId, sessionKey);
          return patchSession(current, conversationId, {
            hasOlderMessages: currentSession.hasOlderMessages,
            isLoadingOlderMessages: false,
          }, sessionKey);
        });
      }
    },

    loadBoundaryMessagesForConversation: async (conversationId, sessionKey, anchor) => {
      const state = get();
      const session = getSession(state, conversationId, sessionKey);
      if (session.isLoadingOlderMessages) return;

      set((current) => patchSession(current, conversationId, {
        hasOlderMessages: session.hasOlderMessages,
        isLoadingOlderMessages: true,
      }, sessionKey));

      try {
        const boundaryWindow = await loadConversationBoundaryWindow(
          conversationId,
          anchor,
          INITIAL_MESSAGE_WINDOW_SIZE,
        );
        set((current) => {
          const currentSession = getSession(current, conversationId, sessionKey);
          const baseConversation = currentSession.conversation ?? {
            id: conversationId,
            title: i18next.t('chat.conversation'),
            threadedMessages: [],
          };
          const cachedThreadedMessages = mergeConversationNodes(
            getConversationTimeline(current, conversationId)?.threadedMessages ?? [],
            boundaryWindow.nodes,
          );
          const messageWindow = boundaryWindow.messageWindow;
          return patchSession(current, conversationId, {
            conversation: {
              ...baseConversation,
              threadedMessages: cachedThreadedMessages,
              messageWindow,
            },
            hasOlderMessages: boundaryWindow.hasOlderMessages,
            isLoadingOlderMessages: false,
            visibleThreadedMessages: boundaryWindow.nodes,
            messageWindow,
          }, sessionKey);
        });
      } catch (error) {
        console.error('[Chat] Erro ao carregar limite da conversa:', error);
        set((current) => {
          const currentSession = getSession(current, conversationId, sessionKey);
          return patchSession(current, conversationId, {
            hasOlderMessages: currentSession.hasOlderMessages,
            isLoadingOlderMessages: false,
          }, sessionKey);
        });
      }
    },

    updateConversationMessage: (conversationId, messageId, content) => {
      set((state) => {
        const session = getSession(state, conversationId);
        if (!session.conversation) return state;
        return patchSession(state, conversationId, {
          conversation: {
            ...session.conversation,
            threadedMessages: updateMessageContentInTree(session.conversation.threadedMessages, messageId, content),
          },
        });
      });
    },

    updateConversationMessageReasoning: (conversationId, messageId, reasoning) => {
      set((state) => {
        const session = getSession(state, conversationId);
        if (!session.conversation) return state;
        return patchSession(state, conversationId, {
          conversation: {
            ...session.conversation,
            threadedMessages: updateMessageReasoningInTree(session.conversation.threadedMessages, messageId, reasoning),
          },
        });
      });
    },

    addInternalMessage: (message) => {
      const conversationId = String(message.conversationId || '');
      if (!conversationId) return;

      set((state) => {
        const session = getSession(state, conversationId);
        if (!session.conversation) return state;
        return patchSession(state, conversationId, {
          conversation: {
            ...session.conversation,
            threadedMessages: appendInternalMessageToTree(session.conversation.threadedMessages, message),
          },
        });
      });
    },

    clearConversationMessages: (conversationId) => {
      set((state) => patchConversation(state, conversationId, (conversation) => ({
        ...conversation,
        threadedMessages: [],
      })));
    },

    sendMessageToConversation: async (conversationId, content, mediaFiles, paramsOverride, options) => {
      if (!conversationId) {
        console.error('[Chat] sendMessageToConversation sem conversationId explícito');
        announce(i18next.t('chat.errors.noActiveConversation'), 'assertive');
        return;
      }
      if (!getConversationTimeline(get(), conversationId)) {
        await get().loadConversationSession(conversationId);
      }
      const sessionKey = options?.origin?.sessionKey;
      const queuedBehindActiveTurn = turnQueue.isQueued(conversationId);
      if (queuedBehindActiveTurn) adjustQueuedTurnCount(conversationId, 1, sessionKey);
      try {
        await turnQueue.enqueue(conversationId, async () => {
          if (queuedBehindActiveTurn) adjustQueuedTurnCount(conversationId, -1, sessionKey);
          await sendMessageInternal(conversationId, content, mediaFiles, paramsOverride, undefined, options);
        });
      } catch (error) {
        if (!isConversationTurnQueueClearedError(error)) throw error;
      }
    },

    retryMessageToConversation: async (conversationId, messageId, paramsOverride, options) => {
      if (!conversationId || !messageId) {
        console.error('[Chat] retryMessageToConversation sem conversationId/messageId válido');
        announce(i18next.t('chat.errors.noActiveConversation'), 'assertive');
        return;
      }
      if (!getConversationTimeline(get(), conversationId)) {
        await get().loadConversationSession(conversationId);
      }
      const sessionKey = options?.origin?.sessionKey;
      const queuedBehindActiveTurn = turnQueue.isQueued(conversationId);
      if (queuedBehindActiveTurn) adjustQueuedTurnCount(conversationId, 1, sessionKey);
      try {
        await turnQueue.enqueue(conversationId, async () => {
          if (queuedBehindActiveTurn) adjustQueuedTurnCount(conversationId, -1, sessionKey);
          await sendMessageInternal(conversationId, '', undefined, paramsOverride, messageId, options);
        });
      } catch (error) {
        if (!isConversationTurnQueueClearedError(error)) throw error;
      }
    },

    cancelConversationTurn: (conversationId) => {
      turnQueue.clear(conversationId);
      stopChatEventController(conversationId);
      setConversationLoading(conversationId, false);
      resetQueuedTurnCount(conversationId);
    },

    getConversationMessages: (conversationId) => (
      flattenThreadedMessages(getConversationTimeline(get(), conversationId)?.threadedMessages)
    ),

    toggleConversationThreadExpanded: (conversationId, messageId, sessionKey) => {
      set((state) => {
        const session = getSession(state, conversationId, sessionKey);
        const expanded = new Set(session.expandedThreads);
        if (expanded.has(messageId)) expanded.delete(messageId);
        else expanded.add(messageId);
        return patchSession(state, conversationId, { expandedThreads: expanded }, sessionKey);
      });
    },

    isConversationThreadExpanded: (conversationId, messageId, sessionKey) => (
      getSession(get(), conversationId, sessionKey).expandedThreads.has(messageId) ?? false
    ),

    toggleConversationReasoningExpanded: (conversationId, messageId, sessionKey) => {
      set((state) => {
        const session = getSession(state, conversationId, sessionKey);
        const expanded = new Set(session.expandedReasonings);
        if (expanded.has(messageId)) expanded.delete(messageId);
        else expanded.add(messageId);
        return patchSession(state, conversationId, { expandedReasonings: expanded }, sessionKey);
      });
    },

    isConversationReasoningExpanded: (conversationId, messageId, sessionKey) => (
      getSession(get(), conversationId, sessionKey).expandedReasonings.has(messageId) ?? false
    ),

    getConversationThreadedMessages: (conversationId) => (
      getConversationTimeline(get(), conversationId)?.threadedMessages
    ),

    loadMessageChildren: async (messageId) => {
      try {
        if (!messageId) {
          console.error('[Chat] Invalid message ID:', messageId);
          return [];
        }
        const frontendNodes = await loadMessageChildrenNodes(messageId);

        set((state) => {
          const targetConversationId = Object.entries(state.timelinesByConversationId).find(([, timeline]) => (
            hasMessageId(timeline.threadedMessages, messageId)
          ))?.[0] ?? Object.entries(state.sessionsByConversationId).find(([, session]) => (
            hasMessageId(session.conversation?.threadedMessages, messageId)
          ))?.[0];
          if (!targetConversationId) return state;
          const session = getSession(state, targetConversationId);
          if (!session.conversation) return state;

          return patchSession(state, targetConversationId, {
            conversation: {
              ...session.conversation,
              threadedMessages: attachChildrenToMessage(session.conversation.threadedMessages, messageId, frontendNodes),
            },
          });
        });

        return frontendNodes;
      } catch (error) {
        console.error('[Chat] Error loading children:', error);
        return [];
      }
    },

    handleConversationDeleted: (conversationId: string) => {
      turnQueue.clear(conversationId);
      stopChatEventController(conversationId);
      set((state) => removeChatSession(state, conversationId));
      if (isChatConversationActive(conversationId)) {
        announce(i18next.t('chat.announce.conversationDeletedPermanently'));
        setTimeout(() => {
          const input = document.querySelector('textarea[placeholder*="mensagem"], textarea[aria-label*="mensagem"]') as HTMLTextAreaElement;
          if (input) input.focus();
        }, 200);
      }
    },

    handleConversationCleared: (conversationId: string) => {
      if (getConversationTimeline(get(), conversationId)) {
        announce(i18next.t('chat.announce.conversationMessagesRemoved'));
        set((state) => ({
          ...patchConversation(state, conversationId, (conversation) => ({
            ...conversation,
            threadedMessages: [],
            title: i18next.t('chat.conversationCleared'),
          })),
        }));
      }
      if (isChatConversationActive(conversationId)) {
        setTimeout(() => {
          const input = document.querySelector('textarea[placeholder*="mensagem"], textarea[aria-label*="mensagem"]') as HTMLTextAreaElement;
          if (input) input.focus();
        }, 200);
      }
    },

    handleConversationRenamed: (conversationId: string, newTitle: string) => {
      set((state) => patchConversation(state, conversationId, (conversation) => ({
        ...conversation,
        title: newTitle,
      })));
    },

    handleDatabaseReset: () => {
      turnQueue.clearAll();
      stopAllChatEventControllers();
      set({
        sessionsByConversationId: {},
        timelinesByConversationId: {},
        surfaceSessionsByKey: {},
        loadingConversationIds: new Set(),
        isInitialized: false,
      });
      announce(i18next.t('chat.announce.databaseReset'));
    },

    assignConversationChannel: async (conversationId, channel, contactId) => {
      await AssignConversationToChannel(conversationId, channel, contactId);
      set((state) => patchConversation(state, conversationId, (conversation) => ({
        ...conversation,
        channel,
        contactId,
      })));
    },

    unassignConversationChannel: async (conversationId) => {
      await UnassignConversationFromChannel(conversationId);
      set((state) => patchConversation(state, conversationId, (conversation) => ({
        ...conversation,
        channel: undefined,
        contactId: undefined,
      })));
    },

    reloadConversationMessages: async (conversationId: string) => {
      if (!conversationId) return;
      try {
        const snapshot = await reloadConversationSnapshot(conversationId, INITIAL_MESSAGE_WINDOW_SIZE);
        set((state) => {
          const session = getSession(state, conversationId);
          if (!session.conversation) return state;
          return patchSession(state, conversationId, {
            conversation: {
              ...session.conversation,
              threadedMessages: snapshot.threadedMessages,
            },
            hasOlderMessages: snapshot.hasOlderMessages,
          });
        });
      } catch (err) {
        console.error('[Chat] Erro ao recarregar mensagens:', err);
      }
    },

    handleExternalIncoming: (data) => {
      void handleExternalChatIncoming(data, {
        hasConversationSession: (conversationId) => !!getConversationTimeline(get(), conversationId),
        loadConversationSession: (conversationId) => get().loadConversationSession(conversationId),
        enqueueExternalTurn: async (conversationId, sessionKey, task) => {
          const queuedBehindActiveTurn = turnQueue.isQueued(conversationId);
          if (queuedBehindActiveTurn) adjustQueuedTurnCount(conversationId, 1, sessionKey);
          try {
            await turnQueue.enqueue(conversationId, async () => {
              if (queuedBehindActiveTurn) adjustQueuedTurnCount(conversationId, -1, sessionKey);
              await task();
            });
          } catch (error) {
            if (!isConversationTurnQueueClearedError(error)) throw error;
          }
        },
        chatEventAdapter,
      });
    },
  };
});
