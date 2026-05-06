import { useRef, useState, useEffect, useCallback, useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import { Alert, Button } from 'antd';
import { useEditorStore } from '../../store/editorStore';
import { useWorkspaceStore } from '../../store/workspaceStore';
import { ttsService } from '../../services/tts';
import { MessageList } from './MessageList';
import { ChatInput } from './ChatInput';
import { ChatToolbar } from './ChatToolbar';
import { ChatSessionProvider } from './ChatSessionContext';
import type {
  ChatSurfaceIdentity,
  ChatSurfaceOrigin,
} from '../../services/chatSessionRegistry';
import { ContextMenu } from '../menu';
import { KeyboardShortcutsHelp } from '../ui/KeyboardShortcutsHelp';
import { useWorkspacePanel } from '../workspace/WorkspacePanelContext';
import { useChatKeyboardNav } from '../../hooks/useChatKeyboardNav';
import { useContextMenu, useMessageActions } from '../../hooks/useContextMenu';
import { isBackendId } from '../../lib/idUtils';
import type { MediaFile } from '../../services/mediaService';
import { DeleteMessage, EditorGetDraftPath } from '@wailsjs/go/app/App';
import { EventsOn } from '@wailsjs/runtime/runtime';
import { announce } from '../../hooks/useAnnouncer';
import { handleError, ErrorSeverity, ErrorMessages } from '../../utils/errorHandler';
import type { EditorSendTargetOption, SendToEditorPayload } from '../../lib/editorSendMenu';
import {
  useChatSurfaceController,
  type ChatSurfaceController,
} from './ChatSurfaceController';
import './ChatSessionView.css';

export interface ChatSessionViewProps {
  variant?: 'page' | 'embedded';
  surface: ChatSurfaceIdentity;
  /** Envio da mensagem (ex.: sendMessage da store ou adaptador do chat modal) */
  onSend: (content: string, mediaFiles: MediaFile[] | undefined, origin: ChatSurfaceOrigin) => Promise<void>;
  showShortcutsHelp?: boolean;
}

export function ChatSessionView({
  variant = 'page',
  surface,
  onSend,
  showShortcutsHelp,
}: ChatSessionViewProps) {
  return (
    <ChatSessionProvider surface={surface}>
      <ChatSessionViewControllerBridge
        variant={variant}
        surface={surface}
        onSend={onSend}
        showShortcutsHelp={showShortcutsHelp}
      />
    </ChatSessionProvider>
  );
}

function ChatSessionViewControllerBridge({
  onSend,
  variant,
  showShortcutsHelp,
}: ChatSessionViewProps) {
  const controller = useChatSurfaceController({
    onSend: (content, mediaFiles, context) => onSend(content, mediaFiles, context.origin),
  });

  return (
    <ChatSessionViewContent
      variant={variant}
      showShortcutsHelp={showShortcutsHelp}
      controller={controller}
    />
  );
}

interface ChatSessionViewContentProps extends Pick<ChatSessionViewProps, 'variant' | 'showShortcutsHelp'> {
  controller: ChatSurfaceController;
}

function ChatSessionViewContent({
  variant = 'page',
  showShortcutsHelp,
  controller,
}: ChatSessionViewContentProps) {
  const { t } = useTranslation();
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
    previousStartIndex: number;
    previousEndIndex: number;
    previousWindowKey: string | null;
  } | null>(null);
  const latestWindowKeyRef = useRef<string | null>(null);
  const { isActive: isPanelActive } = useWorkspacePanel();
  const isInteractiveSurface = variant === 'embedded' || isPanelActive;

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
    retryMessageToConversation,
    updateConversationMessage,
    toggleConversationReasoningExpanded,
    isConversationReasoningExpanded,
    startConversationEditing,
    startConversationReading,
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
      if (message.role === 'tool' || processedTurnIds.has(turnId)) continue;
      processedTurnIds.add(turnId);
      count += 1;
    }
    return count;
  }, [threadedMessages]);
  const usesLocalVisualWindowCount = visibleMessageCount > 0 && visibleMessageCount !== threadedMessages.length;

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
  const [shortcutsHelpOpen, setShortcutsHelpOpen] = useState(false);

  const [lastFailedMessage, setLastFailedMessage] = useState<{ content: string; media?: MediaFile[] } | null>(null);
  const [sendError, setSendError] = useState<string | null>(null);

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

  const { copyMessage, speakMessage } = useMessageActions({
    onAnnounce: announce,
  });

  const handleDeleteMessage = useCallback(
    async (message: { id: string | number }) => {
      const messageId = String(message.id);
      if (!isBackendId(messageId)) return;
      try {
        await DeleteMessage(messageId);
        announce(t('chat.announce.messageDeleted'));
        const conv = getSessionConversation();
        if (conv?.id) {
          await loadConversationSession(conv.id, { refreshSurfaceWindows: true });
        }
      } catch (error) {
        const errorMessage = error instanceof Error ? error.message : String(error);
        const lower = errorMessage.toLowerCase();
        const userCanceled =
          lower.includes('cancelada') ||
          lower.includes('cancelado') ||
          lower.includes('canceled') ||
          lower.includes('cancelled');
        if (userCanceled) {
          announce(t('chat.announce.deleteCancelled'));
          return;
        }

        handleError(error, {
          source: 'ChatSessionView.onDelete',
          userMessage: ErrorMessages.CHAT.DELETE_FAILED,
          severity: ErrorSeverity.RECOVERABLE,
          metadata: { messageId },
        });
      }
    },
    [announce, conversationId, getSessionConversation, loadConversationSession, t, variant],
  );

  const sendToEditor = useCallback(
    async (payload: SendToEditorPayload) => {
      const content = String(payload?.content ?? '');
      if (!content) return;

      const title = payload.title || t('editor.fallback.fromChat');
      const { addTab, setActiveTab } = useWorkspaceStore.getState();
      const ensureActiveEditorTab = async (tabId: string) => {
        await setActiveTab(tabId);
        return useWorkspaceStore.getState().workspace?.activeTabId === tabId;
      };
      const createDraftEditorTab = async () => {
        const draftId =
          typeof crypto !== 'undefined' && crypto.randomUUID
            ? crypto.randomUUID()
            : `editor-${Date.now()}`;
        const draftPath = String((await EditorGetDraftPath(draftId)) ?? '');
        const tabId = await addTab('editor', title, { filePath: draftPath, draftId });
        const activated = await ensureActiveEditorTab(tabId);
        if (!activated) return null;
        useEditorStore.getState().createDocument({
          id: tabId,
          title,
          markdown: '',
          mode: 'markdown',
          filePath: draftPath,
          draftId,
        });
        return tabId;
      };

      if (payload.target === 'new_document') {
        const tabId = await createDraftEditorTab();
        if (!tabId) return;
        useEditorStore.getState().requestInsert({
          target: 'document',
          targetDocumentId: tabId,
          format: payload.format,
          title,
          content,
          focus: true,
        });
        return;
      }

      const targetDocumentId = String(payload.targetDocumentId || '').trim();
      if (!targetDocumentId) return;

      const activated = await ensureActiveEditorTab(targetDocumentId);
      if (!activated) return;

      useEditorStore.getState().requestInsert({
        target: 'document',
        targetDocumentId,
        format: payload.format,
        title,
        content,
        focus: true,
      });
    },
    [t],
  );

  const { menuVisible, menuPosition, menuItems, showMenu, hideMenu } = useContextMenu({
    sessionKey: origin.sessionKey,
    onCopy: copyMessage,
    onReadMessage: (message) => {
      if (conversation?.id) {
        startConversationReading(conversation.id, message.id);
      }
    },
    onSpeak: speakMessage,
    onEdit: (message) => {
      if (conversation?.id) {
        startConversationEditing(conversation.id, message.id);
      }
    },
    onResend: async (message) => {
      const conversationId = getSessionConversation()?.id;
      if (!conversationId || !isBackendId(message.id)) return;
      await retryMessageToConversation(conversationId, message.id, undefined, { origin });
      announce(t('chat.announce.messageResent'));
    },
    onDelete: handleDeleteMessage,
    onSendToEditor: sendToEditor,
    editorTargets,
    onPin: (_message) => {
      announce(t('chat.announce.pinComingSoon'));
    },
    onToggleReasoning: (message) => {
      const targetConversationId = conversation?.id;
      if (!targetConversationId) return;
      const isExpanded = isConversationReasoningExpanded(targetConversationId, message.id);
      toggleConversationReasoningExpanded(targetConversationId, message.id);
      announce(isExpanded ? t('chat.reasoningHidden') : t('chat.reasoningShown'));
    },
    isReasoningExpanded: (messageId: string) => (
      conversation?.id
        ? isConversationReasoningExpanded(conversation.id, messageId)
        : false
    ),
    isTTSDisabled,
  });

  useChatKeyboardNav({
    enabled: isInteractiveSurface,
    inputRef,
    messagesContainerRef,
  });

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
      const target = e.target as HTMLElement;
      const isInputElement = target.tagName === 'INPUT' || target.tagName === 'TEXTAREA';

      if (e.key === '?' && !isInputElement && !shortcutsHelpOpen) {
        e.preventDefault();
        setShortcutsHelpOpen(true);
      }
    };

    document.addEventListener('keypress', handleKeyPress);
    return () => document.removeEventListener('keypress', handleKeyPress);
  }, [isInteractiveSurface, shortcutsHelpOpen, shortcutsOpen]);

  useEffect(() => {
    const handleMessageUpdated = (data: unknown) => {
      const eventData = data as { message_id?: number | string; content?: string };
      if (eventData.message_id && eventData.content !== undefined && conversationId) {
        updateConversationMessage(conversationId, String(eventData.message_id), eventData.content);
      }
    };

    const unsubscribe = EventsOn('message:updated', handleMessageUpdated);
    return () => {
      if (unsubscribe) unsubscribe();
    };
  }, [conversationId, updateConversationMessage]);

  useEffect(() => {
    if (sendError && retryButtonRef.current) {
      retryButtonRef.current.focus();
    }
  }, [sendError]);

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
    if (usesLocalVisualWindowCount) {
      announce(t('chat.announce.messageWindowLoaded', {
        start: 1,
        end: visibleMessageCount,
        total: visibleMessageCount,
      }));
      return;
    }
    announce(t('chat.announce.messageWindowLoaded', {
      start: windowState.startIndex + 1,
      end: windowState.endIndex + 1,
      total: windowState.totalCount,
    }));
  }, [announce, session?.messageWindow, t, usesLocalVisualWindowCount, visibleMessageCount]);

  useEffect(() => {
    if (!isInteractiveSurface) return;
    const handleEscape = (e: KeyboardEvent) => {
      if (e.key === 'Escape' && sendError) {
        setSendError(null);
        setLastFailedMessage(null);
        announce(t('chat.announce.errorDismissed'));
      }
    };

    document.addEventListener('keydown', handleEscape);
    return () => document.removeEventListener('keydown', handleEscape);
  }, [isInteractiveSurface, sendError, announce, t]);

  const handleSendMessage = async (content: string, mediaFiles?: MediaFile[]) => {
    try {
      setSendError(null);
      setLastFailedMessage(null);
      await controller.sendMessage(content, mediaFiles);
    } catch (error: unknown) {
      const errorMessage = error instanceof Error ? error.message : String(error);
      console.error('[ChatSessionView] send error:', errorMessage);
      setLastFailedMessage({ content, media: mediaFiles });
      setSendError(ErrorMessages.CHAT.SEND_FAILED);

      handleError(error, {
        source: 'ChatSessionView.handleSendMessage',
        userMessage: ErrorMessages.CHAT.SEND_FAILED,
        severity: ErrorSeverity.RECOVERABLE,
        onRetry: () => handleRetry(),
      });
    }
  };

  const handleRetry = async () => {
    if (!lastFailedMessage) return;

    try {
      setSendError(null);
      await controller.sendMessage(lastFailedMessage.content, lastFailedMessage.media);
      setLastFailedMessage(null);
    } catch (error) {
      handleError(error, {
        source: 'ChatSessionView.handleRetry',
        userMessage: ErrorMessages.CHAT.SEND_FAILED,
        severity: ErrorSeverity.RECOVERABLE,
      });
    }
  };

  const handleReachEnd = () => {
    inputRef.current?.focus();
  };

  const handleJumpToStart = async () => {
    const previousWindowKey = latestWindowKeyRef.current;
    const windowState = session?.messageWindow;
    pendingWindowAnnouncementRef.current = {
      kind: 'start',
      previousStartIndex: windowState?.startIndex ?? 0,
      previousEndIndex: windowState?.endIndex ?? -1,
      previousWindowKey,
    };
    try {
      await loadStartMessages();
      requestAnimationFrame(() => {
        const container = messagesContainerRef.current;
        const firstMessage = container?.querySelector('[data-message-node]') as HTMLElement | null;
        firstMessage?.focus();
      });
    } finally {
      window.setTimeout(() => {
        if (pendingWindowAnnouncementRef.current?.previousWindowKey === latestWindowKeyRef.current) {
          pendingWindowAnnouncementRef.current = null;
        }
      }, 1_000);
    }
  };

  const handleJumpToEnd = async () => {
    const previousWindowKey = latestWindowKeyRef.current;
    const windowState = session?.messageWindow;
    pendingWindowAnnouncementRef.current = {
      kind: 'end',
      previousStartIndex: windowState?.startIndex ?? 0,
      previousEndIndex: windowState?.endIndex ?? -1,
      previousWindowKey,
    };
    try {
      await loadEndMessages();
      requestAnimationFrame(() => {
        const container = messagesContainerRef.current;
        const rootMessages = container?.querySelectorAll<HTMLElement>('[data-message-node][data-level="0"]');
        const lastMessage = rootMessages?.[rootMessages.length - 1] ?? null;
        lastMessage?.focus();
      });
    } finally {
      window.setTimeout(() => {
        if (pendingWindowAnnouncementRef.current?.previousWindowKey === latestWindowKeyRef.current) {
          pendingWindowAnnouncementRef.current = null;
        }
      }, 1_000);
    }
  };

  const handleLoadOlderMessages = useCallback(async () => {
    const previousWindowKey = latestWindowKeyRef.current;
    const windowState = session?.messageWindow;
    pendingWindowAnnouncementRef.current = {
      kind: 'older',
      previousStartIndex: windowState?.startIndex ?? 0,
      previousEndIndex: windowState?.endIndex ?? -1,
      previousWindowKey,
    };
    try {
      await loadOlderMessages();
    } finally {
      window.setTimeout(() => {
        if (pendingWindowAnnouncementRef.current?.previousWindowKey === latestWindowKeyRef.current) {
          pendingWindowAnnouncementRef.current = null;
        }
      }, 1_000);
    }
  }, [loadOlderMessages, session?.messageWindow]);

  const handleLoadNewerMessages = useCallback(async () => {
    const previousWindowKey = latestWindowKeyRef.current;
    const windowState = session?.messageWindow;
    pendingWindowAnnouncementRef.current = {
      kind: 'newer',
      previousStartIndex: windowState?.startIndex ?? 0,
      previousEndIndex: windowState?.endIndex ?? -1,
      previousWindowKey,
    };
    try {
      await loadNewerMessages();
    } finally {
      window.setTimeout(() => {
        if (pendingWindowAnnouncementRef.current?.previousWindowKey === latestWindowKeyRef.current) {
          pendingWindowAnnouncementRef.current = null;
        }
      }, 1_000);
    }
  }, [loadNewerMessages, session?.messageWindow]);

  const rootClass =
    variant === 'page' ? 'chat-page chat-session-view' : 'chat-session-view chat-session-view--embedded';

  return (
    <div className={rootClass}>
      <div className="ws-content-toolbar">
        <ChatToolbar inputRef={inputRef} conversationId={conversationId} enableShortcuts={isInteractiveSurface} />
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
          onContextMenu={(event, message) => showMenu(event, message, message.role === 'user')}
          onSpeak={hasVoiceConfig ? speakMessage : undefined}
          onDelete={handleDeleteMessage}
          editorTargets={editorTargets}
          onSendToEditor={sendToEditor}
        />

        {sendError && lastFailedMessage && (
          <Alert
            role="alert"
            type="error"
            showIcon
            closable
            message={sendError}
            action={
              <Button
                ref={retryButtonRef}
                size="small"
                danger
                onClick={handleRetry}
                aria-label={t('chat.retryAriaLabel')}
              >
                {t('chat.retry')}
              </Button>
            }
            onClose={() => {
              setSendError(null);
              setLastFailedMessage(null);
            }}
            style={{ flexShrink: 0 }}
          />
        )}

        <ChatInput
          onSend={handleSendMessage}
          disabled={variant === 'embedded' ? false : isLoading}
          ref={inputRef}
          voiceEnabled={true}
          message={draftMessage}
          mediaFiles={draftMediaFiles}
          onMessageChange={setDraftMessage}
          onMediaFilesChange={setDraftMediaFiles}
          onArrowUp={() => {
            const container = messagesContainerRef.current;
            if (container) {
              const lastMessage = container.querySelector('[data-message-node]:last-child') as HTMLElement;
              if (lastMessage) {
                lastMessage.focus();
              } else {
                container.focus();
              }
            }
          }}
        />
      </div>

      <ContextMenu
        visible={menuVisible}
        items={menuItems}
        x={menuPosition.x}
        y={menuPosition.y}
        onClose={hideMenu}
        ariaLabel={t('chat.contextMenuAriaLabel')}
      />

      {shortcutsOpen && (
        <KeyboardShortcutsHelp isOpen={shortcutsHelpOpen} onClose={() => setShortcutsHelpOpen(false)} />
      )}
    </div>
  );
}
