import { useRef, useState, useEffect, useCallback, useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import { useLocation, useNavigate } from 'react-router-dom';
import { Alert, Button } from 'antd';
import { useChatStore, type Message } from '../../store/chatStore';
import { requestConfirm } from '../../store/confirmStore';
import { executeDeepLink } from '../../lib/deepLinks';
import { handleError, ErrorSeverity } from '../../utils/errorHandler';
import { useWorkspaceStore } from '../../store/workspaceStore';
import { ttsService } from '../../services/tts';
import { useChatMessageCommands, isChatMessageCommand } from './useChatMessageCommands';
import { MessageList, type MessageWindowLoadTrigger } from './MessageList';
import { ChatInput } from './ChatInput';
import { ChatToolbar, type ChatToolbarConversationChangeHandler } from './ChatToolbar';
import { useAgentSessionCommands } from './useAgentSessionCommands';
import { ChatSessionProvider } from './ChatSessionContext';
import type {
  ChatSurfaceIdentity,
  ChatSurfaceOrigin,
} from '../../services/chatSessionRegistry';
import { ContextMenu } from '../menu';
import { useShortcutsHelpStore } from '../../store/shortcutsHelpStore';
import { isModalOpen, useModalId, useModalIsTopmost } from '../ui/Modal';
import { useWorkspacePanel } from '../workspace/WorkspacePanelContext';
import { registerWorkspacePanelFocus } from '../workspace/workspacePanelFocusRegistry';
import { useChatKeyboardNav } from '../../hooks/useChatKeyboardNav';
import { useContextMenu } from '../../hooks/useContextMenu';
import { isBackendId } from '../../lib/idUtils';
import type { MediaFile } from '../../services/mediaService';
import type { ChatMessagingExecution } from '../../lib/commandChatMessaging';
import { captureChatMessagingTarget, registerChatMessagingSurface, requestChatMessagingCommand, ChatMessagingStaleError, type ChatMessagingCommandID } from '../../lib/commandChatMessaging';
import { useAuthStore } from '../../store/authStore';
import { registerChatNavigationSurface, requestChatNavigationCommand, getChatMessageNavigationInstanceId, captureChatNavigationTarget, type ChatNavigationTarget } from '../../lib/commandChatNavigation';
import { useWorkspaceChatModalStore } from '../../store/workspaceChatModalStore';
import { getTopmostModalID } from '../../lib/modalRegistry';
import { GetActiveProfile, GetActiveProfileSlug } from '@wailsjs/go/wailsapi/Profiles';
import { EventsOn } from '@wailsjs/runtime/runtime';
import { announce, useAnnouncer } from '../../hooks/useAnnouncer';
import type { EditorSendTargetOption, ChatSendToEditorPayload } from '../../lib/editorSendMenu';
import {
  useChatSurfaceController,
  type ChatSurfaceController,
} from './ChatSurfaceController';
import './ChatSessionView.css';

export interface ChatSessionViewProps {
  variant?: 'page' | 'embedded';
  surface: ChatSurfaceIdentity;
  /** Envio da mensagem (ex.: sendMessage da store ou adaptador do chat modal) */
  onSend: (content: string, mediaFiles: MediaFile[] | undefined, origin: ChatSurfaceOrigin, command?: ChatMessagingExecution) => Promise<boolean | void>;
  /** Solicitação de troca de conversa (controlada pelo dono da superfície). */
  onRequestConversationChange?: ChatToolbarConversationChangeHandler;
  showShortcutsHelp?: boolean;
  profileSlug?: string;
}

/**
 * Um carregamento de janela só anuncia se a janela chegar logo depois de o
 * carregamento terminar. Passado isso o aviso já não descreve a ação que o
 * originou — descreve alguma outra coisa que mexeu na janela.
 */
const PENDING_WINDOW_ANNOUNCEMENT_MAX_AGE_MS = 5_000;

/**
 * Prazo do carregamento em si. Existe para o pendente não ficar armado
 * indefinidamente se a promessa nunca resolver; é generoso porque um backend
 * lento ainda deve conseguir anunciar a paginação que a pessoa pediu.
 */
const PENDING_WINDOW_LOAD_MAX_MS = 60_000;

export function ChatSessionView({
  variant = 'page',
  surface,
  onSend,
  onRequestConversationChange,
  showShortcutsHelp,
  profileSlug,
}: ChatSessionViewProps) {
  return (
    <ChatSessionProvider surface={surface}>
      <ChatSessionViewControllerBridge
        variant={variant}
        surface={surface}
        onSend={onSend}
        onRequestConversationChange={onRequestConversationChange}
        showShortcutsHelp={showShortcutsHelp}
        profileSlug={profileSlug}
      />
    </ChatSessionProvider>
  );
}

function ChatSessionViewControllerBridge({
  onSend,
  onRequestConversationChange,
  variant,
  showShortcutsHelp,
  profileSlug,
}: ChatSessionViewProps) {
  const controller = useChatSurfaceController({
    onSend: (content, mediaFiles, context) => onSend(content, mediaFiles, context.origin, context.command),
  });

  return (
    <ChatSessionViewContent
      variant={variant}
      showShortcutsHelp={showShortcutsHelp}
      onRequestConversationChange={onRequestConversationChange}
      controller={controller}
      profileSlug={profileSlug}
    />
  );
}

interface ChatSessionViewContentProps extends Pick<ChatSessionViewProps, 'variant' | 'showShortcutsHelp' | 'onRequestConversationChange' | 'profileSlug'> {
  controller: ChatSurfaceController;
}

function ChatSessionViewContent({
  variant = 'page',
  showShortcutsHelp,
  onRequestConversationChange,
  profileSlug,
  controller,
}: ChatSessionViewContentProps) {
  const { t } = useTranslation();
  const { announceRequest } = useAnnouncer();
  const rootRef = useRef<HTMLDivElement>(null);
  const inputRef = useRef<HTMLTextAreaElement>(null);
  const messagesContainerRef = useRef<HTMLDivElement>(null);
  const scrollFrameRef = useRef<number | null>(null);
  const restoreScrollFrameRef = useRef<number | null>(null);
  const scrollPersistTimerRef = useRef<number | null>(null);
  const latestScrollStateRef = useRef<{ scrollTop: number; scrollAnchorMessageId: string | null }>({
    scrollTop: 0,
    scrollAnchorMessageId: null,
  });
  const restoredScrollSessionKeyRef = useRef<string | null>(null);
  const hasAutoFocusedRef = useRef(false);
  const retryButtonRef = useRef<HTMLButtonElement>(null);
  const wasLoadingRef = useRef(false);
  const pendingWindowAnnouncementRef = useRef<{
    kind: 'start' | 'end' | 'older' | 'newer';
    trigger: MessageWindowLoadTrigger;
    expiresAt: number;
    previousStartIndex: number;
    previousEndIndex: number;
    previousWindowKey: string | null;
  } | null>(null);
  const latestWindowKeyRef = useRef<string | null>(null);
  const { tab: panelTab, isActive: isPanelActive } = useWorkspacePanel();
  const isInteractiveSurface = variant === 'embedded' || isPanelActive;
  const isPanelActiveRef = useRef(isPanelActive);
  isPanelActiveRef.current = isPanelActive;
  const canFocusWorkspacePanelImmediately = useCallback(() => {
    if (variant !== 'page' || !isPanelActiveRef.current || isModalOpen()) return false;
    const input = inputRef.current;
    return Boolean(input?.isConnected && !input.disabled);
  }, [variant]);

  const [showContinueEnabled, setShowContinueEnabled] = useState(false);
  const [activeProfileSlug, setActiveProfileSlug] = useState('');
  const {
    session,
    conversation,
    threadedMessages,
    isLoading,
    hasOlderMessages,
    hasNewerMessages,
    isLoadingOlderMessages,
    isLoadingMessageWindow,
    loadOlderMessages,
    loadNewerMessages,
    loadStartMessages,
    loadEndMessages,
    loadMessageChildren,
    loadConversationSession,
    updateConversationMessage,
    updateConversationMessagePinned,
    isConversationReasoningExpanded,
    origin,
    conversationId,
    draftMessage,
    draftMediaFiles,
    scrollTop,
    scrollAnchorMessageId,
    setDraftMessage,
    setDraftMediaFiles,
    setScrollState,
  } = controller;

  // Os comandos que o agente de código desta conversa oferece entram no menu da
  // barra do campo de mensagem (AEP-0084 D8).
  const agentCommands = useAgentSessionCommands(conversationId);

  const clearConversationSendFailure = useChatStore((state) => state.clearConversationSendFailure);
  const getSessionConversation = useCallback(() => conversation, [conversation]);
  const visibleMessageCount = useMemo(() => {
    if (!threadedMessages.length) return 0;
    const processedTurnIds = new Set<string>();
    let count = 0;
    for (const node of threadedMessages) {
      const message = node.message;
      if (!message) continue;
      const turnId = message.turnId;
      if (!turnId) {
        count += 1;
        continue;
      }
      if (processedTurnIds.has(turnId)) continue;
      processedTurnIds.add(turnId);
      count += 1;
    }
    return count;
  }, [threadedMessages]);
  const usesLocalVisualWindowCount = visibleMessageCount > 0 && visibleMessageCount !== threadedMessages.length;

  useEffect(() => {
    let mounted = true;

    const loadActiveProfile = async () => {
      try {
        const [profile, profileSlug] = await Promise.all([GetActiveProfile(), GetActiveProfileSlug()]);
        if (!mounted) return;
        // A continuação é habilitada apenas pelo perfil: o backend sempre consegue
        // continuar — via assistant prefill quando o provider suporta, ou via
        // fallback por mensagem de usuário quando não suporta (Issue #124).
        const profileAllowsContinue = profile?.chat?.streaming_recovery_show_continue ?? true;
        setShowContinueEnabled(profileAllowsContinue);
        setActiveProfileSlug(profileSlug);
      } catch {
        if (!mounted) return;
        setShowContinueEnabled(false);
        setActiveProfileSlug('');
      }
    };

    void loadActiveProfile();
    const unsubChanged = EventsOn('profile:changed', () => void loadActiveProfile());
    const unsubUpdated = EventsOn('profile:updated', () => void loadActiveProfile());

    return () => {
      mounted = false;
      unsubChanged();
      unsubUpdated();
    };
  }, []);

  useEffect(() => {
    if (!conversationId) return;
    if (session?.conversation) return;
    void loadConversationSession(conversationId);
  }, [conversationId, loadConversationSession, session?.conversation, variant]);

  useEffect(() => {
    const container = messagesContainerRef.current;
    if (!container || (scrollTop <= 0 && !scrollAnchorMessageId)) return;
    const restoreKey = `${origin.sessionKey}:${conversationId ?? 'none'}`;
    if (restoredScrollSessionKeyRef.current === restoreKey) return;
    if (restoreScrollFrameRef.current !== null) {
      window.cancelAnimationFrame(restoreScrollFrameRef.current);
    }
    restoreScrollFrameRef.current = window.requestAnimationFrame(() => {
      restoreScrollFrameRef.current = null;
      if (`${origin.sessionKey}:${conversationId ?? 'none'}` !== restoreKey) return;
      const currentContainer = messagesContainerRef.current;
      if (!currentContainer) return;
      if (scrollAnchorMessageId) {
        const anchorElement = currentContainer
          .querySelector<HTMLElement>(`[data-message-id="${CSS.escape(scrollAnchorMessageId)}"]`);
        if (anchorElement) {
          anchorElement.scrollIntoView({ block: 'start' });
          restoredScrollSessionKeyRef.current = restoreKey;
          return;
        }
        if (scrollTop > 0) {
          currentContainer.scrollTop = scrollTop;
          restoredScrollSessionKeyRef.current = restoreKey;
        }
        return;
      }
      if (scrollTop > 0) {
        currentContainer.scrollTop = scrollTop;
        restoredScrollSessionKeyRef.current = restoreKey;
      }
    });
    return () => {
      if (restoreScrollFrameRef.current !== null) {
        window.cancelAnimationFrame(restoreScrollFrameRef.current);
        restoreScrollFrameRef.current = null;
      }
    };
  }, [conversationId, origin.sessionKey, scrollAnchorMessageId, scrollTop, threadedMessages.length]);

  useEffect(() => {
    const container = messagesContainerRef.current;
    if (!container || !conversationId) return;

    const getAnchorMessageId = () => {
      if (!document.elementsFromPoint) return null;
      const rect = container.getBoundingClientRect();
      if (rect.width <= 0 || rect.height <= 0) return null;
      const probeX = Math.min(rect.right - 1, rect.left + Math.min(16, Math.max(1, rect.width / 2)));
      const probeYs = [
        rect.top + 1,
        Math.min(rect.bottom - 1, rect.top + 24),
        rect.top + rect.height / 2,
      ];

      for (const probeY of probeYs) {
        const messageNode = document
          .elementsFromPoint(probeX, probeY)
          .map((element) => element.closest?.('[data-message-node]'))
          .find((node): node is HTMLElement => !!node && container.contains(node));
        if (messageNode?.dataset.messageId) {
          return messageNode.dataset.messageId;
        }
      }

      return null;
    };

    const readScrollState = () => ({
      scrollTop: container.scrollTop,
      scrollAnchorMessageId: getAnchorMessageId(),
    });

    const persistLatestScrollState = () => {
      setScrollState(latestScrollStateRef.current);
    };

    const handleScroll = () => {
      if (scrollFrameRef.current !== null) return;
      scrollFrameRef.current = window.requestAnimationFrame(() => {
        scrollFrameRef.current = null;
        latestScrollStateRef.current = readScrollState();
        if (scrollPersistTimerRef.current !== null) {
          window.clearTimeout(scrollPersistTimerRef.current);
        }
        scrollPersistTimerRef.current = window.setTimeout(() => {
          scrollPersistTimerRef.current = null;
          persistLatestScrollState();
        }, 200);
      });
    };

    container.addEventListener('scroll', handleScroll, { passive: true });
    return () => {
      container.removeEventListener('scroll', handleScroll);
      if (scrollFrameRef.current !== null) {
        window.cancelAnimationFrame(scrollFrameRef.current);
        scrollFrameRef.current = null;
      }
      if (scrollPersistTimerRef.current !== null) {
        window.clearTimeout(scrollPersistTimerRef.current);
        scrollPersistTimerRef.current = null;
      }
      latestScrollStateRef.current = readScrollState();
      persistLatestScrollState();
    };
  }, [conversationId, setScrollState]);

  const [hasVoiceConfig, setHasVoiceConfig] = useState(() => ttsService.hasVoiceConfig());
  useEffect(() => {
    const handler = () => setHasVoiceConfig(ttsService.hasVoiceConfig());
    ttsService.on('voiceConfigChanged', handler);
    return () => {
      ttsService.off('voiceConfigChanged', handler);
    };
  }, []);
  const isTTSDisabled = !hasVoiceConfig;

  const shortcutsOpen = showShortcutsHelp ?? variant === 'page';

  const [lastFailedMessage, setLastFailedMessage] = useState<{ content: string; media?: MediaFile[] } | null>(null);
  const [unavailableConversationID, setUnavailableConversationID] = useState<string | null>(null);
  const [sendError, setSendError] = useState<string | null>(null);
  const [dismissedSessionSendError, setDismissedSessionSendError] = useState<string | null>(null);
  const sessionSendFailureMessage = session?.sendFailureMessage ?? null;
  const sessionSendFailureAnnounced = session?.sendFailureAnnounced ?? false;
  const sessionSendFailureRetryable = session?.sendFailureRetryable ?? false;
  const sessionSendFailureRetry = sessionSendFailureRetryable
    && (session?.sendFailureRetryContent !== null || (session?.sendFailureRetryMediaFiles.length ?? 0) > 0)
    ? { content: session?.sendFailureRetryContent ?? '', media: session?.sendFailureRetryMediaFiles ?? [] }
    : null;
  const effectiveSendError = sendError ?? (
    sessionSendFailureMessage && sessionSendFailureMessage !== dismissedSessionSendError
      ? sessionSendFailureMessage
      : null
  );
  const lastAnnouncedSessionSendFailureRef = useRef<string | null>(null);
  const effectiveFailedMessage = lastFailedMessage ?? (sessionSendFailureRetryable ? sessionSendFailureRetry : null);
  const canRetryEffectiveSendError = unavailableConversationID !== conversationId && !!effectiveFailedMessage && (!!sendError || sessionSendFailureRetryable);

  const { pathname } = useLocation();
  const modalId = useModalId();
  const modalIsTopmost = useModalIsTopmost();
  const messagingInstance = useRef(`chat-messaging-${crypto.randomUUID()}`);
  const voiceSetupPromptPendingRef = useRef(false);
  const navigate = useNavigate();
  const messagingLive = useRef({ controller, conversationId, origin, draftMessage, draftMediaFiles, effectiveFailedMessage, isLoading, isInteractiveSurface, pathname, modalIsTopmost, t });
  messagingLive.current = { controller, conversationId, origin, draftMessage, draftMediaFiles, effectiveFailedMessage, isLoading, isInteractiveSurface, pathname, modalIsTopmost, t };
  const messagingUser = useAuthStore(state => state.user);
  const navigationInstance = useRef(`chat-navigation-${crypto.randomUUID()}`);
  const menuNavigationTargets = useRef(new WeakMap<Message, { read?: ChatNavigationTarget; reasoning?: ChatNavigationTarget }>());
  const menuNavigationLeases = useRef<ChatNavigationTarget[]>([]);
  const disposeMenuNavigation = useCallback(() => {
    menuNavigationLeases.current.splice(0).forEach(target => target.dispose());
  }, []);
  useEffect(() => disposeMenuNavigation, [disposeMenuNavigation]);
  useEffect(() => {
    const root = rootRef.current;
    const owner = useAuthStore.getState().user;
    const workspace = useWorkspaceStore.getState().workspace;
    if (!root || !owner || !workspace || !conversationId) return;
    const sessionKey = origin.sessionKey;
    const current = () => {
      const live = messagingLive.current;
      const auth = useAuthStore.getState();
      const ws = useWorkspaceStore.getState().workspace;
      const snapshot = useChatStore.getState().surfaceSessionsByKey[sessionKey];
      return root.isConnected && auth.isAuthenticated && auth.user?.userId === owner.userId &&
        auth.user.sessionId === owner.sessionId && ws?.id === workspace.id && ws.activeTabId === panelTab.id &&
        live.pathname === pathname && live.isInteractiveSurface && live.conversationId === conversationId &&
        live.origin.sessionKey === sessionKey && live.origin.surfaceId === origin.surfaceId &&
        live.origin.surfaceType === origin.surfaceType && snapshot?.conversationId === conversationId;
    };
    const destination = (id: string) => id === 'chat.focus.input' ? inputRef.current :
      root.querySelector<HTMLElement>('.message-list__list,.message-list--empty');
    return registerChatNavigationSurface({
      root, instanceId: navigationInstance.current,
      allowedCommands: ['chat.focus.input', 'chat.focus.messages'],
      readContext: () => ({ pathname, ownerId: owner.userId, sessionId: owner.sessionId,
        workspaceId: workspace.id, tabId: panelTab.id, conversationId, chatSessionKey: sessionKey,
        modalId: modalId ?? undefined }),
      isCurrent: current,
      subscribe: changed => {
        const off = [useAuthStore.subscribe(changed), useWorkspaceStore.subscribe(changed),
          useChatStore.subscribe(changed), useWorkspaceChatModalStore.subscribe(changed)];
        return () => off.forEach(dispose => dispose());
      },
      canOpen: id => {
        const target = destination(id);
        return current() && !!target?.isConnected && !(target instanceof HTMLTextAreaElement && target.disabled);
      },
      open: id => {
        const target = destination(id);
        if (!current() || !target?.isConnected || target instanceof HTMLTextAreaElement && target.disabled) return false;
        target.focus();
        return document.activeElement === target;
      },
    });
  }, [conversationId, origin.sessionKey, origin.surfaceId, origin.surfaceType, panelTab.id, pathname, modalId, messagingUser, isInteractiveSurface]);
  const messageCommands = useChatMessageCommands(rootRef, conversationId, origin.sessionKey, announce);
  const messageCommandsRef = useRef(messageCommands);
  messageCommandsRef.current = messageCommands;
  useEffect(() => {
    const root = rootRef.current;
    const owner = useAuthStore.getState().user;
    const workspace = useWorkspaceStore.getState().workspace;
    if (!root || !owner || !workspace || !conversationId) return;
    const sessionKey = origin.sessionKey;
    const capturedPath = pathname;
    const current = () => {
      const live = messagingLive.current;
      const auth = useAuthStore.getState();
      const ws = useWorkspaceStore.getState().workspace;
      const tab = ws?.tabs.find(item => item.id === panelTab.id);
      const modal = useWorkspaceChatModalStore.getState();
      return auth.isAuthenticated && auth.user?.userId === owner.userId && auth.user.sessionId === owner.sessionId &&
        ws?.id === workspace.id && ws.activeTabId === panelTab.id && Boolean(tab) &&
        live.conversationId === conversationId && live.origin.sessionKey === sessionKey && live.pathname === capturedPath &&
        tab?.conversationId === conversationId &&
        (!modalId || modal.isOpen && modal.boundConversationId === conversationId && modal.boundTabId === panelTab.id);
    };
    const available = () => current() && messagingLive.current.isInteractiveSurface &&
      (modalId ? getTopmostModalID() === modalId && messagingLive.current.modalIsTopmost() : !isModalOpen());
    return registerChatMessagingSurface({
      root, instanceId: messagingInstance.current, queueKey: conversationId, modalId: modalId ?? undefined,
      isCurrent: current,
      canStart: (id, keyboardTarget) => {
        if (!available()) return false;
        if (isChatMessageCommand(id)) return messageCommandsRef.current.canStart(id, keyboardTarget);
        if (keyboardTarget instanceof Element && keyboardTarget.closest('.monaco-editor,.xterm,[role="terminal"],.chat-message__edit,[role="menu"],[role="listbox"]')) return false;
        if (keyboardTarget instanceof Element && keyboardTarget.closest('input,textarea,select,[contenteditable]') && !root.contains(keyboardTarget)) return false;
        return id !== 'chat.response.cancel' || messagingLive.current.isLoading;
      },
      subscribe: changed => {
        const off = [useAuthStore.subscribe(changed), useWorkspaceStore.subscribe(changed), useWorkspaceChatModalStore.subscribe(changed), useChatStore.subscribe(changed), messageCommandsRef.current.subscribe(changed)];
        return () => off.forEach(dispose => dispose());
      },
      prepare: (id, override) => {
        if (isChatMessageCommand(id)) return messageCommandsRef.current.prepare(id, override, current, available);
        const live = messagingLive.current;
        const options = override ? { ...override as { content?: string; media?: MediaFile[]; messageId?: string; continue?: boolean; voice?: boolean; recovery?: boolean } } : undefined;
        const store = useChatStore.getState();
        const snapshot = store.surfaceSessionsByKey[sessionKey];
        if (!snapshot || snapshot.conversationId !== conversationId) return undefined;
        const draftRevision = store.getDraftRevision(sessionKey);
        const content = options?.content ?? snapshot.draftMessage;
        // O callback do input pode anteceder o próximo render. Assim como a
        // revisão, anexos do rascunho vêm da store, não de props antigas.
        const media = (options?.recovery ? snapshot.sendFailureRetryMediaFiles : snapshot.draftMediaFiles).map(item => ({ ...item }));
        if (id === 'chat.message.send' && options?.content !== undefined && !options.voice && !options.recovery && content.trim() !== snapshot.draftMessage.trim()) return undefined;
        if (options?.recovery && (!snapshot.sendFailureRetryable || content !== (snapshot.sendFailureRetryContent ?? ''))) return undefined;
        const interruptedId = snapshot.lastInterruptedMessageId;
        const interrupted = interruptedId ? useChatStore.getState().getConversationMessages(conversationId).find(message => message.id === interruptedId) : undefined;
        const fallbackRetryId = interrupted?.turnId;
        const messageId = id === 'chat.message.retry' ? (options?.messageId ?? fallbackRetryId) : undefined;
        if (id === 'chat.message.retry' && (!messageId || !isBackendId(messageId))) return undefined;
        if (id !== 'chat.response.cancel' && !messageId && !content.trim() && !media.length) return undefined;
        const pipelineRevision = store.getMessagingPipelineRevision(conversationId);
        let runRevision: number | undefined;
        let pipelineStarted = false;
        let disposed = false;
        let invalid = false;
        const isCurrent = () => {
          const valid = !disposed && !invalid && current() &&
            (!pipelineStarted || useChatStore.getState().getMessagingPipelineRevision(conversationId) === runRevision) &&
            (id === 'chat.response.cancel' ? useChatStore.getState().getMessagingPipelineRevision(conversationId) === pipelineRevision :
              (id !== 'chat.message.send' || useChatStore.getState().getDraftRevision(sessionKey) === draftRevision));
          if (!valid) invalid = true;
          return valid;
        };
        const canCommit = () => isCurrent() && available();
        const commandOrigin = { ...live.origin };
        const send = live.controller.sendMessage;
        return {
          isCurrent, canCommit,
          waitForAdmission: id === 'chat.response.cancel' ? undefined : () => useChatStore.getState().waitForMessagingAdmission(conversationId),
          async execute(handoff) {
            if (!canCommit()) throw new ChatMessagingStaleError();
            const command = { handoff, isCurrent: canCommit, onPipelineStarted: (revision: number) => { runRevision = revision; pipelineStarted = true; } };
            if (id === 'chat.response.cancel') {
              useChatStore.getState().finishCommandCancellation(conversationId, sessionKey, pipelineRevision);
            } else if (messageId) {
              await useChatStore.getState().retryMessageToConversation(conversationId, messageId, { allowAssistantPrefill: options?.continue === true }, { origin: commandOrigin, command });
            } else {
              await send(content.trim(), media.length ? media : undefined, command);
            }
          },
          succeeded() {
            if (!current()) return;
            setLastFailedMessage(null);
            setUnavailableConversationID(null);
            setSendError(null);
            const latest = useChatStore.getState();
            if (id === 'chat.message.send' && !options?.recovery && latest.getDraftRevision(sessionKey) === draftRevision) {
              if (options?.voice) latest.setConversationDraftMediaFiles(conversationId, [], sessionKey);
              else latest.clearConversationDraft(conversationId, sessionKey);
            }
            if (id === 'chat.message.retry' || options?.recovery) latest.clearConversationSendFailure(conversationId, sessionKey);
          },
          failed(reason) {
            if (current() && reason === 'conversation_unavailable') {
              setLastFailedMessage(null);
              setUnavailableConversationID(conversationId);
              setSendError(messagingLive.current.t('chat.conversationUnavailable'));
            }
          },
          settled(status) {
            if (id !== 'chat.response.cancel' && runRevision !== undefined && current() && status !== 'succeeded' && status !== 'outcome_unknown') {
              useChatStore.getState().finishCommandCancellation(conversationId, sessionKey, runRevision, false);
            }
          },
          dispose() { disposed = true; },
        };
      },
    });
  }, [conversationId, origin.sessionKey, panelTab.id, modalId, pathname, messagingUser]);
  const requestMessaging = useCallback((id: ChatMessagingCommandID, override?: unknown) => {
    const target = captureChatMessagingTarget(() => messagingLive.current.pathname, id, messagingInstance.current, undefined, override);
    const accepted = !!target && requestChatMessagingCommand(id, messagingInstance.current, target);
    if (id === 'chat.message.edit.save' && !accepted) announce(t('chat.editSaveError'));
    if (id === 'chat.message.send_to_editor' && !accepted) announce(t('commandPalette.executionFailed'));
    return accepted;
  }, [t]);
  const handleCancelStreaming = useCallback(() => { requestMessaging('chat.response.cancel'); }, [requestMessaging]);

  const wsTabs = useWorkspaceStore((state) => state.workspace?.tabs);

  const editorTargets = useMemo<EditorSendTargetOption[]>(
    () =>
      (wsTabs || [])
        .filter((tab) => tab.type === 'editor')
        .map((tab) => ({
          id: tab.id,
          title: String(tab.title || '').trim() || t('editor.fallback.title'),
        })),
    [wsTabs, t],
  );

  const handleSpeakRequest = useCallback(
    async (message: Message) => {
      if (ttsService.hasVoiceConfig()) {
        requestMessaging('chat.message.speak', { messageId: message.id });
        return;
      }
      if (voiceSetupPromptPendingRef.current) return;

      const capturedOwner = useAuthStore.getState().user;
      const capturedWorkspace = useWorkspaceStore.getState().workspace;
      const capturedOrigin = { ...origin };
      const capturedPath = messagingLive.current.pathname;
      let invalid = false;
      const current = () => {
        const auth = useAuthStore.getState().user;
        const ws = useWorkspaceStore.getState().workspace;
        const modal = useWorkspaceChatModalStore.getState();
        const valid = !invalid && rootRef.current?.isConnected && auth?.userId === capturedOwner?.userId &&
          auth?.sessionId === capturedOwner?.sessionId && ws?.id === capturedWorkspace?.id &&
          ws?.activeTabId === capturedOrigin.tabId && ws?.tabs.find(tab => tab.id === capturedOrigin.tabId)?.conversationId === conversationId &&
          messagingLive.current.pathname === capturedPath && messagingLive.current.conversationId === conversationId &&
          useChatStore.getState().getConversationMessages(conversationId!).find(item => item.id === message.id) === message &&
          (!modalId || modal.isOpen && modal.boundTabId === capturedOrigin.tabId && modal.boundConversationId === conversationId);
        if (!valid) invalid = true;
        return Boolean(valid);
      };
      if (!current() || !messagingLive.current.isInteractiveSurface || (modalId ? !modalIsTopmost() : isModalOpen())) return;
      const off = [useAuthStore.subscribe(current), useWorkspaceStore.subscribe(current), useWorkspaceChatModalStore.subscribe(current), useChatStore.subscribe(current)];
      voiceSetupPromptPendingRef.current = true;
      try {
        const shouldConfigure = await requestConfirm({
          title: t('chat.voiceSetup.title'),
          message: t('chat.voiceSetup.description'),
          confirmText: t('chat.voiceSetup.configure'),
          cancelText: t('common.cancel'),
          variant: 'info',
        });
        if (!shouldConfigure || !current()) return;

        const targetProfileSlug = profileSlug || activeProfileSlug || await GetActiveProfileSlug();
        if (!current()) return;
        if (!targetProfileSlug) {
          announce(t('chat.voiceSetup.profileUnavailable'));
          return;
        }

        await executeDeepLink(
          {
            type: 'resource:edit',
            resource: 'profiles',
            resourceId: targetProfileSlug,
            tab: 'voice',
          },
          {
            navigate,
            ...(origin.tabId && (
              origin.surfaceType === 'page'
              || origin.surfaceType === 'embedded'
              || origin.surfaceType === 'modal'
            )
              ? {
                  caller: {
                    kind: 'workspace' as const,
                    tabId: origin.tabId,
                    surfaceId: origin.surfaceId,
                    surfaceType: origin.surfaceType,
                    conversationId: origin.conversationId,
                  },
                }
              : {}),
          },
        );
      } catch (error) {
        if (!current()) return;
        handleError(error, {
          source: 'ChatSessionView.voiceSetup',
          userMessage: t('chat.voiceSetup.error'),
          severity: ErrorSeverity.RECOVERABLE,
          metadata: {
            profileSlug: profileSlug || activeProfileSlug || undefined,
            surfaceId: origin.surfaceId,
          },
        });
      } finally {
        off.forEach(dispose => dispose());
        voiceSetupPromptPendingRef.current = false;
      }
    },
    [activeProfileSlug, navigate, origin, profileSlug, requestMessaging, t, conversationId, modalId, modalIsTopmost],
  );

  const handleDeleteMessage = useCallback((message: { id: string | number }) => {
    requestMessaging('chat.message.delete', { messageId: String(message.id) });
  }, [requestMessaging]);

  const sendToEditor = useCallback((payload: ChatSendToEditorPayload) => {
    requestMessaging('chat.message.send_to_editor', { messageId: payload.messageId, transfer: payload });
  }, [requestMessaging]);

  const { menuVisible, menuPosition, menuItems, showMenu, hideMenu } = useContextMenu({
    sessionKey: origin.sessionKey,
    onCopy: (message, markdown) => { requestMessaging(markdown ? 'chat.message.copy_markdown' : 'chat.message.copy', { messageId: message.id }); },
    onReadMessage: (message) => {
      const target = menuNavigationTargets.current.get(message)?.read;
      if (target?.canOpen('chat.message.read.open')) requestChatNavigationCommand('chat.message.read.open', target.instanceId);
      disposeMenuNavigation();
    },
    onSpeak: handleSpeakRequest,
    onEdit: (message) => {
      requestMessaging('chat.message.edit.open', { messageId: message.id });
    },
    onResend: async (message) => {
      const conversationId = getSessionConversation()?.id;
      if (!conversationId || !isBackendId(message.id)) return;
      requestMessaging('chat.message.retry', { messageId: message.id });
    },
    onContinue: async (message) => {
      const conversationId = getSessionConversation()?.id;
      const turnId = String(message.turnId || '').trim();
      if (!conversationId || !turnId) return;
      requestMessaging('chat.message.retry', { messageId: turnId, continue: true });
    },
    shouldShowContinue: (message) => {
      if (!showContinueEnabled) return false;
      const interruptedId = session?.lastInterruptedMessageId;
      if (!interruptedId) return false;
      if (!isBackendId(String(interruptedId))) return false;
      if (String(message.id) !== String(interruptedId)) return false;
      if (!isBackendId(message.id)) return false;
      if (message.role !== 'assistant' || message.isStreaming) return false;
      if (!String(message.turnId || '').trim()) return false;
      if (!String(message.content || '').trim()) return false;
      return true;
    },
    onDelete: handleDeleteMessage,
    onCancelStreaming: (message) => {
      if (!message.isStreaming) return;
      void handleCancelStreaming();
    },
    onSendToEditor: sendToEditor,
    editorTargets,
    onPin: (message) => { requestMessaging('chat.message.pin.toggle', { messageId: message.id }); },
    onToggleReasoning: (message) => {
      const target = menuNavigationTargets.current.get(message)?.reasoning;
      if (target?.canOpen('chat.message.reasoning.toggle')) requestChatNavigationCommand('chat.message.reasoning.toggle', target.instanceId);
      disposeMenuNavigation();
    },
    isReasoningExpanded: (messageId: string) => (
      conversation?.id
        ? isConversationReasoningExpanded(conversation.id, messageId)
        : false
    ),
    isTTSDisabled,
  });

  useEffect(() => {
    if (!menuVisible) disposeMenuNavigation();
  }, [menuVisible, disposeMenuNavigation]);

  useEffect(() => {
    if (!isInteractiveSurface || !isLoading) return;

    const onKeyDown = (event: KeyboardEvent) => {
      if (event.defaultPrevented || event.repeat || event.isComposing || event.keyCode === 229 || event.getModifierState('AltGraph')) return;
      // Com um modal aberto (ex.: painel de atalhos), o Escape deve fechar o
      // modal — não cancelar o streaming nem o menu na UI de fundo.
      if (isModalOpen()) return;

      if (event.key !== 'Escape') return;

      if (menuVisible) {
        event.preventDefault();
        hideMenu();
        return;
      }

      // O cancelamento por Escape é escopado ao campo de edição (tratado pelo
      // próprio ChatInput). Aqui apenas devolvemos o foco ao input quando o
      // Escape vem de qualquer outro elemento do painel — sem cancelar.
      const input = inputRef.current;
      if (event.target === input) return;
      if (!input) return;

      // Só devolvemos o foco ao input quando o Escape se origina DENTRO do
      // painel do chat. Fora dele (outra superfície/painel, ex.: terminal,
      // editor, task list, ou o chat embutido enquanto inativo), o roteamento
      // "ESC → área padrão do painel atual" é responsabilidade do sistema
      // central de landmarks (useLandmarkNavigation no WorkspaceLayout), que
      // respeita o painel ativo. Assim este listener não "rouba" o foco para o
      // chat a partir de outras áreas do app (Issue #202 / AEP-0058).
      const root = rootRef.current;
      if (!root || !root.contains(event.target as Node | null)) return;

      if (requestChatNavigationCommand('chat.focus.input', navigationInstance.current)) {
        event.preventDefault();
        event.stopPropagation();
      }
    };

    // Registrado na fase de borbulhamento (sem captura) para que handlers locais
    // de Escape (ex.: colapso/navegação em MessageNode) rodem primeiro e possam
    // chamar preventDefault()/stopPropagation(). Aqui só agimos quando o Escape
    // não foi tratado localmente (event.defaultPrevented evita interferência).
    window.addEventListener('keydown', onKeyDown);
    return () => window.removeEventListener('keydown', onKeyDown);
  }, [hideMenu, isInteractiveSurface, isLoading, menuVisible]);

  useChatKeyboardNav({
    enabled: isInteractiveSurface,
    inputRef,
    messagesContainerRef,
  });

  // Foco de painel unificado (só na variante de aba de workspace). O
  // WorkspaceLayout roteia o foco da aba ativa via workspacePanelFocusRegistry
  // (troca por atalho, fechar aba, F6, retorno de modal). O handler marca um
  // pedido; um efeito foca o input do chat quando ele existe — mesmo padrão de
  // editor/tasklist/terminal. Variantes embedded/modal não registram (o tabId do
  // painel pertence à aba hospedeira, ex.: tasklist).
  const [panelFocusNonce, setPanelFocusNonce] = useState(0);
  const consumedPanelFocusNonceRef = useRef(0);

  useEffect(() => {
    if (variant !== 'page') return;
    const tabId = panelTab.id;
    return registerWorkspacePanelFocus(tabId, () => {
      if (!isPanelActiveRef.current || isModalOpen()) return false;
      setPanelFocusNonce((nonce) => nonce + 1);
      return true;
    }, () => {
      if (!canFocusWorkspacePanelImmediately()) return false;
      const input = inputRef.current;
      if (!input) return false;
      input.focus();
      return document.activeElement === input;
    }, canFocusWorkspacePanelImmediately);
  }, [canFocusWorkspacePanelImmediately, panelTab.id, variant]);

  useEffect(() => {
    if (
      variant !== 'page'
      || panelFocusNonce === 0
      || consumedPanelFocusNonceRef.current === panelFocusNonce
      || !isPanelActive
      || isModalOpen()
    ) {
      return;
    }
    const nonce = panelFocusNonce;
    const raf = requestAnimationFrame(() => {
      if (consumedPanelFocusNonceRef.current === nonce) return;
      if (!isPanelActiveRef.current || isModalOpen()) return;
      // Não roubar o foco durante a edição do título da aba.
      if (document.querySelector('.ws-tabs__tab-edit')) return;
      // Não roubar o foco quando ele já está numa mensagem desta conversa: ao
      // fechar um menu de contexto/modal, o foco é restaurado ao elemento que o
      // abriu (ex.: o `.message-node`), e o roteamento de painel (F6/retorno de
      // modal) não pode sobrepor essa restauração intencional.
      const active = document.activeElement as HTMLElement | null;
      if (
        active
        && active !== inputRef.current
        && rootRef.current?.contains(active)
        && active.closest('.message-node')
      ) {
        consumedPanelFocusNonceRef.current = nonce;
        return;
      }
      const input = inputRef.current;
      if (input) {
        input.focus();
        consumedPanelFocusNonceRef.current = nonce;
      }
    });
    return () => cancelAnimationFrame(raf);
  }, [variant, panelFocusNonce, isPanelActive]);

  useEffect(() => {
    if (variant !== 'page' || !isInteractiveSurface) return;
    let focusTimer: ReturnType<typeof setTimeout> | null = null;
    const checkTimer = setInterval(() => {
      const inputElement = inputRef.current;
      if (inputElement && !hasAutoFocusedRef.current) {
        hasAutoFocusedRef.current = true;
        clearInterval(checkTimer);
        focusTimer = setTimeout(() => {
          if (typeof document === 'undefined') return;
          const active = document.activeElement as HTMLElement | null;
          const hasMeaningfulFocus =
            !!active &&
            active !== document.body &&
            active !== document.documentElement &&
            active !== inputElement;
          if (document.querySelector('.ws-tabs__tab-edit')) return;
          if (active?.closest('.ws-tabs')) return;
          if (hasMeaningfulFocus) return;
          inputElement.focus();
        }, 100);
      }
    }, 100);

    return () => {
      clearInterval(checkTimer);
      if (focusTimer) clearTimeout(focusTimer);
    };
  }, [isInteractiveSurface, variant]);

  useEffect(() => {
    if (!isInteractiveSurface) {
      wasLoadingRef.current = isLoading;
      return;
    }
    if (wasLoadingRef.current && !isLoading) {
      const active = document.activeElement as HTMLElement | null;
      const isEditingMessage = active?.closest('.chat-message--editing') !== null;
      const isEditingWorkspaceTab = !!document.querySelector('.ws-tabs__tab-edit');
      if (!isEditingMessage && !isEditingWorkspaceTab) {
        requestAnimationFrame(() => {
          inputRef.current?.focus();
        });
      }
    }
    wasLoadingRef.current = isLoading;
  }, [isInteractiveSurface, isLoading]);

  useEffect(() => {
    if (!shortcutsOpen || !isInteractiveSurface) return;
    const handleKeyPress = (e: KeyboardEvent) => {
      // Não interceptar '?' quando outro modal está aberto: o Modal é portalado
      // para fora de #root, então o evento ainda chega em `document`. Sem esta
      // guarda o painel abriria por cima de outra UI modal e ainda chamaria
      // preventDefault sobre o '?'. (Quando o próprio painel está aberto,
      // isModalOpen() também é true; fechar é via ESC/Ctrl+?/overlay.)
      if (e.defaultPrevented || isModalOpen()) return;

      const target = e.target as HTMLElement;
      const isInputElement = target.tagName === 'INPUT' || target.tagName === 'TEXTAREA';

      if (e.key === '?' && !isInputElement) {
        e.preventDefault();
        useShortcutsHelpStore.getState().open();
      }
    };

    document.addEventListener('keypress', handleKeyPress);
    return () => document.removeEventListener('keypress', handleKeyPress);
  }, [isInteractiveSurface, shortcutsOpen]);

  useEffect(() => {
    const handleMessageUpdated = (data: unknown) => {
      if (!data || typeof data !== 'object' || !conversationId) return;
      const eventData = data as { conversationId?: unknown; messageId?: unknown; content?: unknown };
      if (eventData.conversationId !== conversationId || typeof eventData.messageId !== 'string' ||
          !isBackendId(eventData.messageId) || typeof eventData.content !== 'string') return;
      updateConversationMessage(conversationId, eventData.messageId, eventData.content);
    };

    const unsubscribe = EventsOn('message:updated', handleMessageUpdated);
    return () => {
      if (unsubscribe) unsubscribe();
    };
  }, [conversationId, updateConversationMessage]);

  useEffect(() => {
    const handleMessagePinChanged = (data: unknown) => {
      const eventData = data as { conversationId?: string; messageId?: string; pinned?: boolean };
      if (
        eventData.conversationId !== conversationId
        || !eventData.messageId
        || typeof eventData.pinned !== 'boolean'
      ) return;
      updateConversationMessagePinned(conversationId, eventData.messageId, eventData.pinned);
    };

    const unsubscribe = EventsOn('message:pin_changed', handleMessagePinChanged);
    return () => {
      unsubscribe();
    };
  }, [conversationId, updateConversationMessagePinned]);

  useEffect(() => {
    if (!conversationId) return;
    const unsubscribe = EventsOn('chat:skill_loaded', (data: unknown) => {
      const eventData = data as { conversationId?: string; displayName?: string; slug?: string };
      if (eventData.conversationId !== conversationId) return;
      const name = eventData.displayName || eventData.slug || '';
      if (!name) return;
      announce(t('chat.announce.skillLoaded', { name }));
    });
    return () => {
      if (unsubscribe) unsubscribe();
    };
  }, [announce, conversationId, t]);

  useEffect(() => {
    if (effectiveSendError && retryButtonRef.current) {
      retryButtonRef.current.focus();
    }
  }, [effectiveSendError]);

  useEffect(() => {
    const sessionFailureToAnnounce = sessionSendFailureMessage
      && sessionSendFailureMessage !== dismissedSessionSendError
      ? sessionSendFailureMessage
      : null;
    if (!sessionFailureToAnnounce) {
      lastAnnouncedSessionSendFailureRef.current = null;
      return;
    }
    if (
      sendError
      || sessionSendFailureAnnounced
      || lastAnnouncedSessionSendFailureRef.current === sessionFailureToAnnounce
    ) return;

    lastAnnouncedSessionSendFailureRef.current = sessionFailureToAnnounce;
    announce(sessionFailureToAnnounce, 'assertive');
  }, [announce, dismissedSessionSendError, sendError, sessionSendFailureAnnounced, sessionSendFailureMessage]);

  useEffect(() => {
    const windowState = session?.messageWindow;
    latestWindowKeyRef.current = windowState
      ? `${windowState.startIndex}:${windowState.endIndex}:${windowState.totalCount}`
      : null;
    const pendingAnnouncement = pendingWindowAnnouncementRef.current;
    if (!pendingAnnouncement) return;
    if (!windowState || windowState.totalCount <= 0) {
      pendingWindowAnnouncementRef.current = null;
      return;
    }
    // Não dá para saber se esta mudança de janela veio do carregamento pedido;
    // o prazo é o que garante que o aviso descreva aquela ação e não algo que
    // mexeu na janela muito depois. Dentro dele uma mudança alheia ainda pode
    // disparar o aviso, com números corretos e sem atropelar leitura, e isso é
    // preferível a perder o aviso de uma paginação que a pessoa pediu.
    if (Date.now() > pendingAnnouncement.expiresAt) {
      pendingWindowAnnouncementRef.current = null;
      return;
    }
    const didCompleteRequestedLoad =
      pendingAnnouncement.kind === 'older'
        ? windowState.startIndex < pendingAnnouncement.previousStartIndex || !windowState.hasBefore
        : pendingAnnouncement.kind === 'newer'
          ? windowState.endIndex > pendingAnnouncement.previousEndIndex || !windowState.hasAfter
          : pendingAnnouncement.kind === 'start'
            ? windowState.startIndex === 0
            : windowState.totalCount > 0 && windowState.endIndex >= windowState.totalCount - 1;
    if (!didCompleteRequestedLoad) return;
    pendingWindowAnnouncementRef.current = null;
    // Carregamento por scroll é automático e pode cair no fim de uma resposta:
    // vai como progresso para esperar a leitura do conteúdo terminar. Navegação
    // explícita é resposta a uma ação e não espera.
    const eventType = pendingAnnouncement.trigger === 'scroll' ? 'progress' : 'user-action';
    const message = usesLocalVisualWindowCount
      ? t('chat.announce.messageWindowLoaded', {
        start: 1,
        end: visibleMessageCount,
        total: visibleMessageCount,
      })
      : t('chat.announce.messageWindowLoaded', {
        start: windowState.startIndex + 1,
        end: windowState.endIndex + 1,
        total: windowState.totalCount,
      });
    announceRequest({
      message,
      eventType,
      // Sem origem o broker trataria a superfície como sempre ativa e falaria a
      // paginação de uma aba que a pessoa já deixou para trás.
      origin: { ...origin, conversationId: origin.conversationId ?? undefined },
      // Diferente de um "carregando", o intervalo carregado continua sendo o
      // que está na tela quando a leitura da resposta terminar.
      waitsForReading: true,
    });
  }, [announceRequest, origin, session?.messageWindow, t, usesLocalVisualWindowCount, visibleMessageCount]);

  useEffect(() => {
    if (!isInteractiveSurface) return;
    const handleEscape = (e: KeyboardEvent) => {
      // Com um modal aberto, o Escape fecha o modal; não descarta o banner de
      // erro na UI de fundo.
      if (isModalOpen()) return;
      if (e.key === 'Escape' && effectiveSendError) {
        setSendError(null);
        setLastFailedMessage(null);
        setDismissedSessionSendError(sessionSendFailureMessage);
        if (conversationId) clearConversationSendFailure(conversationId, origin.sessionKey);
        announce(t('chat.announce.errorDismissed'));
      }
    };

    document.addEventListener('keydown', handleEscape);
    return () => document.removeEventListener('keydown', handleEscape);
  }, [isInteractiveSurface, effectiveSendError, sessionSendFailureMessage, conversationId, origin.sessionKey, clearConversationSendFailure, announce, t]);

  const handleSendMessage = async (content: string, mediaFiles?: MediaFile[], options?: { voice?: boolean }) => {
    return requestMessaging('chat.message.send', { content, media: mediaFiles, voice: options?.voice === true });
  };

  const handleRetry = async () => {
    if (!effectiveFailedMessage || !sessionSendFailureRetryable) return;
    requestMessaging('chat.message.send', { content: effectiveFailedMessage.content, media: effectiveFailedMessage.media, recovery: true });
  };

  const handleReachEnd = useCallback(() => {
    inputRef.current?.focus();
  }, []);

  const runWindowLoad = useCallback(async (
    kind: 'start' | 'end' | 'older' | 'newer',
    trigger: MessageWindowLoadTrigger,
    load: () => Promise<void>,
    afterLoad?: () => void,
  ) => {
    const windowState = session?.messageWindow;
    const pending = {
      kind,
      trigger,
      // Teto para o caso de o carregamento nunca terminar: sem ele o pendente
      // ficaria armado para sempre e uma mudança de janela muito posterior
      // anunciaria uma paginação que ninguém pediu.
      expiresAt: Date.now() + PENDING_WINDOW_LOAD_MAX_MS,
      previousStartIndex: windowState?.startIndex ?? 0,
      previousEndIndex: windowState?.endIndex ?? -1,
      previousWindowKey: latestWindowKeyRef.current,
    };
    pendingWindowAnnouncementRef.current = pending;
    try {
      await load();
      afterLoad?.();
    } finally {
      // O prazo curto só começa quando o carregamento termina: backend lento não
      // pode custar o aviso de uma paginação que de fato aconteceu. Um
      // carregamento que não mexeu na janela expira sem anunciar nada. A
      // comparação é por identidade: um carregamento que já foi substituído por
      // outro não tem o que encurtar.
      if (pendingWindowAnnouncementRef.current === pending) {
        pending.expiresAt = Date.now() + PENDING_WINDOW_ANNOUNCEMENT_MAX_AGE_MS;
      }
    }
  }, [session?.messageWindow]);

  const handleJumpToStart = useCallback(() => runWindowLoad('start', 'navigation', loadStartMessages, () => {
    requestAnimationFrame(() => {
      const container = messagesContainerRef.current;
      const firstMessage = container?.querySelector('[data-message-node]') as HTMLElement | null;
      firstMessage?.focus();
    });
  }), [loadStartMessages, runWindowLoad]);

  const handleJumpToEnd = useCallback(() => runWindowLoad('end', 'navigation', loadEndMessages, () => {
    requestAnimationFrame(() => {
      const container = messagesContainerRef.current;
      const rootMessages = container?.querySelectorAll<HTMLElement>('[data-message-node][data-level="0"]');
      const lastMessage = rootMessages?.[rootMessages.length - 1] ?? null;
      lastMessage?.focus();
    });
  }), [loadEndMessages, runWindowLoad]);

  const handleLoadOlderMessages = useCallback(
    (trigger: MessageWindowLoadTrigger) => runWindowLoad('older', trigger, loadOlderMessages),
    [loadOlderMessages, runWindowLoad],
  );

  const handleLoadNewerMessages = useCallback(
    (trigger: MessageWindowLoadTrigger) => runWindowLoad('newer', trigger, loadNewerMessages),
    [loadNewerMessages, runWindowLoad],
  );

  const handleMessageContextMenu = useCallback(
    (event: React.MouseEvent, message: Message) => {
      const capturedMessage = { ...message, convertValues: message.convertValues };
      const root = rootRef.current;
      const instance = root && getChatMessageNavigationInstanceId(root, message.id);
      disposeMenuNavigation();
      if (instance) {
        const readPathname = () => messagingLive.current.pathname;
        const read = captureChatNavigationTarget(readPathname, 'chat.message.read.open', instance);
        const reasoning = captureChatNavigationTarget(readPathname, 'chat.message.reasoning.toggle', instance);
        menuNavigationTargets.current.set(capturedMessage, { read, reasoning });
        menuNavigationLeases.current = [read, reasoning].filter((target): target is ChatNavigationTarget => !!target);
      }
      showMenu(event, capturedMessage, message.role === 'user');
    },
    [disposeMenuNavigation, showMenu],
  );
  const messageListOrigin = useMemo(
    () => ({
      conversationId: origin.conversationId ?? undefined,
      sessionKey: origin.sessionKey,
      surfaceId: origin.surfaceId,
      surfaceType: origin.surfaceType,
      tabId: origin.tabId,
    }),
    [origin.conversationId, origin.sessionKey, origin.surfaceId, origin.surfaceType, origin.tabId],
  );

  const rootClass =
    variant === 'page' ? 'chat-page chat-session-view' : 'chat-session-view chat-session-view--embedded';

  return (
    <div className={rootClass} ref={rootRef}>
      <div className="ws-content-toolbar">
        <ChatToolbar
          inputRef={inputRef}
          conversationId={conversationId}
          enableShortcuts={isInteractiveSurface}
          onRequestConversationChange={onRequestConversationChange}
        />
      </div>
      <div className="ws-content-area">
        <MessageList
          threadedMessages={threadedMessages}
          messageWindow={session?.messageWindow}
          onLoadChildren={loadMessageChildren}
          onReachEnd={handleReachEnd}
          isLoading={isLoading}
          hasOlderMessages={hasOlderMessages}
          hasNewerMessages={hasNewerMessages}
          isLoadingOlderMessages={isLoadingOlderMessages}
          isLoadingMessageWindow={isLoadingMessageWindow}
          onLoadOlder={handleLoadOlderMessages}
          onLoadNewer={handleLoadNewerMessages}
          onJumpToStart={handleJumpToStart}
          onJumpToEnd={handleJumpToEnd}
          ref={messagesContainerRef}
          onContextMenu={handleMessageContextMenu}
          onSpeak={handleSpeakRequest}
          onCopy={(message, markdown) => { requestMessaging(markdown ? 'chat.message.copy_markdown' : 'chat.message.copy', { messageId: message.id }); }}
          onEdit={(message) => { requestMessaging('chat.message.edit.open', { messageId: message.id }); }}
          onSaveEdit={(message) => { requestMessaging('chat.message.edit.save', { messageId: message.id }); }}
          commandPathname={pathname}
          onDelete={handleDeleteMessage}
          editorTargets={editorTargets}
          onSendToEditor={sendToEditor}
          origin={messageListOrigin}
        />

        {effectiveSendError && (
          <Alert

            type="error"
            role="group"
            showIcon
            closable
            message={effectiveSendError}
            action={canRetryEffectiveSendError ? (
              <Button
                ref={retryButtonRef}
                size="small"
                danger
                onClick={handleRetry}
                aria-label={t('chat.retryAriaLabel')}
              >
                {t('chat.retry')}
              </Button>
            ) : undefined}
            onClose={() => {
              setSendError(null);
              setLastFailedMessage(null);
              setDismissedSessionSendError(sessionSendFailureMessage);
              if (conversationId) clearConversationSendFailure(conversationId, origin.sessionKey);
            }}
            style={{ flexShrink: 0 }}
          />
        )}

        <ChatInput
          clearOnSend={false}
          onSend={handleSendMessage}
          disabled={variant === 'embedded' ? false : isLoading}
          isStreaming={isLoading}
          onCancelStreaming={() => void handleCancelStreaming()}
          ref={inputRef}
          voiceEnabled={true}
          message={draftMessage}
          mediaFiles={draftMediaFiles}
          onMessageChange={setDraftMessage}
          onMediaFilesChange={setDraftMediaFiles}
          profileSlug={profileSlug || activeProfileSlug}
          agentCommands={agentCommands}
          onArrowUp={() => {
            const container = messagesContainerRef.current;
            if (!container) return false;
            // Durante o streaming a última mensagem de nível 0 é a que está em
            // curso; focá-la diretamente (em vez de `:last-child`, frágil quando
            // a lista é virtualizada ou há nós auxiliares no fim) garante entrar
            // na lista de forma navegável. (Issue #178)
            const rootMessages = container.querySelectorAll<HTMLElement>('[data-message-node][data-level="0"]');
            const lastMessage = rootMessages[rootMessages.length - 1] ?? null;
            if (!lastMessage) return false;
            lastMessage.focus();
            return document.activeElement === lastMessage;
          }}
        />
      </div>

      <ContextMenu
        restoreFocusOnClose={false}
        visible={menuVisible}
        items={menuItems}
        x={menuPosition.x}
        y={menuPosition.y}
        onClose={hideMenu}
        ariaLabel={t('chat.contextMenuAriaLabel')}
      />
    </div>
  );
}
