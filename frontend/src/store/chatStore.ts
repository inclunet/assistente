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
import {
  startChatEventController,
  stopAllChatEventControllers,
  stopChatEventController,
  type ChatEventSession,
} from '../services/chatEventController';
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
  INITIAL_MESSAGE_WINDOW_SIZE,
  MAX_MESSAGE_WINDOW_NODES,
  MAX_MESSAGE_WINDOW_TURN_BOUNDARY_OVERFLOW,
} from '../services/messageWindowLimits';
import {
  createEmptyChatSurfaceSession,
  getDefaultChatSessionKey,
  getChatSession,
  getConversationTimeline,
  patchChatConversation,
  patchChatSession,
  removeChatSession,
  getTimelineNodeKey,
  isPersistedTimelineNode,
  mergeMessageNode,
  sortTimelineNodes,
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

const isPersistedMessageNode = (node: MessageNode | undefined): boolean => {
  if (!node) return false;
  return isPersistedTimelineNode(node);
};

const stripInternalMessageTurnId = (message: Message): Message => {
  const messageWithoutTurnId = { ...(message as Message & { turnId?: string }) };
  delete messageWithoutTurnId.turnId;
  return messageWithoutTurnId as Message;
};

const getLastPersistedMessageId = (nodes: MessageNode[]): string | null => {
  for (let index = nodes.length - 1; index >= 0; index -= 1) {
    const node = nodes[index];
    if (isPersistedMessageNode(node)) {
      return node.message.id;
    }
  }
  return null;
};

const getFirstPersistedMessageId = (nodes: MessageNode[]): string | null => {
  for (let index = 0; index < nodes.length; index += 1) {
    const node = nodes[index];
    if (isPersistedMessageNode(node)) {
      return node.message.id;
    }
  }
  return null;
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
  loadConversationSession: (id: string, options?: { refreshSurfaceWindows?: boolean }) => Promise<void>;
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
      isLoadingMessageWindow: defaultSession?.isLoadingMessageWindow ?? false,
      visibleThreadedMessages: defaultSession?.visibleThreadedMessages,
      messageWindow: defaultSession?.messageWindow,
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
    const byKey = new Map<string, MessageNode>();
    const mergeTimelineNode = (existingNode: MessageNode, incomingNode: MessageNode, key: string): MessageNode => {
      const existingIsPersisted = isPersistedMessageNode(existingNode);
      const incomingIsPersisted = isPersistedMessageNode(incomingNode);
      if (existingIsPersisted && !incomingIsPersisted) return existingNode;
      if (!existingIsPersisted && incomingIsPersisted) return incomingNode;
      if (
        key.startsWith('turn:')
        && existingIsPersisted
        && incomingIsPersisted
        && String(existingNode.message.id) !== String(incomingNode.message.id)
      ) {
        const existingIndex = existingNode.originalIndex ?? Number.NEGATIVE_INFINITY;
        const incomingIndex = incomingNode.originalIndex ?? Number.NEGATIVE_INFINITY;
        return incomingIndex >= existingIndex ? incomingNode : existingNode;
      }
      return mergeMessageNode(existingNode, incomingNode);
    };
    for (const node of existing) {
      byKey.set(getTimelineNodeKey(node), node);
    }
    for (const node of incoming) {
      const key = getTimelineNodeKey(node);
      const existingNode = byKey.get(key);
      if (!existingNode) {
        byKey.set(key, node);
        continue;
      }
      byKey.set(key, mergeTimelineNode(existingNode, node, key));
    }
    return sortTimelineNodes(Array.from(byKey.values()));
  };

  const capRenderedNodesAtEnd = (nodes: MessageNode[]): MessageNode[] => {
    if (nodes.length <= MAX_MESSAGE_WINDOW_NODES) return nodes;
    const minStartIndex = nodes.length - MAX_MESSAGE_WINDOW_NODES;
    let startIndex = minStartIndex;
    const boundaryTurnId = nodes[startIndex]?.message.turnId;
    while (boundaryTurnId && startIndex > 0 && nodes[startIndex - 1]?.message.turnId === boundaryTurnId) {
      startIndex -= 1;
    }
    if (nodes.length - startIndex > MAX_MESSAGE_WINDOW_NODES + MAX_MESSAGE_WINDOW_TURN_BOUNDARY_OVERFLOW) {
      startIndex = minStartIndex;
    }
    return nodes.slice(startIndex);
  };

  const trimRenderedWindow = (
    nodes: MessageNode[],
    window: MessageWindowState,
    keep: 'start' | 'end',
  ): { nodes: MessageNode[]; window: MessageWindowState } => {
    const reconcileWindowFromNodes = (visibleNodes: MessageNode[], fallbackWindow: MessageWindowState): MessageWindowState => {
      if (visibleNodes.length === 0) {
        return {
          ...fallbackWindow,
          startIndex: 0,
          endIndex: -1,
          hasBefore: false,
          hasAfter: fallbackWindow.totalCount > 0,
        };
      }
      const explicitIndexes = visibleNodes
        .map((node) => node.originalIndex)
        .filter((index): index is number => index !== undefined);
      if (explicitIndexes.length === 0) {
        return fallbackWindow;
      }
      const startIndex = Math.min(...explicitIndexes);
      const endIndex = Math.max(...explicitIndexes);
      const totalCount = fallbackWindow.totalCount;
      return {
        ...fallbackWindow,
        totalCount,
        startIndex,
        endIndex,
        hasBefore: startIndex > 0,
        hasAfter: totalCount > 0 && endIndex < totalCount - 1,
      };
    };

    if (nodes.length <= MAX_MESSAGE_WINDOW_NODES) {
      return { nodes, window: reconcileWindowFromNodes(nodes, window) };
    }

    let trimStart = keep === 'start' ? 0 : nodes.length - MAX_MESSAGE_WINDOW_NODES;
    let trimEnd = keep === 'start' ? MAX_MESSAGE_WINDOW_NODES : nodes.length;
    if (keep === 'start') {
      const boundaryTurnId = nodes[trimEnd - 1]?.message.turnId;
      while (boundaryTurnId && trimEnd < nodes.length && nodes[trimEnd]?.message.turnId === boundaryTurnId) {
        trimEnd += 1;
      }
      if (trimEnd - trimStart > MAX_MESSAGE_WINDOW_NODES + MAX_MESSAGE_WINDOW_TURN_BOUNDARY_OVERFLOW) {
        trimEnd = MAX_MESSAGE_WINDOW_NODES;
      }
    } else {
      const boundaryTurnId = nodes[trimStart]?.message.turnId;
      while (boundaryTurnId && trimStart > 0 && nodes[trimStart - 1]?.message.turnId === boundaryTurnId) {
        trimStart -= 1;
      }
      if (trimEnd - trimStart > MAX_MESSAGE_WINDOW_NODES + MAX_MESSAGE_WINDOW_TURN_BOUNDARY_OVERFLOW) {
        trimStart = nodes.length - MAX_MESSAGE_WINDOW_NODES;
      }
    }
    const trimmedNodes = nodes.slice(trimStart, trimEnd);
    return {
      nodes: trimmedNodes,
      window: reconcileWindowFromNodes(trimmedNodes, window),
    };
  };

  const reconcileLiveMessageWindow = (
    window: MessageWindowState | undefined,
    previousNodes: MessageNode[] | undefined,
    nextNodes: MessageNode[] | undefined,
  ): MessageWindowState | undefined => {
    if (!window || !nextNodes) return window;
    const previousCount = previousNodes?.length ?? 0;
    const nextCount = nextNodes.length;
    const appendedCount = Math.max(0, nextCount - previousCount);
    const explicitIndexes = nextNodes
      .map((node) => node.originalIndex)
      .filter((index): index is number => index !== undefined);
    const explicitStartIndex = explicitIndexes.length ? Math.min(...explicitIndexes) : undefined;
    const explicitEndIndex = explicitIndexes.length ? Math.max(...explicitIndexes) : undefined;

    const appendedToVisibleEnd = appendedCount > 0 && !window.hasAfter;
    const startIndex = explicitStartIndex !== undefined
      ? Math.min(window.startIndex, explicitStartIndex)
      : window.startIndex;
    const endIndex = explicitEndIndex !== undefined
      ? Math.max(window.endIndex, explicitEndIndex, appendedToVisibleEnd ? window.endIndex + appendedCount : explicitEndIndex)
      : window.hasAfter ? window.endIndex : window.endIndex + appendedCount;
    const totalCount = Math.max(
      window.totalCount + (explicitEndIndex === undefined ? appendedCount : 0),
      explicitEndIndex !== undefined ? explicitEndIndex + 1 : 0,
      endIndex + 1,
      nextCount,
    );

    return {
      ...window,
      totalCount,
      startIndex,
      endIndex,
      hasBefore: startIndex > 0,
      hasAfter: totalCount > 0 && endIndex < totalCount - 1,
    };
  };

  const mergeVisibleNodesFromConversation = (
    visibleNodes: MessageNode[] | undefined,
    incomingNodes: MessageNode[],
    window: MessageWindowState | undefined,
    appendNewNodes = false,
  ): MessageNode[] | undefined => {
    if (!visibleNodes) return undefined;
    const incomingByKey = new Map(incomingNodes.map((node) => [getTimelineNodeKey(node), node]));
    const visibleKeys = new Set(visibleNodes.map((node) => getTimelineNodeKey(node)));
    const updatedVisible = visibleNodes.map((node) => {
      const incoming = incomingByKey.get(getTimelineNodeKey(node));
      if (!incoming) return node;
      if (isPersistedMessageNode(node) && !isPersistedMessageNode(incoming)) return node;
      if (!isPersistedMessageNode(node) && isPersistedMessageNode(incoming)) return incoming;
      return mergeMessageNode(node, incoming);
    });
    if (window?.hasAfter && !appendNewNodes) return updatedVisible;
    const appendedNodes = incomingNodes.filter((node) => !visibleKeys.has(getTimelineNodeKey(node)));
    return appendedNodes.length
      ? capRenderedNodesAtEnd([...updatedVisible, ...appendedNodes])
      : updatedVisible;
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
    patchSession: (conversationId: string, patch: Partial<ChatEventSession> & Record<string, unknown>) => {
      set((state) => {
        const { appendVisibleMessages, ...sessionPatch } = patch as Partial<ChatConversationSession> & {
          appendVisibleMessages?: boolean;
        };
        const targetSessionKey = patch.surfaceOrigin?.sessionKey;
        if (targetSessionKey) {
          const session = getSession(state, conversationId, targetSessionKey);
          const visibleThreadedMessages = sessionPatch.visibleThreadedMessages
            ?? (sessionPatch.conversation
              ? mergeVisibleNodesFromConversation(
                session.visibleThreadedMessages,
                sessionPatch.conversation.threadedMessages,
                session.messageWindow,
                appendVisibleMessages === true,
              )
              : undefined)
            ?? session.visibleThreadedMessages;
          const messageWindow = sessionPatch.conversation && !sessionPatch.messageWindow
            ? reconcileLiveMessageWindow(session.messageWindow, session.visibleThreadedMessages, visibleThreadedMessages)
            : sessionPatch.messageWindow;
          return patchSession(state, conversationId, {
            ...sessionPatch,
            ...(visibleThreadedMessages !== undefined ? { visibleThreadedMessages } : {}),
            ...(messageWindow ? { messageWindow } : {}),
          }, targetSessionKey);
        }

        const basePatches = patchSession(state, conversationId, sessionPatch);
        const existingSurfaceSessionsByKey = basePatches.surfaceSessionsByKey ?? state.surfaceSessionsByKey;
        const surfaceSessionsByKey = { ...existingSurfaceSessionsByKey };
        const surfacePatch: Partial<ChatSurfaceSession> = { ...sessionPatch };
        delete (surfacePatch as Partial<ChatConversationSession>).conversation;
        let hasMatchingSurfaceSession = false;

        for (const [sessionKey, session] of Object.entries(existingSurfaceSessionsByKey)) {
          if (session.conversationId !== conversationId) {
            continue;
          }

          hasMatchingSurfaceSession = true;
          const visibleThreadedMessages = surfacePatch.visibleThreadedMessages
            ?? (sessionPatch.conversation
              ? mergeVisibleNodesFromConversation(
                session.visibleThreadedMessages,
                sessionPatch.conversation.threadedMessages,
                session.messageWindow,
              )
              : session.visibleThreadedMessages);
          const messageWindow = sessionPatch.conversation && !surfacePatch.messageWindow
            ? reconcileLiveMessageWindow(session.messageWindow, session.visibleThreadedMessages, visibleThreadedMessages)
            : surfacePatch.messageWindow;
          surfaceSessionsByKey[sessionKey] = {
            ...session,
            ...surfacePatch,
            ...(visibleThreadedMessages !== undefined ? { visibleThreadedMessages } : {}),
            ...(messageWindow ? { messageWindow } : {}),
            surfaceOrigin: sessionPatch.surfaceOrigin ?? session.surfaceOrigin,
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
          isLoadingMessageWindow: defaultSession?.isLoadingMessageWindow ?? false,
          visibleThreadedMessages: defaultSession?.visibleThreadedMessages,
          messageWindow: defaultSession?.messageWindow,
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

    loadConversationSession: async (id, options) => {
      try {
        const snapshot = await loadConversationSnapshot(id, INITIAL_MESSAGE_WINDOW_SIZE);
        set((state) => {
          const existingTimeline = getConversationTimeline(state, id);
          const cachedThreadedMessages = existingTimeline && !options?.refreshSurfaceWindows
            ? mergeConversationNodes(existingTimeline.threadedMessages, snapshot.threadedMessages)
            : snapshot.threadedMessages;
          const { nodes: visibleThreadedMessages, window: messageWindow } = trimRenderedWindow(
            snapshot.threadedMessages,
            snapshot.messageWindow,
            'end',
          );
          const conversation = {
            id,
            title: snapshot.title,
            threadedMessages: cachedThreadedMessages,
            channel: snapshot.channel,
            contactId: snapshot.contactId,
          };
          const patches = patchSession(state, id, {
            conversation,
            isLoading: state.loadingConversationIds.has(id),
            hasOlderMessages: messageWindow.hasBefore,
            isLoadingOlderMessages: false,
            isLoadingMessageWindow: false,
            visibleThreadedMessages,
            messageWindow,
          });
          const surfaceSessionsByKey = { ...(patches.surfaceSessionsByKey ?? state.surfaceSessionsByKey ?? {}) };
          for (const [sessionKey, surfaceSession] of Object.entries(surfaceSessionsByKey)) {
            if (surfaceSession.conversationId !== id) {
              continue;
            }
            const hasMaterializedSurfaceWindow = (surfaceSession.visibleThreadedMessages?.length ?? 0) > 0
              || (surfaceSession.messageWindow?.totalCount ?? 0) > 0;
            const isSurfaceAtLiveTail = !surfaceSession.messageWindow?.hasAfter
              || (
                surfaceSession.messageWindow.totalCount > 0
                && surfaceSession.messageWindow.endIndex >= surfaceSession.messageWindow.totalCount - 1
              );
            if (!options?.refreshSurfaceWindows && hasMaterializedSurfaceWindow) {
              surfaceSessionsByKey[sessionKey] = {
                ...surfaceSession,
                isLoadingOlderMessages: false,
                isLoadingMessageWindow: false,
                ...(isSurfaceAtLiveTail
                  ? {
                    hasOlderMessages: messageWindow.hasBefore,
                    visibleThreadedMessages,
                    messageWindow,
                  }
                  : {}),
              };
              continue;
            }
            surfaceSessionsByKey[sessionKey] = {
              ...surfaceSession,
              hasOlderMessages: messageWindow.hasBefore,
              isLoadingOlderMessages: false,
              isLoadingMessageWindow: false,
              visibleThreadedMessages,
              messageWindow,
            };
          }
          return {
            ...patches,
            surfaceSessionsByKey,
            isInitialized: true,
          };
        });
      } catch (error) {
        console.error('[Chat] Erro ao carregar conversa:', error);
        set((state) => {
          const emptyWindow = {
            scope: 'conversation' as const,
            conversationId: id,
            totalCount: 0,
            startIndex: 0,
            endIndex: -1,
            hasBefore: false,
            hasAfter: false,
          };
          const patches = patchSession(state, id, {
            conversation: { id, title: i18next.t('chat.conversation'), threadedMessages: [] },
            isLoading: state.loadingConversationIds.has(id),
            hasOlderMessages: false,
            isLoadingOlderMessages: false,
            isLoadingMessageWindow: false,
            visibleThreadedMessages: [],
            messageWindow: emptyWindow,
          });
          const surfaceSessionsByKey = { ...(patches.surfaceSessionsByKey ?? state.surfaceSessionsByKey ?? {}) };
          if (options?.refreshSurfaceWindows) {
            for (const [sessionKey, surfaceSession] of Object.entries(surfaceSessionsByKey)) {
              if (surfaceSession.conversationId !== id) continue;
              surfaceSessionsByKey[sessionKey] = {
                ...surfaceSession,
                hasOlderMessages: false,
                isLoadingOlderMessages: false,
                isLoadingMessageWindow: false,
                visibleThreadedMessages: [],
                messageWindow: emptyWindow,
              };
            }
          }
          return {
            ...patches,
            surfaceSessionsByKey,
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
      if (!conversation || session.isLoadingMessageWindow || !session.hasOlderMessages) return;

      const firstMessageId = getFirstPersistedMessageId(conversation.threadedMessages);
      if (!firstMessageId) {
        set((current) => {
          const currentSession = getSession(current, conversationId, sessionKey);
          if (!currentSession.hasOlderMessages && !currentSession.messageWindow?.hasBefore) return current;
          return patchSession(current, conversationId, {
            hasOlderMessages: false,
            messageWindow: currentSession.messageWindow
              ? {
                ...currentSession.messageWindow,
                hasBefore: false,
              }
              : undefined,
          }, sessionKey);
        });
        return;
      }

      set((current) => {
        const currentSession = getSession(current, conversationId, sessionKey);
        const patches = patchSession(current, conversationId, {
          hasOlderMessages: currentSession.hasOlderMessages,
          isLoadingOlderMessages: true,
          isLoadingMessageWindow: true,
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
              isLoadingMessageWindow: false,
            }, sessionKey);
            return patches;
          }
          const expandedVisibleThreadedMessages = mergeConversationNodes(
            olderMessages.nodes,
            currentSession.conversation.threadedMessages,
          );
          const timeline = getConversationTimeline(current, conversation.id) ?? currentSession.conversation;
          // The active surface can contain transient local nodes; only persisted nodes enter the shared timeline cache.
          const cachedThreadedMessages = mergeConversationNodes(
            timeline.threadedMessages,
            expandedVisibleThreadedMessages.filter(isPersistedMessageNode),
          );
          const expandedMessageWindow = {
            ...olderMessages.messageWindow,
            endIndex: currentSession.messageWindow?.endIndex ?? olderMessages.messageWindow.endIndex,
            hasAfter: currentSession.messageWindow?.hasAfter ?? olderMessages.hasNewerMessages,
          };
          const { nodes: visibleThreadedMessages, window: messageWindow } = trimRenderedWindow(
            expandedVisibleThreadedMessages,
            expandedMessageWindow,
            'start',
          );
          const patches = patchSession(current, conversation.id, {
            hasOlderMessages: olderMessages.hasOlderMessages,
            isLoadingOlderMessages: false,
            isLoadingMessageWindow: false,
            visibleThreadedMessages,
            messageWindow,
          }, sessionKey);
          return {
            ...patches,
            timelinesByConversationId: {
              ...(patches.timelinesByConversationId ?? current.timelinesByConversationId ?? {}),
              [conversation.id]: {
                ...timeline,
                threadedMessages: cachedThreadedMessages,
              },
            },
          };
        });
      } catch (error) {
        console.error('[Chat] Erro ao carregar mensagens anteriores:', error);
        set((current) => {
          const currentSession = getSession(current, conversationId, sessionKey);
          const patches = patchSession(current, conversationId, {
            hasOlderMessages: currentSession.hasOlderMessages,
            isLoadingOlderMessages: false,
            isLoadingMessageWindow: false,
          }, sessionKey);
          return patches;
        });
      }
    },

    loadNewerMessagesForConversation: async (conversationId, sessionKey) => {
      const state = get();
      const session = getSession(state, conversationId, sessionKey);
      const conversation = session.conversation;
      if (!conversation || session.isLoadingMessageWindow || !session.messageWindow?.hasAfter) return;

      const lastMessageId = getLastPersistedMessageId(conversation.threadedMessages);
      if (!lastMessageId) {
        set((current) => {
          const currentSession = getSession(current, conversationId, sessionKey);
          const currentWindow = currentSession.messageWindow;
          if (!currentWindow?.hasAfter) return current;
          return patchSession(current, conversationId, {
            messageWindow: {
              ...currentWindow,
              hasAfter: false,
            },
          }, sessionKey);
        });
        return;
      }

      set((current) => {
        const currentSession = getSession(current, conversationId, sessionKey);
        return patchSession(current, conversationId, {
          hasOlderMessages: currentSession.hasOlderMessages,
          isLoadingMessageWindow: true,
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
            return patchSession(current, conversationId, { isLoadingMessageWindow: false }, sessionKey);
          }
          if (newerMessages.nodes.length === 0) {
            const currentWindow = currentSession.messageWindow ?? newerMessages.messageWindow;
            const messageWindow = {
              ...currentWindow,
              totalCount: Math.max(currentWindow.totalCount, newerMessages.messageWindow.totalCount),
              hasAfter: newerMessages.hasNewerMessages,
            };
            return patchSession(current, conversation.id, {
              hasOlderMessages: messageWindow.hasBefore,
              isLoadingMessageWindow: false,
              messageWindow,
            }, sessionKey);
          }
          const expandedVisibleThreadedMessages = mergeConversationNodes(
            currentSession.conversation.threadedMessages,
            newerMessages.nodes,
          );
          const timeline = getConversationTimeline(current, conversation.id) ?? currentSession.conversation;
          // The active surface can contain transient local nodes; only persisted nodes enter the shared timeline cache.
          const cachedThreadedMessages = mergeConversationNodes(
            timeline.threadedMessages,
            expandedVisibleThreadedMessages.filter(isPersistedMessageNode),
          );
          const expandedMessageWindow = {
            ...newerMessages.messageWindow,
            startIndex: currentSession.messageWindow?.startIndex ?? newerMessages.messageWindow.startIndex,
            hasBefore: currentSession.messageWindow?.hasBefore ?? newerMessages.hasOlderMessages,
          };
          const { nodes: visibleThreadedMessages, window: messageWindow } = trimRenderedWindow(
            expandedVisibleThreadedMessages,
            expandedMessageWindow,
            'end',
          );
          const patches = patchSession(current, conversation.id, {
            hasOlderMessages: messageWindow.hasBefore,
            isLoadingMessageWindow: false,
            visibleThreadedMessages,
            messageWindow,
          }, sessionKey);
          return {
            ...patches,
            timelinesByConversationId: {
              ...(patches.timelinesByConversationId ?? current.timelinesByConversationId ?? {}),
              [conversation.id]: {
                ...timeline,
                threadedMessages: cachedThreadedMessages,
              },
            },
          };
        });
      } catch (error) {
        console.error('[Chat] Erro ao carregar mensagens posteriores:', error);
        set((current) => {
          const currentSession = getSession(current, conversationId, sessionKey);
          return patchSession(current, conversationId, {
            hasOlderMessages: currentSession.hasOlderMessages,
            isLoadingMessageWindow: false,
          }, sessionKey);
        });
      }
    },

    loadBoundaryMessagesForConversation: async (conversationId, sessionKey, anchor) => {
      const state = get();
      const session = getSession(state, conversationId, sessionKey);
      if (session.isLoadingMessageWindow) return;
      const window = session.messageWindow;
      if (anchor === 'start' && window?.startIndex === 0) return;
      if (anchor === 'end' && window && window.totalCount > 0 && window.endIndex >= window.totalCount - 1) return;

      set((current) => patchSession(current, conversationId, {
        hasOlderMessages: session.hasOlderMessages,
        isLoadingMessageWindow: true,
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
          const timeline = getConversationTimeline(current, conversationId) ?? baseConversation;
          const cachedThreadedMessages = mergeConversationNodes(
            timeline.threadedMessages,
            boundaryWindow.nodes,
          );
          const cachedById = new Map(cachedThreadedMessages.map((node) => [String(node.message.id), node]));
          const boundaryVisibleThreadedMessages = boundaryWindow.nodes.map((node) => (
            cachedById.get(String(node.message.id)) ?? node
          ));
          const { nodes: visibleThreadedMessages, window: messageWindow } = trimRenderedWindow(
            boundaryVisibleThreadedMessages,
            boundaryWindow.messageWindow,
            anchor === 'start' ? 'start' : 'end',
          );
          const patches = patchSession(current, conversationId, {
            hasOlderMessages: messageWindow.hasBefore,
            isLoadingMessageWindow: false,
            visibleThreadedMessages,
            messageWindow,
          }, sessionKey);
          return {
            ...patches,
            timelinesByConversationId: {
              ...(patches.timelinesByConversationId ?? current.timelinesByConversationId ?? {}),
              [conversationId]: {
                ...timeline,
                threadedMessages: cachedThreadedMessages,
              },
            },
          };
        });
      } catch (error) {
        console.error('[Chat] Erro ao carregar limite da conversa:', error);
        set((current) => {
          const currentSession = getSession(current, conversationId, sessionKey);
          return patchSession(current, conversationId, {
            hasOlderMessages: currentSession.hasOlderMessages,
            isLoadingMessageWindow: false,
          }, sessionKey);
        });
      }
    },

    updateConversationMessage: (conversationId, messageId, content) => {
      set((state) => {
        return patchConversation(state, conversationId, (conversation) => ({
          ...conversation,
          threadedMessages: updateMessageContentInTree(conversation.threadedMessages, messageId, content),
        }));
      });
    },

    updateConversationMessageReasoning: (conversationId, messageId, reasoning) => {
      set((state) => {
        return patchConversation(state, conversationId, (conversation) => ({
          ...conversation,
          threadedMessages: updateMessageReasoningInTree(conversation.threadedMessages, messageId, reasoning),
        }));
      });
    },

    addInternalMessage: (message) => {
      const conversationId = String(message.conversationId || '');
      if (!conversationId) return;
      const messageForTree = message.turnId
        ? stripInternalMessageTurnId(message)
        : message;
      if (message.turnId) {
        const warning = '[Chat] addInternalMessage recebeu turnId; removendo para evitar colisão com timeline canônica';
        if (import.meta.env.DEV) {
          console.error(new Error(warning));
        } else {
          console.warn(warning);
        }
      }

      set((state) => {
        const session = getSession(state, conversationId);
        if (!session.conversation) return state;
        const patches = patchSession(state, conversationId, {
          conversation: {
            ...session.conversation,
            threadedMessages: appendInternalMessageToTree(session.conversation.threadedMessages, messageForTree),
          },
        });
        const updatedTimeline = patches.timelinesByConversationId?.[conversationId]
          ?? getConversationTimeline(state, conversationId)
          ?? session.conversation;
        const timelinesByConversationId = { ...(patches.timelinesByConversationId ?? state.timelinesByConversationId ?? {}) };
        timelinesByConversationId[conversationId] = {
          ...updatedTimeline,
          threadedMessages: updatedTimeline.threadedMessages.filter(isPersistedMessageNode),
        };
        const surfaceSessionsByKey = { ...(patches.surfaceSessionsByKey ?? state.surfaceSessionsByKey ?? {}) };
        const defaultSessionKey = getDefaultChatSessionKey(conversationId);
        for (const [key, surfaceSession] of Object.entries(state.surfaceSessionsByKey ?? {})) {
          if (key === defaultSessionKey || surfaceSession.conversationId !== conversationId || !surfaceSession.visibleThreadedMessages) {
            continue;
          }
          surfaceSessionsByKey[key] = {
            ...surfaceSession,
            visibleThreadedMessages: capRenderedNodesAtEnd(
              appendInternalMessageToTree(surfaceSession.visibleThreadedMessages, messageForTree),
            ),
          };
        }
        return {
          ...patches,
          timelinesByConversationId,
          surfaceSessionsByKey,
        };
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
          return patchConversation(state, targetConversationId, (conversation) => ({
            ...conversation,
            threadedMessages: attachChildrenToMessage(conversation.threadedMessages, messageId, frontendNodes),
          }));
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
