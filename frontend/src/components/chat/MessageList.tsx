import React, { useEffect, useRef, forwardRef } from 'react';
import { MessageNode as MessageNodeComponent } from './MessageNode';
import { MessageNode, Message } from '../../store/chatStore';
import './MessageList.css';

export interface MessageListProps {
  isLoading?: boolean;
  // Estrutura hierárquica de mensagens (threads)
  threadedMessages: MessageNode[];
  // Callback para carregar filhos de uma mensagem
  onLoadChildren?: (messageId: string) => Promise<MessageNode[]>;
  // Callback quando chega ao fim da lista principal
  onReachEnd?: () => void;
  // Callbacks de ações
  onContextMenu?: (event: React.MouseEvent, message: Message) => void;
  onOpenDetail?: (message: Message) => void;
  onSpeak?: (message: Message) => void;
}

export const MessageList = forwardRef<HTMLDivElement, MessageListProps>((
  { isLoading = false, threadedMessages, onLoadChildren, onReachEnd, onContextMenu, onOpenDetail, onSpeak },
  ref
) => {
  const messagesEndRef = useRef<HTMLDivElement>(null);
  const internalContainerRef = useRef<HTMLDivElement>(null);
  
  // Use external ref if provided, otherwise use internal ref
  const containerRef = (ref as React.RefObject<HTMLDivElement>) || internalContainerRef;

  // Usa visualização hierárquica

  const scrollToBottom = (behavior: ScrollBehavior = 'smooth') => {
    messagesEndRef.current?.scrollIntoView({ behavior });
  };

  useEffect(() => {
    // Scroll to bottom when messages change
    scrollToBottom();
  }, [threadedMessages]);

  useEffect(() => {
    // Instant scroll on mount
    scrollToBottom('instant');
  }, []);

  if (threadedMessages.length === 0) {
    return (
      <div 
        className="message-list message-list--empty"
        role="region"
        aria-label="Lista de mensagens da conversa"
      >
        <div className="message-list__empty-state">
          <div className="message-list__empty-icon">💬</div>
          <h3 className="message-list__empty-title">
            Comece uma nova conversa
          </h3>
          <p className="message-list__empty-description">
            Digite sua mensagem abaixo para começar a conversar com o assistente de IA.
          </p>
        </div>
      </div>
    );
  }

  return (
    <div 
      className="message-list" 
      ref={containerRef}
      aria-label="Lista de mensagens da conversa"
    >
      <div className="message-list__messages">
        {/* Visualização hierárquica com threads */}
        <div 
          role="list" 
          aria-label="Mensagens da conversa"
          tabIndex={0}
          onKeyDown={(e) => {
            const target = e.currentTarget;
            const firstChild = target.querySelector('[data-message-node]') as HTMLElement;
            if (firstChild && e.key === 'ArrowDown') {
              e.preventDefault();
              firstChild.focus();
            }
          }}
        >
          {threadedMessages.map((node, index) => (
            <MessageNodeComponent
              key={node.message.id}
              node={node}
              level={0}
              siblingIndex={index}
              siblingCount={threadedMessages.length}
              onLoadChildren={onLoadChildren}
              onReachEnd={onReachEnd}
              onContextMenu={onContextMenu}
              onOpenDetail={onOpenDetail}
              onSpeak={onSpeak}
            />
          ))}
        </div>
        {isLoading && (
          <div 
            className="message-list__loading"
            role="status"
            aria-label="Assistente está digitando"
          >
            <div className="message-list__loading-dots" aria-hidden="true">
              <span></span>
              <span></span>
              <span></span>
            </div>
          </div>
        )}
        <div ref={messagesEndRef} />
      </div>
    </div>
  );
});
