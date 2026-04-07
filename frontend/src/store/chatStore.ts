import { create } from 'zustand';
import {
  SendMessage,
  GetMessages,
  GetConversationInfo,
  EnsureConversation,
  AssignConversationToChannel,
  UnassignConversationFromChannel,
  GetMessageChildren,
  RenameConversation,
} from '@wailsjs/go/main/App';
import { EventsOn } from '@wailsjs/runtime/runtime';
import { MediaFile } from '../services/mediaService';
import { llm, main } from '../../wailsjs/go/models';
import { announce } from '../hooks/useAnnouncer';
import { playSendSound, playReceiveSound } from '../services/audioFeedback';
import { ttsService } from '../services/tts';
import { messageAudioService } from '../services/messageAudio';
import { stripMarkdown } from '../lib/stripMarkdown';
import type { ToolCallStatus } from '../components/chat/ToolCallsSection';

import type { VoiceRole } from '../services/tts';

/**
 * Ponto único de disparo do auto-read TTS.
 * Se messageId for informado, checa o DB por áudio em cache antes de gerar novo TTS.
 * Para áudio em andamento, limpa markdown e fala via speakAsRole (ou cache).
 */
function triggerAutoRead(text: string, role: VoiceRole, messageId?: number): void {
  messageAudioService.stopAll();
  ttsService.stop();
  const clean = stripMarkdown(text);
  const volume = ttsService.getVolume();

  if (messageId && messageId > 0) {
    // Tenta cache do DB → gera+salva → fallback speakAsRole
    messageAudioService.getAudioFromDB(messageId).then((cached) => {
      if (cached) {
        return messageAudioService.playAudioBase64(cached.audio, cached.mimeType, volume);
      }
      return messageAudioService.generateAndSaveAudio(messageId, clean).then((generated) => {
        if (generated) {
          return messageAudioService.playAudioBase64(generated.audio, generated.mimeType, volume);
        }
        return ttsService.speakAsRole(clean, role);
      });
    }).catch((err: unknown) => {
      console.error(`[Chat] TTS auto-read error (${role}):`, err);
    });
  } else {
    // Sem messageId (streaming parcial, etc.) → TTS direto
    ttsService.speakAsRole(clean, role).catch((err: unknown) => {
      console.error(`[Chat] TTS auto-read error (${role}):`, err);
    });
  }
}

const MAX_MESSAGE_CONTENT_SIZE = 500 * 1024;
const MAX_MEDIA_SIZE = 10 * 1024 * 1024;
const STREAM_UPDATE_DEBOUNCE_MS = 16;
const DEFAULT_TITLE_PATTERNS = /^nova\s+conversa$/i;

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
  userMessageId?: number | string;
  userContent?: string;
  conversationId?: number;
}

interface ChatStreamEvent {
  content?: string;
  done?: boolean;
  error?: string;
  messageId?: number;
}

interface ChatThinkingEvent {
  started?: boolean;
  done?: boolean;
  content?: string;
}

interface ChatToolStartEvent {
  name: string;
  callId: string;
  args?: string;
}

interface ChatToolEndEvent {
  callId: string;
  name?: string;
  status?: string;
  summary?: string;
}

interface ChatSegmentDoneEvent {
  hasMore?: boolean;
  content?: string;
}

export interface NewMessageData {
  role: string;
  content: string;
  isStreaming?: boolean;
  parentId?: string;
  source?: string;
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

const generateId = () => `${Date.now()}-${Math.random().toString(36).substr(2, 9)}`;

const activeListeners = new Map<string, () => void>();

const streamUpdateTimers = new Map<string, NodeJS.Timeout>();
const pendingStreamUpdates = new Map<string, { messageId: string; content: string }>();

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
  hadToolCalls: boolean;
  completedSegments: TurnSegment[];

  setContextProfileSlug: (slug: string | null) => void;
  setEditingMessageId: (id: string | null) => void;
  startEditing: (id: string) => void;
  consumeSkipFocusRestore: () => boolean;
  setReadingMessageId: (id: string | null) => void;
  startReading: (id: string) => void;

  createConversation: (title?: string) => Promise<number>;
  loadConversation: (id: number) => Promise<void>;

  addMessage: (message: NewMessageData) => string;
  updateMessage: (messageId: string, content: string) => void;
  updateMessageReasoning: (messageId: string, reasoning: string) => void;
  addInternalMessage: (message: Message) => void;
  clearMessages: () => void;

  toggleThreadExpanded: (messageId: string) => void;
  isThreadExpanded: (messageId: string) => boolean;
  toggleReasoningExpanded: (messageId: string) => void;
  isReasoningExpanded: (messageId: string) => boolean;

  sendMessage: (content: string, mediaFiles?: MediaFile[]) => Promise<void>;
  sendMessageWithParams: (content: string, mediaFiles?: MediaFile[], paramsOverride?: Partial<llm.ChatParams>) => Promise<void>;
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
    hadToolCalls: false,
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

    loadConversation: async (id) => {
      // Para TTS/áudio da conversa anterior ao trocar
      messageAudioService.stopAll();
      ttsService.stop();

      try {
        const [conv, backendNodes] = await Promise.all([
          GetConversationInfo(id),
          GetMessages(id, null),
        ]);
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
        console.error('[Chat] Erro ao carregar conversa:', error);
        set({
          activeConversationId: id,
          activeConversation: { id, title: 'Conversa', threadedMessages: [] },
          isInitialized: true,
        });
      }
    },

    addMessage: (message) => {
      const messageId = generateId();
      const newMessage = new main.EnrichedMessage({
        ...message,
        id: messageId,
        timestamp: Date.now(),
        conversationId: get().activeConversationId || 0,
        isStreaming: message.isStreaming ?? false,
        internal: false,
        createdAt: new Date().toISOString(),
      });

      const newNode = new main.MessageNode({
        message: newMessage,
        children: [],
        level: 0,
        childCount: 0,
      });

      set((state) => ({
        activeConversation: state.activeConversation
          ? {
              ...state.activeConversation,
              threadedMessages: [...state.activeConversation.threadedMessages, newNode],
            }
          : state.activeConversation,
      }));

      if (message.role === 'user') {
        playSendSound();
        if (ttsService.isEnabledForUser()) {
          triggerAutoRead(message.content, 'user');
        } else if (ttsService.shouldUseAriaLiveForUser()) {
          const cleanContent = stripMarkdown(message.content);
          announce(`Você: ${cleanContent}`);
        }
      } else if (message.role === 'assistant' && !message.isStreaming) {
        playReceiveSound();
        if (ttsService.shouldUseAriaLiveForAgent()) {
          const cleanContent = stripMarkdown(message.content);
          announce(`Assistente: ${cleanContent}`);
        }
      }

      return messageId;
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

    sendMessage: async (content, mediaFiles) => {
      return get().sendMessageWithParams(content, mediaFiles);
    },

    sendMessageWithParams: async (content, mediaFiles, paramsOverride) => {
      const { addMessage } = get();

      if (content.length > MAX_MESSAGE_CONTENT_SIZE) {
        const errorMsg = `Mensagem muito grande (${content.length} bytes). Máximo permitido: ${MAX_MESSAGE_CONTENT_SIZE} bytes (500KB)`;
        console.error('[Chat]', errorMsg);
        announce(errorMsg);
        return;
      }

      if (mediaFiles && mediaFiles.length > 0) {
        const totalSize = mediaFiles.reduce((acc, f) => acc + f.file.size, 0);
        const estimatedBase64Size = Math.ceil(totalSize * 1.37);
        if (estimatedBase64Size > MAX_MEDIA_SIZE) {
          const errorMsg = `Arquivos de mídia muito grandes (~${Math.round(estimatedBase64Size / 1024 / 1024)}MB). Máximo permitido: 10MB`;
          console.error('[Chat]', errorMsg);
          announce(errorMsg);
          return;
        }
      }

      let conversationId = get().activeConversationId || 0;
      if (conversationId === 0) {
        try {
          conversationId = await get().createConversation();
        } catch (err) {
          console.error('[Chat] Erro ao criar conversa:', err);
          return;
        }
      }

      const conversationIdStr = conversationId.toString();

      const userMessageId = addMessage({ role: 'user', content });
      const assistantMessageId = addMessage({ role: 'assistant', content: '', isStreaming: true });

      set({ isLoading: true, streamingMessageId: assistantMessageId });

      let unsubscribe: (() => void) | null = null;

      const unsubscribeMessagesReady = EventsOn('chat:messages_ready', (data: ChatMessagesReadyEvent) => {
        if (data.userMessageId) {
          const backendUserId = data.userMessageId.toString();
          set((state) => {
            if (!state.activeConversation) return state;
            const updateMessageId = (nodes: MessageNode[]): MessageNode[] => {
              return nodes.map(node => {
                if (node.message.id === userMessageId) {
                  const updatedMessage = new main.EnrichedMessage({ ...node.message, id: backendUserId });
                  return new main.MessageNode({ ...node, message: updatedMessage });
                }
                return node;
              });
            };
            return {
              activeConversation: {
                ...state.activeConversation,
                threadedMessages: updateMessageId(state.activeConversation.threadedMessages),
              },
              activeConversationId: data.conversationId || state.activeConversationId,
            };
          });
        }
      });

      try {
        let unsubscribeStream: (() => void) | null = null;
        let unsubscribeComplete: (() => void) | null = null;
        let cleanupExecuted = false;
        let streamingAnnounced = false;

        const cleanup = () => {
          if (cleanupExecuted) return;
          cleanupExecuted = true;
          if (unsubscribeStream) { unsubscribeStream(); unsubscribeStream = null; }
          if (unsubscribeComplete) { unsubscribeComplete(); unsubscribeComplete = null; }
          activeListeners.delete(conversationIdStr);
          set({ isLoading: false, streamingMessageId: null, streamingReasoning: null, isThinking: false, activeToolCalls: [], completedSegments: [] });
        };

        const existingCleanup = activeListeners.get(conversationIdStr);
        if (existingCleanup) {
          existingCleanup();
          await new Promise(resolve => setTimeout(resolve, 0));
        }

        unsubscribeStream = EventsOn('chat:stream', (event: ChatStreamEvent) => {
          if (!activeListeners.has(conversationIdStr)) return;

          if (event.content) {
            if (!streamingAnnounced && !event.done && !event.error) {
              streamingAnnounced = true;
              announce('Assistente está respondendo', 'polite');
            }
            if (!event.done && !event.error) {
              debouncedUpdateMessage(assistantMessageId, event.content, get().updateMessage);
            } else {
              flushPendingUpdate(assistantMessageId, get().updateMessage);
              get().updateMessage(assistantMessageId, event.content);
            }
          }

          if (event.error) {
            console.error('[Chat] Stream error:', event.error);
            flushPendingUpdate(assistantMessageId, get().updateMessage);
            get().updateMessage(assistantMessageId, `Erro: ${event.error}`);
            cleanup();
          }

          if (event.done) {
            const currentState = get();
            const flatMessages = flattenThreadedMessages(currentState.activeConversation?.threadedMessages);
            const finalMessage = flatMessages.find(m => m.id === assistantMessageId);

            const backendAssistantId = event.messageId && event.messageId > 0
              ? event.messageId.toString()
              : null;

            set((state) => {
              if (!state.activeConversation) return state;
              return {
                activeConversation: {
                  ...state.activeConversation,
                  threadedMessages: state.activeConversation.threadedMessages.map((node) => {
                    const updateStreamingStatus = (n: MessageNode): MessageNode => {
                      if (n.message.id === assistantMessageId) {
                        n.message.isStreaming = false;
                        if (backendAssistantId) n.message.id = backendAssistantId;
                      }
                      if (n.children && n.children.length > 0) n.children = n.children.map(updateStreamingStatus);
                      return n;
                    };
                    return updateStreamingStatus(node);
                  }),
                },
              };
            });

            if (finalMessage?.content) {
              const isActiveConv = currentState.activeConversationId === conversationId;
              if (isActiveConv) playReceiveSound();
              if (ttsService.isAutoReadEnabled() && isActiveConv && !cleanupExecuted) {
                triggerAutoRead(finalMessage.content, 'assistant', event.messageId);
              }
              if (ttsService.shouldUseAriaLiveForAgent() && isActiveConv) {
                const cleanContent = stripMarkdown(finalMessage.content);
                announce(`Assistente: ${cleanContent}`);
              }
            }
          }
        });

        let unsubscribeThinking: (() => void) | null = null;
        unsubscribeThinking = EventsOn('chat:thinking', (event: ChatThinkingEvent) => {
          if (!activeListeners.has(conversationIdStr)) return;
          if (event.started) {
            set({ isThinking: true, streamingReasoning: event.content || '' });
            announce('O modelo está pensando...', 'polite');
          } else if (event.done) {
            set({ isThinking: false });
            if (event.content) get().updateMessageReasoning(assistantMessageId, event.content);
          } else {
            set({ streamingReasoning: event.content || '' });
          }
        });

        let unsubscribeToolStart: (() => void) | null = null;
        unsubscribeToolStart = EventsOn('chat:tool_start', (data: ChatToolStartEvent) => {
          if (!activeListeners.has(conversationIdStr)) return;
          set((state) => ({
            hadToolCalls: true,
            activeToolCalls: [
              ...state.activeToolCalls,
              { name: data.name, callId: data.callId, args: data.args, status: 'running' as const },
            ],
          }));
        });

        let unsubscribeToolEnd: (() => void) | null = null;
        unsubscribeToolEnd = EventsOn('chat:tool_end', (data: ChatToolEndEvent) => {
          if (!activeListeners.has(conversationIdStr)) return;
          set((state) => ({
            activeToolCalls: state.activeToolCalls.map((tc) =>
              tc.callId === data.callId
                ? { ...tc, status: (data.status === 'error' ? 'error' : 'done') as 'done' | 'error', summary: data.summary }
                : tc
            ),
          }));
          if (data.status === 'error') announce(`Ferramenta ${data.name} falhou`, 'assertive');
        });

        let unsubscribeSegmentDone: (() => void) | null = null;
        unsubscribeSegmentDone = EventsOn('chat:segment_done', (data: ChatSegmentDoneEvent) => {
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
              if (ttsService.isAutoReadEnabled()) {
                triggerAutoRead(data.content, 'assistant');
              } else {
                const cleanContent = stripMarkdown(data.content);
                announce(cleanContent, 'assertive');
              }
            }
            set({ completedSegments: newSegments, activeToolCalls: [] });
            flushPendingUpdate(assistantMessageId, get().updateMessage);
            get().updateMessage(assistantMessageId, '');
          }
        });

        unsubscribeComplete = EventsOn('chat:done', () => {
          if (!activeListeners.has(conversationIdStr)) return;
          const didUseTools = get().hadToolCalls;

          set((state) => {
            if (!state.activeConversation) return state;
            return {
              activeConversation: {
                ...state.activeConversation,
                threadedMessages: state.activeConversation.threadedMessages.map((node) => {
                  const updateStreamingStatus = (n: MessageNode): MessageNode => {
                    if (n.message.id === assistantMessageId) n.message.isStreaming = false;
                    if (n.children && n.children.length > 0) n.children = n.children.map(updateStreamingStatus);
                    return n;
                  };
                  return updateStreamingStatus(node);
                }),
              },
            };
          });

          if (didUseTools && get().activeConversationId === conversationId) {
            GetMessages(conversationId, null).then((backendNodes) => {
              const messageNodes: MessageNode[] = backendNodes.map(withOriginalIndex);
              set((state) => {
                if (state.activeConversationId !== conversationId) return state;
                return {
                  activeConversation: state.activeConversation
                    ? { ...state.activeConversation, threadedMessages: messageNodes }
                    : null,
                  hadToolCalls: false,
                  completedSegments: [],
                };
              });
            }).catch((err) => {
              console.error('[Chat] Erro ao recarregar mensagens:', err);
            });
          }

          {
            const conv = get().activeConversation;
            if (conv && conv.id === conversationId && DEFAULT_TITLE_PATTERNS.test(conv.title.trim())) {
              const firstUserMsg = flattenThreadedMessages(conv.threadedMessages)
                .find(m => m.role === 'user' && !m.internal);
              if (firstUserMsg?.content) {
                const fallbackTitle = firstUserMsg.content.length > 50
                  ? firstUserMsg.content.slice(0, 50) + '...'
                  : firstUserMsg.content;
                RenameConversation(conversationId, fallbackTitle).catch((err) => {
                  console.error('[Chat] Fallback rename failed:', err);
                });
              }
            }
          }

          cleanup();
        });

        const originalCleanup = cleanup;
        const enhancedCleanup = () => {
          originalCleanup();
          if (unsubscribeThinking) unsubscribeThinking();
          if (unsubscribeToolStart) unsubscribeToolStart();
          if (unsubscribeToolEnd) unsubscribeToolEnd();
          if (unsubscribeSegmentDone) unsubscribeSegmentDone();
        };

        activeListeners.set(conversationIdStr, enhancedCleanup);

        unsubscribe = () => {
          cleanup();
          if (unsubscribeMessagesReady) unsubscribeMessagesReady();
        };

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
        };
        await SendMessage(conversationId, content, mediaJson, mergedParams);

      } catch (error: unknown) {
        console.error('[Chat] Error sending message:', error);
        if (unsubscribe) unsubscribe();
        const errorMsg = getErrorMessage(error);
        get().updateMessage(assistantMessageId, `Erro ao enviar mensagem: ${errorMsg}`);

        set((state) => {
          if (!state.activeConversation) return state;
          return {
            activeConversation: {
              ...state.activeConversation,
              threadedMessages: state.activeConversation.threadedMessages.map((node) => {
                const updateStreamingStatus = (n: MessageNode): MessageNode => {
                  if (n.message.id === assistantMessageId) n.message.isStreaming = false;
                  if (n.children && n.children.length > 0) n.children = n.children.map(updateStreamingStatus);
                  return n;
                };
                return updateStreamingStatus(node);
              }),
            },
          };
        });

        set({ isLoading: false, streamingMessageId: null });
      }
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
        hadToolCalls: false,
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

      const { addMessage } = get();

      const userMessageId = addMessage({
        role: 'user',
        content: text || 'Transcrevendo áudio...',
        source: channel,
      });

      const assistantMessageId = addMessage({
        role: 'assistant',
        content: '',
        isStreaming: true,
      });

      set({ isLoading: true, streamingMessageId: assistantMessageId });

      const conversationIdStr = conversationId.toString();
      let cleanupExecuted = false;
      let streamingAnnounced = false;

      const cleanup = () => {
        if (cleanupExecuted) return;
        cleanupExecuted = true;
        unsubStream();
        unsubThinking();
        unsubToolStart();
        unsubToolEnd();
        unsubSegmentDone();
        unsubDone();
        unsubReady();
        activeListeners.delete(conversationIdStr);
        set({ isLoading: false, streamingMessageId: null, streamingReasoning: null, isThinking: false, activeToolCalls: [], completedSegments: [] });
      };

      const existingCleanup = activeListeners.get(conversationIdStr);
      if (existingCleanup) existingCleanup();

      const unsubReady = EventsOn('chat:messages_ready', (event: ChatMessagesReadyEvent) => {
        if (!activeListeners.has(conversationIdStr)) return;
        if (event.userMessageId) {
          const backendUserId = event.userMessageId.toString();
          set((state) => {
            if (!state.activeConversation) return state;
            const updateNodes = (nodes: MessageNode[]): MessageNode[] => {
              return nodes.map(node => {
                if (node.message.id === userMessageId) {
                  const updatedMessage = new main.EnrichedMessage({
                    ...node.message,
                    id: backendUserId,
                    content: event.userContent || node.message.content,
                  });
                  return new main.MessageNode({ ...node, message: updatedMessage });
                }
                return node;
              });
            };
            return {
              activeConversation: {
                ...state.activeConversation,
                threadedMessages: updateNodes(state.activeConversation.threadedMessages),
              },
              activeConversationId: event.conversationId || state.activeConversationId,
            };
          });
          if (event.userContent) {
            const cleanContent = stripMarkdown(event.userContent);
            announce(`${from} via ${channel}: ${cleanContent}`);
          }
        }
      });

      const unsubStream = EventsOn('chat:stream', (event: ChatStreamEvent) => {
        if (!activeListeners.has(conversationIdStr)) return;
        if (event.content) {
          if (!streamingAnnounced && !event.done && !event.error) {
            streamingAnnounced = true;
            announce('Assistente está respondendo', 'polite');
          }
          if (!event.done && !event.error) {
            debouncedUpdateMessage(assistantMessageId, event.content, get().updateMessage);
          } else {
            flushPendingUpdate(assistantMessageId, get().updateMessage);
            get().updateMessage(assistantMessageId, event.content);
          }
        }
        if (event.error) {
          flushPendingUpdate(assistantMessageId, get().updateMessage);
          get().updateMessage(assistantMessageId, `Erro: ${event.error}`);
          cleanup();
        }
        if (event.done) {
          const currentState = get();
          const flatMessages = flattenThreadedMessages(currentState.activeConversation?.threadedMessages);
          const finalMessage = flatMessages.find(m => m.id === assistantMessageId);
          const backendAssistantId = event.messageId && event.messageId > 0
            ? event.messageId.toString()
            : null;

          set((state) => {
            if (!state.activeConversation) return state;
            return {
              activeConversation: {
                ...state.activeConversation,
                threadedMessages: state.activeConversation.threadedMessages.map((node) => {
                  const markDone = (n: MessageNode): MessageNode => {
                    if (n.message.id === assistantMessageId) {
                      n.message.isStreaming = false;
                      if (backendAssistantId) n.message.id = backendAssistantId;
                    }
                    if (n.children?.length) n.children = n.children.map(markDone);
                    return n;
                  };
                  return markDone(node);
                }),
              },
            };
          });
          if (finalMessage?.content) {
            const isActive = currentState.activeConversationId === conversationId;
            if (isActive) playReceiveSound();
            if (ttsService.isAutoReadEnabled() && isActive && !cleanupExecuted) {
              triggerAutoRead(finalMessage.content, 'assistant', event.messageId);
            }
            if (ttsService.shouldUseAriaLiveForAgent() && isActive) {
              announce(`Assistente: ${stripMarkdown(finalMessage.content)}`);
            }
          }
        }
      });

      const unsubThinking = EventsOn('chat:thinking', (event: ChatThinkingEvent) => {
        if (!activeListeners.has(conversationIdStr)) return;
        if (event.started) {
          set({ isThinking: true, streamingReasoning: event.content || '' });
          announce('O modelo está pensando...', 'polite');
        } else if (event.done) {
          set({ isThinking: false });
          if (event.content) get().updateMessageReasoning(assistantMessageId, event.content);
        } else {
          set({ streamingReasoning: event.content || '' });
        }
      });

      const unsubToolStart = EventsOn('chat:tool_start', (event: ChatToolStartEvent) => {
        if (!activeListeners.has(conversationIdStr)) return;
        set((state) => ({
          hadToolCalls: true,
          activeToolCalls: [...state.activeToolCalls, {
            name: event.name, callId: event.callId, args: event.args, status: 'running' as const,
          }],
        }));
        announce(`Executando ferramenta: ${event.name}`, 'polite');
      });

      const unsubToolEnd = EventsOn('chat:tool_end', (event: ChatToolEndEvent) => {
        if (!activeListeners.has(conversationIdStr)) return;
        set((state) => ({
          activeToolCalls: state.activeToolCalls.map((tc) =>
            tc.callId === event.callId
              ? { ...tc, status: (event.status === 'error' ? 'error' : 'done') as 'done' | 'error', summary: event.summary }
              : tc
          ),
        }));
        const statusLabel = event.status === 'error' ? 'falhou' : 'concluída';
        announce(`Ferramenta ${event.name} ${statusLabel}`, 'polite');
      });

      const unsubSegmentDone = EventsOn('chat:segment_done', (event: ChatSegmentDoneEvent) => {
        if (!activeListeners.has(conversationIdStr)) return;
        if (event.hasMore && event.content && ttsService.isAutoReadEnabled()) {
          triggerAutoRead(event.content, 'assistant');
        }
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
          flushPendingUpdate(assistantMessageId, get().updateMessage);
          get().updateMessage(assistantMessageId, '');
        }
      });

      const unsubDone = EventsOn('chat:done', () => {
        if (!activeListeners.has(conversationIdStr)) return;
        const didUseTools = get().hadToolCalls;

        set((state) => {
          if (!state.activeConversation) return state;
          return {
            activeConversation: {
              ...state.activeConversation,
              threadedMessages: state.activeConversation.threadedMessages.map((node) => {
                const markDone = (n: MessageNode): MessageNode => {
                  if (n.message.id === assistantMessageId) n.message.isStreaming = false;
                  if (n.children?.length) n.children = n.children.map(markDone);
                  return n;
                };
                return markDone(node);
              }),
            },
          };
        });

        if (didUseTools && get().activeConversationId === conversationId) {
          GetMessages(conversationId, null).then((backendNodes) => {
            const messageNodes: MessageNode[] = backendNodes.map(withOriginalIndex);
            set((state) => {
              if (state.activeConversationId !== conversationId) return state;
              return {
                activeConversation: state.activeConversation
                  ? { ...state.activeConversation, threadedMessages: messageNodes }
                  : null,
                hadToolCalls: false,
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
