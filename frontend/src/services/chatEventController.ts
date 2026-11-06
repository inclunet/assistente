import { chatGPTErrorKey } from '../lib/chatgptErrors';
import { logger } from '../utils/logger';
import i18next from 'i18next';
import { type ToolCallStatus, type ToolOrigin } from '../types/chat';
import type { MediaFile } from './mediaService';
import { announce } from '../hooks/useAnnouncer';
import { stripMarkdown } from '../lib/stripMarkdown';
import {
  announceChatBackgroundResponseDone,
  announceForActiveChatConversation,
  getChatConversationVoiceOrigin,
  playChatReceiveSoundIfActive,
  playChatErrorSoundIfActive,
} from './chatArbitration';
import { announceWithOrigin } from './voiceAccessibility/announcerBroker';
import { formatToolPresentation, presentTool } from '../lib/toolPresentation';
import { handleChatSpeak, type ChatSpeakEvent } from './chatSpeak';
import { createChatProgressAnnouncer } from './chatProgressAnnouncer';
import type { ChatSurfaceOrigin } from './chatSessionRegistry';
import { clearChatTurnRoutes, createChatTurnEventRouter } from './chatEventHub';
import { translateErrorMarkers } from '../lib/providerErrorMessage';

const translateBackendChatError = (message: string) => {
  const translatedError = translateErrorMarkers(message);
  if (translatedError !== message) return translatedError;

  const chatGPTKey = chatGPTErrorKey(message);
  if (chatGPTKey) return i18next.t(chatGPTKey);

  if (message === 'assistant_placeholder_error') {
    return i18next.t('chat.errors.assistantPlaceholder');
  }
  if (message === 'mcp_preparation_error') {
    return i18next.t('chat.errors.mcpPreparation');
  }
  if (message === 'persistence_error') {
    return i18next.t('chat.errors.persistenceError');
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
  if (message === 'streaming_retries_exhausted') {
    return i18next.t('chat.errors.streamingRetriesExhausted');
  }
  if (message === 'streaming_prompt_cache_hint_rejected') {
    return i18next.t('chat.errors.streamingPromptCacheHintRejected');
  }
  if (message === 'streaming_token_rate_limit') {
    return i18next.t('chat.errors.streamingTokenRateLimit');
  }
  return message;
};

interface ChatMessagesReadyEvent {
  itemId: string;
  conversationId: string;
  userMessageId: string;
  userContent: string;
  turnId?: string;
  surfaceOrigin?: ChatSurfaceOrigin;
}

interface ChatStreamEvent {
  itemId?: string;
  conversationId: string;
  delta?: string;
  diagnostic?: boolean;
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
  serverLabel?: string;
  turnId?: string;
  surfaceOrigin?: ChatSurfaceOrigin;
}

interface ChatToolEndEvent {
  errorCode?: string;
  conversationId: string;
  assistantMessageId?: string;
  callId: string;
  name?: string;
  status?: string;
  errorKind?: string;
  summary?: string;
  origin?: ToolOrigin;
  serverLabel?: string;
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
  errorKind?: string;
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
  itemId?: string;
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

function terminalToolStatus(event: ChatToolEndEvent): 'done' | 'error' | 'cancelled' {
  const status = event.status?.toLowerCase();
  if (event.errorKind === 'cancelled' || status === 'cancelled' || status === 'canceled') return 'cancelled';
  if (status === 'error' || status === 'failed') return 'error';
  if (status === 'ok' || status === 'done' || status === 'completed' || status === 'succeeded' || status === 'success') return 'done';
  // Um tool_end é terminal. Status ausente ou desconhecido não pode promover
  // uma execução a sucesso; o snapshot canônico corrigirá o estado ao final.
  return 'error';
}

export interface ChatEventSession {
  conversation: { id: string; title: string } | null;
  sendFailureMessage?: string | null;
  sendFailureAnnounced?: boolean;
  sendFailureRetryable?: boolean;
  sendFailureRetryContent?: string | null;
  sendFailureRetryMediaFiles?: MediaFile[];
  surfaceOrigin?: ChatSurfaceOrigin;
}

export interface ChatEventControllerAdapter {
  getSession: (conversationId: string, sessionKey?: string) => ChatEventSession;
  patchSession: (conversationId: string, patch: Partial<Omit<ChatEventSession, 'conversation'>> & Record<string, unknown>) => void;
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
  handleSendFailure: (message: string, provenNotSubmitted?: boolean) => void;
  done: Promise<void>;
}

const activeControllers = new Map<string, { cleanup: () => void; executionId?: string }>();

export function getChatEventControllerCleanup(conversationId: string) {
  return activeControllers.get(conversationId.toString())?.cleanup;
}

export function getChatEventControllerExecutionId(conversationId: string) {
  return activeControllers.get(conversationId.toString())?.executionId;
}

export function stopChatEventController(conversationId: string) {
  const cleanup = activeControllers.get(conversationId.toString())?.cleanup;
  if (cleanup) cleanup();
}

export function stopAllChatEventControllers() {
  activeControllers.forEach(({ cleanup }) => cleanup());
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
  let cleanupExecuted = false;
  let streamingAnnounced = false;
  let turnHadAssistantText = false;
  let streamSequence = -1;
  let streamInitialized = false;
  let assistantMessageId: string | null = null;
  let currentTurnId: string | null = null;
  // Identidade necessária aos anúncios; conteúdo visual pertence à timeline.
  const announcementTools = new Map<string, ToolCallStatus>();
  const announcedCancellationCallIds = new Set<string>();
  let resolveDone: () => void = () => {};
  const done = new Promise<void>((resolve) => {
    resolveDone = resolve;
  });
  const getCurrentSession = () => adapter.getSession(conversationId, origin?.sessionKey);
  const patchCurrentSession = (patch: Parameters<ChatEventControllerAdapter['patchSession']>[1]) => {
    adapter.patchSession(conversationId, origin ? { ...patch, surfaceOrigin: origin } : patch);
  };

  adapter.setConversationLoading(conversationId, true, origin?.sessionKey);
  patchCurrentSession({
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

  const isActive = () => activeControllers.get(conversationIdStr)?.cleanup === cleanup;
  const getEventOrigin = (event: { surfaceOrigin?: ChatSurfaceOrigin }) => event.surfaceOrigin ?? origin;
  const progressAnnouncer = createChatProgressAnnouncer({
    announce: (groups) => {
      const messages = groups.flatMap(({ state, tools }) => tools.map((tool) => {
        const presentation = presentTool(tool.name, tool.origin, tool.serverLabel, tool.args);
        const action = formatToolPresentation(presentation, (key, values) => i18next.t(key, values), state === 'done');
        return `${action}. ${i18next.t(state === 'done' ? 'chat.toolStatusSucceeded' : 'chat.toolStatusRunning')}`;
      }));
      if (messages.length === 0) return;
      const eventOrigin = groups.flatMap((group) => group.tools).find((tool) => tool.surfaceOrigin)?.surfaceOrigin ?? origin;
      announceForActiveChatConversation(
        conversationId,
        messages.join('; '),
        'polite',
        eventOrigin,
      );
    },
  });

  const cleanup = () => {
    if (cleanupExecuted) return;
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
    progressAnnouncer.dispose();
    announcementTools.clear();
    turnEvents.unregister();
    activeControllers.delete(conversationIdStr);
    adapter.setConversationLoading(conversationId, false, origin?.sessionKey);
    patchCurrentSession({
      isLoading: false,
    });
    resolveDone();
  };

  const reportError = (message: string, eventOrigin?: ChatSurfaceOrigin) => {
    const translated = translateBackendChatError(message.trim());
    announceWithOrigin({ message: translated,
      origin: getChatConversationVoiceOrigin(conversationId, undefined, eventOrigin),
      eventType: 'error', announcePriority: 'assertive' });
    playChatErrorSoundIfActive(conversationId, eventOrigin);
    if (!assistantMessageId) patchCurrentSession({ sendFailureMessage: translated, sendFailureAnnounced: true, sendFailureRetryable: false });
  };

  const existingCleanup = activeControllers.get(conversationIdStr)?.cleanup;
  if (existingCleanup) existingCleanup();

  const turnEvents = createChatTurnEventRouter(
    conversationIdStr,
    () => currentTurnId,
    (turnId) => { currentTurnId = turnId; },
    origin?.executionId,
  );

  unsubError = turnEvents.on('chat:error', (event: ChatErrorEvent) => {
    if (event.conversationId !== conversationId && event.conversationId !== '') return;
    if (!isActive()) return;
    announce(translateBackendChatError(event.error));
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
    assistantMessageId = null;
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
      announceWithOrigin({
        message: i18next.t('chat.mediaProcessing.failed'),
        origin: getChatConversationVoiceOrigin(conversationId, undefined, getEventOrigin(event)),
        eventType: 'error',
        announcePriority: 'assertive',
      });
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
    if (event.conversationId !== conversationId || !isActive()) return;
    assistantMessageId = event.messageId || assistantMessageId;
    if (event.diagnostic) return;
    if (event.delta && !event.done && !event.error) {
      if (!Number.isSafeInteger(event.sequence) || (event.reset
        ? event.sequence !== 0 : !streamInitialized || event.sequence !== streamSequence + 1)) return;
      streamInitialized = true;
      streamSequence = event.sequence;
      if (event.delta.trim() || event.baseContent?.trim()) turnHadAssistantText = true;
      if (!streamingAnnounced) {
        streamingAnnounced = true;
        announceForActiveChatConversation(conversationId, i18next.t('chat.announce.assistantResponding'), 'polite', getEventOrigin(event));
      }
    }
    if (event.error) {
      reportError(event.error, getEventOrigin(event));
      cleanup();
    } else if (event.done && turnHadAssistantText) {
      playChatReceiveSoundIfActive(conversationId, getEventOrigin(event));
    }
  });

  unsubThinking = turnEvents.on('chat:thinking', (event: ChatThinkingEvent) => {
    if (event.conversationId !== conversationId || !isActive()) return;
    assistantMessageId = event.assistantMessageId || assistantMessageId;
    if (event.started) announceForActiveChatConversation(conversationId, i18next.t('chat.announce.modelThinking'), 'polite', getEventOrigin(event));
  });

  unsubToolStart = turnEvents.on('chat:tool_start', (event: ChatToolStartEvent) => {
    if (event.conversationId !== conversationId) return;
    if (!isActive()) return;
    assistantMessageId = event.assistantMessageId || assistantMessageId;
    announcementTools.set(event.callId, { name: event.name, callId: event.callId, args: event.args,
      status: 'running', summary: event.summary, origin: event.origin, serverLabel: event.serverLabel });
    if (!external) {
      progressAnnouncer.toolStarted({
        callId: event.callId,
        name: event.name,
        args: event.args,
        origin: event.origin,
        serverLabel: event.serverLabel,
        surfaceOrigin: getEventOrigin(event),
      });
    }
    if (external) {
      const runningMessage = `${formatToolPresentation(
        presentTool(event.name, event.origin, event.serverLabel, event.args),
        (key, values) => i18next.t(key, values),
      )}. ${i18next.t('chat.toolStatusRunning')}`;
      announceForActiveChatConversation(conversationId, runningMessage, 'polite', getEventOrigin(event));
    }
  });

  unsubToolEnd = turnEvents.on('chat:tool_end', (event: ChatToolEndEvent) => {
    if (event.conversationId !== conversationId) return;
    if (!isActive()) return;
    const terminalStatus = terminalToolStatus(event);
    const cancelled = terminalStatus === 'cancelled';
    const startedCall = announcementTools.get(event.callId);
    const finishedCall: ToolCallStatus = {
      ...startedCall, callId: event.callId, name: event.name ?? startedCall?.name ?? '',
      status: terminalStatus, origin: event.origin ?? startedCall?.origin,
      serverLabel: event.origin === 'builtin' ? undefined : event.serverLabel ?? startedCall?.serverLabel,
    };
    announcementTools.set(event.callId, finishedCall);
    // Falha/retry pode chegar depois de tool_end sem repetir a origem.
    // A identidade permanece até o fim do segmento para os anúncios.
    if (!external) progressAnnouncer.toolEnded(event.callId, terminalStatus === 'done' ? 'ok' : 'error', finishedCall);
    if (!external && !cancelled) return;
    if (cancelled) {
      const cancelledMessage = `${formatToolPresentation(
        presentTool(event.name ?? finishedCall?.name ?? '', event.origin ?? finishedCall?.origin, event.serverLabel ?? finishedCall?.serverLabel, finishedCall?.args),
        (key, values) => i18next.t(key, values),
      )}. ${i18next.t('chat.toolStatusCancelled')}`;
      announceForActiveChatConversation(conversationId, cancelledMessage, 'polite', getEventOrigin(event));
      announcedCancellationCallIds.add(event.callId);
      return;
    }
    if (terminalStatus === 'done') {
      const doneMessage = `${formatToolPresentation(
        presentTool(event.name ?? finishedCall?.name ?? '', event.origin ?? finishedCall?.origin, event.serverLabel ?? finishedCall?.serverLabel, finishedCall?.args),
        (key, values) => i18next.t(key, values),
        true,
      )}. ${i18next.t('chat.toolStatusSucceeded')}`;
      announceForActiveChatConversation(conversationId, doneMessage, 'polite', getEventOrigin(event));
      return;
    }
    if (!('attempt' in event)) {
      const failedCall = finishedCall;
      const failedLabel = formatToolPresentation(
        presentTool(event.name ?? failedCall?.name ?? '', event.origin ?? failedCall?.origin, event.serverLabel ?? failedCall?.serverLabel, failedCall?.args),
        (key, values) => i18next.t(key, values),
      );
      announceWithOrigin({
        message: `${failedLabel}. ${i18next.t('chat.toolStatusFailed')}`,
        origin: getChatConversationVoiceOrigin(conversationId, undefined, getEventOrigin(event)),
        eventType: 'error',
        announcePriority: 'assertive',
      });
    }
  });

  unsubToolFailure = turnEvents.on('chat:tool_failure', (event: ChatToolFailureEvent) => {
    if (event.conversationId !== conversationId) return;
    if (!isActive()) return;
    if (!external) progressAnnouncer.toolFailed(event.callId, event.willRetry);
    const failedCall = announcementTools.get(event.callId);
    if (event.errorKind === 'cancelled') {
      if (!announcedCancellationCallIds.has(event.callId)) {
        const cancelledLabel = formatToolPresentation(
          presentTool(event.name ?? failedCall?.name ?? '', event.origin ?? failedCall?.origin, failedCall?.serverLabel, failedCall?.args),
          (key, values) => i18next.t(key, values),
        );
        announceForActiveChatConversation(
          conversationId,
          `${cancelledLabel}. ${i18next.t('chat.toolStatusCancelled')}`,
          'polite',
          getEventOrigin(event),
        );
        announcedCancellationCallIds.add(event.callId);
      }
      return;
    }
    const failedLabel = formatToolPresentation(
      presentTool(event.name ?? failedCall?.name ?? '', event.origin ?? failedCall?.origin, failedCall?.serverLabel, failedCall?.args),
      (key, values) => i18next.t(key, values),
    );
    if (event.willRetry) {
      announceForActiveChatConversation(conversationId, `${failedLabel}. ${i18next.t('chat.toolStatusRetrying')}`, 'polite', getEventOrigin(event));
      return;
    }
    announceWithOrigin({
      message: `${failedLabel}. ${i18next.t('chat.toolStatusFailed')}`,
      origin: getChatConversationVoiceOrigin(conversationId, undefined, getEventOrigin(event)),
      eventType: 'error',
      announcePriority: 'assertive',
    });
    playChatErrorSoundIfActive(conversationId, getEventOrigin(event));
  });

  unsubSegmentDone = turnEvents.on('chat:segment_done', (event: ChatSegmentDoneEvent) => {
    if (event.conversationId !== conversationId || !isActive()) return;
    assistantMessageId = event.assistantMessageId || assistantMessageId;
    if (!external) progressAnnouncer.finishSegment();
    if (event.content?.trim()) turnHadAssistantText = true;
    if (event.hasMore) {
      announcementTools.clear();
      streamSequence = -1;
      streamInitialized = false;
    }
  });

  unsubDone = turnEvents.on('chat:done', (event: ChatDoneEvent) => {
    if (event.conversationId !== conversationId || !isActive()) return;
    if (!external) progressAnnouncer.finishSegment();
    assistantMessageId = event.assistantMessageId || assistantMessageId;
    const interrupted = !!event.errorMessage || event.reason === 'error' || event.reason === 'cancelled'
      || event.finishReason === 'cancelled' || event.reason === 'limit_reached' || event.reason === 'output_limit';
    if (event.errorMessage) reportError(event.errorMessage, getEventOrigin(event));
    else if (event.reason === 'output_limit') {
      announceWithOrigin({ message: i18next.t('chat.outputLimitReached'),
        origin: getChatConversationVoiceOrigin(conversationId, undefined, getEventOrigin(event)),
        eventType: 'system', announcePriority: 'assertive' });
    } else if (!interrupted) {
      if (event.hadToolCalls && !turnHadAssistantText) {
        announceForActiveChatConversation(conversationId, i18next.t('chat.progressLabel'), 'polite', getEventOrigin(event));
      }
      announceChatBackgroundResponseDone(conversationId, getCurrentSession().conversation?.title, getEventOrigin(event));
    }
    cleanup();
  });

  activeControllers.set(conversationIdStr, { cleanup, executionId: origin?.executionId });

  return {
    cleanup,
    done,
    handleSendCancellation: () => {
      if (cleanupExecuted) return;
      cleanup();
      adapter.setConversationLoading(conversationId, false, origin?.sessionKey);
      patchCurrentSession({
        isLoading: false,
      });
    },
    handleSendFailure: (message: string, provenNotSubmitted = false) => {
      if (cleanupExecuted) return;
      logger.error('[Chat] Error sending message:', message);
      playChatErrorSoundIfActive(conversationId, origin);
      const sendFailureMessage = i18next.t('chat.sendErrorPrefix', { message });
      cleanup();
      adapter.setConversationLoading(conversationId, false, origin?.sessionKey);
      patchCurrentSession({
        isLoading: false,
        sendFailureMessage,
        sendFailureAnnounced: false,
        sendFailureRetryable: provenNotSubmitted,
        sendFailureRetryContent: provenNotSubmitted ? initialUserContent || null : null,
        sendFailureRetryMediaFiles: provenNotSubmitted ? initialMediaFiles ?? [] : [],
      });
    },
  };
}
