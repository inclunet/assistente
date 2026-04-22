import { create } from 'zustand';
import {
  SendMessage,
  RetryMessage,
  GetMessages,
  GetConversationInfo,
  EnsureConversation,
  AssignConversationToChannel,
  UnassignConversationFromChannel,
  GetMessageChildren,
} from '@wailsjs/go/app/App';
import { EventsOn } from '@wailsjs/runtime/runtime';
import { MediaFile } from '../services/mediaService';
import { llm, main } from '../../wailsjs/go/models';
import { announce } from '../hooks/useAnnouncer';
import i18next from 'i18next';
import { playSendSound, playReceiveSound } from '../services/audioFeedback';
import { ttsService } from '../services/tts';
import { messageAudioService } from '../services/messageAudio';
import { stripMarkdown } from '../lib/stripMarkdown';
import type { ToolCallStatus } from '../components/chat/ToolCallsSection';
import { handleChatSpeak } from '../services/chatSpeak';
import type { ChatSpeakEvent } from '../services/chatSpeak';

const MAX_MESSAGE_CONTENT_SIZE = 512 * 1024;       // must match backend MaxMessageContentSize
const MAX_MEDIA_SIZE = 20 * 1024 * 1024;            // must match backend MaxMediaSize
const STREAM_UPDATE_DEBOUNCE_MS = 16;

interface MediaData {
  name: string;
  type: string;
  data: string;
  size: number;
}

export type MessageNode = main.MessageNode & {
  originalIndex?: number;
  isExpanded?: boolean;
};

export type Message = main.EnrichedMessage & {
  _turnSegments?: TurnSegment[];
};

export interface TurnSegment {
  type: 'text' | 'tool_calls';
  content?: string;
  toolCalls?: Array<{
    id: string;
    type: string;
    function: { name: string; arguments: string };
    result?: string;
  }>;
}

interface ChatMessagesReadyEvent {
  conversationId: number;
  userMessageId: number;
  userContent: string;
}

interface ChatStreamEvent {
  conversationId: number;
  content?: string;
  done?: boolean;
  error?: string;
  messageId?: number;
}

interface ChatThinkingEvent {
  conversationId: number;
  started?: boolean;
  done?: boolean;
  content?: string;
}

interface ChatToolStartEvent {
  conversationId: number;
  name: string;
  callId: string;
  args?: string;
  serverLabel?: string;
  /** @deprecated Use origin instead (AEP-0039) */
  native?: boolean;
  origin?: 'builtin' | 'mcp_bridge' | 'mcp_native';
  attempt?: number;
}

interface ChatToolEndEvent {
  conversationId: number;
  callId: string;
  name?: string;
  status?: string;
  summary?: string;
  error?: string;
  serverLabel?: string;
  /** @deprecated Use origin instead (AEP-0039) */
  native?: boolean;
  origin?: 'builtin' | 'mcp_bridge' | 'mcp_native';
  durationMs?: number;
  attempt?: number;
}

// AEP-0039 Fase 3: structured failure event (distinct from tool_end with status='error')
interface ChatToolFailureEvent {
  conversationId: number;
  name: string;
  callId: string;
  errorKind: 'timeout' | 'invalid_args' | 'not_found' | 'panic' | 'unknown';
  retryable: boolean;
  message?: string;
  durationMs?: number;
  origin?: 'builtin' | 'mcp_bridge' | 'mcp_native';
  willRetry?: boolean;
  attempt?: number;
}

interface ChatSegmentDoneEvent {
  conversationId: number;
  hasMore?: boolean;
  content?: string;
  iteration?: number;
  // AEP-0039 Fase 2+3
  toolsInIteration?: Array<{ name: string; status: string; errorKind?: string; durationMs?: number; origin?: string; serverLabel?: string }>;
}

interface ChatDoneEvent {
  conversationId: number;
  assistantMessageId?: number;
  hadToolCalls?: boolean;
  // AEP-0039 Fase 2
  reason?: string;
  iterationCount?: number;
  toolCallCount?: number;
  toolsUsed?: string[];
  promptTokens?: number;
  completionTokens?: number;
  errorMessage?: string;
}

interface ChatErrorEvent {
  conversationId: number;
  error: string;
}

export interface ActiveConversation {
  id: number;
  title: string;
  threadedMessages: MessageNode[];
  channel?: string;
  contactId?: string;
}

function flattenThreadedMessages(nodes: MessageNode[] | undefined): Message[] {
  if (!nodes || nodes.length === 0) return [];
  const flat: Message[] = [];
  function traverse(node: MessageNode) {
    flat.push(node.message);
    if (node.children && node.children.length > 0) {
      node.children.forEach(traverse);
    }
  }
  nodes.forEach(traverse);
  return flat;
}

const withOriginalIndex = (node: main.MessageNode, index: number): MessageNode => {
  const typed = node as MessageNode;
  typed.originalIndex = index;
  return typed;
};

const getErrorMessage = (error: unknown): string => {
  if (error instanceof Error) return error.message;
  if (typeof error === 'string') return error;
  try {
    return JSON.stringify(error);
  } catch {
    return String(error);
  }
};

const activeListeners = new Map<string, () => void>();
let streamingMessageSeq = 0;

const streamUpdateTimers = new Map<string, NodeJS.Timeout>();
const pendingStreamUpdates = new Map<string, { messageId: string; content: string }>();

const createStreamingMessageId = (conversationId: number): string => {
  streamingMessageSeq += 1;
  return `streaming-${conversationId}-${streamingMessageSeq}`;
};

const hasMessageId = (
  nodes: MessageNode[] | undefined,
  targetId: string,
  excludeId?: string,
): boolean => {
  if (!nodes || nodes.length === 0) return false;
  for (const node of nodes) {
    const id = String(node.message.id);
    if (id === targetId && id !== excludeId) return true;
    if (node.children?.length && hasMessageId(node.children, targetId, excludeId)) return true;
  }
  return false;
};

const finalizeStreamingNode = (
  conversation: ActiveConversation,
  syntheticId: string,
  finalId?: string | null,
): ActiveConversation => {
  const collidesWithExistingRealId = !!finalId && hasMessageId(conversation.threadedMessages, finalId, syntheticId);
  const markDone = (nodes: MessageNode[]): MessageNode[] => nodes.flatMap((node) => {
    const id = String(node.message.id);
    if (id === syntheticId) {
      if (collidesWithExistingRealId) {
        return [];
      }
      node.message.isStreaming = false;
      if (finalId) node.message.id = finalId;
    } else if (collidesWithExistingRealId && finalId && id === finalId) {
      node.message.isStreaming = false;
    }
    if (node.children?.length) node.children = markDone(node.children);
    return [node];
  });

  return {
    ...conversation,
    threadedMessages: markDone(conversation.threadedMessages),
  };
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

const debouncedUpdateMessage = (
  messageId: string,
  content: string,
  updateFn: (messageId: string, content: string) => void
) => {
  pendingStreamUpdates.set(messageId, { messageId, content });
  const existingTimer = streamUpdateTimers.get(messageId);
  if (existingTimer) clearTimeout(existingTimer);
  const timer = setTimeout(() => {
    const pending = pendingStreamUpdates.get(messageId);
    if (pending) {
      updateFn(pending.messageId, pending.content);
      pendingStreamUpdates.delete(messageId);
      streamUpdateTimers.delete(messageId);
    }
  }, STREAM_UPDATE_DEBOUNCE_MS);
  streamUpdateTimers.set(messageId, timer);
};

const flushPendingUpdate = (
  messageId: string,
  updateFn: (messageId: string, content: string) => void
) => {
  const existingTimer = streamUpdateTimers.get(messageId);
  if (existingTimer) {
    clearTimeout(existingTimer);
    streamUpdateTimers.delete(messageId);
  }
  const pending = pendingStreamUpdates.get(messageId);
  if (pending) {
    updateFn(pending.messageId, pending.content);
    pendingStreamUpdates.delete(messageId);
  }
};

interface ChatStore {
  activeConversationId: number | null;
  activeConversation: ActiveConversation | null;
  isLoading: boolean;
  streamingMessageId: string | null;
  isInitialized: boolean;
  expandedThreads: Set<string>;
  editingMessageId: string | null;
  readingMessageId: string | null;
  skipFocusRestore: boolean;
  streamingReasoning: string | null;
  isThinking: boolean;
  expandedReasonings: Set<string>;
  contextProfileSlug: string | null;
  activeToolCalls: ToolCallStatus[];
  completedSegments: TurnSegment[];

  setContextProfileSlug: (slug: string | null) => void;
  setEditingMessageId: (id: string | null) => void;
  startEditing: (id: string) => void;
  consumeSkipFocusRestore: () => boolean;
  setReadingMessageId: (id: string | null) => void;
  startReading: (id: string) => void;

  createConversation: (title?: string) => Promise<number>;
  loadConversation: (id: number) => Promise<void>;
  /** Limpa a conversa ativa (ex.: ao focar editor/terminal/tasklist sem conversa até abrir o chat modal). */
  clearActiveConversation: () => void;

  updateMessage: (messageId: string, content: string) => void;
  updateMessageReasoning: (messageId: string, reasoning: string) => void;
  addInternalMessage: (message: Message) => void;
  clearMessages: () => void;

  toggleThreadExpanded: (messageId: string) => void;
  isThreadExpanded: (messageId: string) => boolean;
  toggleReasoningExpanded: (messageId: string) => void;
  isReasoningExpanded: (messageId: string) => boolean;

  sendMessage: (content: string, mediaFiles?: MediaFile[], paramsOverride?: Partial<llm.ChatParams>) => Promise<void>;
  sendMessageToConversation: (
    conversationId: number,
    content: string,
    mediaFiles?: MediaFile[],
    paramsOverride?: Partial<llm.ChatParams>,
  ) => Promise<void>;
  retryMessageToConversation: (
    conversationId: number,
    messageId: number,
    paramsOverride?: Partial<llm.ChatParams>,
  ) => Promise<void>;
  stopStreaming: () => void;

  getActiveConversation: () => ActiveConversation | null;
  getMessages: () => Message[];
  getThreadedMessages: () => MessageNode[] | undefined;
  loadMessageChildren: (messageId: string) => Promise<MessageNode[]>;

  handleConversationDeleted: (conversationId: number) => void;
  handleConversationCleared: (conversationId: number) => void;
  handleConversationRenamed: (conversationId: number, newTitle: string) => void;
  handleDatabaseReset: () => void;

  assignChannel: (channel: string, contactId: string) => Promise<void>;
  unassignChannel: () => Promise<void>;

  reloadMessages: () => Promise<void>;
  reloadConversationMessages: (conversationId: number) => Promise<void>;
  handleExternalIncoming: (data: {
    channel: string; from: string; fromId?: string; text: string; conversationId: number;
    newConversation?: boolean;
  }) => void;
}

export const useChatStore = create<ChatStore>()((set, get) => {
  let loadConversationSeq = 0;

  const sendMessageInternal = async (
    conversationId: number,
    content: string,
    mediaFiles?: MediaFile[],
    paramsOverride?: Partial<llm.ChatParams>,
    retryMessageId?: number,
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

    const conversationIdStr = conversationId.toString();

    // Backend-driven: sem addMessage local — user msg vem do chat:messages_ready, assistant do chat:stream
    set({ isLoading: true, completedSegments: [], activeToolCalls: [] });
    playSendSound();

    // ID determinístico para placeholder de streaming (substituído pelo ID real do backend em chat:done)
    const streamingMsgId = createStreamingMessageId(conversationId);
    let cleanupExecuted = false;
    let streamingAnnounced = false;
    let assistantNodeCreated = false;

    // Declarar unsubs antes de cleanup para evitar TDZ (temporal dead zone)
    const noop = () => { /* no-op */ };
    let unsubMessagesReady = noop;
    let unsubStream = noop;
    let unsubThinking = noop;
    let unsubToolStart = noop;
    let unsubToolEnd = noop;
    let unsubToolFailure = noop;
    let unsubSegmentDone = noop;
    let unsubDone = noop;
    let unsubError = noop;
    let unsubSpeak = noop;

    const ensureAssistantNode = () => {
      if (assistantNodeCreated) return;
      assistantNodeCreated = true;
      const assistantMsg = new main.EnrichedMessage({
        id: streamingMsgId,
        role: 'assistant',
        content: '',
        timestamp: Date.now(),
        conversationId,
        isStreaming: true,
        internal: false,
        createdAt: new Date().toISOString(),
      });
      const assistantNode = new main.MessageNode({ message: assistantMsg, children: [], level: 0, childCount: 0 });
      set((state) => ({
        activeConversation: state.activeConversation
          ? { ...state.activeConversation, threadedMessages: [...state.activeConversation.threadedMessages, assistantNode] }
          : state.activeConversation,
        streamingMessageId: streamingMsgId,
      }));
    };

    const cleanup = () => {
      if (cleanupExecuted) return;
      cleanupExecuted = true;
      unsubMessagesReady();
      unsubStream();
      unsubThinking();
      unsubToolStart();
      unsubToolEnd();
      unsubToolFailure();
      unsubSegmentDone();
      unsubDone();
      unsubError();
      unsubSpeak();
      activeListeners.delete(conversationIdStr);
      set({ isLoading: false, streamingMessageId: null, streamingReasoning: null, isThinking: false, activeToolCalls: [], completedSegments: [] });
    };

    const existingCleanup = activeListeners.get(conversationIdStr);
    if (existingCleanup) {
      existingCleanup();
      await new Promise(resolve => setTimeout(resolve, 0));
    }

    // chat:error → erro de validação do backend (tamanho, provider, etc.)
    unsubError = EventsOn('chat:error', (event: ChatErrorEvent) => {
      if (event.conversationId !== conversationId && event.conversationId !== 0) return;
      if (!activeListeners.has(conversationIdStr)) return;
      set((state) => {
        if (!state.activeConversation) return state;
        return {
          activeConversation: finalizeStreamingNode(state.activeConversation, streamingMsgId),
        };
      });
      announce(event.error);
      cleanup();
    });

    // chat:speak → TTS proativo disparado pelo backend antes de chat:done, para evitar cleanup prematuro dos listeners
    unsubSpeak = EventsOn('chat:speak', (event: ChatSpeakEvent) => {
      if (event.conversationId !== conversationId) return;
      if (!activeListeners.has(conversationIdStr)) return;
      void handleChatSpeak(event).catch((err) => {
        announce(i18next.t('chat.autoReadError'));
        console.error('[chat:speak] falha ao processar evento TTS', err);
      });
    });

    // chat:messages_ready → insere mensagem do usuário com ID REAL do backend (sem temp ID)
    unsubMessagesReady = EventsOn('chat:messages_ready', (data: ChatMessagesReadyEvent) => {
      if (data.conversationId !== conversationId) return;
      if (!activeListeners.has(conversationIdStr)) return;
      if (!data.userMessageId) return;
      if (hasMessageId(get().activeConversation?.threadedMessages, String(data.userMessageId))) return;
      const userMsg = new main.EnrichedMessage({
        id: data.userMessageId.toString(),
        role: 'user',
        content: data.userContent || content,
        timestamp: Date.now(),
        conversationId: data.conversationId,
        isStreaming: false,
        internal: false,
        createdAt: new Date().toISOString(),
      });
      const userNode = new main.MessageNode({ message: userMsg, children: [], level: 0, childCount: 0 });
      set((state) => ({
        activeConversation: state.activeConversation
          ? {
              ...state.activeConversation,
              threadedMessages: [...state.activeConversation.threadedMessages, userNode],
            }
          : state.activeConversation,
      }));
    });

    // chat:stream → cria placeholder de assistant na primeira chunk, atualiza conteúdo
    unsubStream = EventsOn('chat:stream', (event: ChatStreamEvent) => {
      if (event.conversationId !== conversationId) return;
      if (!activeListeners.has(conversationIdStr)) return;

      if (event.content && !event.done && !event.error) {
        ensureAssistantNode();
        if (!streamingAnnounced) {
          streamingAnnounced = true;
          announce(i18next.t('chat.announce.assistantResponding'), 'polite');
        }
        debouncedUpdateMessage(streamingMsgId, event.content, get().updateMessage);
      }

      if (event.error) {
        ensureAssistantNode();
        flushPendingUpdate(streamingMsgId, get().updateMessage);
        get().updateMessage(
          streamingMsgId,
          i18next.t('chat.errorPrefix', { message: event.error }),
        );
        set((state) => {
          if (!state.activeConversation) return state;
          return {
            activeConversation: finalizeStreamingNode(state.activeConversation, streamingMsgId),
          };
        });
        cleanup();
      }

      if (event.done) {
        ensureAssistantNode();
        flushPendingUpdate(streamingMsgId, get().updateMessage);
        if (event.content) get().updateMessage(streamingMsgId, event.content);

        const backendAssistantId = event.messageId && event.messageId > 0
          ? event.messageId.toString() : null;

        set((state) => {
          if (!state.activeConversation) return state;
          const nextConversation = finalizeStreamingNode(state.activeConversation, streamingMsgId, backendAssistantId);
          return {
            activeConversation: nextConversation,
          };
        });

        const currentState = get();
        const flatMessages = flattenThreadedMessages(currentState.activeConversation?.threadedMessages);
        const finalMessage = flatMessages.find(m => m.id === (backendAssistantId || streamingMsgId));
        if (finalMessage?.content) {
          const isActiveConv = currentState.activeConversationId === conversationId;
          if (isActiveConv) playReceiveSound();
        }
      }
    });

    unsubThinking = EventsOn('chat:thinking', (event: ChatThinkingEvent) => {
      if (event.conversationId !== conversationId) return;
      if (!activeListeners.has(conversationIdStr)) return;
      ensureAssistantNode();
      if (event.started) {
        set({ isThinking: true, streamingReasoning: event.content || '' });
        announce(i18next.t('chat.announce.modelThinking'), 'polite');
      } else if (event.done) {
        set({ isThinking: false });
        if (event.content) get().updateMessageReasoning(streamingMsgId, event.content);
      } else {
        set({ streamingReasoning: event.content || '' });
      }
    });

    unsubToolStart = EventsOn('chat:tool_start', (data: ChatToolStartEvent) => {
      if (data.conversationId !== conversationId) return;
      if (!activeListeners.has(conversationIdStr)) return;
      ensureAssistantNode();
      set((state) => {
        const existing = state.activeToolCalls.findIndex((tc) => tc.callId === data.callId);
        if (existing >= 0) {
          // Retry: upsert — reseta status para 'running' sem apagar args anteriores
          return {
            activeToolCalls: state.activeToolCalls.map((tc) =>
              tc.callId === data.callId
                ? { ...tc, name: data.name, callId: data.callId, args: data.args ?? tc.args, status: 'running' as const, summary: undefined }
                : tc
            ),
          };
        }
        return {
          activeToolCalls: [
            ...state.activeToolCalls,
            { name: data.name, callId: data.callId, args: data.args, status: 'running' as const },
          ],
        };
      });
    });

    unsubToolEnd = EventsOn('chat:tool_end', (data: ChatToolEndEvent) => {
      if (data.conversationId !== conversationId) return;
      if (!activeListeners.has(conversationIdStr)) return;
      set((state) => ({
        activeToolCalls: state.activeToolCalls.map((tc) =>
          tc.callId === data.callId
            ? { ...tc, status: (data.status === 'error' ? 'error' : 'done') as 'done' | 'error', summary: data.summary }
            : tc
        ),
      }));
      if (data.status === 'error' && (data.attempt === undefined || data.attempt > 0)) {
        announce(i18next.t('chat.toolFailed', { name: data.name }), 'assertive');
      }
    });

    // AEP-0039 Fase 3: structured failure listener
    unsubToolFailure = EventsOn('chat:tool_failure', (data: ChatToolFailureEvent) => {
      if (data.conversationId !== conversationId) return;
      if (!activeListeners.has(conversationIdStr)) return;
      if (data.willRetry) {
        announce(i18next.t('chat.toolRetrying', { name: data.name }), 'polite');
        return;
      }
      announce(i18next.t('chat.toolFailed', { name: data.name }), 'assertive');
    });

    unsubSegmentDone = EventsOn('chat:segment_done', (data: ChatSegmentDoneEvent) => {
      if (data.conversationId !== conversationId) return;
      if (!activeListeners.has(conversationIdStr)) return;
      if (data.hasMore) {
        const state = get();
        const newSegments: TurnSegment[] = [...state.completedSegments];
        if (state.activeToolCalls.length > 0) {
          const toolCount = state.activeToolCalls.length;
          newSegments.push({
            type: 'tool_calls',
            toolCalls: state.activeToolCalls.map(tc => ({
              id: tc.callId,
              type: 'function',
              function: { name: tc.name, arguments: tc.args || '' },
              result: tc.summary,
            })),
          });
          announce(toolCount === 1 ? state.activeToolCalls[0].name : `${toolCount} ferramentas`, 'polite');
        }
        if (data.content) {
          newSegments.push({ type: 'text', content: data.content });
        }
        set({ completedSegments: newSegments, activeToolCalls: [] });
        flushPendingUpdate(streamingMsgId, get().updateMessage);
        get().updateMessage(streamingMsgId, '');
      }
    });

    unsubDone = EventsOn('chat:done', (event: ChatDoneEvent) => {
      if (event.conversationId !== conversationId) return;
      if (!activeListeners.has(conversationIdStr)) return;

      set((state) => {
        if (!state.activeConversation) return state;
        return {
          activeConversation: finalizeStreamingNode(state.activeConversation, streamingMsgId),
        };
      });

      if (event.hadToolCalls && get().activeConversationId === conversationId) {
        GetMessages(conversationId, null).then((backendNodes) => {
          const messageNodes: MessageNode[] = backendNodes.map(withOriginalIndex);
          set((state) => {
            if (state.activeConversationId !== conversationId) return state;
            return {
              activeConversation: state.activeConversation
                ? { ...state.activeConversation, threadedMessages: messageNodes }
                : null,
              completedSegments: [],
            };
          });
        }).catch((err) => {
          console.error('[Chat] Erro ao recarregar mensagens:', err);
        });
      }

      cleanup();
    });

    activeListeners.set(conversationIdStr, cleanup);

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
      if (cleanupExecuted) return; // Already handled by chat:error listener
      console.error('[Chat] Error sending message:', error);
      cleanup();
      const errorMsg = getErrorMessage(error);
      ensureAssistantNode();
      get().updateMessage(
        streamingMsgId,
        i18next.t('chat.sendErrorPrefix', { message: errorMsg }),
      );
      set((state) => {
        if (!state.activeConversation) return state;
        return {
          activeConversation: finalizeStreamingNode(state.activeConversation, streamingMsgId),
        };
      });
      set({ isLoading: false, streamingMessageId: null });
    }
  };

  return {
    activeConversationId: null,
    activeConversation: null,
    isLoading: false,
    streamingMessageId: null,
    isInitialized: false,
    expandedThreads: new Set<string>(),
    editingMessageId: null,
    readingMessageId: null,
    skipFocusRestore: false,
    streamingReasoning: null,
    isThinking: false,
    expandedReasonings: new Set<string>(),
    contextProfileSlug: null,
    activeToolCalls: [],
    completedSegments: [],

    setContextProfileSlug: (slug) => set({ contextProfileSlug: slug }),

    setEditingMessageId: (id) => set({ editingMessageId: id }),

    startEditing: (id) => set({ editingMessageId: id, skipFocusRestore: true }),

    setReadingMessageId: (id) => set({ readingMessageId: id }),

    startReading: (id) => set({ readingMessageId: id, skipFocusRestore: true }),

    consumeSkipFocusRestore: () => {
      const shouldSkip = get().skipFocusRestore;
      if (shouldSkip) set({ skipFocusRestore: false });
      return shouldSkip;
    },

    createConversation: async (title) => {
      const conv = await EnsureConversation(title || 'Nova Conversa');
      set({
        activeConversationId: conv.id,
        activeConversation: {
          id: conv.id,
          title: conv.title || title || 'Nova Conversa',
          threadedMessages: [],
        },
        isInitialized: true,
      });
      return conv.id;
    },

    clearActiveConversation: () => {
      messageAudioService.stopCurrentAudio();
      ttsService.stop();
      // Invalida loads pendentes para não restaurarem uma conversa antiga após troca de aba.
      loadConversationSeq++;
      const prevId = get().activeConversationId;
      if (prevId) {
        const key = String(prevId);
        const cleanup = activeListeners.get(key);
        if (cleanup) {
          cleanup();
          activeListeners.delete(key);
        }
      }
      set({
        activeConversationId: null,
        activeConversation: null,
        isLoading: false,
        streamingMessageId: null,
        streamingReasoning: null,
        isThinking: false,
        activeToolCalls: [],
        completedSegments: [],
      });
    },

    loadConversation: async (id) => {
      // Para TTS/áudio da conversa anterior ao trocar
      messageAudioService.stopCurrentAudio();
      ttsService.stop();
      const seq = ++loadConversationSeq;

      try {
        const [conv, backendNodes] = await Promise.all([
          GetConversationInfo(id),
          GetMessages(id, null),
        ]);
        if (seq !== loadConversationSeq) return;
        const messageNodes: MessageNode[] = (backendNodes || []).map(withOriginalIndex);
        set({
          activeConversationId: id,
          activeConversation: {
            id,
            title: conv?.title || 'Conversa',
            threadedMessages: messageNodes,
            channel: conv?.channel || undefined,
            contactId: conv?.contact_id || undefined,
          },
          isInitialized: true,
        });
      } catch (error) {
        if (seq !== loadConversationSeq) return;
        console.error('[Chat] Erro ao carregar conversa:', error);
        set({
          activeConversationId: id,
          activeConversation: { id, title: 'Conversa', threadedMessages: [] },
          isInitialized: true,
        });
      }
    },

    updateMessage: (messageId, content) => {
      set((state) => {
        if (!state.activeConversation) return state;
        const updateNodeContent = (n: MessageNode): boolean => {
          if (n.message.id === messageId) {
            n.message.content = content;
            return true;
          }
          if (n.children && n.children.length > 0) {
            for (const child of n.children) {
              if (updateNodeContent(child)) return true;
            }
          }
          return false;
        };
        state.activeConversation.threadedMessages.forEach((node) => updateNodeContent(node));
        return {
          activeConversation: { ...state.activeConversation },
        };
      });
    },

    updateMessageReasoning: (messageId, reasoning) => {
      set((state) => {
        if (!state.activeConversation) return state;
        const updateNodeReasoning = (n: MessageNode): boolean => {
          if (n.message.id === messageId) {
            n.message.reasoning = reasoning;
            return true;
          }
          if (n.children && n.children.length > 0) {
            for (const child of n.children) {
              if (updateNodeReasoning(child)) return true;
            }
          }
          return false;
        };
        state.activeConversation.threadedMessages.forEach((node) => updateNodeReasoning(node));
        return {
          activeConversation: { ...state.activeConversation },
        };
      });
    },

    addInternalMessage: (message) => {
      const parentId = message.parentId?.toString();

      set((state) => {
        if (!state.activeConversation) return state;

        if (!parentId) {
          const newNode = new main.MessageNode({
            message,
            children: [],
            level: 0,
            childCount: 0,
          });
          return {
            activeConversation: {
              ...state.activeConversation,
              threadedMessages: [...state.activeConversation.threadedMessages, newNode],
            },
          };
        }

        const addToTree = (nodes: MessageNode[], targetParentId: string, level: number): { nodes: MessageNode[], found: boolean } => {
          let found = false;
          const updatedNodes = nodes.map(node => {
            if (node.message.id === targetParentId) {
              found = true;
              const existsInChildren = (node.children || []).some(child => child.message.id === message.id);
              if (existsInChildren) return node;
              const newChildNode = new main.MessageNode({
                message,
                children: [],
                level: level + 1,
                childCount: 0,
              });
              return new main.MessageNode({
                ...node,
                children: [...(node.children || []), newChildNode],
                childCount: (node.childCount || 0) + 1,
              });
            }
            if (node.children && node.children.length > 0) {
              const result = addToTree(node.children, targetParentId, level + 1);
              if (result.found) {
                found = true;
                return new main.MessageNode({ ...node, children: result.nodes });
              }
            }
            return node;
          });
          return { nodes: updatedNodes, found };
        };

        const result = addToTree(state.activeConversation.threadedMessages, parentId, 0);
        if (result.found) {
          return {
            activeConversation: { ...state.activeConversation, threadedMessages: result.nodes },
          };
        }

        // Fallback: last user message
        const findLastUserMessage = (nodes: MessageNode[]): MessageNode | null => {
          for (let i = nodes.length - 1; i >= 0; i--) {
            if (nodes[i].message.role === 'user') return nodes[i];
          }
          return null;
        };
        const lastUserMessage = findLastUserMessage(state.activeConversation.threadedMessages);
        if (lastUserMessage) {
          const fallbackResult = addToTree(state.activeConversation.threadedMessages, lastUserMessage.message.id, 0);
          if (fallbackResult.found) {
            return {
              activeConversation: { ...state.activeConversation, threadedMessages: fallbackResult.nodes },
            };
          }
        }

        const newNode = new main.MessageNode({
          message,
          children: [],
          level: message.parentId ? 1 : 0,
          childCount: 0,
        });
        return {
          activeConversation: {
            ...state.activeConversation,
            threadedMessages: [...state.activeConversation.threadedMessages, newNode],
          },
        };
      });
    },

    clearMessages: () => {
      set((state) => ({
        activeConversation: state.activeConversation
          ? { ...state.activeConversation, threadedMessages: [] }
          : null,
      }));
    },

    sendMessage: async (content, mediaFiles, paramsOverride) => {
      const conversationId = get().activeConversationId ?? 0;
      if (conversationId === 0) {
        console.error('[Chat] sendMessage sem conversa ativa (use ensureWorkspaceTabHasConversation ao abrir o painel)');
        announce(i18next.t('chat.errors.noActiveConversation'), 'assertive');
        return;
      }
      await sendMessageInternal(conversationId, content, mediaFiles, paramsOverride);
    },

    sendMessageToConversation: async (conversationId, content, mediaFiles, paramsOverride) => {
      if (!conversationId) {
        console.error('[Chat] sendMessageToConversation sem conversationId explícito');
        announce(i18next.t('chat.errors.noActiveConversation'), 'assertive');
        return;
      }
      if (get().activeConversationId !== conversationId) {
        await get().loadConversation(conversationId);
      }
      await sendMessageInternal(conversationId, content, mediaFiles, paramsOverride);
    },

    retryMessageToConversation: async (conversationId, messageId, paramsOverride) => {
      if (!conversationId || !messageId) {
        console.error('[Chat] retryMessageToConversation sem conversationId/messageId válido');
        announce(i18next.t('chat.errors.noActiveConversation'), 'assertive');
        return;
      }
      if (get().activeConversationId !== conversationId) {
        await get().loadConversation(conversationId);
      }
      await sendMessageInternal(conversationId, '', undefined, paramsOverride, messageId);
    },

    stopStreaming: () => {
      set({ isLoading: false, streamingMessageId: null, completedSegments: [] });
    },

    getActiveConversation: () => get().activeConversation,

    getMessages: () => flattenThreadedMessages(get().activeConversation?.threadedMessages),

    toggleThreadExpanded: (messageId) => {
      set((state) => {
        const expanded = new Set(state.expandedThreads);
        if (expanded.has(messageId)) expanded.delete(messageId);
        else expanded.add(messageId);
        return { expandedThreads: expanded };
      });
    },

    isThreadExpanded: (messageId) => get().expandedThreads.has(messageId),

    toggleReasoningExpanded: (messageId) => {
      set((state) => {
        const expanded = new Set(state.expandedReasonings);
        if (expanded.has(messageId)) expanded.delete(messageId);
        else expanded.add(messageId);
        return { expandedReasonings: expanded };
      });
    },

    isReasoningExpanded: (messageId) => get().expandedReasonings.has(messageId),

    getThreadedMessages: () => get().activeConversation?.threadedMessages,

    loadMessageChildren: async (messageId) => {
      try {
        const messageIdNum = parseInt(messageId, 10);
        if (isNaN(messageIdNum)) {
          console.error('[Chat] Invalid message ID:', messageId);
          return [];
        }

        const backendNodes = await GetMessageChildren(messageIdNum);
        const frontendNodes: MessageNode[] = (backendNodes || []).map(withOriginalIndex);

        set((state) => {
          if (!state.activeConversation) return state;

          const updateTreeWithChildren = (nodes: MessageNode[]): MessageNode[] => {
            return nodes.map(node => {
              if (node.message.id === messageId) {
                return new main.MessageNode({ ...node, children: frontendNodes });
              }
              if (node.children && node.children.length > 0) {
                return new main.MessageNode({ ...node, children: updateTreeWithChildren(node.children) });
              }
              return node;
            });
          };

          return {
            activeConversation: {
              ...state.activeConversation,
              threadedMessages: updateTreeWithChildren(state.activeConversation.threadedMessages),
            },
          };
        });

        return frontendNodes;
      } catch (error) {
        console.error('[Chat] Error loading children:', error);
        return [];
      }
    },

    handleConversationDeleted: (conversationId: number) => {
      if (get().activeConversationId === conversationId) {
        set({ activeConversationId: null, activeConversation: null });
        announce('Conversa apagada permanentemente');
        setTimeout(() => {
          const input = document.querySelector('textarea[placeholder*="mensagem"], textarea[aria-label*="mensagem"]') as HTMLTextAreaElement;
          if (input) input.focus();
        }, 200);
      }
    },

    handleConversationCleared: (conversationId: number) => {
      if (get().activeConversationId === conversationId) {
        announce('Mensagens da conversa removidas');
        set((state) => ({
          activeConversation: state.activeConversation
            ? { ...state.activeConversation, threadedMessages: [], title: 'Conversa limpa' }
            : null,
        }));
        setTimeout(() => {
          const input = document.querySelector('textarea[placeholder*="mensagem"], textarea[aria-label*="mensagem"]') as HTMLTextAreaElement;
          if (input) input.focus();
        }, 200);
      }
    },

    handleConversationRenamed: (conversationId: number, newTitle: string) => {
      if (get().activeConversationId === conversationId) {
        set((state) => ({
          activeConversation: state.activeConversation
            ? { ...state.activeConversation, title: newTitle }
            : null,
        }));
      }
    },

    handleDatabaseReset: () => {
      activeListeners.forEach((cleanup) => cleanup());
      activeListeners.clear();
      set({
        activeConversationId: null,
        activeConversation: null,
        isLoading: false,
        streamingMessageId: null,
        isInitialized: false,
        expandedThreads: new Set<string>(),
        expandedReasonings: new Set<string>(),
        streamingReasoning: null,
        isThinking: false,
        activeToolCalls: [],
        completedSegments: [],
      });
      announce('Banco de dados resetado. Conversas reinicializadas.');
    },

    assignChannel: async (channel, contactId) => {
      const convId = get().activeConversationId;
      if (!convId) return;
      await AssignConversationToChannel(convId, channel, contactId);
      set((state) => ({
        activeConversation: state.activeConversation
          ? { ...state.activeConversation, channel, contactId }
          : null,
      }));
    },

    unassignChannel: async () => {
      const convId = get().activeConversationId;
      if (!convId) return;
      await UnassignConversationFromChannel(convId);
      set((state) => ({
        activeConversation: state.activeConversation
          ? { ...state.activeConversation, channel: undefined, contactId: undefined }
          : null,
      }));
    },

    reloadMessages: async () => {
      const { activeConversationId } = get();
      if (!activeConversationId) return;
      try {
        const backendNodes = await GetMessages(activeConversationId, null);
        const messageNodes: MessageNode[] = backendNodes.map(withOriginalIndex);
        set((state) => ({
          activeConversation: state.activeConversation
            ? { ...state.activeConversation, threadedMessages: messageNodes }
            : null,
        }));
      } catch (err) {
        console.error('[Chat] Erro ao recarregar mensagens:', err);
      }
    },

    reloadConversationMessages: async (conversationId: number) => {
      if (get().activeConversationId !== conversationId) return;
      return get().reloadMessages();
    },

    handleExternalIncoming: (data) => {
      const { channel, from, text, conversationId } = data;

      // Only handle streaming UI for the active conversation
      if (get().activeConversationId !== conversationId) return;

      const conversationIdStr = conversationId.toString();
      const streamingMsgId = createStreamingMessageId(conversationId);
      let cleanupExecuted = false;
      let streamingAnnounced = false;
      let assistantNodeCreated = false;

      // Backend-driven: sem addMessage local
      set({ isLoading: true, completedSegments: [], activeToolCalls: [] });

      // Declarar unsubs antes de cleanup para evitar TDZ (temporal dead zone)
      const noopExt = () => { /* no-op */ };
      let unsubStream = noopExt;
      let unsubThinking = noopExt;
      let unsubToolStart = noopExt;
      let unsubToolEnd = noopExt;
      let unsubToolFailure = noopExt;
      let unsubSegmentDone = noopExt;
      let unsubDone = noopExt;
      let unsubReady = noopExt;
      let unsubError = noopExt;
      let unsubSpeak = noopExt;

      const ensureAssistantNode = () => {
        if (assistantNodeCreated) return;
        assistantNodeCreated = true;
        const assistantMsg = new main.EnrichedMessage({
          id: streamingMsgId,
          role: 'assistant',
          content: '',
          timestamp: Date.now(),
          conversationId,
          isStreaming: true,
          internal: false,
          createdAt: new Date().toISOString(),
        });
        const assistantNode = new main.MessageNode({ message: assistantMsg, children: [], level: 0, childCount: 0 });
        set((state) => ({
          activeConversation: state.activeConversation
            ? { ...state.activeConversation, threadedMessages: [...state.activeConversation.threadedMessages, assistantNode] }
            : state.activeConversation,
          streamingMessageId: streamingMsgId,
        }));
      };

      const cleanup = () => {
        if (cleanupExecuted) return;
        cleanupExecuted = true;
        unsubStream();
        unsubThinking();
        unsubToolStart();
        unsubToolEnd();
        unsubToolFailure();
        unsubSegmentDone();
        unsubDone();
        unsubReady();
        unsubError();
        unsubSpeak();
        activeListeners.delete(conversationIdStr);
        set({ isLoading: false, streamingMessageId: null, streamingReasoning: null, isThinking: false, activeToolCalls: [], completedSegments: [] });
      };

      const existingCleanup = activeListeners.get(conversationIdStr);
      if (existingCleanup) existingCleanup();

      // chat:error → erro de validação do backend
      unsubError = EventsOn('chat:error', (event: ChatErrorEvent) => {
        if (event.conversationId !== conversationId && event.conversationId !== 0) return;
        if (!activeListeners.has(conversationIdStr)) return;
        set((state) => {
          if (!state.activeConversation) return state;
          return {
            activeConversation: finalizeStreamingNode(state.activeConversation, streamingMsgId),
          };
        });
        announce(event.error);
        cleanup();
      });

      // chat:speak → TTS proativo disparado pelo backend
      unsubSpeak = EventsOn('chat:speak', (event: ChatSpeakEvent) => {
        if (event.conversationId !== conversationId) return;
        if (!activeListeners.has(conversationIdStr)) return;
        void handleChatSpeak(event).catch((err) => {
          announce(i18next.t('chat.autoReadError'));
          console.error('[chat:speak] falha ao processar evento TTS', err);
        });
      });

      // chat:messages_ready → insere mensagem do usuário com ID real do backend
      unsubReady = EventsOn('chat:messages_ready', (event: ChatMessagesReadyEvent) => {
        if (event.conversationId !== conversationId) return;
        if (!activeListeners.has(conversationIdStr)) return;
        if (!event.userMessageId) return;
        if (hasMessageId(get().activeConversation?.threadedMessages, String(event.userMessageId))) return;
        const userMsg = new main.EnrichedMessage({
          id: event.userMessageId.toString(),
          role: 'user',
          content: event.userContent || text || '',
          timestamp: Date.now(),
          conversationId: event.conversationId,
          isStreaming: false,
          internal: false,
          createdAt: new Date().toISOString(),
          source: channel,
        });
        const userNode = new main.MessageNode({ message: userMsg, children: [], level: 0, childCount: 0 });
        set((state) => ({
          activeConversation: state.activeConversation
            ? { ...state.activeConversation, threadedMessages: [...state.activeConversation.threadedMessages, userNode] }
            : state.activeConversation,
        }));
        if (event.userContent) {
          announce(`${from} via ${channel}: ${stripMarkdown(event.userContent)}`);
        }
      });

      unsubStream = EventsOn('chat:stream', (event: ChatStreamEvent) => {
        if (event.conversationId !== conversationId) return;
        if (!activeListeners.has(conversationIdStr)) return;
        if (event.content && !event.done && !event.error) {
          ensureAssistantNode();
          if (!streamingAnnounced) {
            streamingAnnounced = true;
            announce(i18next.t('chat.announce.assistantResponding'), 'polite');
          }
          debouncedUpdateMessage(streamingMsgId, event.content, get().updateMessage);
        }
        if (event.error) {
          ensureAssistantNode();
          flushPendingUpdate(streamingMsgId, get().updateMessage);
          get().updateMessage(
            streamingMsgId,
            i18next.t('chat.errorPrefix', { message: event.error }),
          );
          set((state) => {
            if (!state.activeConversation) return state;
            return {
              activeConversation: finalizeStreamingNode(state.activeConversation, streamingMsgId),
            };
          });
          cleanup();
        }
        if (event.done) {
          ensureAssistantNode();
          flushPendingUpdate(streamingMsgId, get().updateMessage);
          if (event.content) get().updateMessage(streamingMsgId, event.content);
          const backendAssistantId = event.messageId && event.messageId > 0
            ? event.messageId.toString() : null;

          set((state) => {
            if (!state.activeConversation) return state;
            return {
              activeConversation: finalizeStreamingNode(state.activeConversation, streamingMsgId, backendAssistantId),
            };
          });
          const currentState = get();
          const flatMessages = flattenThreadedMessages(currentState.activeConversation?.threadedMessages);
          const finalMessage = flatMessages.find(m => m.id === (backendAssistantId || streamingMsgId));
          if (finalMessage?.content) {
            const isActive = currentState.activeConversationId === conversationId;
            if (isActive) playReceiveSound();
          }
        }
      });

      unsubThinking = EventsOn('chat:thinking', (event: ChatThinkingEvent) => {
        if (event.conversationId !== conversationId) return;
        if (!activeListeners.has(conversationIdStr)) return;
        ensureAssistantNode();
        if (event.started) {
          set({ isThinking: true, streamingReasoning: event.content || '' });
          announce(i18next.t('chat.announce.modelThinking'), 'polite');
        } else if (event.done) {
          set({ isThinking: false });
          if (event.content) get().updateMessageReasoning(streamingMsgId, event.content);
        } else {
          set({ streamingReasoning: event.content || '' });
        }
      });

      unsubToolStart = EventsOn('chat:tool_start', (event: ChatToolStartEvent) => {
        if (event.conversationId !== conversationId) return;
        if (!activeListeners.has(conversationIdStr)) return;
        ensureAssistantNode();
        set((state) => {
          const existing = state.activeToolCalls.findIndex((tc) => tc.callId === event.callId);
          if (existing >= 0) {
            // Retry: upsert — reseta status para 'running' sem apagar args anteriores
            return {
              activeToolCalls: state.activeToolCalls.map((tc) =>
                tc.callId === event.callId
                  ? { ...tc, name: event.name, callId: event.callId, args: event.args ?? tc.args, status: 'running' as const, summary: undefined }
                  : tc
              ),
            };
          }
          return {
            activeToolCalls: [...state.activeToolCalls, {
              name: event.name, callId: event.callId, args: event.args, status: 'running' as const,
            }],
          };
        });
        announce(i18next.t('chat.toolRunning', { name: event.name }), 'polite');
      });

      unsubToolEnd = EventsOn('chat:tool_end', (event: ChatToolEndEvent) => {
        if (event.conversationId !== conversationId) return;
        if (!activeListeners.has(conversationIdStr)) return;
        set((state) => ({
          activeToolCalls: state.activeToolCalls.map((tc) =>
            tc.callId === event.callId
              ? { ...tc, status: (event.status === 'error' ? 'error' : 'done') as 'done' | 'error', summary: event.summary }
              : tc
          ),
        }));
        if (event.status === 'error' && (event.attempt === undefined || event.attempt > 0)) {
          announce(i18next.t('chat.toolFailed', { name: event.name }), 'assertive');
        } else if (event.status !== 'error') {
          announce(i18next.t('chat.toolDone', { name: event.name }), 'polite');
        }
      });

      // AEP-0039 Fase 3: structured failure listener
      unsubToolFailure = EventsOn('chat:tool_failure', (data: ChatToolFailureEvent) => {
        if (data.conversationId !== conversationId) return;
        if (!activeListeners.has(conversationIdStr)) return;
        if (data.willRetry) {
          announce(i18next.t('chat.toolRetrying', { name: data.name }), 'polite');
          return;
        }
        announce(i18next.t('chat.toolFailed', { name: data.name }), 'assertive');
      });

      unsubSegmentDone = EventsOn('chat:segment_done', (event: ChatSegmentDoneEvent) => {
        if (event.conversationId !== conversationId) return;
        if (!activeListeners.has(conversationIdStr)) return;
        if (event.hasMore) {
          const state = get();
          const newSegments: TurnSegment[] = [...state.completedSegments];
          if (state.activeToolCalls.length > 0) {
            newSegments.push({
              type: 'tool_calls',
              toolCalls: state.activeToolCalls.map(tc => ({
                id: tc.callId,
                type: 'function',
                function: { name: tc.name, arguments: tc.args || '' },
                result: tc.summary,
              })),
            });
          }
          if (event.content) {
            newSegments.push({ type: 'text', content: event.content });
          }
          set({ completedSegments: newSegments, activeToolCalls: [] });
          flushPendingUpdate(streamingMsgId, get().updateMessage);
          get().updateMessage(streamingMsgId, '');
        }
      });

      unsubDone = EventsOn('chat:done', (event: ChatDoneEvent) => {
        if (event.conversationId !== conversationId) return;
        if (!activeListeners.has(conversationIdStr)) return;

        set((state) => {
          if (!state.activeConversation) return state;
          return {
            activeConversation: finalizeStreamingNode(state.activeConversation, streamingMsgId),
          };
        });

        if (event.hadToolCalls && get().activeConversationId === conversationId) {
          GetMessages(conversationId, null).then((backendNodes) => {
            const messageNodes: MessageNode[] = backendNodes.map(withOriginalIndex);
            set((state) => {
              if (state.activeConversationId !== conversationId) return state;
              return {
                activeConversation: state.activeConversation
                  ? { ...state.activeConversation, threadedMessages: messageNodes }
                  : null,
                completedSegments: [],
              };
            });
          });
        }

        cleanup();
      });

      activeListeners.set(conversationIdStr, cleanup);
    },
  };
});
