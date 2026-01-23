import { useRef, useState } from 'react';
import { useChatStore } from '../store/chatStore';
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
import './ChatPage.css';

export default function ChatPage() {
  const inputRef = useRef<HTMLTextAreaElement>(null);
  const messagesContainerRef = useRef<HTMLDivElement>(null);
  
  const {
    isLoading,
    sendMessage,
    getThreadedMessages,
    loadMessageChildren,
  } = useChatStore();

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
      console.log('Editar mensagem:', message);
      // TODO: Implementar edição
    },
    onResend: (message) => {
      console.log('Reenviar mensagem:', message);
      // TODO: Implementar reenvio
    },
    onDelete: (message) => {
      if (confirm('Excluir esta mensagem?')) {
        console.log('Excluir mensagem:', message);
        // TODO: Implementar exclusão
      }
    },
    onPin: (message) => {
      console.log('Fixar/desafixar mensagem:', message);
      // TODO: Implementar pin
    },
    isTTSDisabled: false, // TODO: Pegar do store de configurações
  });

  // Enable keyboard navigation for chat messages
  useChatKeyboardNav({
    enabled: true,
    inputRef,
    messagesContainerRef,
  });

  // Enable global keyboard shortcuts for tabs (Ctrl+T, Ctrl+W, Ctrl+Tab, etc.)
  useTabsKeyboardShortcuts();
  
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
      />
      <ChatInput 
        onSend={handleSendMessage} 
        disabled={isLoading}
        ref={inputRef}
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
