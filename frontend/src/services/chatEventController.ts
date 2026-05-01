import { GetMessages } from '@wailsjs/go/app/App';
import { EventsOn } from '@wailsjs/runtime/runtime';
import i18next from 'i18next';
import { main } from '../../wailsjs/go/models';
import type { ToolCallStatus } from '../components/chat/ToolCallsSection';
import { announce } from '../hooks/useAnnouncer';
import {
  finalizeStreamingNode,
  flattenThreadedMessages,
  hasMessageId,
  withOriginalIndex,
  type ChatTreeConversation,
  type Message,
  type MessageNode,
  type TurnSegment,
} from '../lib/chatMessageTree';
import { stripMarkdown } from '../lib/stripMarkdown';
import {
  announceChatBackgroundResponseDone,
  announceForActiveChatConversation,
  playChatReceiveSoundIfActive,
} from './chatArbitration';
import { handleChatSpeak, type ChatSpeakEvent } from './chatSpeak';

const STREAM_UPDATE_DEBOUNCE_MS = 16;

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
}

interface ChatToolEndEvent {
  conversationId: string;
  callId: string;
  name?: string;
  status?: string;
  summary?: string;
  attempt?: number;
}

interface ChatToolFailureEvent {
  conversationId: string;
  name: string;
  callId: string;
  willRetry?: boolean;
}

interface ChatSegmentDoneEvent {
  conversationId: string;
  hasMore?: boolean;
  content?: string;
}

interface ChatDoneEvent {
  conversationId: string;
  hadToolCalls?: boolean;
  errorMessage?: string;
}

interface ChatErrorEvent {
  conversationId: string;
  error: string;
}

export interface ChatEventSession {
  conversation: ChatTreeConversation | null;
  activeToolCalls: ToolCallStatus[];
  completedSegments: TurnSegment[];
}

export interface ChatEventControllerAdapter {
  getSession: (conversationId: string) => ChatEventSession;
  patchSession: (conversationId: string, patch: Partial<ChatEventSession> & Record<string, unknown>) => void;
  patchConversation: (
    conversationId: string,
    updater: (conversation: ChatTreeConversation) => ChatTreeConversation,
  ) => void;
  updateMessage: (conversationId: string, messageId: string, content: string) => void;
  updateReasoning: (conversationId: string, messageId: string, reasoning: string) => void;
  setConversationLoading: (conversationId: string, isLoading: boolean) => void;
}

interface ChatEventControllerOptions {
  conversationId: string;
  initialUserContent?: string;
  external?: {
    channel: string;
    from: string;
    text: string;
  };
  adapter: ChatEventControllerAdapter;
}

export interface ChatEventControllerHandle {
  cleanup: () => void;
  handleSendFailure: (message: string) => void;
}

const activeControllers = new Map<string, () => void>();
const streamUpdateTimers = new Map<string, NodeJS.Timeout>();
const pendingStreamUpdates = new Map<string, { messageId: string; content: string }>();
let streamingMessageSeq = 0;

const createStreamingMessageId = (conversationId: string): string => {
  streamingMessageSeq += 1;
  return `streaming-${conversationId}-${streamingMessageSeq}`;
};

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
  external,
  adapter,
}: ChatEventControllerOptions): ChatEventControllerHandle {
  const conversationIdStr = conversationId.toString();
  const streamingMsgId = createStreamingMessageId(conversationId);
  let cleanupExecuted = false;
  let streamingAnnounced = false;
  let assistantNodeCreated = false;

  adapter.setConversationLoading(conversationId, true);
  adapter.patchSession(conversationId, { completedSegments: [], activeToolCalls: [] });

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
    }) as Message;
    const assistantNode = new main.MessageNode({ message: assistantMsg, children: [], level: 0, childCount: 0 });
    const session = adapter.getSession(conversationId);
    if (!session.conversation) return;
    adapter.patchSession(conversationId, {
      conversation: {
        ...session.conversation,
        threadedMessages: [...session.conversation.threadedMessages, assistantNode as MessageNode],
      },
      streamingMessageId: streamingMsgId,
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
    discardPendingUpdate(streamingMsgId);
    activeControllers.delete(conversationIdStr);
    adapter.setConversationLoading(conversationId, false);
    adapter.patchSession(conversationId, {
      streamingMessageId: null,
      streamingReasoning: null,
      isThinking: false,
      activeToolCalls: [],
      completedSegments: [],
    });
  };

  const finalizeStreaming = (finalId?: string | null) => {
    adapter.patchConversation(
      conversationId,
      (conversation) => finalizeStreamingNode(conversation, streamingMsgId, finalId),
    );
  };

  const updateStreamingMessage = (content: string) => {
    adapter.updateMessage(conversationId, streamingMsgId, content);
  };

  const flushStreamingUpdate = () => {
    flushPendingUpdate(streamingMsgId, (_messageId, nextContent) => updateStreamingMessage(nextContent));
  };

  const existingCleanup = activeControllers.get(conversationIdStr);
  if (existingCleanup) existingCleanup();

  unsubError = EventsOn('chat:error', (event: ChatErrorEvent) => {
    if (event.conversationId !== conversationId && event.conversationId !== '') return;
    if (!isActive()) return;
    finalizeStreaming();
    announce(event.error);
    cleanup();
  });

  unsubSpeak = EventsOn('chat:speak', (event: ChatSpeakEvent) => {
    if (event.conversationId !== conversationId) return;
    if (!isActive()) return;
    void handleChatSpeak(event).catch((err) => {
      announce(i18next.t('chat.autoReadError'));
      console.error('[chat:speak] falha ao processar evento TTS', err);
    });
  });

  unsubMessagesReady = EventsOn('chat:messages_ready', (event: ChatMessagesReadyEvent) => {
    if (event.conversationId !== conversationId) return;
    if (!isActive()) return;
    if (!event.userMessageId) return;
    if (hasMessageId(adapter.getSession(conversationId).conversation?.threadedMessages, String(event.userMessageId))) return;
    const userMsg = new main.EnrichedMessage({
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
    const userNode = new main.MessageNode({ message: userMsg, children: [], level: 0, childCount: 0 });
    const session = adapter.getSession(conversationId);
    if (!session.conversation) return;
    adapter.patchSession(conversationId, {
      conversation: {
        ...session.conversation,
        threadedMessages: [...session.conversation.threadedMessages, userNode as MessageNode],
      },
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
      ensureAssistantNode();
      if (!streamingAnnounced) {
        streamingAnnounced = true;
        announceForActiveChatConversation(conversationId, i18next.t('chat.announce.assistantResponding'), 'polite');
      }
      debouncedUpdateMessage(streamingMsgId, event.content, (_messageId, nextContent) => updateStreamingMessage(nextContent));
    }

    if (event.error) {
      ensureAssistantNode();
      flushStreamingUpdate();
      updateStreamingMessage(i18next.t('chat.errorPrefix', { message: event.error }));
      finalizeStreaming();
      cleanup();
    }

    if (event.done) {
      ensureAssistantNode();
      flushStreamingUpdate();
      if (event.content) updateStreamingMessage(event.content);
      const backendAssistantId = event.messageId && event.messageId !== '' ? event.messageId : null;
      finalizeStreaming(backendAssistantId);

      const flatMessages = flattenThreadedMessages(adapter.getSession(conversationId).conversation?.threadedMessages);
      const finalMessage = flatMessages.find(m => m.id === (backendAssistantId || streamingMsgId));
      if (finalMessage?.content) playChatReceiveSoundIfActive(conversationId);
    }
  });

  unsubThinking = EventsOn('chat:thinking', (event: ChatThinkingEvent) => {
    if (event.conversationId !== conversationId) return;
    if (!isActive()) return;
    ensureAssistantNode();
    if (event.started) {
      adapter.patchSession(conversationId, {
        isThinking: true,
        streamingReasoning: event.content || '',
      });
      announceForActiveChatConversation(conversationId, i18next.t('chat.announce.modelThinking'), 'polite');
    } else if (event.done) {
      adapter.patchSession(conversationId, { isThinking: false });
      if (event.content) adapter.updateReasoning(conversationId, streamingMsgId, event.content);
    } else {
      adapter.patchSession(conversationId, { streamingReasoning: event.content || '' });
    }
  });

  unsubToolStart = EventsOn('chat:tool_start', (event: ChatToolStartEvent) => {
    if (event.conversationId !== conversationId) return;
    if (!isActive()) return;
    ensureAssistantNode();
    const session = adapter.getSession(conversationId);
    const existing = session.activeToolCalls.findIndex((tc) => tc.callId === event.callId);
    adapter.patchSession(conversationId, {
      activeToolCalls: existing >= 0
        ? session.activeToolCalls.map((tc) =>
          tc.callId === event.callId
            ? { ...tc, name: event.name, callId: event.callId, args: event.args ?? tc.args, status: 'running' as const, summary: undefined }
            : tc
        )
        : [...session.activeToolCalls, { name: event.name, callId: event.callId, args: event.args, status: 'running' as const }],
    });
    if (external) {
      announceForActiveChatConversation(conversationId, i18next.t('chat.toolRunning', { name: event.name }), 'polite');
    }
  });

  unsubToolEnd = EventsOn('chat:tool_end', (event: ChatToolEndEvent) => {
    if (event.conversationId !== conversationId) return;
    if (!isActive()) return;
    const session = adapter.getSession(conversationId);
    adapter.patchSession(conversationId, {
      activeToolCalls: session.activeToolCalls.map((tc) =>
        tc.callId === event.callId
          ? { ...tc, status: (event.status === 'error' ? 'error' : 'done') as 'done' | 'error', summary: event.summary }
          : tc
      ),
    });
    if (!external) return;
    if (event.status !== 'error') {
      announceForActiveChatConversation(conversationId, i18next.t('chat.toolDone', { name: event.name }), 'polite');
      return;
    }
    if (!('attempt' in event)) {
      announce(i18next.t('chat.toolFailed', { name: event.name }), 'assertive');
    }
  });

  unsubToolFailure = EventsOn('chat:tool_failure', (event: ChatToolFailureEvent) => {
    if (event.conversationId !== conversationId) return;
    if (!isActive()) return;
    if (event.willRetry) {
      announceForActiveChatConversation(conversationId, i18next.t('chat.toolRetrying', { name: event.name }), 'polite');
      return;
    }
    announce(i18next.t('chat.toolFailed', { name: event.name }), 'assertive');
  });

  unsubSegmentDone = EventsOn('chat:segment_done', (event: ChatSegmentDoneEvent) => {
    if (event.conversationId !== conversationId) return;
    if (!isActive()) return;
    if (!event.hasMore) return;

    const session = adapter.getSession(conversationId);
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
        );
      }
    }
    if (event.content) {
      newSegments.push({ type: 'text', content: event.content });
    }
    adapter.patchSession(conversationId, {
      completedSegments: newSegments,
      activeToolCalls: [],
    });
    flushStreamingUpdate();
    updateStreamingMessage('');
  });

  unsubDone = EventsOn('chat:done', (event: ChatDoneEvent) => {
    if (event.conversationId !== conversationId) return;
    if (!isActive()) return;

    if (event.errorMessage) {
      ensureAssistantNode();
      flushStreamingUpdate();
      updateStreamingMessage(i18next.t('chat.errorPrefix', { message: event.errorMessage }));
      finalizeStreaming();
      cleanup();
      return;
    }

    finalizeStreaming();

    if (event.hadToolCalls) {
      GetMessages(conversationId, null).then((backendNodes) => {
        const conversation = adapter.getSession(conversationId).conversation;
        if (!conversation) return;
        adapter.patchSession(conversationId, {
          conversation: {
            ...conversation,
            threadedMessages: backendNodes.map(withOriginalIndex),
          },
          completedSegments: [],
        });
      }).catch((err) => {
        console.error('[Chat] Erro ao recarregar mensagens:', err);
      });
    }

    announceChatBackgroundResponseDone(conversationId, adapter.getSession(conversationId).conversation?.title);
    cleanup();
  });

  activeControllers.set(conversationIdStr, cleanup);

  return {
    cleanup,
    handleSendFailure: (message: string) => {
      if (cleanupExecuted) return;
      console.error('[Chat] Error sending message:', message);
      cleanup();
      ensureAssistantNode();
      updateStreamingMessage(i18next.t('chat.sendErrorPrefix', { message }));
      finalizeStreaming();
      adapter.setConversationLoading(conversationId, false);
      adapter.patchSession(conversationId, { streamingMessageId: null });
    },
  };
}
