import { logger } from '../utils/logger';
import { EventsOn } from '@wailsjs/runtime/runtime';
import i18next from 'i18next';
import { chat } from '../../wailsjs/go/models';
import type { ToolCallStatus } from '../types/chat';
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
import { reloadConversationSnapshot } from './chatSessionLoader';
import { INITIAL_MESSAGE_WINDOW_SIZE } from './messageWindowLimits';
import type { ChatSurfaceOrigin, MessageWindowState } from './chatSessionRegistry';

const STREAM_UPDATE_DEBOUNCE_MS = 16;

const translateBackendChatError = (message: string) => {
  if (message === 'assistant_placeholder_error') {
    return i18next.t('chat.errors.assistantPlaceholder');
  }
  if (message === 'internal_error') {
    return i18next.t('chat.errors.internalError');
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
  content?: string;
  done?: boolean;
  error?: string;
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
  attempt?: number;
  turnId?: string;
  surfaceOrigin?: ChatSurfaceOrigin;
}

interface ChatToolFailureEvent {
  conversationId: string;
  assistantMessageId?: string;
  name: string;
  callId: string;
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
  errorMessage?: string;
  surfaceOrigin?: ChatSurfaceOrigin;
}

interface ChatErrorEvent {
  conversationId: string;
  error: string;
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
  handleSendFailure: (message: string) => void;
  done: Promise<void>;
}

const activeControllers = new Map<string, () => void>();
const streamUpdateTimers = new Map<string, NodeJS.Timeout>();
const pendingStreamUpdates = new Map<string, { messageId: string; content: string }>();

const debouncedUpdateMessage = (
  messageId: string,
  content: string,
  updateFn: (messageId: string, content: string) => void,
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
  updateFn: (messageId: string, content: string) => void,
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

const discardPendingUpdate = (messageId: string) => {
  const existingTimer = streamUpdateTimers.get(messageId);
  if (existingTimer) {
    clearTimeout(existingTimer);
    streamUpdateTimers.delete(messageId);
  }
  pendingStreamUpdates.delete(messageId);
};

export function stopChatEventController(conversationId: string) {
  const cleanup = activeControllers.get(conversationId.toString());
  if (cleanup) cleanup();
}

export function stopAllChatEventControllers() {
  activeControllers.forEach((cleanup) => cleanup());
  activeControllers.clear();
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
    if (currentAssistantNodeId) {
      discardPendingUpdate(currentAssistantNodeId);
    }
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
    const assistantNodeId = currentAssistantNodeId;
    adapter.patchConversation(
      conversationId,
      (conversation) => finalizeStreamingNode(conversation, assistantNodeId, finalId, finalTurnId),
    );
  };

  const updateStreamingMessage = (content: string) => {
    if (!currentAssistantNodeId) return;
    adapter.updateMessage(conversationId, currentAssistantNodeId, content);
  };

  const getCurrentAssistantContent = () => {
    if (!currentAssistantNodeId) return '';
    const messages = flattenThreadedMessages(getCurrentSession().conversation?.threadedMessages);
    return String(messages.find(m => m.id === currentAssistantNodeId)?.content || '');
  };

  const updateEmptyAssistantWithError = (message: string) => {
    if (getCurrentAssistantContent().trim()) return;
    updateStreamingMessage(i18next.t('chat.errorPrefix', { message }));
  };

  const flushStreamingUpdate = () => {
    if (!currentAssistantNodeId) return;
    flushPendingUpdate(currentAssistantNodeId, (_messageId, nextContent) => updateStreamingMessage(nextContent));
  };

  const existingCleanup = activeControllers.get(conversationIdStr);
  if (existingCleanup) existingCleanup();

  unsubError = EventsOn('chat:error', (event: ChatErrorEvent) => {
    if (event.conversationId !== conversationId && event.conversationId !== '') return;
    if (!isActive()) return;
    finalizeStreaming();
    announce(event.error);
    playChatErrorSoundIfActive(conversationId, origin);
    cleanup();
  });

  unsubSpeak = EventsOn('chat:speak', (event: ChatSpeakEvent) => {
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

  unsubMessagesReady = EventsOn('chat:messages_ready', (event: ChatMessagesReadyEvent) => {
    if (event.conversationId !== conversationId) return;
    if (!isActive()) return;
    if (!event.userMessageId) return;
    currentTurnId = event.turnId || event.userMessageId.toString();
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

  unsubStream = EventsOn('chat:stream', (event: ChatStreamEvent) => {
    if (event.conversationId !== conversationId) return;
    if (!isActive()) return;

    if (event.content && !event.done && !event.error) {
      currentTurnId = event.turnId || currentTurnId;
      if (event.content.trim()) turnHadAssistantText = true;
      const backendAssistantId = event.messageId && event.messageId !== '' ? event.messageId : null;
      if (!ensureAssistantNode(backendAssistantId) && !currentAssistantNodeId) return;
      const assistantNodeId = currentAssistantNodeId;
      if (!assistantNodeId) return;
      if (!streamingAnnounced) {
        streamingAnnounced = true;
        announceForActiveChatConversation(conversationId, i18next.t('chat.announce.assistantResponding'), 'polite', getEventOrigin(event));
      }
      debouncedUpdateMessage(assistantNodeId, event.content, (_messageId, nextContent) => updateStreamingMessage(nextContent));
    }

    if (event.error) {
      currentTurnId = event.turnId || currentTurnId;
      const backendAssistantId = event.messageId && event.messageId !== '' ? event.messageId : null;
      const hasAssistantNode = ensureAssistantNode(backendAssistantId) || currentAssistantNodeId !== null;
      flushStreamingUpdate();
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
        updateEmptyAssistantWithError(errorMessage);
      } else {
        patchCurrentSession({ sendFailureMessage: errorMessage, sendFailureAnnounced: true, sendFailureRetryable: false });
      }
      const interruptedId = backendAssistantId || currentAssistantNodeId;
      patchCurrentSession({ lastInterruptedMessageId: interruptedId });
      finalizeStreaming();
      cleanup();
    }

    if (event.done) {
      currentTurnId = event.turnId || currentTurnId;
      const backendAssistantId = event.messageId && event.messageId !== '' ? event.messageId : null;
      ensureAssistantNode(backendAssistantId);
      flushStreamingUpdate();
      if (event.content) {
        if (event.content.trim()) turnHadAssistantText = true;
        updateStreamingMessage(event.content);
      }
      finalizeStreaming(backendAssistantId, event.turnId || currentTurnId);

      const flatMessages = flattenThreadedMessages(getCurrentSession().conversation?.threadedMessages);
      const finalMessage = flatMessages.find(m => m.id === (backendAssistantId || currentAssistantNodeId));
      if (finalMessage?.content) playChatReceiveSoundIfActive(conversationId, getEventOrigin(event));
    }
  });

  unsubThinking = EventsOn('chat:thinking', (event: ChatThinkingEvent) => {
    if (event.conversationId !== conversationId) return;
    if (!isActive()) return;
    currentTurnId = event.turnId || currentTurnId;
    ensureAssistantNode(event.assistantMessageId);
    if (event.started) {
      patchCurrentSession({
        isThinking: true,
        streamingReasoning: event.content || '',
      });
      announceForActiveChatConversation(conversationId, i18next.t('chat.announce.modelThinking'), 'polite', getEventOrigin(event));
    } else if (event.done) {
      patchCurrentSession({ isThinking: false });
      if (event.content && currentAssistantNodeId) adapter.updateReasoning(conversationId, currentAssistantNodeId, event.content);
    } else {
      patchCurrentSession({ streamingReasoning: event.content || '' });
    }
  });

  unsubToolStart = EventsOn('chat:tool_start', (event: ChatToolStartEvent) => {
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
            ? { ...tc, name: event.name, callId: event.callId, args: event.args ?? tc.args, status: 'running' as const, summary: undefined }
            : tc
        )
        : [...session.activeToolCalls, { name: event.name, callId: event.callId, args: event.args, status: 'running' as const }],
    });
    if (external) {
      announceForActiveChatConversation(conversationId, i18next.t('chat.toolRunning', { name: event.name }), 'polite', getEventOrigin(event));
    }
  });

  unsubToolEnd = EventsOn('chat:tool_end', (event: ChatToolEndEvent) => {
    if (event.conversationId !== conversationId) return;
    if (!isActive()) return;
    currentTurnId = event.turnId || currentTurnId;
    ensureAssistantNode(event.assistantMessageId);
    const session = getCurrentSession();
    patchCurrentSession({
      activeToolCalls: session.activeToolCalls.map((tc) =>
        tc.callId === event.callId
          ? { ...tc, status: (event.status === 'error' ? 'error' : 'done') as 'done' | 'error', summary: event.summary }
          : tc
      ),
    });
    if (!external) return;
    if (event.status !== 'error') {
      announceForActiveChatConversation(conversationId, i18next.t('chat.toolDone', { name: event.name }), 'polite', getEventOrigin(event));
      return;
    }
    if (!('attempt' in event)) {
      announce(i18next.t('chat.toolFailed', { name: event.name }), 'assertive');
    }
  });

  unsubToolFailure = EventsOn('chat:tool_failure', (event: ChatToolFailureEvent) => {
    if (event.conversationId !== conversationId) return;
    if (!isActive()) return;
    currentTurnId = event.turnId || currentTurnId;
    ensureAssistantNode(event.assistantMessageId);
    if (event.willRetry) {
      announceForActiveChatConversation(conversationId, i18next.t('chat.toolRetrying', { name: event.name }), 'polite', getEventOrigin(event));
      return;
    }
    announce(i18next.t('chat.toolFailed', { name: event.name }), 'assertive');
    playChatErrorSoundIfActive(conversationId, getEventOrigin(event));
  });

  unsubSegmentDone = EventsOn('chat:segment_done', (event: ChatSegmentDoneEvent) => {
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
    flushStreamingUpdate();
    if (currentAssistantNodeId) updateStreamingMessage('');
  });

  unsubDone = EventsOn('chat:done', (event: ChatDoneEvent) => {
    if (event.conversationId !== conversationId) return;
    if (!isActive()) return;
    currentTurnId = event.turnId || currentTurnId;

    if (event.errorMessage) {
      const backendAssistantId = event.assistantMessageId && event.assistantMessageId !== '' ? event.assistantMessageId : null;
      const hasAssistantNode = ensureAssistantNode(backendAssistantId) || currentAssistantNodeId !== null;
      flushStreamingUpdate();
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
        updateEmptyAssistantWithError(errorMessage);
      } else {
        patchCurrentSession({ sendFailureMessage: errorMessage, sendFailureAnnounced: true, sendFailureRetryable: false });
      }
      const interruptedId = backendAssistantId || currentAssistantNodeId;
      patchCurrentSession({ lastInterruptedMessageId: interruptedId });
      finalizeStreaming(backendAssistantId, currentTurnId);
      cleanup();
      return;
    }

    const backendAssistantId = event.assistantMessageId && event.assistantMessageId !== '' ? event.assistantMessageId : null;
    finalizeStreaming(backendAssistantId, event.turnId || currentTurnId);
    patchCurrentSession({ lastInterruptedMessageId: null });

    if (event.hadToolCalls) {
      if (!turnHadAssistantText) {
        announceForActiveChatConversation(
          conversationId,
          i18next.t('chat.progressLabel'),
          'polite',
          getEventOrigin(event),
        );
      }
      reloadConversationSnapshot(conversationId, INITIAL_MESSAGE_WINDOW_SIZE).then((snapshot) => {
        const conversation = getCurrentSession().conversation;
        if (!conversation) return;
        if (snapshot.threadedMessages.length === 0) {
          patchCurrentSession({ completedSegments: [] });
          return;
        }
        const currentSession = getCurrentSession();
        const isSurfaceAtLiveTail = !currentSession.messageWindow?.hasAfter
          || (
            currentSession.messageWindow.totalCount > 0
            && currentSession.messageWindow.endIndex >= currentSession.messageWindow.totalCount - 1
          );
        if (
          currentSession.messageWindow
          && !isSurfaceAtLiveTail
          && currentSession.messageWindow.totalCount > snapshot.messageWindow.totalCount
          && import.meta.env.DEV
        ) {
          logger.warn('[Chat] snapshot retornou totalCount menor que a janela paginada atual', {
            conversationId,
            currentTotalCount: currentSession.messageWindow.totalCount,
            snapshotTotalCount: snapshot.messageWindow.totalCount,
          });
        }
        const pagedWindowUpdate = currentSession.messageWindow && !isSurfaceAtLiveTail
          ? {
            messageWindow: {
              ...currentSession.messageWindow,
              totalCount: Math.max(currentSession.messageWindow.totalCount, snapshot.messageWindow.totalCount),
              hasAfter: snapshot.messageWindow.totalCount > currentSession.messageWindow.endIndex + 1,
            },
          }
          : {};
        patchCurrentSession({
          conversation: {
            ...conversation,
            threadedMessages: snapshot.threadedMessages,
          },
          ...(isSurfaceAtLiveTail
            ? {
              visibleThreadedMessages: snapshot.threadedMessages,
              messageWindow: snapshot.messageWindow,
              hasOlderMessages: snapshot.hasOlderMessages,
            }
            : pagedWindowUpdate),
          completedSegments: [],
        });
      }).catch((err) => {
        logger.error('[Chat] Erro ao recarregar mensagens:', err);
      });
    }

    announceChatBackgroundResponseDone(conversationId, getCurrentSession().conversation?.title, getEventOrigin(event));
    cleanup();
  });

  activeControllers.set(conversationIdStr, cleanup);

  return {
    cleanup,
    done,
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
