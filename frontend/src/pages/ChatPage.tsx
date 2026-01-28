import { useRef, useState, useEffect } from 'react';
import { useChatStore } from '../store/chatStore';
import { useSettingsStore } from '../store/settingsStore';
import { MessageList } from '../components/chat/MessageList';
import { ChatInput } from '../components/chat/ChatInput';
import { ChatToolbar } from '../components/chat/ChatToolbar';
import { ChatTabs } from '../components/tabs/ChatTabs';
import { ContextMenu } from '../components/ui/ContextMenu';
import { MessageDetailModal } from '../components/ui/MessageDetailModal';
import { useChatKeyboardNav } from '../hooks/useChatKeyboardNav';
import { useTabsKeyboardShortcuts } from '../hooks/useTabsKeyboardShortcuts';
import { useContextMenu, useMessageActions } from '../hooks/useContextMenu';
import { Message } from '../store/chatStore';
import { MediaFile } from '../services/mediaService';
import { DeleteMessage } from '../../wailsjs/go/main/App';
import { announce } from '../hooks/useAnnouncer';
import './ChatPage.css';

export default function ChatPage() {
  const inputRef = useRef<HTMLTextAreaElement>(null);
  const messagesContainerRef = useRef<HTMLDivElement>(null);
  const hasAutoFocusedRef = useRef(false);
  
  const {
    isLoading,
    sendMessage,
    getThreadedMessages,
    loadMessageChildren,
    getActiveTab,
  } = useChatStore();

  const { config } = useSettingsStore();

  // TTS está desabilitado se não há voz configurada ou se a voz é "disabled"
  const isTTSDisabled = !config?.voice || config.voice === 'disabled' || config.voice.includes('disabled');

  // Estado do modal de detalhes
  const [detailModalOpen, setDetailModalOpen] = useState(false);
  const [detailMessage, setDetailMessage] = useState<Message | null>(null);

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
      // Edição de mensagem requer UI adicional (modal de edição)
      // Por enquanto, copia o conteúdo para o input para o usuário editar manualmente
      if (inputRef.current && message.content) {
        inputRef.current.value = message.content;
        inputRef.current.focus();
        announce('Conteúdo da mensagem copiado para edição');
      }
    },
    onResend: async (message) => {
      if (message.content) {
        await sendMessage(message.content);
        announce('Mensagem reenviada');
      }
    },
    onDelete: async (message) => {
      if (confirm('Excluir esta mensagem e todas as suas respostas?')) {
        try {
          const messageId = parseInt(message.id, 10);
          if (!isNaN(messageId)) {
            await DeleteMessage(messageId);
            announce('Mensagem excluída');
            // A UI será atualizada via evento 'message:deleted' ou reload
            // Por simplicidade, recarrega a conversa atual
            const activeTab = getActiveTab();
            if (activeTab?.conversationId) {
              // Força reload da conversa
              window.location.reload();
            }
          }
        } catch (error) {
          console.error('Erro ao excluir mensagem:', error);
          announce('Erro ao excluir mensagem');
        }
      }
    },
    onPin: (message) => {
      // Pin requer campo adicional no modelo ChatMessage
      // Deixar para implementação futura
      console.log('Fixar/desafixar mensagem:', message);
      announce('Funcionalidade de fixar mensagem será implementada em breve');
    },
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
  
  // Usa os MessageNode[] que o backend já enviou com childCount correto
  const threadedMessages = getThreadedMessages() || [];

  const handleSendMessage = async (content: string, mediaFiles?: MediaFile[]) => {
    await sendMessage(content, mediaFiles);
  };

  const handleReachEnd = () => {
    // Quando chega ao fim da lista principal, foca no input
    inputRef.current?.focus();
  };

  return (
    <div className="chat-page">
      <ChatTabs />
      <ChatToolbar voiceEnabled={true} inputRef={inputRef} />
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
    </div>
  );
}
