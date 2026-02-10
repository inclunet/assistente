import { useRef, useState, useEffect } from 'react';
import { useChatStore } from '../store/chatStore';
import { useSettingsStore } from '../store/settingsStore';
import { MessageList } from '../components/chat/MessageList';
import { ChatInput } from '../components/chat/ChatInput';
import { ChatToolbar } from '../components/chat/ChatToolbar';
import { ChatTabs } from '../components/tabs/ChatTabs';
import { ContextMenu } from '../components/ui/ContextMenu';
import { MessageDetailModal } from '../components/ui/MessageDetailModal';
import { ConfirmDialog } from '../components/ui/ConfirmDialog';
import { KeyboardShortcutsHelp } from '../components/ui/KeyboardShortcutsHelp';
import { useChatKeyboardNav } from '../hooks/useChatKeyboardNav';
import { useTabsKeyboardShortcuts } from '../hooks/useTabsKeyboardShortcuts';
import { useContextMenu, useMessageActions } from '../hooks/useContextMenu';
import { Message } from '../store/chatStore';
import { MediaFile } from '../services/mediaService';
import { DeleteMessage } from '../../wailsjs/go/main/App';
import { EventsOn } from '../../wailsjs/runtime/runtime';
import { announce } from '../hooks/useAnnouncer';
import { handleError, ErrorSeverity, ErrorMessages } from '../utils/errorHandler';
import './ChatPage.css';

export default function ChatPage() {
  const inputRef = useRef<HTMLTextAreaElement>(null);
  const messagesContainerRef = useRef<HTMLDivElement>(null);
  const hasAutoFocusedRef = useRef(false);
  const retryButtonRef = useRef<HTMLButtonElement>(null);

  const {
    isLoading,
    sendMessage,
    getThreadedMessages,
    loadMessageChildren,
    getActiveTab,
    loadConversationInActiveTab,
    updateMessage,
    toggleReasoningExpanded,
    isReasoningExpanded,
  } = useChatStore();

  const { config } = useSettingsStore();

  // TTS está desabilitado se não há voz configurada ou se a voz é "disabled"
  const isTTSDisabled = !config?.voice || config.voice === 'disabled' || config.voice.includes('disabled');

  // Estado do modal de detalhes
  const [detailModalOpen, setDetailModalOpen] = useState(false);
  const [detailMessage, setDetailMessage] = useState<Message | null>(null);

  // Estado do dialog de confirmação de delete
  const [deleteDialogOpen, setDeleteDialogOpen] = useState(false);
  const [messageToDelete, setMessageToDelete] = useState<Message | null>(null);

  // Estado do painel de ajuda de atalhos
  const [shortcutsHelpOpen, setShortcutsHelpOpen] = useState(false);

  // Estado de erro para recovery
  const [lastFailedMessage, setLastFailedMessage] = useState<{ content: string; media?: MediaFile[] } | null>(null);
  const [sendError, setSendError] = useState<string | null>(null);

  // Edição de mensagem via store (acionado pelo menu de contexto)
  const startEditing = useChatStore(state => state.startEditing);

  // Ações de mensagem
  const { copyMessage, speakMessage } = useMessageActions({
    onAnnounce: (msg) => console.log('Anúncio:', msg),
  });

  // Menu de contexto
  const { menuVisible, menuPosition, menuItems, showMenu, hideMenu } = useContextMenu({
    onCopy: copyMessage,
    onOpenDetail: (message) => {
      setDetailMessage(message);
      setDetailModalOpen(true);
    },
    onSpeak: speakMessage,
    onEdit: (message) => {
      // Aciona a edição via store - também marca para pular restauração de foco
      startEditing(message.id);
    },
    onResend: async (message) => {
      if (message.content) {
        await sendMessage(message.content);
        announce('Mensagem reenviada');
      }
    },
    onDelete: (message) => {
      setMessageToDelete(message);
      setDeleteDialogOpen(true);
    },
    onPin: (message) => {
      // Pin requer campo adicional no modelo ChatMessage
      // Deixar para implementação futura
      console.log('Fixar/desafixar mensagem:', message);
      announce('Funcionalidade de fixar mensagem será implementada em breve');
    },
    onToggleReasoning: (message) => {
      toggleReasoningExpanded(message.id);
      const isExpanded = isReasoningExpanded(message.id);
      announce(isExpanded ? 'Raciocínio ocultado' : 'Raciocínio exibido');
    },
    isReasoningExpanded: (messageId: string) => isReasoningExpanded(messageId),
    isTTSDisabled,
  });

  // Enable keyboard navigation for chat messages
  useChatKeyboardNav({
    enabled: true,
    inputRef,
    messagesContainerRef,
  });

  // Enable global keyboard shortcuts for tabs (Ctrl+T, Ctrl+W, Ctrl+Tab, etc.)
  useTabsKeyboardShortcuts();

  // Handle delete confirmation
  const handleConfirmDelete = async () => {
    if (!messageToDelete) return;

    try {
      const messageId = parseInt(messageToDelete.id, 10);
      if (!isNaN(messageId)) {
        await DeleteMessage(messageId);
        announce('Mensagem excluída');
        // Recarrega a conversa atual sem page reload
        const activeTab = getActiveTab();
        if (activeTab?.conversationId) {
          await loadConversationInActiveTab(activeTab.conversationId, activeTab.title || 'Conversa');
        }
      }
    } catch (error) {
      handleError(error, {
        source: 'ChatPage.handleConfirmDelete',
        userMessage: ErrorMessages.CHAT.DELETE_FAILED,
        severity: ErrorSeverity.RECOVERABLE,
        metadata: { messageId: messageToDelete.id },
      });
    } finally {
      setDeleteDialogOpen(false);
      setMessageToDelete(null);
    }
  };

  const handleCancelDelete = () => {
    setDeleteDialogOpen(false);
    setMessageToDelete(null);
  };

  // Usa os MessageNode[] que o backend já enviou com childCount correto
  const threadedMessages = getThreadedMessages() || [];

  // Auto-focus no input ao carregar a página - apenas uma vez
  useEffect(() => {
    // Aguarda o componente renderizar completamente
    const checkTimer = setInterval(() => {
      const inputElement = inputRef.current;
      if (inputElement && !hasAutoFocusedRef.current) {
        hasAutoFocusedRef.current = true;
        clearInterval(checkTimer);
        
        // Pequeno delay para garantir que tudo está pronto
        setTimeout(() => {
          inputElement.focus();
        }, 100);
      }
    }, 100);
    
    return () => {
      clearInterval(checkTimer);
    };
  }, []); // Array vazio - roda apenas no mount

  // Keyboard shortcut to open help (? key)
  useEffect(() => {
    const handleKeyPress = (e: KeyboardEvent) => {
      // Only open if not in an input/textarea and not already open
      const target = e.target as HTMLElement;
      const isInputElement = target.tagName === 'INPUT' || target.tagName === 'TEXTAREA';

      if (e.key === '?' && !isInputElement && !shortcutsHelpOpen) {
        e.preventDefault();
        setShortcutsHelpOpen(true);
      }
    };

    document.addEventListener('keypress', handleKeyPress);
    return () => document.removeEventListener('keypress', handleKeyPress);
  }, [shortcutsHelpOpen]);

  // Listen for message:updated events from backend
  useEffect(() => {
    const handleMessageUpdated = (data: any) => {
      console.log('[ChatPage] Message updated:', data);
      
      // Em vez de recarregar toda a conversa, atualiza apenas a mensagem na store
      // Isso preserva o estado de foco e evita re-renders desnecessários
      const activeTab = getActiveTab();
      if (activeTab && data.message_id && data.content !== undefined) {
        // Atualiza a mensagem localmente na store (sem recarregar toda a conversa)
        updateMessage(activeTab.id, String(data.message_id), data.content);
      }
    };

    const unsubscribe = EventsOn('message:updated', handleMessageUpdated);
    return () => {
      if (unsubscribe) unsubscribe();
    };
  }, [getActiveTab, updateMessage]);

  // Auto-focus retry button when error banner appears
  useEffect(() => {
    if (sendError && retryButtonRef.current) {
      retryButtonRef.current.focus();
    }
  }, [sendError]);

  // Escape key to dismiss error banner
  useEffect(() => {
    const handleEscape = (e: KeyboardEvent) => {
      if (e.key === 'Escape' && sendError) {
        setSendError(null);
        setLastFailedMessage(null);
        announce('Erro descartado');
      }
    };

    document.addEventListener('keydown', handleEscape);
    return () => document.removeEventListener('keydown', handleEscape);
  }, [sendError]);

  const handleSendMessage = async (content: string, mediaFiles?: MediaFile[]) => {
    try {
      setSendError(null); // Clear previous error
      setLastFailedMessage(null);
      await sendMessage(content, mediaFiles);
    } catch (error) {
      // Store failed message for retry
      setLastFailedMessage({ content, media: mediaFiles });
      setSendError(ErrorMessages.CHAT.SEND_FAILED);

      handleError(error, {
        source: 'ChatPage.handleSendMessage',
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
      await sendMessage(lastFailedMessage.content, lastFailedMessage.media);
      setLastFailedMessage(null); // Clear on success
    } catch (error) {
      handleError(error, {
        source: 'ChatPage.handleRetry',
        userMessage: ErrorMessages.CHAT.SEND_FAILED,
        severity: ErrorSeverity.RECOVERABLE,
      });
    }
  };

  const handleReachEnd = () => {
    // Quando chega ao fim da lista principal, foca no input
    inputRef.current?.focus();
  };

  return (
    <div className="chat-page">
      <ChatTabs />
      <ChatToolbar inputRef={inputRef} />
      <MessageList
        threadedMessages={threadedMessages}
        onLoadChildren={loadMessageChildren}
        onReachEnd={handleReachEnd}
        isLoading={isLoading}
        ref={messagesContainerRef}
        onContextMenu={(event, message) => showMenu(event, message, message.role === 'user')}
        onOpenDetail={(message) => {
          setDetailMessage(message);
          setDetailModalOpen(true);
        }}
        onSpeak={speakMessage}
      />

      {/* Error banner with retry */}
      {sendError && lastFailedMessage && (
        <div
          className="chat-page__error-banner"
          role="alert"
          aria-live="assertive"
        >
          <span className="chat-page__error-message">{sendError}</span>
          <button
            ref={retryButtonRef}
            className="chat-page__retry-button"
            onClick={handleRetry}
            aria-label="Tentar enviar novamente"
          >
            Tentar novamente
          </button>
          <button
            className="chat-page__dismiss-button"
            onClick={() => {
              setSendError(null);
              setLastFailedMessage(null);
            }}
            aria-label="Descartar erro"
          >
            ✕
          </button>
        </div>
      )}

      <ChatInput 
        onSend={handleSendMessage} 
        disabled={isLoading}
        ref={inputRef}
        voiceEnabled={true}
        onArrowUp={() => {
          const container = messagesContainerRef.current;
          if (container) {
            // Foca na última mensagem da lista
            const lastMessage = container.querySelector('[data-message-node]:last-child') as HTMLElement;
            if (lastMessage) {
              lastMessage.focus();
            } else {
              // Se não houver mensagens em estrutura de árvore, foca no container
              container.focus();
            }
          }
        }}
      />

      {/* Menu de contexto */}
      <ContextMenu
        visible={menuVisible}
        items={menuItems}
        x={menuPosition.x}
        y={menuPosition.y}
        onClose={hideMenu}
        ariaLabel="Ações da mensagem"
      />

      {/* Modal de detalhes */}
      <MessageDetailModal
        open={detailModalOpen}
        content={detailMessage?.content || ''}
        role={detailMessage?.role === 'user' ? 'Você' : 'Assistente'}
        media={Array.isArray(detailMessage?.media) ? detailMessage.media : undefined}
        onClose={() => {
          setDetailModalOpen(false);
          setDetailMessage(null);
        }}
        onImageClick={(src, alt) => {
          console.log('Abrir imagem:', src, alt);
          // TODO: Implementar modal de imagem
        }}
      />

      {/* Dialog de confirmação de exclusão */}
      <ConfirmDialog
        isOpen={deleteDialogOpen}
        title="Excluir mensagem"
        message="Tem certeza que deseja excluir esta mensagem e todas as suas respostas? Esta ação não pode ser desfeita."
        confirmText="Excluir"
        cancelText="Cancelar"
        variant="danger"
        onConfirm={handleConfirmDelete}
        onCancel={handleCancelDelete}
      />

      {/* Painel de ajuda de atalhos de teclado */}
      <KeyboardShortcutsHelp
        isOpen={shortcutsHelpOpen}
        onClose={() => setShortcutsHelpOpen(false)}
      />
    </div>
  );
}
