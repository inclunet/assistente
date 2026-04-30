import { create } from 'zustand';
import {
  SendMessage,
  RetryMessage,
  GetMessages,
  GetRecentMessages,
  GetMessagesBefore,
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
import { useWorkspaceStore } from './workspaceStore';

const MAX_MESSAGE_CONTENT_SIZE = 512 * 1024;       // must match backend MaxMessageContentSize
const MAX_MEDIA_SIZE = 20 * 1024 * 1024;            // must match backend MaxMediaSize
const STREAM_UPDATE_DEBOUNCE_MS = 16;
const INITIAL_MESSAGE_WINDOW_SIZE = 120;

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
  conversationId: string;
  userMessageId: string;
  userContent: string;
}

interface ChatStreamEvent {
  conversationId: string;
  content?: string;
  done?: boolean;
  error?: string;
  messageId?: string;
}

interface ChatThinkingEvent {
  conversationId: string;
  started?: boolean;
  done?: boolean;
  content?: string;
}

interface ChatToolStartEvent {
  conversationId: string;
  name: string;
  callId: string;
  args?: string;
  serverLabel?: string;
  origin?: 'builtin' | 'mcp_bridge' | 'mcp_native';
  attempt?: number;
}

interface ChatToolEndEvent {
  conversationId: string;
  callId: string;
  name?: string;
  status?: string;
  summary?: string;
  error?: string;
  serverLabel?: string;
  origin?: 'builtin' | 'mcp_bridge' | 'mcp_native';
  durationMs?: number;
  attempt?: number;
}

// AEP-0039 Fase 3: structured failure event (distinct from tool_end with status='error')
interface ChatToolFailureEvent {
  conversationId: string;
  name: string;
  callId: string;
  errorKind: 'timeout' | 'invalid_args' | 'not_found' | 'panic' | 'cancelled' | 'unknown';
  retryable: boolean;
  message?: string;
  durationMs?: number;
  origin?: 'builtin' | 'mcp_bridge' | 'mcp_native';
  willRetry?: boolean;
  attempt?: number;
}

interface ChatSegmentDoneEvent {
  conversationId: string;
  hasMore?: boolean;
  content?: string;
  iteration?: number;
  // AEP-0039 Fase 2+3
  toolsInIteration?: Array<{ name: string; status: string; errorKind?: string; durationMs?: number; origin?: string; serverLabel?: string }>;
}

interface ChatDoneEvent {
  conversationId: string;
  assistantMessageId?: string;
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
  conversationId: string;
  error: string;
}

export interface ActiveConversation {
  id: string;
  title: string;
  threadedMessages: MessageNode[];
  channel?: string;
  contactId?: string;
}

export interface ChatConversationSession {
  conversation: ActiveConversation | null;
  isLoading: boolean;
  hasOlderMessages: boolean;
  isLoadingOlderMessages: boolean;
  streamingMessageId: string | null;
  streamingReasoning: string | null;
  isThinking: boolean;
  activeToolCalls: ToolCallStatus[];
  completedSegments: TurnSegment[];
  expandedThreads: Set<string>;
  expandedReasonings: Set<string>;
  editingMessageId: string | null;
  readingMessageId: string | null;
  skipFocusRestore: boolean;
}

const createEmptyChatSession = (): ChatConversationSession => ({
  conversation: null,
  isLoading: false,
  hasOlderMessages: false,
  isLoadingOlderMessages: false,
  streamingMessageId: null,
  streamingReasoning: null,
  isThinking: false,
  activeToolCalls: [],
  completedSegments: [],
  expandedThreads: new Set<string>(),
  expandedReasonings: new Set<string>(),
  editingMessageId: null,
  readingMessageId: null,
  skipFocusRestore: false,
});

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

const createStreamingMessageId = (conversationId: string): string => {
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
    state.sessionsByConversationId[conversationId] ?? createEmptyChatSession()
  );

  const patchSession = (
    state: ChatStore,
    conversationId: string,
    patch: Partial<ChatConversationSession> | ((session: ChatConversationSession) => ChatConversationSession),
  ): Partial<ChatStore> => {
    const currentSession = getSession(state, conversationId);
    const nextSession = typeof patch === 'function'
      ? patch(currentSession)
      : { ...currentSession, ...patch };
    const sessionsByConversationId = {
      ...state.sessionsByConversationId,
      [conversationId]: nextSession,
    };
    return {
      sessionsByConversationId,
    };
  };

  const patchConversation = (
    state: ChatStore,
    conversationId: string,
    updater: (conversation: ActiveConversation) => ActiveConversation,
  ): Partial<ChatStore> => {
    const session = getSession(state, conversationId);
    if (!session.conversation) return state;
    return patchSession(state, conversationId, {
      conversation: updater(session.conversation),
    });
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

  const getConversationAnnouncementLabel = (conversationId: string): string => {
    const workspace = useWorkspaceStore.getState().workspace;
    const tab = workspace?.tabs.find((candidate) => candidate.conversationId === conversationId);
    const title = tab?.title || get().sessionsByConversationId[conversationId]?.conversation?.title || '';
    return String(title || i18next.t('chat.conversation', { defaultValue: 'Conversa' })).trim();
  };

  const isWorkspaceConversationActive = (conversationId: string): boolean => (
    useWorkspaceStore.getState().getActiveTab?.()?.conversationId === conversationId
  );

  const announceForActiveConversation = (
    conversationId: string,
    message: string,
    priority: 'polite' | 'assertive' = 'polite',
  ) => {
    if (isWorkspaceConversationActive(conversationId)) {
      announce(message, priority);
    }
  };

  const announceBackgroundResponseDone = (conversationId: string) => {
    if (isWorkspaceConversationActive(conversationId)) return;
    announce(
      i18next.t('chat.announce.backgroundResponseDone', {
        title: getConversationAnnouncementLabel(conversationId),
      }),
      'polite',
    );
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

    const conversationIdStr = conversationId.toString();

    // Backend-driven: sem addMessage local — user msg vem do chat:messages_ready, assistant do chat:stream
    setConversationLoading(conversationId, true);
    set((state) => patchSession(state, conversationId, { completedSegments: [], activeToolCalls: [] }));
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
      set((state) => {
        const session = getSession(state, conversationId);
        if (!session.conversation) return state;
        return patchSession(state, conversationId, {
          conversation: {
            ...session.conversation,
            threadedMessages: [...session.conversation.threadedMessages, assistantNode],
          },
          streamingMessageId: streamingMsgId,
        });
      });
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
      setConversationLoading(conversationId, false);
      set((state) => patchSession(state, conversationId, {
        streamingMessageId: null,
        streamingReasoning: null,
        isThinking: false,
        activeToolCalls: [],
        completedSegments: [],
      }));
    };

    const existingCleanup = activeListeners.get(conversationIdStr);
    if (existingCleanup) {
      existingCleanup();
      await new Promise(resolve => setTimeout(resolve, 0));
    }

    // chat:error → erro de validação do backend (tamanho, provider, etc.)
    unsubError = EventsOn('chat:error', (event: ChatErrorEvent) => {
      if (event.conversationId !== conversationId && event.conversationId !== "") return;
      if (!activeListeners.has(conversationIdStr)) return;
      set((state) => patchConversation(
        state,
        conversationId,
        (conversation) => finalizeStreamingNode(conversation, streamingMsgId),
      ));
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
      if (hasMessageId(get().sessionsByConversationId[conversationId]?.conversation?.threadedMessages, String(data.userMessageId))) return;
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
      set((state) => {
        const session = getSession(state, conversationId);
        if (!session.conversation) return state;
        return patchSession(state, conversationId, {
          conversation: {
            ...session.conversation,
            threadedMessages: [...session.conversation.threadedMessages, userNode],
          },
        });
      });
    });

    // chat:stream → cria placeholder de assistant na primeira chunk, atualiza conteúdo
    unsubStream = EventsOn('chat:stream', (event: ChatStreamEvent) => {
      if (event.conversationId !== conversationId) return;
      if (!activeListeners.has(conversationIdStr)) return;

      if (event.content && !event.done && !event.error) {
        ensureAssistantNode();
        if (!streamingAnnounced) {
          streamingAnnounced = true;
          announceForActiveConversation(conversationId, i18next.t('chat.announce.assistantResponding'), 'polite');
        }
        debouncedUpdateMessage(
          streamingMsgId,
          event.content,
          (messageId, nextContent) => get().updateConversationMessage(conversationId, messageId, nextContent),
        );
      }

      if (event.error) {
        ensureAssistantNode();
        flushPendingUpdate(
          streamingMsgId,
          (messageId, nextContent) => get().updateConversationMessage(conversationId, messageId, nextContent),
        );
        get().updateConversationMessage(
          conversationId,
          streamingMsgId,
          i18next.t('chat.errorPrefix', { message: event.error }),
        );
        set((state) => patchConversation(
          state,
          conversationId,
          (conversation) => finalizeStreamingNode(conversation, streamingMsgId),
        ));
        cleanup();
      }

      if (event.done) {
        ensureAssistantNode();
        flushPendingUpdate(
          streamingMsgId,
          (messageId, nextContent) => get().updateConversationMessage(conversationId, messageId, nextContent),
        );
        if (event.content) get().updateConversationMessage(conversationId, streamingMsgId, event.content);

        const backendAssistantId = event.messageId && event.messageId !== ''
          ? event.messageId : null;

        set((state) => patchConversation(
          state,
          conversationId,
          (conversation) => finalizeStreamingNode(conversation, streamingMsgId, backendAssistantId),
        ));

        const currentState = get();
        const flatMessages = flattenThreadedMessages(
          currentState.sessionsByConversationId[conversationId]?.conversation?.threadedMessages,
        );
        const finalMessage = flatMessages.find(m => m.id === (backendAssistantId || streamingMsgId));
        if (finalMessage?.content) {
          if (isWorkspaceConversationActive(conversationId)) playReceiveSound();
        }
      }
    });

    unsubThinking = EventsOn('chat:thinking', (event: ChatThinkingEvent) => {
      if (event.conversationId !== conversationId) return;
      if (!activeListeners.has(conversationIdStr)) return;
      ensureAssistantNode();
      if (event.started) {
        set((state) => patchSession(state, conversationId, {
          isThinking: true,
          streamingReasoning: event.content || '',
        }));
        announceForActiveConversation(conversationId, i18next.t('chat.announce.modelThinking'), 'polite');
      } else if (event.done) {
        set((state) => patchSession(state, conversationId, { isThinking: false }));
        if (event.content) get().updateConversationMessageReasoning(conversationId, streamingMsgId, event.content);
      } else {
        set((state) => patchSession(state, conversationId, { streamingReasoning: event.content || '' }));
      }
    });

    unsubToolStart = EventsOn('chat:tool_start', (data: ChatToolStartEvent) => {
      if (data.conversationId !== conversationId) return;
      if (!activeListeners.has(conversationIdStr)) return;
      ensureAssistantNode();
      set((state) => {
        const session = getSession(state, conversationId);
        const existing = session.activeToolCalls.findIndex((tc) => tc.callId === data.callId);
        if (existing >= 0) {
          // Retry: upsert — reseta status para 'running' sem apagar args anteriores
          return patchSession(state, conversationId, {
            activeToolCalls: session.activeToolCalls.map((tc) =>
              tc.callId === data.callId
                ? { ...tc, name: data.name, callId: data.callId, args: data.args ?? tc.args, status: 'running' as const, summary: undefined }
                : tc
            ),
          });
        }
        return patchSession(state, conversationId, {
          activeToolCalls: [
            ...session.activeToolCalls,
            { name: data.name, callId: data.callId, args: data.args, status: 'running' as const },
          ],
        });
      });
    });

    unsubToolEnd = EventsOn('chat:tool_end', (data: ChatToolEndEvent) => {
      if (data.conversationId !== conversationId) return;
      if (!activeListeners.has(conversationIdStr)) return;
      set((state) => {
        const session = getSession(state, conversationId);
        return patchSession(state, conversationId, {
          activeToolCalls: session.activeToolCalls.map((tc) =>
            tc.callId === data.callId
              ? { ...tc, status: (data.status === 'error' ? 'error' : 'done') as 'done' | 'error', summary: data.summary }
              : tc
          ),
        });
      });
      // tool_end apenas atualiza estado visual; anúncio de falha é centralizado em tool_failure.
    });

    // AEP-0039 Fase 3: structured failure listener
    // Centraliza anúncio de falha: tool_end com attempt=0 não anuncia (tool_failure cuida).
    unsubToolFailure = EventsOn('chat:tool_failure', (data: ChatToolFailureEvent) => {
      if (data.conversationId !== conversationId) return;
      if (!activeListeners.has(conversationIdStr)) return;
      if (data.willRetry) {
        announceForActiveConversation(conversationId, i18next.t('chat.toolRetrying', { name: data.name }), 'polite');
        return;
      }
      announce(i18next.t('chat.toolFailed', { name: data.name }), 'assertive');
    });

    unsubSegmentDone = EventsOn('chat:segment_done', (data: ChatSegmentDoneEvent) => {
      if (data.conversationId !== conversationId) return;
      if (!activeListeners.has(conversationIdStr)) return;
      if (data.hasMore) {
        const state = get();
        const session = getSession(state, conversationId);
        const newSegments: TurnSegment[] = [...session.completedSegments];
        if (session.activeToolCalls.length > 0) {
          const toolCount = session.activeToolCalls.length;
          newSegments.push({
            type: 'tool_calls',
            toolCalls: session.activeToolCalls.map(tc => ({
              id: tc.callId,
              type: 'function',
              function: { name: tc.name, arguments: tc.args || '' },
              result: tc.summary,
            })),
          });
          announceForActiveConversation(
            conversationId,
            toolCount === 1 ? session.activeToolCalls[0].name : `${toolCount} ferramentas`,
            'polite',
          );
        }
        if (data.content) {
          newSegments.push({ type: 'text', content: data.content });
        }
        set((current) => patchSession(current, conversationId, {
          completedSegments: newSegments,
          activeToolCalls: [],
        }));
        flushPendingUpdate(
          streamingMsgId,
          (messageId, nextContent) => get().updateConversationMessage(conversationId, messageId, nextContent),
        );
        get().updateConversationMessage(conversationId, streamingMsgId, '');
      }
    });

    unsubDone = EventsOn('chat:done', (event: ChatDoneEvent) => {
      if (event.conversationId !== conversationId) return;
      if (!activeListeners.has(conversationIdStr)) return;

      // chat:done com errorMessage: exibe erro na UI (substitui chat:stream terminal)
      if (event.errorMessage) {
        ensureAssistantNode();
        flushPendingUpdate(
          streamingMsgId,
          (messageId, nextContent) => get().updateConversationMessage(conversationId, messageId, nextContent),
        );
        get().updateConversationMessage(
          conversationId,
          streamingMsgId,
          i18next.t('chat.errorPrefix', { message: event.errorMessage }),
        );
        set((state) => patchConversation(
          state,
          conversationId,
          (conversation) => finalizeStreamingNode(conversation, streamingMsgId),
        ));
        cleanup();
        return;
      }

      set((state) => patchConversation(
        state,
        conversationId,
        (conversation) => finalizeStreamingNode(conversation, streamingMsgId),
      ));

      if (event.hadToolCalls) {
        GetMessages(conversationId, null).then((backendNodes) => {
          const messageNodes: MessageNode[] = backendNodes.map(withOriginalIndex);
          set((state) => {
            const session = getSession(state, conversationId);
            if (!session.conversation) return state;
            return patchSession(state, conversationId, {
              conversation: {
                ...session.conversation,
                threadedMessages: messageNodes,
              },
              completedSegments: [],
            });
          });
        }).catch((err) => {
          console.error('[Chat] Erro ao recarregar mensagens:', err);
        });
      }

      announceBackgroundResponseDone(conversationId);
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
      get().updateConversationMessage(
        conversationId,
        streamingMsgId,
        i18next.t('chat.sendErrorPrefix', { message: errorMsg }),
      );
      set((state) => patchConversation(
        state,
        conversationId,
        (conversation) => finalizeStreamingNode(conversation, streamingMsgId),
      ));
      setConversationLoading(conversationId, false);
      set((state) => patchSession(state, conversationId, { streamingMessageId: null }));
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
      const conv = await EnsureConversation(title || 'Nova Conversa');
      set((state) => {
        const conversation = {
          id: conv.id,
          title: conv.title || title || 'Nova Conversa',
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
        const requestedLimit = INITIAL_MESSAGE_WINDOW_SIZE + 1;
        const [conv, backendNodes] = await Promise.all([
          GetConversationInfo(id),
          GetRecentMessages(id, requestedLimit),
        ]);
        if (seq !== loadConversationSeq) return;
        const fetchedNodes = backendNodes || [];
        const hasOlderMessages = fetchedNodes.length > INITIAL_MESSAGE_WINDOW_SIZE;
        const visibleNodes = hasOlderMessages ? fetchedNodes.slice(1) : fetchedNodes;
        const messageNodes: MessageNode[] = visibleNodes.map(withOriginalIndex);
        set((state) => {
          if (options.activate !== false && seq !== loadConversationSeq) return state;
          const conversation = {
            id,
            title: conv?.title || 'Conversa',
            threadedMessages: messageNodes,
            channel: conv?.channel || undefined,
            contactId: conv?.contact_id || undefined,
          };
          const sessionsByConversationId = {
            ...state.sessionsByConversationId,
            [id]: {
              ...getSession(state, id),
              conversation,
              isLoading: state.loadingConversationIds.has(id),
              hasOlderMessages,
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
              conversation: { id, title: 'Conversa', threadedMessages: [] },
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
        const requestedLimit = INITIAL_MESSAGE_WINDOW_SIZE + 1;
        const backendNodes = await GetMessagesBefore(conversation.id, firstMessageId, requestedLimit);
        const fetchedNodes = backendNodes || [];
        const hasOlderMessages = fetchedNodes.length > INITIAL_MESSAGE_WINDOW_SIZE;
        const visibleNodes = hasOlderMessages ? fetchedNodes.slice(1) : fetchedNodes;
        const olderNodes: MessageNode[] = visibleNodes.map(withOriginalIndex);

        set((current) => {
          const currentSession = getSession(current, conversation.id);
          if (!currentSession.conversation) {
            return patchSession(current, conversationId, { isLoadingOlderMessages: false });
          }
          const existingIds = new Set(currentSession.conversation.threadedMessages.map((node) => node.message.id));
          const dedupedOlderNodes = olderNodes.filter((node) => !existingIds.has(node.message.id));
          return patchSession(current, conversation.id, {
            conversation: {
              ...currentSession.conversation,
              threadedMessages: [...dedupedOlderNodes, ...currentSession.conversation.threadedMessages],
            },
            hasOlderMessages,
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
        session.conversation.threadedMessages.forEach((node) => updateNodeContent(node));
        return patchSession(state, conversationId, {
          conversation: { ...session.conversation },
        });
      });
    },

    updateConversationMessageReasoning: (conversationId, messageId, reasoning) => {
      set((state) => {
        const session = getSession(state, conversationId);
        if (!session.conversation) return state;
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
        session.conversation.threadedMessages.forEach((node) => updateNodeReasoning(node));
        return patchSession(state, conversationId, {
          conversation: { ...session.conversation },
        });
      });
    },

    addInternalMessage: (message) => {
      const parentId = message.parentId?.toString();
      const conversationId = String(message.conversationId || '');
      if (!conversationId) return;

      set((state) => {
        const session = getSession(state, conversationId);
        if (!session.conversation) return state;

        if (!parentId) {
          const newNode = new main.MessageNode({
            message,
            children: [],
            level: 0,
            childCount: 0,
          });
          return patchSession(state, conversationId, {
            conversation: {
              ...session.conversation,
              threadedMessages: [...session.conversation.threadedMessages, newNode],
            },
          });
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

        const result = addToTree(session.conversation.threadedMessages, parentId, 0);
        if (result.found) {
          return patchSession(state, conversationId, {
            conversation: { ...session.conversation, threadedMessages: result.nodes },
          });
        }

        // Fallback: last user message
        const findLastUserMessage = (nodes: MessageNode[]): MessageNode | null => {
          for (let i = nodes.length - 1; i >= 0; i--) {
            if (nodes[i].message.role === 'user') return nodes[i];
          }
          return null;
        };
        const lastUserMessage = findLastUserMessage(session.conversation.threadedMessages);
        if (lastUserMessage) {
          const fallbackResult = addToTree(session.conversation.threadedMessages, lastUserMessage.message.id, 0);
          if (fallbackResult.found) {
            return patchSession(state, conversationId, {
              conversation: { ...session.conversation, threadedMessages: fallbackResult.nodes },
            });
          }
        }

        const newNode = new main.MessageNode({
          message,
          children: [],
          level: message.parentId ? 1 : 0,
          childCount: 0,
        });
        return patchSession(state, conversationId, {
          conversation: {
            ...session.conversation,
            threadedMessages: [...session.conversation.threadedMessages, newNode],
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

        const backendNodes = await GetMessageChildren(messageId);
        const frontendNodes: MessageNode[] = (backendNodes || []).map(withOriginalIndex);

        set((state) => {
          const targetConversationId = Object.entries(state.sessionsByConversationId).find(([, session]) => (
            hasMessageId(session.conversation?.threadedMessages, messageId)
          ))?.[0];
          if (!targetConversationId) return state;
          const session = getSession(state, targetConversationId);
          if (!session.conversation) return state;

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

          return patchSession(state, targetConversationId, {
            conversation: {
              ...session.conversation,
              threadedMessages: updateTreeWithChildren(session.conversation.threadedMessages),
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
        set((state) => {
          const sessionsByConversationId = { ...state.sessionsByConversationId };
          delete sessionsByConversationId[conversationId];
          return {
            sessionsByConversationId,
          };
        });
      }
      if (isWorkspaceConversationActive(conversationId)) {
        announce('Conversa apagada permanentemente');
        setTimeout(() => {
          const input = document.querySelector('textarea[placeholder*="mensagem"], textarea[aria-label*="mensagem"]') as HTMLTextAreaElement;
          if (input) input.focus();
        }, 200);
      }
    },

    handleConversationCleared: (conversationId: string) => {
      if (get().sessionsByConversationId[conversationId]) {
        announce('Mensagens da conversa removidas');
        set((state) => ({
          ...patchConversation(state, conversationId, (conversation) => ({
            ...conversation,
            threadedMessages: [],
            title: 'Conversa limpa',
          })),
        }));
      }
      if (isWorkspaceConversationActive(conversationId)) {
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
      activeListeners.forEach((cleanup) => cleanup());
      activeListeners.clear();
      set({
        sessionsByConversationId: {},
        loadingConversationIds: new Set(),
        isInitialized: false,
      });
      announce('Banco de dados resetado. Conversas reinicializadas.');
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
        const requestedLimit = INITIAL_MESSAGE_WINDOW_SIZE + 1;
        const backendNodes = await GetRecentMessages(conversationId, requestedLimit);
        const fetchedNodes = backendNodes || [];
        const hasOlderMessages = fetchedNodes.length > INITIAL_MESSAGE_WINDOW_SIZE;
        const visibleNodes = hasOlderMessages ? fetchedNodes.slice(1) : fetchedNodes;
        const messageNodes: MessageNode[] = visibleNodes.map(withOriginalIndex);
        set((state) => {
          const session = getSession(state, conversationId);
          if (!session.conversation) return state;
          return patchSession(state, conversationId, {
            conversation: {
              ...session.conversation,
              threadedMessages: messageNodes,
            },
            hasOlderMessages,
          });
        });
      } catch (err) {
        console.error('[Chat] Erro ao recarregar mensagens:', err);
      }
    },

    handleExternalIncoming: (data) => {
      const { channel, from, text, conversationId } = data;
      if (!conversationId) return;
      if (!get().sessionsByConversationId[conversationId]?.conversation) {
        void get().loadConversationSession(conversationId, { activate: false });
      }

      const conversationIdStr = conversationId.toString();
      const streamingMsgId = createStreamingMessageId(conversationId);
      let cleanupExecuted = false;
      let streamingAnnounced = false;
      let assistantNodeCreated = false;

      // Backend-driven: sem addMessage local
      set((state) => patchSession(state, conversationId, {
        isLoading: true,
        completedSegments: [],
        activeToolCalls: [],
      }));

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
        set((state) => {
          const session = getSession(state, conversationId);
          if (!session.conversation) return state;
          return patchSession(state, conversationId, {
            conversation: {
              ...session.conversation,
              threadedMessages: [...session.conversation.threadedMessages, assistantNode],
            },
            streamingMessageId: streamingMsgId,
          });
        });
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
        set((state) => patchSession(state, conversationId, {
          isLoading: false,
          streamingMessageId: null,
          streamingReasoning: null,
          isThinking: false,
          activeToolCalls: [],
          completedSegments: [],
        }));
      };

      const existingCleanup = activeListeners.get(conversationIdStr);
      if (existingCleanup) existingCleanup();

      // chat:error → erro de validação do backend
      unsubError = EventsOn('chat:error', (event: ChatErrorEvent) => {
        if (event.conversationId !== conversationId && event.conversationId !== "") return;
        if (!activeListeners.has(conversationIdStr)) return;
        set((state) => patchConversation(
          state,
          conversationId,
          (conversation) => finalizeStreamingNode(conversation, streamingMsgId),
        ));
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
        if (hasMessageId(get().sessionsByConversationId[conversationId]?.conversation?.threadedMessages, String(event.userMessageId))) return;
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
        set((state) => {
          const session = getSession(state, conversationId);
          if (!session.conversation) return state;
          return patchSession(state, conversationId, {
            conversation: {
              ...session.conversation,
              threadedMessages: [...session.conversation.threadedMessages, userNode],
            },
          });
        });
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
            announceForActiveConversation(conversationId, i18next.t('chat.announce.assistantResponding'), 'polite');
          }
          debouncedUpdateMessage(
            streamingMsgId,
            event.content,
            (messageId, nextContent) => get().updateConversationMessage(conversationId, messageId, nextContent),
          );
        }
        if (event.error) {
          ensureAssistantNode();
          flushPendingUpdate(
            streamingMsgId,
            (messageId, nextContent) => get().updateConversationMessage(conversationId, messageId, nextContent),
          );
          get().updateConversationMessage(
            conversationId,
            streamingMsgId,
            i18next.t('chat.errorPrefix', { message: event.error }),
          );
          set((state) => patchConversation(
            state,
            conversationId,
            (conversation) => finalizeStreamingNode(conversation, streamingMsgId),
          ));
          cleanup();
        }
        if (event.done) {
          ensureAssistantNode();
          flushPendingUpdate(
            streamingMsgId,
            (messageId, nextContent) => get().updateConversationMessage(conversationId, messageId, nextContent),
          );
          if (event.content) get().updateConversationMessage(conversationId, streamingMsgId, event.content);
          const backendAssistantId = event.messageId && event.messageId !== ''
            ? event.messageId : null;

          set((state) => patchConversation(
            state,
            conversationId,
            (conversation) => finalizeStreamingNode(conversation, streamingMsgId, backendAssistantId),
          ));
          const currentState = get();
          const flatMessages = flattenThreadedMessages(
            currentState.sessionsByConversationId[conversationId]?.conversation?.threadedMessages,
          );
          const finalMessage = flatMessages.find(m => m.id === (backendAssistantId || streamingMsgId));
          if (finalMessage?.content) {
            if (isWorkspaceConversationActive(conversationId)) playReceiveSound();
          }
        }
      });

      unsubThinking = EventsOn('chat:thinking', (event: ChatThinkingEvent) => {
        if (event.conversationId !== conversationId) return;
        if (!activeListeners.has(conversationIdStr)) return;
        ensureAssistantNode();
        if (event.started) {
          set((state) => patchSession(state, conversationId, {
            isThinking: true,
            streamingReasoning: event.content || '',
          }));
          announceForActiveConversation(conversationId, i18next.t('chat.announce.modelThinking'), 'polite');
        } else if (event.done) {
          set((state) => patchSession(state, conversationId, { isThinking: false }));
          if (event.content) get().updateConversationMessageReasoning(conversationId, streamingMsgId, event.content);
        } else {
          set((state) => patchSession(state, conversationId, { streamingReasoning: event.content || '' }));
        }
      });

      unsubToolStart = EventsOn('chat:tool_start', (event: ChatToolStartEvent) => {
        if (event.conversationId !== conversationId) return;
        if (!activeListeners.has(conversationIdStr)) return;
        ensureAssistantNode();
        set((state) => {
          const session = getSession(state, conversationId);
          const existing = session.activeToolCalls.findIndex((tc) => tc.callId === event.callId);
          if (existing >= 0) {
            // Retry: upsert — reseta status para 'running' sem apagar args anteriores
            return patchSession(state, conversationId, {
              activeToolCalls: session.activeToolCalls.map((tc) =>
                tc.callId === event.callId
                  ? { ...tc, name: event.name, callId: event.callId, args: event.args ?? tc.args, status: 'running' as const, summary: undefined }
                  : tc
              ),
            });
          }
          return patchSession(state, conversationId, {
            activeToolCalls: [...session.activeToolCalls, {
              name: event.name, callId: event.callId, args: event.args, status: 'running' as const,
            }],
          });
        });
        announceForActiveConversation(conversationId, i18next.t('chat.toolRunning', { name: event.name }), 'polite');
      });

      unsubToolEnd = EventsOn('chat:tool_end', (event: ChatToolEndEvent) => {
        if (event.conversationId !== conversationId) return;
        if (!activeListeners.has(conversationIdStr)) return;
        set((state) => {
          const session = getSession(state, conversationId);
          return patchSession(state, conversationId, {
            activeToolCalls: session.activeToolCalls.map((tc) =>
              tc.callId === event.callId
                ? { ...tc, status: (event.status === 'error' ? 'error' : 'done') as 'done' | 'error', summary: event.summary }
                : tc
            ),
          });
        });
        // tool_end apenas atualiza estado visual; anúncio de falha centralizado em tool_failure.
        if (event.status !== 'error') {
          announceForActiveConversation(conversationId, i18next.t('chat.toolDone', { name: event.name }), 'polite');
          return;
        }
        // Fallback retrocompatibilidade: payloads não-enriquecidos (sem attempt)
        // podem não emitir tool_failure, então anunciamos a falha aqui.
        const hasStructuredFailureMetadata = 'attempt' in event;
        if (!hasStructuredFailureMetadata) {
          announce(i18next.t('chat.toolFailed', { name: event.name }), 'assertive');
        }
      });

      // AEP-0039 Fase 3: structured failure listener
      // Centraliza anúncio de falha: tool_end com attempt=0 não anuncia (tool_failure cuida).
      unsubToolFailure = EventsOn('chat:tool_failure', (data: ChatToolFailureEvent) => {
        if (data.conversationId !== conversationId) return;
        if (!activeListeners.has(conversationIdStr)) return;
        if (data.willRetry) {
          announceForActiveConversation(conversationId, i18next.t('chat.toolRetrying', { name: data.name }), 'polite');
          return;
        }
        announce(i18next.t('chat.toolFailed', { name: data.name }), 'assertive');
      });

      unsubSegmentDone = EventsOn('chat:segment_done', (event: ChatSegmentDoneEvent) => {
        if (event.conversationId !== conversationId) return;
        if (!activeListeners.has(conversationIdStr)) return;
        if (event.hasMore) {
          const state = get();
          const session = getSession(state, conversationId);
          const newSegments: TurnSegment[] = [...session.completedSegments];
          if (session.activeToolCalls.length > 0) {
            newSegments.push({
              type: 'tool_calls',
              toolCalls: session.activeToolCalls.map(tc => ({
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
          set((current) => patchSession(current, conversationId, {
            completedSegments: newSegments,
            activeToolCalls: [],
          }));
          flushPendingUpdate(
            streamingMsgId,
            (messageId, nextContent) => get().updateConversationMessage(conversationId, messageId, nextContent),
          );
          get().updateConversationMessage(conversationId, streamingMsgId, '');
        }
      });

      unsubDone = EventsOn('chat:done', (event: ChatDoneEvent) => {
        if (event.conversationId !== conversationId) return;
        if (!activeListeners.has(conversationIdStr)) return;

        // chat:done com errorMessage: exibe erro na UI (substitui chat:stream terminal)
        if (event.errorMessage) {
          ensureAssistantNode();
          flushPendingUpdate(
            streamingMsgId,
            (messageId, nextContent) => get().updateConversationMessage(conversationId, messageId, nextContent),
          );
          get().updateConversationMessage(
            conversationId,
            streamingMsgId,
            i18next.t('chat.errorPrefix', { message: event.errorMessage }),
          );
          set((state) => patchConversation(
            state,
            conversationId,
            (conversation) => finalizeStreamingNode(conversation, streamingMsgId),
          ));
          cleanup();
          return;
        }

        set((state) => patchConversation(
          state,
          conversationId,
          (conversation) => finalizeStreamingNode(conversation, streamingMsgId),
        ));

        if (event.hadToolCalls) {
          GetMessages(conversationId, null).then((backendNodes) => {
            const messageNodes: MessageNode[] = backendNodes.map(withOriginalIndex);
            set((state) => {
              const session = getSession(state, conversationId);
              if (!session.conversation) return state;
              return patchSession(state, conversationId, {
                conversation: {
                  ...session.conversation,
                  threadedMessages: messageNodes,
                },
                completedSegments: [],
              });
            });
          });
        }

        announceBackgroundResponseDone(conversationId);
        cleanup();
      });

      activeListeners.set(conversationIdStr, cleanup);
    },
  };
});
