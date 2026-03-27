import { useEffect, useRef, useState, useCallback } from 'react';
import { ClearOutlined, PlusOutlined } from '@ant-design/icons';
import { useTranslation } from 'react-i18next';
import { Modal } from '../ui/Modal';
import { MessageList } from '../chat/MessageList';
import { ChatInput } from '../chat/ChatInput';
import { ContextMenu } from '../menu';
import { Toolbar } from '../ui/Toolbar';
import { useChatStore, type Message } from '../../store/chatStore';
import type { MediaFile } from '../../services/mediaService';
import { useContextMenu, useMessageActions } from '../../hooks/useContextMenu';
import { ClearConversation, DeleteMessage } from '@wailsjs/go/main/App';
import { announce } from '../../hooks/useAnnouncer';

import './EditorInlineChatModal.css';

export interface EditorInlineChatPatch {
  replacement: string;
  notes?: string;
  format?: 'markdown' | 'html' | string;
}

export interface EditorInlineChatModalProps {
  isOpen: boolean;
  title?: string;
  selectedText: string;
  error: string | null;
  focusNonce?: number;
  onClose: () => void;
  onSend: (instruction: string, mediaFiles?: MediaFile[]) => Promise<void>;
}

export function EditorInlineChatModal({
  isOpen,
  title,
  selectedText,
  error,
  focusNonce,
  onClose,
  onSend,
}: EditorInlineChatModalProps) {
  const { t } = useTranslation();
  const inputRef = useRef<HTMLTextAreaElement>(null);
  const messagesContainerRef = useRef<HTMLDivElement>(null);

  const { isLoading, getThreadedMessages, loadMessageChildren, getActiveConversation, loadConversation, createConversation } = useChatStore();

  const activeConversation = getActiveConversation();
  const conversationTitle = activeConversation?.title || t('editor.inlineChat.conversation');

  const { copyMessage, speakMessage } = useMessageActions({
    onAnnounce: (msg) => announce(msg),
  });

  const [deleteBusyId, setDeleteBusyId] = useState<string | null>(null);
  const handleDeleteMessage = useCallback(
    async (message: Message) => {
      const msgId = String(message?.id || '');
      if (!msgId) return;

      try {
        setDeleteBusyId(msgId);
        const numericId = parseInt(msgId, 10);
        if (!isNaN(numericId)) {
          await DeleteMessage(numericId);
          announce(t('editor.inlineChat.messageDeleted'));
          const conv = getActiveConversation();
          if (conv?.id) {
            await loadConversation(conv.id);
          }
        }
      } catch (e: unknown) {
        console.error('[EditorInlineChatModal] delete error:', e);
        announce(t('editor.inlineChat.deleteError'), 'assertive');
      } finally {
        setDeleteBusyId(null);
      }
    },
    [getActiveConversation, loadConversation, t]
  );

  const { showMenu, hideMenu, menuItems, menuPosition, menuVisible } = useContextMenu({
    onCopy: copyMessage,
    onReadMessage: (m) => useChatStore.getState().startReading(String(m.id)),
    onSpeak: speakMessage,
    onEdit: (m) => useChatStore.getState().startEditing(String(m.id)),
    onResend: async (m) => {
      if (m.content) {
        await useChatStore.getState().sendMessage(String(m.content));
        announce(t('editor.inlineChat.messageResent'));
      }
    },
    onDelete: handleDeleteMessage,
    onPin: () => null,
    onToggleReasoning: (m) => {
      useChatStore.getState().toggleReasoningExpanded(String(m.id));
    },
    isReasoningExpanded: (messageId: string) => useChatStore.getState().isReasoningExpanded(messageId),
    isTTSDisabled: false,
    onAnnounce: (msg) => announce(msg),
  });

  const threadedMessages = getThreadedMessages() || [];

  const focusInput = useCallback(() => {
    requestAnimationFrame(() => inputRef.current?.focus());
  }, []);

  const handleNewConversation = useCallback(async () => {
    try {
      await createConversation(t('editor.inlineChat.newConversationTitle'));
      announce(t('editor.inlineChat.newConversation'));
    } catch (e) {
      console.error('[EditorInlineChatModal] new conversation error:', e);
      announce(t('editor.inlineChat.newConversationError'), 'assertive');
    } finally {
      focusInput();
    }
  }, [focusInput, createConversation]);

  const handleClearConversation = useCallback(async () => {
    const conv = getActiveConversation();
    try {
      if (conv?.id) {
        await ClearConversation(conv.id);
        await loadConversation(conv.id);
        announce(t('editor.inlineChat.conversationCleared'));
      } else {
        useChatStore.getState().clearMessages();
        announce(t('editor.inlineChat.conversationCleared'));
      }
    } catch (e) {
      console.error('[EditorInlineChatModal] clear conversation error:', e);
      announce(t('editor.inlineChat.clearError'), 'assertive');
    } finally {
      focusInput();
    }
  }, [focusInput, getActiveConversation, loadConversation]);

  const handleKeyDownCapture = useCallback((e: React.KeyboardEvent) => {
    if (!e.ctrlKey) return;

    const key = e.key.toLowerCase();
    if (key === 'n') {
      e.preventDefault();
      e.stopPropagation();
      void handleNewConversation();
      return;
    }

    if (key === 'l') {
      e.preventDefault();
      e.stopPropagation();
      void handleClearConversation();
    }
  }, [handleClearConversation, handleNewConversation]);

  const handleReachEnd = useCallback(() => {
    inputRef.current?.focus();
  }, []);

  const handleArrowUpFromInput = useCallback(() => {
    const container = messagesContainerRef.current;
    if (!container) return;

    // Foca na última mensagem da lista
    const lastMessage = container.querySelector('[data-message-node]:last-child') as HTMLElement | null;
    if (lastMessage) {
      lastMessage.focus();
      return;
    }

    // Se não houver mensagens em estrutura de árvore, foca no container
    container.focus();
  }, []);

  // Permite que o caller (EditorPage) devolva foco ao input após ações externas
  // (ex: fechar questionário, rejeitar sugestão).
  useEffect(() => {
    if (!isOpen) return;
    requestAnimationFrame(() => {
      inputRef.current?.focus();
    });
  }, [isOpen, focusNonce]);

  return (
    <Modal
      isOpen={isOpen}
      title={`${title ?? t('editor.inlineChat.title')} — ${conversationTitle}`}
      onClose={onClose}
      size="lg"
    >
      <div className="editor-inline-chat" onKeyDownCapture={handleKeyDownCapture}>
        <Toolbar
          className="editor-inline-chat__toolbar"
          ariaLabel={t('editor.inlineChat.toolbarLabel')}
          left={<h3 className="editor-inline-chat__toolbar-title">{conversationTitle}</h3>}
          actions={[
            {
              key: 'new-conversation',
              label: t('editor.inlineChat.newBtn'),
              icon: <PlusOutlined />,
              onClick: () => void handleNewConversation(),
              disabled: isLoading,
              shortcut: 'Ctrl+N',
            },
            {
              key: 'clear-conversation',
              label: t('editor.inlineChat.clearBtn'),
              icon: <ClearOutlined />,
              onClick: () => void handleClearConversation(),
              disabled: isLoading,
              shortcut: 'Ctrl+L',
              variant: 'danger',
            },
          ]}
        />

        <details className="editor-inline-chat__context" open={false}>
          <summary className="editor-inline-chat__context-summary">Contexto</summary>
          <pre className="editor-inline-chat__context-pre">{selectedText}</pre>
        </details>

        {error && (
          <div className="editor-inline-chat__error" role="alert">
            {error}
          </div>
        )}

        <div className="editor-inline-chat__messages">
          <MessageList
            threadedMessages={threadedMessages}
            onLoadChildren={loadMessageChildren}
            onReachEnd={handleReachEnd}
            isLoading={isLoading}
            ref={messagesContainerRef}
            onContextMenu={(event, message: Message) => showMenu(event, message, message.role === 'user')}
            onSpeak={speakMessage}
            onDelete={(message) => { void handleDeleteMessage(message); }}
          />
        </div>

        <div className="editor-inline-chat__input">
          <ChatInput
            onSend={(message, mediaFiles) => { void onSend(message, mediaFiles); }}
            disabled={isLoading}
            ref={inputRef}
            voiceEnabled={true}
            onArrowUp={handleArrowUpFromInput}
          />
        </div>

        <ContextMenu
          visible={menuVisible}
          items={menuItems}
          x={menuPosition.x}
          y={menuPosition.y}
          onClose={hideMenu}
          ariaLabel={t('editor.inlineChat.messageActions')}
        />

        {/* Evita warning de estado não usado em dev */}
        {deleteBusyId ? null : null}
      </div>
    </Modal>
  );
}
