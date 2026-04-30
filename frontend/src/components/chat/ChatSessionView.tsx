import { useRef, useState, useEffect, useCallback, useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import { Alert, Button } from 'antd';
import { useChatStore } from '../../store/chatStore';
import { useEditorStore } from '../../store/editorStore';
import { useWorkspaceStore } from '../../store/workspaceStore';
import { ttsService } from '../../services/tts';
import { MessageList } from './MessageList';
import { ChatInput } from './ChatInput';
import { ChatToolbar } from './ChatToolbar';
import { ContextMenu } from '../menu';
import { KeyboardShortcutsHelp } from '../ui/KeyboardShortcutsHelp';
import { useWorkspacePanel } from '../workspace/WorkspacePanelContext';
import { useChatKeyboardNav } from '../../hooks/useChatKeyboardNav';
import { useTabScrollState } from '../../hooks/useTabScrollState';
import { useContextMenu, useMessageActions } from '../../hooks/useContextMenu';
import { isBackendId } from '../../lib/idUtils';
import type { MediaFile } from '../../services/mediaService';
import { DeleteMessage, EditorGetDraftPath } from '@wailsjs/go/app/App';
import { EventsOn } from '@wailsjs/runtime/runtime';
import { announce } from '../../hooks/useAnnouncer';
import { handleError, ErrorSeverity, ErrorMessages } from '../../utils/errorHandler';
import type { EditorSendTargetOption, SendToEditorPayload } from '../../lib/editorSendMenu';
import './ChatSessionView.css';

const EMPTY_MESSAGES: never[] = [];

export interface ChatSessionViewProps {
  variant?: 'page' | 'embedded';
  conversationId?: string | null;
  /** Envio da mensagem (ex.: sendMessage da store ou adaptador do chat modal) */
  onSend: (content: string, mediaFiles?: MediaFile[]) => Promise<void>;
  showShortcutsHelp?: boolean;
}

export function ChatSessionView({
  variant = 'page',
  conversationId,
  onSend,
  showShortcutsHelp,
}: ChatSessionViewProps) {
  const { t } = useTranslation();
  const inputRef = useRef<HTMLTextAreaElement>(null);
  const messagesContainerRef = useRef<HTMLDivElement>(null);
  useTabScrollState(messagesContainerRef);
  const hasAutoFocusedRef = useRef(false);
  const retryButtonRef = useRef<HTMLButtonElement>(null);
  const wasLoadingRef = useRef(false);
  const { isActive: isPanelActive } = useWorkspacePanel();
  const isInteractiveSurface = variant === 'embedded' || isPanelActive;

  const session = useChatStore((s) => conversationId ? s.sessionsByConversationId[conversationId] ?? null : null);
  const activeConversation = session?.conversation ?? null;
  const threadedMessages = activeConversation?.threadedMessages ?? EMPTY_MESSAGES;
  const isLoading = session?.isLoading ?? false;
  const hasOlderMessages = session?.hasOlderMessages ?? false;
  const isLoadingOlderMessages = session?.isLoadingOlderMessages ?? false;
  const loadOlderMessagesForConversation = useChatStore((s) => s.loadOlderMessagesForConversation);
  const loadOlderMessages = useCallback(async () => {
    if (conversationId) {
      await loadOlderMessagesForConversation(conversationId);
      return;
    }
    await useChatStore.getState().loadOlderMessages();
  }, [conversationId, loadOlderMessagesForConversation]);
  const loadMessageChildren = useChatStore((s) => s.loadMessageChildren);
  const loadConversationSession = useChatStore((s) => s.loadConversationSession);
  const retryMessageToConversation = useChatStore((s) => s.retryMessageToConversation);
  const updateMessage = useChatStore((s) => s.updateMessage);
  const updateConversationMessage = useChatStore((s) => s.updateConversationMessage);
  const toggleConversationReasoningExpanded = useChatStore((s) => s.toggleConversationReasoningExpanded);
  const isConversationReasoningExpanded = useChatStore((s) => s.isConversationReasoningExpanded);
  const getActiveConversation = useCallback(() => activeConversation, [activeConversation]);

  useEffect(() => {
    if (!conversationId) return;
    if (session?.conversation) return;
    void loadConversationSession(conversationId, { activate: variant === 'page' });
  }, [conversationId, loadConversationSession, session?.conversation, variant]);

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

  const startConversationEditing = useChatStore((state) => state.startConversationEditing);
  const startConversationReading = useChatStore((state) => state.startConversationReading);
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
        const conv = getActiveConversation();
        if (conv?.id) {
          await loadConversationSession(conv.id, { activate: !conversationId || variant === 'page' });
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
    [announce, conversationId, getActiveConversation, loadConversationSession, t, variant],
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
    onCopy: copyMessage,
    onReadMessage: (message) => {
      if (activeConversation?.id) {
        startConversationReading(activeConversation.id, message.id);
      }
    },
    onSpeak: speakMessage,
    onEdit: (message) => {
      if (activeConversation?.id) {
        startConversationEditing(activeConversation.id, message.id);
      }
    },
    onResend: async (message) => {
      const conversationId = getActiveConversation()?.id;
      if (!conversationId || !isBackendId(message.id)) return;
      await retryMessageToConversation(conversationId, message.id);
      announce(t('chat.announce.messageResent'));
    },
    onDelete: handleDeleteMessage,
    onSendToEditor: sendToEditor,
    editorTargets,
    onPin: (_message) => {
      announce(t('chat.announce.pinComingSoon'));
    },
    onToggleReasoning: (message) => {
      const targetConversationId = activeConversation?.id;
      if (!targetConversationId) return;
      const isExpanded = isConversationReasoningExpanded(targetConversationId, message.id);
      toggleConversationReasoningExpanded(targetConversationId, message.id);
      announce(isExpanded ? t('chat.reasoningHidden') : t('chat.reasoningShown'));
    },
    isReasoningExpanded: (messageId: string) => (
      activeConversation?.id
        ? isConversationReasoningExpanded(activeConversation.id, messageId)
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
      if (eventData.message_id && eventData.content !== undefined) {
        if (conversationId) {
          updateConversationMessage(conversationId, String(eventData.message_id), eventData.content);
        } else {
          updateMessage(String(eventData.message_id), eventData.content);
        }
      }
    };

    const unsubscribe = EventsOn('message:updated', handleMessageUpdated);
    return () => {
      if (unsubscribe) unsubscribe();
    };
  }, [conversationId, updateConversationMessage, updateMessage]);

  useEffect(() => {
    if (sendError && retryButtonRef.current) {
      retryButtonRef.current.focus();
    }
  }, [sendError]);

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
      await onSend(content, mediaFiles);
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
      await onSend(lastFailedMessage.content, lastFailedMessage.media);
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
          onLoadChildren={loadMessageChildren}
          onReachEnd={handleReachEnd}
          isLoading={isLoading}
          hasOlderMessages={hasOlderMessages}
          isLoadingOlderMessages={isLoadingOlderMessages}
          onLoadOlder={loadOlderMessages}
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
