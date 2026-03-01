import { useEffect, useRef, useState, useCallback } from 'react';
import { SimpleModal } from '../ui/SimpleModal';
import { MessageList } from '../chat/MessageList';
import { ChatInput } from '../chat/ChatInput';
import { ContextMenu } from '../ui/ContextMenu';
import { Toolbar } from '../ui/Toolbar';
import { useChatStore, type Message } from '../../store/chatStore';
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
  onSend: (instruction: string, mediaFiles?: any[]) => Promise<void>;
}

export function EditorInlineChatModal({
  isOpen,
  title = 'Mini-chat',
  selectedText,
  error,
  focusNonce,
  onClose,
  onSend,
}: EditorInlineChatModalProps) {
  const inputRef = useRef<HTMLTextAreaElement>(null);
  const messagesContainerRef = useRef<HTMLDivElement>(null);

  const { isLoading, getThreadedMessages, loadMessageChildren, getActiveTab, loadConversationInActiveTab } = useChatStore();

  const activeTab = getActiveTab();
  const conversationTitle = activeTab?.title || 'Conversa';

  const { copyMessage, speakMessage } = useMessageActions({
    onAnnounce: (msg) => announce(msg),
  });

  const [deleteBusyId, setDeleteBusyId] = useState<string | null>(null);
  const handleDeleteMessage = useCallback(
    async (message: any) => {
      const msgId = String(message?.id || '');
      if (!msgId) return;

      try {
        setDeleteBusyId(msgId);
        const numericId = parseInt(msgId, 10);
        if (!isNaN(numericId)) {
          await DeleteMessage(numericId);
          announce('Mensagem excluída');
          const tab = getActiveTab();
          if (tab?.conversationId) {
            await loadConversationInActiveTab(tab.conversationId, tab.title || 'Conversa');
          }
        }
      } catch (e: any) {
        console.error('[EditorInlineChatModal] delete error:', e);
        announce('Erro ao excluir mensagem', 'assertive');
      } finally {
        setDeleteBusyId(null);
      }
    },
    [getActiveTab, loadConversationInActiveTab]
  );

  const { showMenu, hideMenu, menuItems, menuPosition, menuVisible } = useContextMenu({
    onCopy: copyMessage,
    onReadMessage: (m) => useChatStore.getState().startReading(String(m.id)),
    onSpeak: speakMessage,
    onEdit: (m) => useChatStore.getState().startEditing(String(m.id)),
    onResend: async (m) => {
      if (m.content) {
        await useChatStore.getState().sendMessage(String(m.content));
        announce('Mensagem reenviada');
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
      await loadConversationInActiveTab(0, 'Nova Conversa');
      announce('Nova conversa');
    } catch (e) {
      console.error('[EditorInlineChatModal] new conversation error:', e);
      announce('Erro ao criar nova conversa', 'assertive');
    } finally {
      focusInput();
    }
  }, [focusInput, loadConversationInActiveTab]);

  const handleClearConversation = useCallback(async () => {
    const tab = getActiveTab();
    try {
      if (tab?.conversationId) {
        await ClearConversation(tab.conversationId);
        await loadConversationInActiveTab(tab.conversationId, tab.title || 'Conversa');
        announce('Conversa limpa');
      } else {
        useChatStore.getState().clearActiveTab();
        announce('Conversa limpa');
      }
    } catch (e) {
      console.error('[EditorInlineChatModal] clear conversation error:', e);
      announce('Erro ao limpar conversa', 'assertive');
    } finally {
      focusInput();
    }
  }, [focusInput, getActiveTab, loadConversationInActiveTab]);

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
    <SimpleModal
      isOpen={isOpen}
      title={`${title} — ${conversationTitle}`}
      onClose={onClose}
      size="lg"
    >
      <div className="editor-inline-chat" onKeyDownCapture={handleKeyDownCapture}>
        <Toolbar
          className="editor-inline-chat__toolbar"
          ariaLabel="Ferramentas do mini-chat. Use setas para navegar entre os botões"
          left={<h3 className="editor-inline-chat__toolbar-title">{conversationTitle}</h3>}
          actions={[
            {
              key: 'new-conversation',
              label: 'Nova',
              icon: '➕',
              onClick: () => void handleNewConversation(),
              disabled: isLoading,
              shortcut: 'Ctrl+N',
            },
            {
              key: 'clear-conversation',
              label: 'Limpar',
              icon: '🧹',
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
            onDelete={handleDeleteMessage as any}
          />
        </div>

        <div className="editor-inline-chat__input">
          <ChatInput
            onSend={onSend as any}
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
          ariaLabel="Ações da mensagem"
        />

        {/* Evita warning de estado não usado em dev */}
        {deleteBusyId ? null : null}
      </div>
    </SimpleModal>
  );
}
