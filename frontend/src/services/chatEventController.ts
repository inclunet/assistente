import { logger } from '../utils/logger';
import i18next from 'i18next';
import { chat } from '../../wailsjs/go/models';
import { isAppToolEvent, type ToolCallStatus, type ToolOrigin } from '../types/chat';
import type { MediaFile } from './mediaService';
import { announce } from '../hooks/useAnnouncer';
import {
  finalizeStreamingNode,
  flattenThreadedMessages,
  hasMessageId,
  markMessageStreamingInTree,
  type ChatTreeConversation,
  type Message,
  type MessageNode,
  type TurnSegment,
} from '../lib/chatMessageTree';
import { stripMarkdown } from '../lib/stripMarkdown';
import {
  announceChatBackgroundResponseDone,
  announceForActiveChatConversation,
  getChatConversationVoiceOrigin,
  playChatReceiveSoundIfActive,
  playChatErrorSoundIfActive,
} from './chatArbitration';
import { announceWithOrigin } from './voiceAccessibility/announcerBroker';
import { handleChatSpeak, type ChatSpeakEvent } from './chatSpeak';
import type { ChatSurfaceOrigin, MessageWindowState } from './chatSessionRegistry';
import { clearChatTurnRoutes, createChatTurnEventRouter } from './chatEventHub';

const translateBackendChatError = (message: string) => {
  if (message === 'assistant_placeholder_error') {
    return i18next.t('chat.errors.assistantPlaceholder');
  }
  if (message === 'internal_error') {
    return i18next.t('chat.errors.internalError');
  }
  if (message === 'streaming_interrupted') {
    return i18next.t('chat.errors.streamingInterrupted');
  }
  if (message === 'streaming_idle_timeout') {
    return i18next.t('chat.errors.streamingIdleTimeout');
  }
  return message;
};

interface ChatMessagesReadyEvent {
  conversationId: string;
  userMessageId: string;
  userContent: string;
  turnId?: string;
  surfaceOrigin?: ChatSurfaceOrigin;
}

interface ChatStreamEvent {
  conversationId: string;
  delta?: string;
  reset?: boolean;
  baseContent?: string;
  sequence: number;
  done?: boolean;
  error?: string;
  finishReason?: 'stop' | 'tool_calls' | 'max_tokens' | 'content_filter' | 'cancelled' | 'other';
  rawReason?: string;
  provider?: string;
  model?: string;
  effectiveOutputLimit?: number;
  outputTokens?: number;
  reasoningTokens?: number;
  responseBytes?: number;
  messageId?: string;
  turnId?: string;
  surfaceOrigin?: ChatSurfaceOrigin;
}

interface ChatThinkingEvent {
  conversationId: string;
  assistantMessageId?: string;
  started?: boolean;
  done?: boolean;
  content?: string;
  turnId?: string;
  surfaceOrigin?: ChatSurfaceOrigin;
}

interface ChatToolStartEvent {
  conversationId: string;
  assistantMessageId?: string;
  name: string;
  callId: string;
  args?: string;
  summary?: string;
  origin?: ToolOrigin;
  turnId?: string;
  surfaceOrigin?: ChatSurfaceOrigin;
}

interface ChatToolEndEvent {
  conversationId: string;
  assistantMessageId?: string;
  callId: string;
  name?: string;
  status?: string;
  summary?: string;
  origin?: ToolOrigin;
  attempt?: number;
  turnId?: string;
  surfaceOrigin?: ChatSurfaceOrigin;
}

interface ChatToolFailureEvent {
  conversationId: string;
  assistantMessageId?: string;
  name: string;
  callId: string;
  origin?: ToolOrigin;
  willRetry?: boolean;
  turnId?: string;
  surfaceOrigin?: ChatSurfaceOrigin;
}

interface ChatSegmentDoneEvent {
  conversationId: string;
  assistantMessageId?: string;
  hasMore?: boolean;
  content?: string;
  turnId?: string;
  surfaceOrigin?: ChatSurfaceOrigin;
}

interface ChatDoneEvent {
  conversationId: string;
  assistantMessageId?: string;
  turnId?: string;
  hadToolCalls?: boolean;
  reason?: 'completed' | 'limit_reached' | 'output_limit' | 'error' | 'cancelled';
  finishReason?: 'stop' | 'tool_calls' | 'max_tokens' | 'content_filter' | 'cancelled' | 'other';
  rawReason?: string;
  provider?: string;
  model?: string;
  effectiveOutputLimit?: number;
  outputTokens?: number;
  reasoningTokens?: number;
  responseBytes?: number;
  errorMessage?: string;
  surfaceOrigin?: ChatSurfaceOrigin;
  turnPatch?: ChatTurnPatch;
}

interface ChatErrorEvent {
  conversationId: string;
  error: string;
}

interface ChatMediaProcessingEvent {
  conversationId: string;
  messageId?: string;
  turnId?: string;
  status: 'started' | 'completed' | 'failed' | 'cancelled';
  error?: string;
  surfaceOrigin?: ChatSurfaceOrigin;
}

interface ChatTurnPatch {
  message: {
    id: string;
    conversationId: string;
    parentId?: string;
    turnId: string;
    content: string;
    reasoning?: string;
    toolCalls?: string;
    promptTokens?: number;
    completionTokens?: number;
    totalTokens?: number;
    cacheReadTokens?: number;
    cacheWriteTokens?: number;
    cacheMissTokens?: number;
    model?: string;
    createdAt: string;
    timestamp: number;
    turnSegments?: TurnSegment[];
  };
}

export interface ChatEventSession {
  conversation: ChatTreeConversation | null;
  activeToolCalls: ToolCallStatus[];
  completedSegments: TurnSegment[];
  sendFailureMessage?: string | null;
  sendFailureAnnounced?: boolean;
  sendFailureRetryable?: boolean;
  sendFailureRetryContent?: string | null;
  sendFailureRetryMediaFiles?: MediaFile[];
  messageWindow?: MessageWindowState;
  surfaceOrigin?: ChatSurfaceOrigin;
}

export interface ChatEventControllerAdapter {
  getSession: (conversationId: string, sessionKey?: string) => ChatEventSession;
  patchSession: (conversationId: string, patch: Partial<ChatEventSession> & Record<string, unknown>) => void;
  patchConversation: (
    conversationId: string,
    updater: (conversation: ChatTreeConversation) => ChatTreeConversation,
  ) => void;
  updateMessage: (conversationId: string, messageId: string, content: string) => void;
  commitMessage: (conversationId: string, messageId: string, content: string) => void;
  updateReasoning: (conversationId: string, messageId: string, reasoning: string) => void;
  setConversationLoading: (conversationId: string, isLoading: boolean, sessionKey?: string) => void;
}

interface ChatEventControllerOptions {
  conversationId: string;
  initialUserContent?: string;
  initialMediaFiles?: MediaFile[];
  external?: {
    channel: string;
    from: string;
    text: string;
  };
  origin?: ChatSurfaceOrigin;
  adapter: ChatEventControllerAdapter;
}

export interface ChatEventControllerHandle {
  cleanup: () => void;
  handleSendCancellation: () => void;
  handleSendFailure: (message: string) => void;
  done: Promise<void>;
}

const activeControllers = new Map<string, () => void>();

/**
 * Origem já conhecida da ferramenta. O evento de fim costuma repeti-la, mas se
 * vier sem ela o anúncio não pode creditar ao app o que o agente fez.
 */
const knownToolOrigin = (session: ChatEventSession, callId: string): ToolOrigin | undefined =>
  session.activeToolCalls.find((tc) => tc.callId === callId)?.origin;

export function stopChatEventController(conversationId: string) {
  const cleanup = activeControllers.get(conversationId.toString());
  if (cleanup) cleanup();
}

export function stopAllChatEventControllers() {
  activeControllers.forEach((cleanup) => cleanup());
  activeControllers.clear();
  clearChatTurnRoutes();
}

export function startChatEventController({
  conversationId,
  initialUserContent = '',
  initialMediaFiles,
  external,
  origin,
  adapter,
}: ChatEventControllerOptions): ChatEventControllerHandle {
  const conversationIdStr = conversationId.toString();
  let currentAssistantNodeId: string | null = null;
  let cleanupExecuted = false;
  let streamingAnnounced = false;
  let assistantNodeCreated = false;
  // Turnos agênticos que terminam sem texto não têm nada para o backend falar
  // via chat:speak; o leitor de tela precisa de um aviso de conclusão próprio.
  let turnHadAssistantText = false;
  let currentTurnId: string | null = null;
  let provisionalThinkingTurnId: string | null = null;
  let provisionalThinkingNodeId: string | null = null;
  let streamedContent = '';
  let streamSequence = -1;
  let streamInitialized = false;
  let pendingVisualContent: string | null = null;
  let animationFrameId: number | null = null;
  let streamingCommitted = false;
  let resolveDone: () => void = () => {};
  const done = new Promise<void>((resolve) => {
    resolveDone = resolve;
  });
  const getCurrentSession = () => adapter.getSession(conversationId, origin?.sessionKey);
  const patchCurrentSession = (patch: Partial<ChatEventSession> & Record<string, unknown>) => {
    adapter.patchSession(conversationId, origin ? { ...patch, surfaceOrigin: origin } : patch);
  };

  adapter.setConversationLoading(conversationId, true, origin?.sessionKey);
  patchCurrentSession({
    completedSegments: [],
    activeToolCalls: [],
    isLoading: true,
    sendFailureMessage: null,
    sendFailureAnnounced: false,
    sendFailureRetryable: false,
    sendFailureRetryContent: null,
    sendFailureRetryMediaFiles: [],
  });

  const noop = () => { /* no-op */ };
  let unsubMessagesReady = noop;
  let unsubMediaProcessing = noop;
  let unsubStream = noop;
  let unsubThinking = noop;
  let unsubToolStart = noop;
  let unsubToolEnd = noop;
  let unsubToolFailure = noop;
  let unsubSegmentDone = noop;
  let unsubDone = noop;
  let unsubError = noop;
  let unsubSpeak = noop;

  const isActive = () => activeControllers.has(conversationIdStr);
  const getEventOrigin = (event: { surfaceOrigin?: ChatSurfaceOrigin }) => event.surfaceOrigin ?? origin;

  const ensureAssistantNode = (messageId?: string | null) => {
    const backendMessageId = messageId && messageId !== '' ? messageId : null;
    if (!backendMessageId) return false;
    const session = getCurrentSession();
    if (!session.conversation) return false;
    currentAssistantNodeId = backendMessageId;
    if (assistantNodeCreated) return true;
    if (hasMessageId(session.conversation.threadedMessages, backendMessageId)) {
      assistantNodeCreated = true;
      adapter.patchConversation(
        conversationId,
        (conversation) => ({
          ...conversation,
          threadedMessages: markMessageStreamingInTree(conversation.threadedMessages, backendMessageId, currentTurnId),
        }),
      );
      patchCurrentSession({
        streamingMessageId: backendMessageId,
        lastInterruptedMessageId: null,
      });
      return true;
    }
    assistantNodeCreated = true;
    const assistantMsg = new chat.EnrichedMessage({
      id: backendMessageId,
      role: 'assistant',
      content: '',
      timestamp: Date.now(),
      conversationId,
      turnId: currentTurnId ?? undefined,
      isStreaming: true,
      internal: false,
      createdAt: new Date().toISOString(),
    }) as Message;
    const assistantNode = new chat.MessageNode({ message: assistantMsg, children: [], level: 0, childCount: 0 });
    patchCurrentSession({
      conversation: {
        ...session.conversation,
        threadedMessages: [...session.conversation.threadedMessages, assistantNode as MessageNode],
      },
      appendVisibleMessages: true,
      streamingMessageId: backendMessageId,
      lastInterruptedMessageId: null,
    });
    return true;
  };

  const flushVisualStreamingUpdate = () => {
    if (animationFrameId !== null) {
      cancelAnimationFrame(animationFrameId);
      animationFrameId = null;
    }
    const content = pendingVisualContent;
    pendingVisualContent = null;
    if (content === null || cleanupExecuted || !currentAssistantNodeId) return;
    adapter.updateMessage(conversationId, currentAssistantNodeId, content);
  };

  const commitStreamingContent = () => {
    if (streamingCommitted || !currentAssistantNodeId) return;
    const finalContent = pendingVisualContent ?? streamedContent;
    flushVisualStreamingUpdate();
    adapter.commitMessage(conversationId, currentAssistantNodeId, finalContent);
    streamingCommitted = true;
  };

  const cleanup = () => {
    if (cleanupExecuted) return;
    commitStreamingContent();
    cleanupExecuted = true;
    unsubMessagesReady();
    unsubMediaProcessing();
    unsubStream();
    unsubThinking();
    unsubToolStart();
    unsubToolEnd();
    unsubToolFailure();
    unsubSegmentDone();
    unsubDone();
    unsubError();
    unsubSpeak();
    turnEvents.unregister();
    activeControllers.delete(conversationIdStr);
    adapter.setConversationLoading(conversationId, false, origin?.sessionKey);
    patchCurrentSession({
      isLoading: false,
      streamingMessageId: null,
      streamingReasoning: null,
      isThinking: false,
      activeToolCalls: [],
      completedSegments: [],
    });
    resolveDone();
  };

  const finalizeStreaming = (finalId?: string | null, finalTurnId: string | null = currentTurnId) => {
    if (finalId && !currentAssistantNodeId) {
      ensureAssistantNode(finalId);
    }
    if (!currentAssistantNodeId) return;
    commitStreamingContent();
    const assistantNodeId = currentAssistantNodeId;
    adapter.patchConversation(
      conversationId,
      (conversation) => finalizeStreamingNode(conversation, assistantNodeId, finalId, finalTurnId),
    );
  };

  const applyTurnPatch = (patch?: ChatTurnPatch) => {
    if (!patch?.message || patch.message.conversationId !== conversationId) return;
    if (currentTurnId && patch.message.turnId !== currentTurnId) return;
    const persistedMessage = new chat.EnrichedMessage({
      ...patch.message,
      role: 'assistant',
      turnId: patch.message.turnId,
      isStreaming: false,
      internal: false,
      pinned: false,
    }) as Message;
    const persistedNode = new chat.MessageNode({
      message: persistedMessage,
      children: [],
      level: 0,
      childCount: 0,
    }) as MessageNode;
    adapter.patchConversation(conversationId, (conversation) => {
      let replaced = false;
      const threadedMessages = conversation.threadedMessages.map((node) => {
        const sameTurn = node.message.role === 'assistant' && node.message.turnId === patch.message.turnId;
        const sameMessage = node.message.id === patch.message.id;
        if (!sameTurn && !sameMessage) return node;
        replaced = true;
        return new chat.MessageNode({
          message: persistedMessage,
          children: node.children,
          level: node.level,
          childCount: node.childCount,
          originalIndex: node.originalIndex,
        }) as MessageNode;
      });
      return {
        ...conversation,
        threadedMessages: replaced ? threadedMessages : [...threadedMessages, persistedNode],
      };
    });
    currentAssistantNodeId = patch.message.id;
    assistantNodeCreated = true;
    if (animationFrameId !== null) {
      cancelAnimationFrame(animationFrameId);
      animationFrameId = null;
    }
    pendingVisualContent = null;
    streamedContent = patch.message.content;
    streamInitialized = true;
    streamingCommitted = false;
    turnHadAssistantText = turnHadAssistantText
      || patch.message.content.trim().length > 0
      || (patch.message.turnSegments ?? []).some(
        (segment) => segment.type === 'text' && Boolean(segment.content?.trim()),
      );
    patchCurrentSession({ completedSegments: [], streamingMessageId: patch.message.id });
  };

  const updateStreamingMessage = (content: string) => {
    if (!currentAssistantNodeId) return;
    pendingVisualContent = content;
    streamingCommitted = false;
    if (animationFrameId !== null) return;
    animationFrameId = requestAnimationFrame(() => {
      animationFrameId = null;
      const nextContent = pendingVisualContent;
      pendingVisualContent = null;
      if (nextContent === null || cleanupExecuted || !currentAssistantNodeId) return;
      adapter.updateMessage(conversationId, currentAssistantNodeId, nextContent);
    });
  };

  const getCurrentAssistantContent = () => {
    if (!currentAssistantNodeId) return '';
    if (pendingVisualContent !== null) return pendingVisualContent;
    if (streamInitialized) return streamedContent;
    const messages = flattenThreadedMessages(getCurrentSession().conversation?.threadedMessages);
    return String(messages.find(m => m.id === currentAssistantNodeId)?.content || '');
  };

  const updateAssistantWithError = (message: string) => {
    const currentContent = getCurrentAssistantContent();
    const hasContent = currentContent.trim().length > 0;
    const formattedError = i18next.t('chat.errorPrefix', { message });
    updateStreamingMessage(
      hasContent ? `${currentContent}\n\n${formattedError}` : formattedError,
    );
  };

  const existingCleanup = activeControllers.get(conversationIdStr);
  if (existingCleanup) existingCleanup();

  const turnEvents = createChatTurnEventRouter(
    conversationIdStr,
    () => currentTurnId,
    (turnId) => { currentTurnId = turnId; },
  );

  unsubError = turnEvents.on('chat:error', (event: ChatErrorEvent) => {
    if (event.conversationId !== conversationId && event.conversationId !== '') return;
    if (!isActive()) return;
    finalizeStreaming();
    announce(event.error);
    playChatErrorSoundIfActive(conversationId, origin);
    cleanup();
  });

  unsubSpeak = turnEvents.on('chat:speak', (event: ChatSpeakEvent) => {
    if (event.conversationId !== conversationId) return;
    if (!isActive()) return;
    const eventOrigin = getEventOrigin(event);
    const voiceOrigin = getChatConversationVoiceOrigin(conversationId, undefined, eventOrigin);
    void handleChatSpeak({
      ...event,
      accessibilityOrigin: origin
        ? {
          tabId: origin.tabId,
          surfaceId: origin.surfaceId,
          sessionKey: origin.sessionKey,
          conversationId,
          surfaceType: origin.surfaceType,
          profileSlug: voiceOrigin.profileSlug,
          title: voiceOrigin.title,
        }
        : voiceOrigin,
    }).catch((err) => {
      announce(i18next.t('chat.autoReadError'));
      logger.error('[chat:speak] falha ao processar evento TTS', err);
    });
  });

  unsubMessagesReady = turnEvents.on('chat:messages_ready', (event: ChatMessagesReadyEvent) => {
    if (event.conversationId !== conversationId) return;
    if (!isActive()) return;
    if (!event.userMessageId) return;
    const nextTurnId = event.turnId || event.userMessageId.toString();
    if (
      provisionalThinkingTurnId
      && provisionalThinkingTurnId !== nextTurnId
    ) {
      if (currentAssistantNodeId && provisionalThinkingNodeId === currentAssistantNodeId) {
        const staleAssistantId = currentAssistantNodeId;
        adapter.patchConversation(conversationId, (conversation) => ({
          ...conversation,
          threadedMessages: conversation.threadedMessages.filter(
            (node) => node.message.id !== staleAssistantId,
          ),
        }));
      }
      currentAssistantNodeId = null;
      assistantNodeCreated = false;
      streamedContent = '';
      streamInitialized = false;
      pendingVisualContent = null;
      patchCurrentSession({ streamingMessageId: null, streamingReasoning: null, isThinking: false });
    }
    provisionalThinkingTurnId = null;
    provisionalThinkingNodeId = null;
    currentTurnId = nextTurnId;
    if (hasMessageId(getCurrentSession().conversation?.threadedMessages, String(event.userMessageId))) return;
    const userMsg = new chat.EnrichedMessage({
      id: event.userMessageId.toString(),
      role: 'user',
      content: event.userContent || external?.text || initialUserContent,
      timestamp: Date.now(),
      conversationId: event.conversationId,
      isStreaming: false,
      internal: false,
      createdAt: new Date().toISOString(),
      source: external?.channel,
    }) as Message;
    const userNode = new chat.MessageNode({ message: userMsg, children: [], level: 0, childCount: 0 });
    const session = getCurrentSession();
    if (!session.conversation) return;
    patchCurrentSession({
      conversation: {
        ...session.conversation,
        threadedMessages: [...session.conversation.threadedMessages, userNode as MessageNode],
      },
      appendVisibleMessages: true,
    });
    if (external && event.userContent) {
      announce(i18next.t('chat.announce.externalMessage', {
        from: external.from,
        channel: external.channel,
        message: stripMarkdown(event.userContent),
      }));
    }
  });

  unsubMediaProcessing = turnEvents.on('chat:media_processing', (event: ChatMediaProcessingEvent) => {
    if (event.conversationId !== conversationId || !isActive()) return;
    if (event.status === 'started') {
      announceForActiveChatConversation(
        conversationId,
        i18next.t('chat.mediaProcessing.started'),
        'polite',
        getEventOrigin(event),
      );
    } else if (event.status === 'completed') {
      announceForActiveChatConversation(
        conversationId,
        i18next.t('chat.mediaProcessing.completed'),
        'polite',
        getEventOrigin(event),
      );
    } else if (event.status === 'failed') {
      announce(i18next.t('chat.mediaProcessing.failed'), 'assertive');
    } else if (event.status === 'cancelled') {
      announceForActiveChatConversation(
        conversationId,
        i18next.t('chat.mediaProcessing.cancelled'),
        'polite',
        getEventOrigin(event),
      );
    }
  });

  unsubStream = turnEvents.on('chat:stream', (event: ChatStreamEvent) => {
    if (event.conversationId !== conversationId) return;
    if (!isActive()) return;

    if (event.delta && !event.done && !event.error) {
      if (
        !Number.isSafeInteger(event.sequence)
        || (event.reset
          ? event.sequence !== 0
          : !streamInitialized || event.sequence !== streamSequence + 1)
      ) {
        return;
      }
      currentTurnId = event.turnId || currentTurnId;
      const backendAssistantId = event.messageId && event.messageId !== '' ? event.messageId : null;
      if (!ensureAssistantNode(backendAssistantId) && !currentAssistantNodeId) return;
      if (event.reset) {
        streamedContent = event.baseContent ?? '';
        streamSequence = -1;
        streamInitialized = true;
        streamingCommitted = false;
      }
      streamSequence = event.sequence;
      streamedContent += event.delta;
      if (streamedContent.trim()) turnHadAssistantText = true;
      if (!streamingAnnounced) {
        streamingAnnounced = true;
        announceForActiveChatConversation(conversationId, i18next.t('chat.announce.assistantResponding'), 'polite', getEventOrigin(event));
      }
      updateStreamingMessage(streamedContent);
    }

    if (event.error) {
      currentTurnId = event.turnId || currentTurnId;
      const backendAssistantId = event.messageId && event.messageId !== '' ? event.messageId : null;
      const hasAssistantNode = ensureAssistantNode(backendAssistantId) || currentAssistantNodeId !== null;
      const errorMessage = translateBackendChatError(String(event.error || '').trim());
      const eventOrigin = getEventOrigin(event);
      announceWithOrigin({
        message: errorMessage,
        origin: getChatConversationVoiceOrigin(conversationId, undefined, eventOrigin),
        eventType: 'error',
        announcePriority: 'assertive',
      });
      playChatErrorSoundIfActive(conversationId, eventOrigin);
      if (hasAssistantNode) {
        updateAssistantWithError(errorMessage);
      } else {
        patchCurrentSession({ sendFailureMessage: errorMessage, sendFailureAnnounced: true, sendFailureRetryable: false });
      }
      const interruptedId = backendAssistantId || currentAssistantNodeId;
      patchCurrentSession({ lastInterruptedMessageId: interruptedId });
      finalizeStreaming();
      cleanup();
      return;
    }

    if (event.done) {
      currentTurnId = event.turnId || currentTurnId;
      const backendAssistantId = event.messageId && event.messageId !== '' ? event.messageId : null;
      ensureAssistantNode(backendAssistantId);
      finalizeStreaming(backendAssistantId, event.turnId || currentTurnId);

      const flatMessages = flattenThreadedMessages(getCurrentSession().conversation?.threadedMessages);
      const finalMessage = flatMessages.find(m => m.id === (backendAssistantId || currentAssistantNodeId));
      if (finalMessage?.content) playChatReceiveSoundIfActive(conversationId, getEventOrigin(event));
    }
  });

  unsubThinking = turnEvents.on('chat:thinking', (event: ChatThinkingEvent) => {
    if (event.conversationId !== conversationId) return;
    if (!isActive()) return;
    let existingProvisionalNode = false;
    if (!currentTurnId && event.turnId) {
      provisionalThinkingTurnId = event.turnId;
      existingProvisionalNode = hasMessageId(
        getCurrentSession().conversation?.threadedMessages,
        String(event.assistantMessageId || ''),
      );
      if (!existingProvisionalNode && event.assistantMessageId) {
        provisionalThinkingNodeId = event.assistantMessageId;
      }
    }
    if (!existingProvisionalNode) ensureAssistantNode(event.assistantMessageId);
    if (event.started) {
      patchCurrentSession({
        isThinking: true,
        streamingReasoning: event.content || '',
      });
      announceForActiveChatConversation(conversationId, i18next.t('chat.announce.modelThinking'), 'polite', getEventOrigin(event));
    } else if (event.done) {
      patchCurrentSession({ isThinking: false, streamingReasoning: '' });
      if (currentAssistantNodeId) {
        adapter.updateReasoning(conversationId, currentAssistantNodeId, event.content || '');
      }
    } else {
      patchCurrentSession({ streamingReasoning: event.content || '' });
    }
  });

  unsubToolStart = turnEvents.on('chat:tool_start', (event: ChatToolStartEvent) => {
    if (event.conversationId !== conversationId) return;
    if (!isActive()) return;
    currentTurnId = event.turnId || currentTurnId;
    ensureAssistantNode(event.assistantMessageId);
    const session = getCurrentSession();
    const existing = session.activeToolCalls.findIndex((tc) => tc.callId === event.callId);
    patchCurrentSession({
      activeToolCalls: existing >= 0
        ? session.activeToolCalls.map((tc) =>
          tc.callId === event.callId
            ? { ...tc, name: event.name, callId: event.callId, args: event.args ?? tc.args, status: 'running' as const, summary: event.summary, origin: event.origin ?? tc.origin }
            : tc
        )
        : [...session.activeToolCalls, { name: event.name, callId: event.callId, args: event.args, status: 'running' as const, summary: event.summary, origin: event.origin }],
    });
    if (external) {
      const runningMessage = isAppToolEvent(event.origin ?? knownToolOrigin(session, event.callId))
        ? i18next.t('chat.toolRunning', { name: event.name })
        : i18next.t('chat.agentToolRunning', { name: event.name });
      announceForActiveChatConversation(conversationId, runningMessage, 'polite', getEventOrigin(event));
    }
  });

  unsubToolEnd = turnEvents.on('chat:tool_end', (event: ChatToolEndEvent) => {
    if (event.conversationId !== conversationId) return;
    if (!isActive()) return;
    currentTurnId = event.turnId || currentTurnId;
    ensureAssistantNode(event.assistantMessageId);
    const session = getCurrentSession();
    patchCurrentSession({
      activeToolCalls: session.activeToolCalls.map((tc) =>
        tc.callId === event.callId
          ? { ...tc, status: (event.status === 'error' ? 'error' : 'done') as 'done' | 'error', summary: event.summary, origin: event.origin ?? tc.origin }
          : tc
      ),
    });
    if (!external) return;
    const fromApp = isAppToolEvent(event.origin ?? knownToolOrigin(session, event.callId));
    if (event.status !== 'error') {
      const doneMessage = fromApp
        ? i18next.t('chat.toolDone', { name: event.name })
        : i18next.t('chat.agentToolDone', { name: event.name });
      announceForActiveChatConversation(conversationId, doneMessage, 'polite', getEventOrigin(event));
      return;
    }
    if (!('attempt' in event)) {
      announce(
        fromApp
          ? i18next.t('chat.toolFailed', { name: event.name })
          : i18next.t('chat.agentToolFailed', { name: event.name }),
        'assertive',
      );
    }
  });

  unsubToolFailure = turnEvents.on('chat:tool_failure', (event: ChatToolFailureEvent) => {
    if (event.conversationId !== conversationId) return;
    if (!isActive()) return;
    currentTurnId = event.turnId || currentTurnId;
    ensureAssistantNode(event.assistantMessageId);
    if (event.willRetry) {
      announceForActiveChatConversation(conversationId, i18next.t('chat.toolRetrying', { name: event.name }), 'polite', getEventOrigin(event));
      return;
    }
    announce(
      isAppToolEvent(event.origin ?? knownToolOrigin(getCurrentSession(), event.callId))
        ? i18next.t('chat.toolFailed', { name: event.name })
        : i18next.t('chat.agentToolFailed', { name: event.name }),
      'assertive',
    );
    playChatErrorSoundIfActive(conversationId, getEventOrigin(event));
  });

  unsubSegmentDone = turnEvents.on('chat:segment_done', (event: ChatSegmentDoneEvent) => {
    if (event.conversationId !== conversationId) return;
    if (!isActive()) return;
    currentTurnId = event.turnId || currentTurnId;
    ensureAssistantNode(event.assistantMessageId);
    if (!event.hasMore) return;

    const session = getCurrentSession();
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
          origin: tc.origin,
        })),
      });
      if (!external) {
        announceForActiveChatConversation(
          conversationId,
          toolCount === 1 ? session.activeToolCalls[0].name : `${toolCount} ferramentas`,
          'polite',
          getEventOrigin(event),
        );
      }
    }
    if (event.content) {
      if (event.content.trim()) turnHadAssistantText = true;
      newSegments.push({ type: 'text', content: event.content });
    }
    patchCurrentSession({
      completedSegments: newSegments,
      activeToolCalls: [],
    });
    flushVisualStreamingUpdate();
    if (currentAssistantNodeId) adapter.updateMessage(conversationId, currentAssistantNodeId, '');
    streamedContent = '';
    streamSequence = -1;
    streamInitialized = false;
    streamingCommitted = false;
  });

  unsubDone = turnEvents.on('chat:done', (event: ChatDoneEvent) => {
    if (event.conversationId !== conversationId) return;
    if (!isActive()) return;
    currentTurnId = event.turnId || currentTurnId;

    if (event.errorMessage) {
      // O snapshot persistido contém apenas o parcial. Aplique-o antes do
      // marcador de erro para que o patch não apague o diagnóstico terminal.
      applyTurnPatch(event.turnPatch);
      const backendAssistantId = event.assistantMessageId && event.assistantMessageId !== '' ? event.assistantMessageId : null;
      const hasAssistantNode = ensureAssistantNode(backendAssistantId) || currentAssistantNodeId !== null;
      const errorMessage = translateBackendChatError(String(event.errorMessage || '').trim());
      const eventOrigin = getEventOrigin(event);
      announceWithOrigin({
        message: errorMessage,
        origin: getChatConversationVoiceOrigin(conversationId, undefined, eventOrigin),
        eventType: 'error',
        announcePriority: 'assertive',
      });
      playChatErrorSoundIfActive(conversationId, eventOrigin);
      if (hasAssistantNode) {
        updateAssistantWithError(errorMessage);
      } else {
        patchCurrentSession({ sendFailureMessage: errorMessage, sendFailureAnnounced: true, sendFailureRetryable: false });
      }
      const interruptedId = backendAssistantId || currentAssistantNodeId;
      patchCurrentSession({ lastInterruptedMessageId: interruptedId });
      finalizeStreaming(backendAssistantId, currentTurnId);
      cleanup();
      return;
    }

    if (event.reason === 'output_limit') {
      const backendAssistantId = event.assistantMessageId && event.assistantMessageId !== '' ? event.assistantMessageId : null;
      const hasAssistantNode = ensureAssistantNode(backendAssistantId) || currentAssistantNodeId !== null;
      const message = i18next.t('chat.outputLimitReached');
      announceWithOrigin({
        message,
        origin: getChatConversationVoiceOrigin(conversationId, undefined, getEventOrigin(event)),
        eventType: 'system',
        announcePriority: 'assertive',
      });
      if (hasAssistantNode && !getCurrentAssistantContent().trim()) {
        updateStreamingMessage(message);
      }
      const interruptedId = backendAssistantId || currentAssistantNodeId;
      patchCurrentSession({ lastInterruptedMessageId: interruptedId });
      finalizeStreaming(backendAssistantId, currentTurnId);
      applyTurnPatch(event.turnPatch);
      cleanup();
      return;
    }

    const backendAssistantId = event.assistantMessageId && event.assistantMessageId !== '' ? event.assistantMessageId : null;
    finalizeStreaming(backendAssistantId, event.turnId || currentTurnId);
    applyTurnPatch(event.turnPatch);
    patchCurrentSession({ lastInterruptedMessageId: null });

    if (event.hadToolCalls) {
      // O aviso genérico existe só para o turno que termina em silêncio. Se
      // houve texto — inclusive só em segmentos intermediários — o chat:speak
      // já o verbalizou e o aviso viraria ruído em cima da fala.
      if (!turnHadAssistantText) {
        announceForActiveChatConversation(
          conversationId,
          i18next.t('chat.progressLabel'),
          'polite',
          getEventOrigin(event),
        );
      }
    }

    announceChatBackgroundResponseDone(conversationId, getCurrentSession().conversation?.title, getEventOrigin(event));
    cleanup();
  });

  activeControllers.set(conversationIdStr, cleanup);

  return {
    cleanup,
    done,
    handleSendCancellation: () => {
      if (cleanupExecuted) return;
      cleanup();
      adapter.setConversationLoading(conversationId, false, origin?.sessionKey);
      patchCurrentSession({
        isLoading: false,
        streamingMessageId: null,
      });
    },
    handleSendFailure: (message: string) => {
      if (cleanupExecuted) return;
      logger.error('[Chat] Error sending message:', message);
      playChatErrorSoundIfActive(conversationId, origin);
      const sendFailureMessage = i18next.t('chat.sendErrorPrefix', { message });
      cleanup();
      adapter.setConversationLoading(conversationId, false, origin?.sessionKey);
      patchCurrentSession({
        isLoading: false,
        streamingMessageId: null,
        sendFailureMessage,
        sendFailureAnnounced: false,
        sendFailureRetryable: true,
        sendFailureRetryContent: initialUserContent || null,
        sendFailureRetryMediaFiles: initialMediaFiles ?? [],
      });
    },
  };
}
