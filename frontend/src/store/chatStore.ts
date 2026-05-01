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
import { ttsService } from '../services/tts';
import { messageAudioService } from '../services/messageAudio';
import { isChatConversationActive } from '../services/chatArbitration';
import { startChatEventController, stopAllChatEventControllers } from '../services/chatEventController';
import { handleExternalChatIncoming } from '../services/externalChatController';
import {
  loadConversationSnapshot,
  loadMessageChildrenNodes,
  loadOlderConversationMessages,
  reloadConversationSnapshot,
} from '../services/chatSessionLoader';
import {
  getChatSession,
  patchChatConversation,
  patchChatSession,
  removeChatSession,
  type ActiveConversation,
  type ChatConversationSession,
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

interface MediaData {
  name: string;
  type: string;
  data: string;
  size: number;
}

export type { Message, MessageNode, TurnSegment } from '../lib/chatMessageTree';
export type { ActiveConversation, ChatConversationSession } from '../services/chatSessionRegistry';

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
  loadingConversationIds: Set<string>;
  isInitialized: boolean;
  contextProfileSlug: string | null;

  setContextProfileSlug: (slug: string | null) => void;
  setConversationEditingMessageId: (conversationId: string, id: string | null) => void;
  startConversationEditing: (conversationId: string, id: string) => void;
  consumeSkipFocusRestore: (conversationId: string) => boolean;
  setConversationReadingMessageId: (conversationId: string, id: string | null) => void;
  startConversationReading: (conversationId: string, id: string) => void;

  createConversation: (title?: string) => Promise<string>;
  loadConversationSession: (id: string, options?: { activate?: boolean }) => Promise<void>;
  getConversationSession: (conversationId: string | null | undefined) => ChatConversationSession | null;
  loadOlderMessagesForConversation: (conversationId: string) => Promise<void>;

  updateConversationMessage: (conversationId: string, messageId: string, content: string) => void;
  updateConversationMessageReasoning: (conversationId: string, messageId: string, reasoning: string) => void;
  addInternalMessage: (message: Message) => void;
  clearConversationMessages: (conversationId: string) => void;

  toggleConversationThreadExpanded: (conversationId: string, messageId: string) => void;
  isConversationThreadExpanded: (conversationId: string, messageId: string) => boolean;
  toggleConversationReasoningExpanded: (conversationId: string, messageId: string) => void;
  isConversationReasoningExpanded: (conversationId: string, messageId: string) => boolean;

  sendMessageToConversation: (
    conversationId: string,
    content: string,
    mediaFiles?: MediaFile[],
    paramsOverride?: Partial<llm.ChatParams>,
  ) => Promise<void>;
  retryMessageToConversation: (
    conversationId: string,
    messageId: string,
    paramsOverride?: Partial<llm.ChatParams>,
  ) => Promise<void>;

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
  let loadConversationSeq = 0;

  const getSession = (state: ChatStore, conversationId: string): ChatConversationSession => (
    getChatSession(state, conversationId)
  );

  const patchSession = (
    state: ChatStore,
    conversationId: string,
    patch: Partial<ChatConversationSession> | ((session: ChatConversationSession) => ChatConversationSession),
  ): Partial<ChatStore> => {
    return patchChatSession(state, conversationId, patch);
  };

  const patchConversation = (
    state: ChatStore,
    conversationId: string,
    updater: (conversation: ActiveConversation) => ActiveConversation,
  ): Partial<ChatStore> => {
    return patchChatConversation(state, conversationId, updater);
  };

  const setConversationLoading = (conversationId: string, isLoading: boolean) => {
    set((state) => {
      const loadingConversationIds = new Set(state.loadingConversationIds);
      if (isLoading) {
        loadingConversationIds.add(conversationId);
      } else {
        loadingConversationIds.delete(conversationId);
      }
      return {
        loadingConversationIds,
        ...patchSession(state, conversationId, { isLoading }),
      };
    });
  };

  const chatEventAdapter = {
    getSession: (conversationId: string) => getSession(get(), conversationId),
    patchSession: (conversationId: string, patch: Partial<ChatConversationSession>) => {
      set((state) => patchSession(state, conversationId, patch));
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
        profileSlug: paramsOverride?.profileSlug ?? get().contextProfileSlug ?? undefined,
        tabType: paramsOverride?.tabType,
        activeFilePath: paramsOverride?.activeFilePath,
        surfaceStateJson: paramsOverride?.surfaceStateJson,
        surfaceContextJson: paramsOverride?.surfaceContextJson,
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
    loadingConversationIds: new Set(),
    isInitialized: false,
    contextProfileSlug: null,

    setContextProfileSlug: (slug) => set({ contextProfileSlug: slug }),

    setConversationEditingMessageId: (conversationId, id) => {
      set((state) => patchSession(state, conversationId, { editingMessageId: id }));
    },

    startConversationEditing: (conversationId, id) => {
      set((state) => patchSession(state, conversationId, { editingMessageId: id, skipFocusRestore: true }));
    },

    setConversationReadingMessageId: (conversationId, id) => {
      set((state) => patchSession(state, conversationId, { readingMessageId: id }));
    },

    startConversationReading: (conversationId, id) => {
      set((state) => patchSession(state, conversationId, { readingMessageId: id, skipFocusRestore: true }));
    },

    consumeSkipFocusRestore: (conversationId) => {
      const session = get().sessionsByConversationId[conversationId];
      const shouldSkip = !!session?.skipFocusRestore;
      if (shouldSkip) {
        set((state) => patchSession(state, conversationId, { skipFocusRestore: false }));
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
        const sessionsByConversationId = {
          ...state.sessionsByConversationId,
          [conv.id]: {
            ...getSession(state, conv.id),
            conversation,
          },
        };
        return {
          sessionsByConversationId,
          isInitialized: true,
        };
      });
      return conv.id;
    },

    loadConversationSession: async (id, options = { activate: true }) => {
      // Para TTS/áudio da conversa anterior ao trocar
      if (options.activate !== false) {
        messageAudioService.stopCurrentAudio();
        ttsService.stop();
      }
      const seq = options.activate === false ? loadConversationSeq : ++loadConversationSeq;

      try {
        const snapshot = await loadConversationSnapshot(id, INITIAL_MESSAGE_WINDOW_SIZE);
        if (options.activate !== false && seq !== loadConversationSeq) return;
        set((state) => {
          if (options.activate !== false && seq !== loadConversationSeq) return state;
          const conversation = {
            id,
            title: snapshot.title,
            threadedMessages: snapshot.threadedMessages,
            channel: snapshot.channel,
            contactId: snapshot.contactId,
          };
          const sessionsByConversationId = {
            ...state.sessionsByConversationId,
            [id]: {
              ...getSession(state, id),
              conversation,
              isLoading: state.loadingConversationIds.has(id),
              hasOlderMessages: snapshot.hasOlderMessages,
              isLoadingOlderMessages: false,
            },
          };
          return {
            sessionsByConversationId,
            isInitialized: true,
          };
        });
      } catch (error) {
        if (options.activate !== false && seq !== loadConversationSeq) return;
        console.error('[Chat] Erro ao carregar conversa:', error);
        set((state) => {
          const sessionsByConversationId = {
            ...state.sessionsByConversationId,
            [id]: {
              ...getSession(state, id),
              conversation: { id, title: i18next.t('chat.conversation'), threadedMessages: [] },
              isLoading: state.loadingConversationIds.has(id),
              hasOlderMessages: false,
              isLoadingOlderMessages: false,
            },
          };
          return {
            sessionsByConversationId,
            isInitialized: true,
          };
        });
      }
    },

    getConversationSession: (conversationId) => {
      if (!conversationId) return null;
      return get().sessionsByConversationId[conversationId] ?? null;
    },

    loadOlderMessagesForConversation: async (conversationId) => {
      const state = get();
      const session = getSession(state, conversationId);
      const conversation = session.conversation;
      if (!conversation || session.isLoadingOlderMessages || !session.hasOlderMessages) return;

      const firstMessageId = conversation.threadedMessages[0]?.message.id;
      if (!firstMessageId) return;

      set((current) => patchSession(current, conversationId, { isLoadingOlderMessages: true }));
      try {
        const olderMessages = await loadOlderConversationMessages(
          conversation.id,
          firstMessageId,
          INITIAL_MESSAGE_WINDOW_SIZE,
        );

        set((current) => {
          const currentSession = getSession(current, conversation.id);
          if (!currentSession.conversation) {
            return patchSession(current, conversationId, { isLoadingOlderMessages: false });
          }
          const existingIds = new Set(currentSession.conversation.threadedMessages.map((node) => node.message.id));
          const dedupedOlderNodes = olderMessages.nodes.filter((node) => !existingIds.has(node.message.id));
          return patchSession(current, conversation.id, {
            conversation: {
              ...currentSession.conversation,
              threadedMessages: [...dedupedOlderNodes, ...currentSession.conversation.threadedMessages],
            },
            hasOlderMessages: olderMessages.hasOlderMessages,
            isLoadingOlderMessages: false,
          });
        });
      } catch (error) {
        console.error('[Chat] Erro ao carregar mensagens anteriores:', error);
        set((current) => patchSession(current, conversationId, { isLoadingOlderMessages: false }));
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

    sendMessageToConversation: async (conversationId, content, mediaFiles, paramsOverride) => {
      if (!conversationId) {
        console.error('[Chat] sendMessageToConversation sem conversationId explícito');
        announce(i18next.t('chat.errors.noActiveConversation'), 'assertive');
        return;
      }
      if (!get().sessionsByConversationId[conversationId]?.conversation) {
        await get().loadConversationSession(conversationId, { activate: false });
      }
      await sendMessageInternal(conversationId, content, mediaFiles, paramsOverride);
    },

    retryMessageToConversation: async (conversationId, messageId, paramsOverride) => {
      if (!conversationId || !messageId) {
        console.error('[Chat] retryMessageToConversation sem conversationId/messageId válido');
        announce(i18next.t('chat.errors.noActiveConversation'), 'assertive');
        return;
      }
      if (!get().sessionsByConversationId[conversationId]?.conversation) {
        await get().loadConversationSession(conversationId, { activate: false });
      }
      await sendMessageInternal(conversationId, '', undefined, paramsOverride, messageId);
    },

    getConversationMessages: (conversationId) => (
      flattenThreadedMessages(get().sessionsByConversationId[conversationId]?.conversation?.threadedMessages)
    ),

    toggleConversationThreadExpanded: (conversationId, messageId) => {
      set((state) => {
        const session = getSession(state, conversationId);
        const expanded = new Set(session.expandedThreads);
        if (expanded.has(messageId)) expanded.delete(messageId);
        else expanded.add(messageId);
        return patchSession(state, conversationId, { expandedThreads: expanded });
      });
    },

    isConversationThreadExpanded: (conversationId, messageId) => (
      get().sessionsByConversationId[conversationId]?.expandedThreads.has(messageId) ?? false
    ),

    toggleConversationReasoningExpanded: (conversationId, messageId) => {
      set((state) => {
        const session = getSession(state, conversationId);
        const expanded = new Set(session.expandedReasonings);
        if (expanded.has(messageId)) expanded.delete(messageId);
        else expanded.add(messageId);
        return patchSession(state, conversationId, { expandedReasonings: expanded });
      });
    },

    isConversationReasoningExpanded: (conversationId, messageId) => (
      get().sessionsByConversationId[conversationId]?.expandedReasonings.has(messageId) ?? false
    ),

    getConversationThreadedMessages: (conversationId) => (
      get().sessionsByConversationId[conversationId]?.conversation?.threadedMessages
    ),

    loadMessageChildren: async (messageId) => {
      try {
        if (!messageId) {
          console.error('[Chat] Invalid message ID:', messageId);
          return [];
        }
        const frontendNodes = await loadMessageChildrenNodes(messageId);

        set((state) => {
          const targetConversationId = Object.entries(state.sessionsByConversationId).find(([, session]) => (
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
      if (get().sessionsByConversationId[conversationId]) {
        set((state) => removeChatSession(state, conversationId));
      }
      if (isChatConversationActive(conversationId)) {
        announce(i18next.t('chat.announce.conversationDeletedPermanently'));
        setTimeout(() => {
          const input = document.querySelector('textarea[placeholder*="mensagem"], textarea[aria-label*="mensagem"]') as HTMLTextAreaElement;
          if (input) input.focus();
        }, 200);
      }
    },

    handleConversationCleared: (conversationId: string) => {
      if (get().sessionsByConversationId[conversationId]) {
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
      stopAllChatEventControllers();
      set({
        sessionsByConversationId: {},
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
        hasConversationSession: (conversationId) => !!get().sessionsByConversationId[conversationId]?.conversation,
        loadConversationSession: (conversationId) => get().loadConversationSession(conversationId, { activate: false }),
        chatEventAdapter,
      });
    },
  };
});
