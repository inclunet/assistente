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
import { useChatKeyboardNav } from '../../hooks/useChatKeyboardNav';
import { useTabScrollState } from '../../hooks/useTabScrollState';
import { useContextMenu, useMessageActions } from '../../hooks/useContextMenu';
import type { MediaFile } from '../../services/mediaService';
import { DeleteMessage, EditorGetDraftPath } from '@wailsjs/go/app/App';
import { EventsOn } from '@wailsjs/runtime/runtime';
import { announce } from '../../hooks/useAnnouncer';
import { handleError, ErrorSeverity, ErrorMessages } from '../../utils/errorHandler';
import type { EditorSendTargetOption, SendToEditorPayload } from '../../lib/editorSendMenu';
import './ChatSessionView.css';

export interface ChatSessionViewProps {
  variant?: 'page' | 'embedded';
  /** Envio da mensagem (ex.: sendMessage da store ou adaptador do chat modal) */
  onSend: (content: string, mediaFiles?: MediaFile[]) => Promise<void>;
  showShortcutsHelp?: boolean;
}

export function ChatSessionView({
  variant = 'page',
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

  const isLoading = useChatStore((s) => s.isLoading);
  const activeConversation = useChatStore((s) => s.activeConversation);
  const threadedMessages = useChatStore((s) => s.activeConversation?.threadedMessages) || [];
  const loadMessageChildren = useChatStore((s) => s.loadMessageChildren);
  const loadConversation = useChatStore((s) => s.loadConversation);
  const retryMessageToConversation = useChatStore((s) => s.retryMessageToConversation);
  const updateMessage = useChatStore((s) => s.updateMessage);
  const toggleReasoningExpanded = useChatStore((s) => s.toggleReasoningExpanded);
  const isReasoningExpanded = useChatStore((s) => s.isReasoningExpanded);
  const getActiveConversation = useCallback(() => activeConversation, [activeConversation]);
  const getThreadedMessages = useCallback(() => threadedMessages, [threadedMessages]);

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

  const startEditing = useChatStore((state) => state.startEditing);
  const startReading = useChatStore((state) => state.startReading);
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
      let messageId: number | null = null;
      try {
        const parsedId = typeof message.id === 'number' ? message.id : parseInt(String(message.id), 10);
        messageId = Number.isNaN(parsedId) ? null : parsedId;
        if (messageId !== null) {
          await DeleteMessage(messageId);
          announce(t('chat.announce.messageDeleted'));
          const conv = getActiveConversation();
          if (conv?.id) {
            await loadConversation(conv.id);
          }
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
    [announce, getActiveConversation, loadConversation, t],
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
      startReading(message.id);
    },
    onSpeak: speakMessage,
    onEdit: (message) => {
      startEditing(message.id);
    },
    onResend: async (message) => {
      const conversationId = getActiveConversation()?.id;
      const messageId = typeof message.id === 'number' ? message.id : parseInt(String(message.id), 10);
      if (!conversationId || Number.isNaN(messageId)) return;
      await retryMessageToConversation(conversationId, messageId);
      announce(t('chat.announce.messageResent'));
    },
    onDelete: handleDeleteMessage,
    onSendToEditor: (payload) => sendToEditor(payload),
    editorTargets,
    onPin: (_message) => {
      announce(t('chat.announce.pinComingSoon'));
    },
    onToggleReasoning: (message) => {
      toggleReasoningExpanded(message.id);
      const isExpanded = isReasoningExpanded(message.id);
      announce(isExpanded ? t('chat.reasoningHidden') : t('chat.reasoningShown'));
    },
    isReasoningExpanded: (messageId: string) => isReasoningExpanded(messageId),
    isTTSDisabled,
  });

  useChatKeyboardNav({
    enabled: true,
    inputRef,
    messagesContainerRef,
  });

  useEffect(() => {
    if (variant !== 'page') return;
    const checkTimer = setInterval(() => {
      const inputElement = inputRef.current;
      if (inputElement && !hasAutoFocusedRef.current) {
        hasAutoFocusedRef.current = true;
        clearInterval(checkTimer);
        setTimeout(() => {
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
    };
  }, [variant]);

  useEffect(() => {
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
  }, [isLoading]);

  useEffect(() => {
    if (!shortcutsOpen) return;
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
  }, [shortcutsHelpOpen, shortcutsOpen]);

  useEffect(() => {
    const handleMessageUpdated = (data: unknown) => {
      const eventData = data as { message_id?: number | string; content?: string };
      if (eventData.message_id && eventData.content !== undefined) {
        updateMessage(String(eventData.message_id), eventData.content);
      }
    };

    const unsubscribe = EventsOn('message:updated', handleMessageUpdated);
    return () => {
      if (unsubscribe) unsubscribe();
    };
  }, [updateMessage]);

  useEffect(() => {
    if (sendError && retryButtonRef.current) {
      retryButtonRef.current.focus();
    }
  }, [sendError]);

  useEffect(() => {
    const handleEscape = (e: KeyboardEvent) => {
      if (e.key === 'Escape' && sendError) {
        setSendError(null);
        setLastFailedMessage(null);
        announce(t('chat.announce.errorDismissed'));
      }
    };

    document.addEventListener('keydown', handleEscape);
    return () => document.removeEventListener('keydown', handleEscape);
  }, [sendError, announce, t]);

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
        <ChatToolbar inputRef={inputRef} />
      </div>
      <div className="ws-content-area">
        <MessageList
          threadedMessages={threadedMessages}
          onLoadChildren={loadMessageChildren}
          onReachEnd={handleReachEnd}
          isLoading={isLoading}
          ref={messagesContainerRef}
          onContextMenu={(event, message) => showMenu(event, message, message.role === 'user')}
          onSpeak={hasVoiceConfig ? speakMessage : undefined}
          onDelete={handleDeleteMessage}
          editorTargets={editorTargets}
          onSendToEditor={(payload) => sendToEditor(payload)}
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
