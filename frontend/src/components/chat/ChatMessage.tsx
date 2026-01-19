import React from 'react';
import { Message } from '../../store/chatStore';
import './ChatMessage.css';

export interface ChatMessageProps {
  message: Message;
}

export const ChatMessage: React.FC<ChatMessageProps> = ({ message }) => {
  const { role, content, timestamp, isStreaming } = message;

  const formatTime = (timestamp: number) => {
    const date = new Date(timestamp);
    return date.toLocaleTimeString('pt-BR', {
      hour: '2-digit',
      minute: '2-digit',
    });
  };

  const getAriaLabel = () => {
    const roleLabel = role === 'user' ? 'Você' : 'Assistente';
    const contentPreview = content ? content.substring(0, 100) : 'Escrevendo';
    const timeLabel = formatTime(timestamp);
    return `${roleLabel}: ${contentPreview}. ${timeLabel}`;
  };

  return (
    <div 
      className={`chat-message chat-message--${role}`}
      role="article"
      aria-label={getAriaLabel()}
      aria-live={isStreaming ? 'polite' : undefined}
      aria-atomic={isStreaming ? 'true' : undefined}
      tabIndex={-1}
    >
      <div className="chat-message__avatar">
        {role === 'user' ? (
          <div className="chat-message__avatar-user">U</div>
        ) : (
          <div className="chat-message__avatar-assistant">AI</div>
        )}
      </div>
      <div className="chat-message__content">
        <div className="chat-message__header">
          <span className="chat-message__role">
            {role === 'user' ? 'Você' : 'Assistente'}
          </span>
          <span className="chat-message__timestamp">
            {formatTime(timestamp)}
          </span>
        </div>
        <div className="chat-message__text">
          {content || (isStreaming && <span className="chat-message__cursor">▋</span>)}
        </div>
      </div>
    </div>
  );
};
