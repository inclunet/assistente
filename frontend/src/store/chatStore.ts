import { create } from 'zustand';
import {
  SendMessage,
  RetryMessage,
} from '@wailsjs/go/wailsapi/Chat';
import { CancelStreamingForConversation, CancelStreamingExecution } from '@wailsjs/go/wailsapi/LLMModels';
import {
  AssignConversationToChannel,
  UnassignConversationFromChannel,
} from '@wailsjs/go/wailsapi/Messaging';
import { EnsureConversation } from '@wailsjs/go/wailsapi/Conversations';
import { MediaFile } from '../services/mediaService';
import { ChatMessagingStaleError, type ChatMessagingExecution } from '../lib/commandChatMessaging';
import {
  cancelMediaSerialization,
  isMediaSerializationError,
  serializeMediaForConversation,
} from '../services/mediaSerialization';
import { llm } from '../../wailsjs/go/models';
import { announce } from '../hooks/useAnnouncer';
import { logger } from '../utils/logger';
import i18next from 'i18next';
import { translateProviderErrorMessage } from '../lib/providerErrorMessage';
import { playSendSound } from '../services/audioFeedback';
import { isChatConversationActive } from '../services/chatArbitration';
import {
  startChatEventController,
  getChatEventControllerCleanup,
  getChatEventControllerExecutionId,
  stopAllChatEventControllers,
  stopChatEventController,
  type ChatEventSession,
  type ChatEventControllerHandle,
} from '../services/chatEventController';
import { handleExternalChatIncoming } from '../services/externalChatController';
import {
  createConversationTurnQueue,
  isConversationTurnQueueCancelledError,
  isConversationTurnQueueClearedError,
} from '../services/chatTurnQueue';
import { loadConversationSnapshot } from '../services/chatSessionLoader';
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
} from '../services/chatSessionRegistry';
import {
  getUtf8ByteLength,
  MAX_MESSAGE_CONTENT_BYTES,
  MAX_MESSAGE_CONTENT_KIB,
} from '../lib/messageContentLimit';

const MAX_MEDIA_SIZE = 20 * 1024 * 1024;            // must match backend MaxMediaSize

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

interface ChatStore {
  sessionsByConversationId: Record<string, ChatConversationSession>;
  timelinesByConversationId: Record<string, ConversationTimeline>;
  surfaceSessionsByKey: Record<string, ChatSurfaceSession>;
  loadingConversationIds: Set<string>;
  isInitialized: boolean;

  setConversationEditingMessageId: (conversationId: string, id: string | null, sessionKey: string) => void;
  startConversationEditing: (conversationId: string, id: string, sessionKey: string) => void;
  consumeSkipFocusRestore: (conversationId: string, sessionKey: string) => boolean;
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
  clearConversationSendFailure: (conversationId: string, sessionKey: string) => void;

  createConversation: (title?: string) => Promise<string>;
  loadConversationSession: (id: string, options?: { refreshSurfaceWindows?: boolean }) => Promise<void>;
  getConversationSession: (conversationId: string | null | undefined) => ChatConversationSession | null;


  toggleConversationThreadExpanded: (conversationId: string, messageId: string, sessionKey: string) => void;
  isConversationThreadExpanded: (conversationId: string, messageId: string, sessionKey: string) => boolean;
  toggleConversationReasoningExpanded: (conversationId: string, messageId: string, sessionKey: string) => void;
  isConversationReasoningExpanded: (conversationId: string, messageId: string, sessionKey: string) => boolean;

  sendMessageToConversation: (
    conversationId: string,
    content: string,
    mediaFiles?: MediaFile[],
    paramsOverride?: Partial<llm.ChatParams>,
    options?: { origin?: ChatSurfaceOrigin; command?: ChatMessagingExecution; onExecutionStarted?: (executionId: string) => void },
  ) => Promise<boolean>;
  retryMessageToConversation: (
    conversationId: string,
    messageId: string,
    paramsOverride?: Partial<llm.ChatParams>,
    options?: { origin?: ChatSurfaceOrigin; command?: ChatMessagingExecution; onExecutionStarted?: (executionId: string) => void },
  ) => Promise<boolean>;
  cancelStreaming: (conversationId: string, options?: { origin?: ChatSurfaceOrigin }) => Promise<void>;
  waitForMessagingAdmission: (conversationId: string, sessionKey?: string) => Promise<void>;
  registerMessagingAdmission: (conversationId: string, sessionKey?: string) => { activate: () => boolean; complete: () => void; cancelled: Promise<never> };
  finishCommandCancellation: (conversationId: string, sessionKey: string | undefined, pipelineRevision: number, announceCancelled?: boolean) => void;
  getMessagingPipelineRevision: (conversationId: string) => number;
  getDraftRevision: (sessionKey: string) => number;
  cancelConversationTurn: (conversationId: string) => void;
  cancelNextQueuedTurn: (conversationId: string) => boolean;


  handleConversationDeleted: (conversationId: string) => void;
  handleConversationCleared: (conversationId: string) => void;
  handleConversationRenamed: (conversationId: string, newTitle: string) => void;
  handleDatabaseReset: () => void;

  assignConversationChannel: (conversationId: string, channel: string, contactId: string) => Promise<void>;
  unassignConversationChannel: (conversationId: string) => Promise<void>;
  handleExternalIncoming: (data: {
    channel: string; from: string; fromId?: string; text: string; conversationId: string;
    newConversation?: boolean;
  }) => void;
}

export const useChatStore = create<ChatStore>()((set, get) => {
  const turnQueue = createConversationTurnQueue();
  const pipelineRevisions = new Map<string, number>();
  const draftRevisions = new Map<string, number>();

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

  const queuedTurnCountGenerations = new Map<string, number>();
  const adjustQueuedTurnCount = (conversationId: string, delta: number, sessionKey?: string) => {
    set((state) => {
      const session = getSession(state, conversationId, sessionKey);
      const queuedTurnCount = Math.max(0, (session.queuedTurnCount ?? 0) + delta);
      return patchSession(state, conversationId, { queuedTurnCount }, sessionKey);
    });
  };

  const countQueuedTurn = (conversationId: string, sessionKey?: string) => {
    const generation = queuedTurnCountGenerations.get(conversationId) ?? 0;
    adjustQueuedTurnCount(conversationId, 1, sessionKey);
    let counted = true;
    return () => {
      if (!counted) return;
      counted = false;
      if ((queuedTurnCountGenerations.get(conversationId) ?? 0) === generation) {
        adjustQueuedTurnCount(conversationId, -1, sessionKey);
      }
    };
  };

  const resetQueuedTurnCount = (conversationId: string) => {
    queuedTurnCountGenerations.set(conversationId, (queuedTurnCountGenerations.get(conversationId) ?? 0) + 1);
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
    patchSession: (conversationId: string, patch: Partial<Omit<ChatEventSession, 'conversation'>> & Record<string, unknown>) => {
      set(state => {
        const sessionKey = patch.surfaceOrigin?.sessionKey;
        if (sessionKey) return patchSession(state, conversationId, patch, sessionKey);
        const base = patchSession(state, conversationId, patch);
        const surfaces = { ...(base.surfaceSessionsByKey ?? state.surfaceSessionsByKey) };
        for (const [key, surface] of Object.entries(surfaces)) {
          if (surface.conversationId === conversationId) surfaces[key] = { ...surface, ...patch };
        }
        return { ...base, surfaceSessionsByKey: surfaces };
      });
    },
    setConversationLoading,
  };

  const sendMessageInternal = async (
    conversationId: string,
    content: string,
    mediaFiles?: MediaFile[],
    paramsOverride?: Partial<llm.ChatParams>,
    retryMessageId?: string,
    options?: { origin?: ChatSurfaceOrigin; command?: ChatMessagingExecution; onExecutionStarted?: (executionId: string) => void },
  ): Promise<{ accepted: boolean; controller?: ChatEventControllerHandle }> => {
    if (options?.command && !options.command.isCurrent()) throw new ChatMessagingStaleError();
    if (mediaFiles && mediaFiles.length > 0) {
      const totalSize = mediaFiles.reduce((acc, f) => acc + f.file.size, 0);
      const estimatedBase64Size = Math.ceil(totalSize * 1.37);
      if (estimatedBase64Size > MAX_MEDIA_SIZE) {
        announce(i18next.t('chat.validation.mediaTooLarge', {
          defaultValue: 'Arquivos de mídia muito grandes (~{{size}}MB). Máximo permitido: {{max}}MB',
          size: Math.round(estimatedBase64Size / 1024 / 1024),
          max: Math.round(MAX_MEDIA_SIZE / 1024 / 1024),
        }));
        if (options?.command) throw new ChatMessagingStaleError();
        return { accepted: false };
      }
    }

    pipelineRevisions.set(conversationId, (pipelineRevisions.get(conversationId) ?? 0) + 1);
    options?.command?.onPipelineStarted?.(pipelineRevisions.get(conversationId)!);
    playSendSound();
    const executionOrigin = options?.origin
      ? { ...options.origin, executionId: crypto.randomUUID() }
      : undefined;
    const controller = startChatEventController({
      conversationId,
      initialUserContent: content,
      initialMediaFiles: mediaFiles,
      origin: executionOrigin,
      adapter: chatEventAdapter,
    });
    let submitted = false;
    try {
      if (executionOrigin) options?.onExecutionStarted?.(executionOrigin.executionId);
      const mediaJson = mediaFiles && mediaFiles.length > 0
        ? await serializeMediaForConversation(conversationId, mediaFiles)
        : '';
      if (options?.command && !options.command.isCurrent()) throw new ChatMessagingStaleError();

      const mergedParams = new llm.ChatParams({
        model: paramsOverride?.model ?? '',
        temperature: paramsOverride?.temperature,
        maxTokens: paramsOverride?.maxTokens,
        maxTokensMode: paramsOverride?.maxTokensMode,
        topP: paramsOverride?.topP,
        reasoningEffort: paramsOverride?.reasoningEffort,
        profileSlug: paramsOverride?.profileSlug,
        allowAssistantPrefill: paramsOverride?.allowAssistantPrefill,
        tabType: paramsOverride?.tabType,
        activeFilePath: paramsOverride?.activeFilePath,
        surfaceStateJson: paramsOverride?.surfaceStateJson,
        surfaceContextJson: paramsOverride?.surfaceContextJson,
        surfaceSessionKey: options?.origin?.sessionKey,
        surfaceExecutionId: executionOrigin?.executionId,
        surfaceId: options?.origin?.surfaceId,
        surfaceType: options?.origin?.surfaceType,
        surfaceTabId: options?.origin?.tabId,
        ...(options?.command ? { command: { ...options.command.handoff } } : {}),
      } satisfies Omit<llm.ChatParams, 'convertValues'>);
      submitted = true;
      if (retryMessageId) {
        await RetryMessage(conversationId, retryMessageId, mergedParams);
      } else {
        await SendMessage(conversationId, content, mediaJson, mergedParams);
      }
      return { accepted: true, controller };
    } catch (error: unknown) {
      if (options?.command) {
        if (error instanceof ChatMessagingStaleError || (error instanceof DOMException && error.name === 'AbortError') || isMediaSerializationError(error)) {
          controller.handleSendCancellation();
          throw new ChatMessagingStaleError();
        }
        const rawErrorMessage = getErrorMessage(error);
        const errorMessage = translateProviderErrorMessage(rawErrorMessage) ?? rawErrorMessage;
        if (submitted) {
          // A Wails rejection may be pre-effect (denied/stale) or uncertain
          // after persistence. Surface it without offering replay; the command
          // ledger's settled status decides whether the event controller stays.
          set(state => patchSession(state, conversationId, {
            sendFailureMessage: i18next.t('chat.sendErrorPrefix', { message: errorMessage }),
            sendFailureAnnounced: false,
            sendFailureRetryable: false,
            sendFailureRetryContent: null,
            sendFailureRetryMediaFiles: [],
          }, options.origin?.sessionKey));
        } else {
          // Before invoking Wails no message could have been persisted, so the
          // existing recovery UI may safely offer an explicit retry.
          controller.handleSendFailure(errorMessage, true);
        }
        throw error;
      }
      if (error instanceof DOMException && error.name === 'AbortError') {
        controller.handleSendCancellation();
        return { accepted: false, controller };
      }
      const errorMsg = isMediaSerializationError(error)
        ? i18next.t('chat.errors.mediaSerializationFailed')
        : getErrorMessage(error);
      controller.handleSendFailure(errorMsg, !submitted);
      return { accepted: false, controller };
    }
  };

  const validateSendPayload = (content: string, mediaFiles?: MediaFile[]): boolean => {
    const contentBytes = getUtf8ByteLength(content);
    if (contentBytes > MAX_MESSAGE_CONTENT_BYTES) {
      announce(i18next.t('chat.validation.messageTooLarge', {
        sizeBytes: contentBytes,
        maxKiB: MAX_MESSAGE_CONTENT_KIB,
      }), 'assertive');
      return false;
    }

    if (mediaFiles && mediaFiles.length > 0) {
      const totalSize = mediaFiles.reduce((acc, f) => acc + f.file.size, 0);
      const estimatedBase64Size = Math.ceil(totalSize * 1.37);
      if (estimatedBase64Size > MAX_MEDIA_SIZE) {
        announce(i18next.t('chat.validation.mediaTooLarge', {
          defaultValue: 'Arquivos de mídia muito grandes (~{{size}}MB). Máximo permitido: {{max}}MB',
          size: Math.round(estimatedBase64Size / 1024 / 1024),
          max: Math.round(MAX_MEDIA_SIZE / 1024 / 1024),
        }), 'assertive');
        return false;
      }
    }
    return true;
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

    setConversationDraftMessage: (conversationId, message, sessionKey) => {
      draftRevisions.set(sessionKey, (draftRevisions.get(sessionKey) ?? 0) + 1);
      set((state) => patchSurfaceSession(state, conversationId, { draftMessage: message }, sessionKey));
    },

    setConversationDraftMediaFiles: (conversationId, mediaFiles, sessionKey) => {
      draftRevisions.set(sessionKey, (draftRevisions.get(sessionKey) ?? 0) + 1);
      set((state) => patchSurfaceSession(state, conversationId, { draftMediaFiles: mediaFiles }, sessionKey));
    },

    clearConversationDraft: (conversationId, sessionKey) => {
      draftRevisions.set(sessionKey, (draftRevisions.get(sessionKey) ?? 0) + 1);
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

    clearConversationSendFailure: (conversationId, sessionKey) => {
      set((state) => patchSession(state, conversationId, {
        sendFailureMessage: null,
        sendFailureAnnounced: false,
        sendFailureRetryable: false,
        sendFailureRetryContent: null,
        sendFailureRetryMediaFiles: [],
      }, sessionKey));
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
      const snapshot = await loadConversationSnapshot(id);
      set(state => ({ ...patchSession(state, id, {
        conversation: { id, ...snapshot },
        isLoading: state.loadingConversationIds.has(id),
      }), isInitialized: true }));
    },

    getConversationSession: (conversationId) => {
      if (!conversationId) return null;
      return get().sessionsByConversationId[conversationId] ?? null;
    },

    sendMessageToConversation: async (conversationId, content, mediaFiles, paramsOverride, options) => {
      if (options?.command && (!conversationId || !options.command.isCurrent())) throw new ChatMessagingStaleError();
      if (!conversationId) {
        logger.error('[Chat] sendMessageToConversation sem conversationId explícito');
        announce(i18next.t('chat.errors.noActiveConversation'), 'assertive');
        return false;
      }
      const contentBytes = getUtf8ByteLength(content);
      if (contentBytes > MAX_MESSAGE_CONTENT_BYTES) {
        announce(i18next.t('chat.validation.messageTooLarge', {
          sizeBytes: contentBytes,
          maxKiB: MAX_MESSAGE_CONTENT_KIB,
        }), 'assertive');
        if (options?.command) throw new ChatMessagingStaleError();
        return false;
      }
      if (!validateSendPayload(content, mediaFiles)) return false;
      if (!getConversationTimeline(get(), conversationId)) {
        await get().loadConversationSession(conversationId);
      }
      const sessionKey = options?.origin?.sessionKey;
      const queuedBehindActiveTurn = turnQueue.isQueued(conversationId);
      const settleQueuedCount = queuedBehindActiveTurn ? countQueuedTurn(conversationId, sessionKey) : undefined;
      let markAccepted!: (accepted: boolean) => void;
      let rejectAccepted!: (error: unknown) => void;
      const accepted = new Promise<boolean>((resolve, reject) => { markAccepted = resolve; rejectAccepted = reject; });
      void turnQueue.enqueue(conversationId, async () => {
          settleQueuedCount?.();
          const result = await sendMessageInternal(conversationId, content, mediaFiles, paramsOverride, undefined, options);
          markAccepted(result.accepted);
          await result.controller?.done;
        }, settleQueuedCount, settleQueuedCount).catch((error) => {
          if (options?.command) rejectAccepted(
            isConversationTurnQueueClearedError(error) || isConversationTurnQueueCancelledError(error)
              ? new ChatMessagingStaleError()
              : error,
          );
          else markAccepted(false);
          if (!isConversationTurnQueueClearedError(error) && !isConversationTurnQueueCancelledError(error)) {
            logger.error('[Chat] falha inesperada na fila do turno', error);
          }
        });
      return accepted;
    },

    retryMessageToConversation: async (conversationId, messageId, paramsOverride, options) => {
      if (options?.command && (!conversationId || !messageId || !options.command.isCurrent())) throw new ChatMessagingStaleError();
      if (!conversationId || !messageId) {
        logger.error('[Chat] retryMessageToConversation sem conversationId/messageId válido');
        announce(i18next.t('chat.errors.noActiveConversation'), 'assertive');
        return false;
      }
      if (!getConversationTimeline(get(), conversationId)) {
        await get().loadConversationSession(conversationId);
      }
      const sessionKey = options?.origin?.sessionKey;
      const queuedBehindActiveTurn = turnQueue.isQueued(conversationId);
      const settleQueuedCount = queuedBehindActiveTurn ? countQueuedTurn(conversationId, sessionKey) : undefined;
      let markAccepted!: (accepted: boolean) => void;
      let rejectAccepted!: (error: unknown) => void;
      const accepted = new Promise<boolean>((resolve, reject) => { markAccepted = resolve; rejectAccepted = reject; });
      void turnQueue.enqueue(conversationId, async () => {
          settleQueuedCount?.();
          const result = await sendMessageInternal(conversationId, '', undefined, paramsOverride, messageId, options);
          markAccepted(result.accepted);
          await result.controller?.done;
        }, settleQueuedCount, settleQueuedCount).catch((error) => {
          if (options?.command) rejectAccepted(
            isConversationTurnQueueClearedError(error) || isConversationTurnQueueCancelledError(error)
              ? new ChatMessagingStaleError()
              : error,
          );
          else markAccepted(false);
          if (!isConversationTurnQueueClearedError(error) && !isConversationTurnQueueCancelledError(error)) {
            logger.error('[Chat] falha inesperada na fila de retry', error);
          }
        });
      return accepted;
    },

    waitForMessagingAdmission: async (conversationId, sessionKey) => {
      const queuedBehindActiveTurn = turnQueue.isQueued(conversationId);
      const settleCount = queuedBehindActiveTurn ? countQueuedTurn(conversationId, sessionKey) : undefined;
      try {
        await turnQueue.enqueue(
          conversationId,
          async () => {},
          settleCount,
          settleCount,
        );
      } catch (error) {
        settleCount?.();
        if (isConversationTurnQueueCancelledError(error) || isConversationTurnQueueClearedError(error)) {
          throw new ChatMessagingStaleError();
        }
        throw error;
      } finally { settleCount?.(); }
    },
    registerMessagingAdmission: (conversationId, sessionKey) => {
      const queuedBehindActiveWork = turnQueue.isQueued(conversationId) || turnQueue.hasPendingAdmission(conversationId);
      const settleCount = queuedBehindActiveWork ? countQueuedTurn(conversationId, sessionKey) : undefined;
      let settled = false;
      const settle = () => {
        if (settled) return;
        settled = true;
        settleCount?.();
      };
      const registration = turnQueue.registerPendingAdmission(conversationId, settle);
      return {
        cancelled: registration.cancelled,
        activate: () => {
          const activated = registration.activate();
          if (activated) settle();
          return activated;
        },
        complete: () => {
          registration.complete();
          settle();
        },
      };
    },
    getMessagingPipelineRevision: conversationId => pipelineRevisions.get(conversationId) ?? 0,
    getDraftRevision: sessionKey => draftRevisions.get(sessionKey) ?? 0,
    finishCommandCancellation: (conversationId, sessionKey, pipelineRevision, announceCancelled = true) => {
      if ((pipelineRevisions.get(conversationId) ?? 0) !== pipelineRevision) return;
      pipelineRevisions.set(conversationId, pipelineRevision + 1);
      cancelMediaSerialization(conversationId);
      stopChatEventController(conversationId);
      setConversationLoading(conversationId, false, sessionKey);
      if (announceCancelled) announce(i18next.t('chat.announce.streamingCancelled'));
    },
    cancelStreaming: async (conversationId, options) => {
      if (!conversationId) {
        logger.error('[Chat] cancelStreaming sem conversationId explícito');
        announce(i18next.t('chat.errors.noActiveConversation'), 'assertive');
        return;
      }

      const sessionKey = options?.origin?.sessionKey;
      const revision = get().getMessagingPipelineRevision(conversationId);
      const cancelledController = getChatEventControllerCleanup(conversationId);
      const cancelledExecutionId = getChatEventControllerExecutionId(conversationId);

      try {
        cancelMediaSerialization(conversationId);
        if (cancelledExecutionId) {
          await CancelStreamingExecution(conversationId, cancelledExecutionId);
        } else {
          await CancelStreamingForConversation(conversationId);
        }
        // Um terminal pode liberar a fila enquanto o binding retorna. Não
        // finalize o controller do próximo envio por usar a mesma conversa.
        if (getChatEventControllerCleanup(conversationId) !== cancelledController) return;
        get().finishCommandCancellation(conversationId, sessionKey, revision);
      } catch (error: unknown) {
        const errorMsg = getErrorMessage(error);
        logger.error('[Chat] falha ao cancelar streaming', error);
        announce(i18next.t('chat.errors.cancelStreamingFailed', { message: errorMsg }), 'assertive');
      }
    },

    cancelConversationTurn: (conversationId) => {
      cancelMediaSerialization(conversationId);
      turnQueue.clear(conversationId);
      stopChatEventController(conversationId);
      setConversationLoading(conversationId, false);
      resetQueuedTurnCount(conversationId);
    },

    cancelNextQueuedTurn: (conversationId) => turnQueue.cancelNext(conversationId),

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

    handleConversationDeleted: (conversationId: string) => {
      turnQueue.clear(conversationId);
      stopChatEventController(conversationId);
      set(state => removeChatSession(state, conversationId));
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

    handleExternalIncoming: (data) => {
      void handleExternalChatIncoming(data, {
        hasConversationSession: (conversationId) => !!getConversationTimeline(get(), conversationId),
        loadConversationSession: (conversationId) => get().loadConversationSession(conversationId),
        // Mensagens recebidas por canal não são envios da toolbar e não podem
        // ser descartadas pela ação local de cancelar o próximo turno. Elas
        // continuam serializadas, mas não entram na contagem cancelável da UI.
        enqueueExternalTurn: async (conversationId, _sessionKey, task) => {
          try {
            await turnQueue.enqueue(conversationId, async () => {
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
