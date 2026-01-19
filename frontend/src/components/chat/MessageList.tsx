import React, { useEffect, useRef, forwardRef } from 'react';
import { ChatMessage } from './ChatMessage';
import { Message } from '../../store/chatStore';
import './MessageList.css';

export interface MessageListProps {
  messages: Message[];
  isLoading?: boolean;
}

export const MessageList = forwardRef<HTMLDivElement, MessageListProps>((
  { messages, isLoading = false },
  ref
) => {
  const messagesEndRef = useRef<HTMLDivElement>(null);
  const internalContainerRef = useRef<HTMLDivElement>(null);
  
  // Use external ref if provided, otherwise use internal ref
  const containerRef = (ref as React.RefObject<HTMLDivElement>) || internalContainerRef;

  const scrollToBottom = (behavior: ScrollBehavior = 'smooth') => {
    messagesEndRef.current?.scrollIntoView({ behavior });
  };

  useEffect(() => {
    // Scroll to bottom when messages change
    scrollToBottom();
  }, [messages]);

  useEffect(() => {
    // Instant scroll on mount
    scrollToBottom('instant');
  }, []);

  if (messages.length === 0) {
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
      role="log"
      aria-label="Lista de mensagens da conversa"
      aria-live="polite"
      aria-atomic="false"
    >
      <div className="message-list__messages">
        {messages.map((message) => (
          <ChatMessage key={message.id} message={message} />
        ))}
        {isLoading && (
          <div 
            className="message-list__loading"
            role="status"
            aria-live="polite"
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
