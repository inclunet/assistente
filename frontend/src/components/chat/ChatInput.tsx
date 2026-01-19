import React, { useState, useRef, KeyboardEvent, useEffect, forwardRef } from 'react';
import { Button } from '../ui/Button';
import './ChatInput.css';

export interface ChatInputProps {
  onSend: (message: string) => void;
  disabled?: boolean;
  placeholder?: string;
}

export const ChatInput = forwardRef<HTMLTextAreaElement, ChatInputProps>((
  { onSend, disabled = false, placeholder = 'Digite sua mensagem...' },
  ref
) => {
  const [message, setMessage] = useState('');
  const internalTextareaRef = useRef<HTMLTextAreaElement>(null);
  const hintId = 'chat-input-hint';
  
  // Use external ref if provided, otherwise use internal ref
  const textareaRef = (ref as React.RefObject<HTMLTextAreaElement>) || internalTextareaRef;

  const adjustTextareaHeight = () => {
    const textarea = textareaRef.current;
    if (textarea) {
      textarea.style.height = 'auto';
      const newHeight = Math.min(textarea.scrollHeight, 200); // Max 200px
      textarea.style.height = `${newHeight}px`;
    }
  };

  useEffect(() => {
    adjustTextareaHeight();
  }, [message]);

  const handleSend = () => {
    const trimmedMessage = message.trim();
    if (trimmedMessage && !disabled) {
      onSend(trimmedMessage);
      setMessage('');
      
      // Reset textarea height
      if (textareaRef.current) {
        textareaRef.current.style.height = 'auto';
      }
    }
  };

  const handleKeyDown = (e: KeyboardEvent<HTMLTextAreaElement>) => {
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault();
      handleSend();
    }
  };

  return (
    <div className="chat-input">
      <div className="chat-input__container">
        <textarea
          ref={textareaRef}
          className="chat-input__textarea"
          value={message}
          onChange={(e) => setMessage(e.target.value)}
          onKeyDown={handleKeyDown}
          placeholder={placeholder}
          disabled={disabled}
          rows={1}
          aria-label="Digite sua mensagem"
          aria-describedby={hintId}
          aria-multiline="true"
        />
        <Button
          onClick={handleSend}
          disabled={disabled || !message.trim()}
          variant="primary"
          size="md"
          className="chat-input__button"
          aria-label="Enviar mensagem"
        >
          <svg
            width="20"
            height="20"
            viewBox="0 0 24 24"
            fill="none"
            stroke="currentColor"
            strokeWidth="2"
            strokeLinecap="round"
            strokeLinejoin="round"
          >
            <line x1="22" y1="2" x2="11" y2="13" />
            <polygon points="22 2 15 22 11 13 2 9 22 2" />
          </svg>
        </Button>
      </div>
      <div className="chat-input__hint" id={hintId}>
        Pressione Enter para enviar, Shift+Enter para nova linha
      </div>
    </div>
  );
});
