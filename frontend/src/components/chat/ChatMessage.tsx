import React from 'react';
import { Message } from '../../store/chatStore';
import { MarkdownRenderer } from '../ui/MarkdownRenderer';
import { ThreadIndicator } from './ThreadIndicator';
import { formatAgentName, isAgentMessage } from '../../lib/chatUtils';
import { stripMarkdown } from '../../lib/stripMarkdown';
import { formatRelativeTime } from '../../lib/dateUtils';
import './ChatMessage.css';

export interface ChatMessageProps {
  message: Message;
  // Thread indicator props
  hasThreadIndicator?: boolean;
  threadChildCount?: number;
  isThreadExpanded?: boolean;
  isThreadLoading?: boolean;
  onThreadToggle?: () => void;
  // Event handlers
  onContextMenu?: (event: React.MouseEvent, message: Message) => void;
  onOpenDetail?: (message: Message) => void;
  onSpeak?: (message: Message) => void;
}

export const ChatMessage: React.FC<ChatMessageProps> = React.memo(({ 
  message,
  hasThreadIndicator = false,
  threadChildCount = 0,
  isThreadExpanded = false,
  isThreadLoading = false,
  onThreadToggle,
  onContextMenu,
  onOpenDetail,
  onSpeak,
}) => {
  const { role, content, timestamp, isStreaming, agentName, toolName } = message;

  const formatTime = (timestamp: number) => {
    const date = new Date(timestamp);
    return date.toLocaleTimeString('pt-BR', {
      hour: '2-digit',
      minute: '2-digit',
    });
  };

  const getDisplayRole = () => {
    // Usuário
    if (role === 'user') return 'Você';
    
    // Resposta de ferramenta - usa o nome da ferramenta
    if (role === 'tool' && toolName) {
      return formatAgentName(toolName);
    }
    
    // Mensagem genérica de ferramenta sem nome específico
    if (role === 'tool') return 'Ferramenta';
    
    // Agente específico (file_manager, etc)
    if (agentName) return formatAgentName(agentName);
    
    // Assistente padrão
    return 'Assistente';
  };

  const getAriaLabel = () => {
    const roleLabel = getDisplayRole();
    const contentPreview = content ? stripMarkdown(content) : 'Escrevendo';
    const playHint = role === 'assistant' && !isStreaming ? ' Pressione Espaço para reproduzir áudio.' : '';
    
    // Timestamp relativo com prefixo
    const relativeTime = formatRelativeTime(timestamp);
    const timePrefix = role === 'user' ? 'enviado' : 'recebido';
    
    return `${roleLabel}: ${contentPreview}. ${timePrefix} ${relativeTime}.${playHint}`;
  };

  const handleKeyDown = (e: React.KeyboardEvent) => {
    // Spacebar - reproduz TTS da mensagem (assistente ou usuário)
    if (e.key === ' ' && !isStreaming) {
      e.preventDefault();
      if (onSpeak) {
        onSpeak(message);
      }
    }
    
    // Enter abre modal de detalhes
    if (e.key === 'Enter' && !isStreaming) {
      e.preventDefault();
      if (onOpenDetail) {
        onOpenDetail(message);
      }
    }
  };
  
  const handleKeyUp = (e: React.KeyboardEvent) => {
    // Tecla ContextMenu/Application (Windows) ou Shift+F10 abre menu de contexto
    // Usa onKeyUp para prevenir menu nativo do navegador
    if (e.key === 'ContextMenu' || (e.shiftKey && e.key === 'F10')) {
      e.preventDefault();
      e.stopPropagation();
      if (onContextMenu) {
        // Simula evento de mouse no centro do elemento
        const rect = e.currentTarget.getBoundingClientRect();
        const syntheticEvent = {
          preventDefault: () => {},
          stopPropagation: () => {},
          clientX: rect.left + rect.width / 2,
          clientY: rect.top + rect.height / 2,
          currentTarget: e.currentTarget, // Para restaurar foco após fechar menu
          target: e.currentTarget,
        } as unknown as React.MouseEvent;
        onContextMenu(syntheticEvent, message);
      }
    }
  };

  const handleContextMenuEvent = (e: React.MouseEvent) => {
    e.preventDefault();
    onContextMenu?.(e, message);
  };
  
  const handleFocus = () => {
    // Mensagem recebeu foco
  };

  return (
    <div 
      className={`chat-message chat-message--${role}`}
      role="article"
      aria-label={getAriaLabel()}
      onKeyDown={handleKeyDown}
      onKeyUp={handleKeyUp}
      onContextMenu={handleContextMenuEvent}
      onFocus={handleFocus}
      tabIndex={-1}
    >
      <div className="chat-message__avatar" aria-hidden="true">
        {role === 'user' ? (
          <div className="chat-message__avatar-user">U</div>
        ) : role === 'tool' ? (
          <div className="chat-message__avatar-tool">🛠️</div>
        ) : (
          <div className="chat-message__avatar-assistant">
            {isAgentMessage(message) ? '🤖' : 'AI'}
          </div>
        )}
      </div>
      <div className="chat-message__content">
        <div className="chat-message__header">
          <h3 className="chat-message__role">
            {getDisplayRole()}
          </h3>
          <span className="chat-message__timestamp">
            {formatTime(timestamp)}
          </span>
          {hasThreadIndicator && onThreadToggle && (
            <ThreadIndicator
              childCount={threadChildCount}
              isExpanded={isThreadExpanded}
              isLoading={isThreadLoading}
              onToggle={onThreadToggle}
            />
          )}
        </div>
        
        <div className="chat-message__text">
          {role === 'assistant' && content ? (
            <MarkdownRenderer content={content} />
          ) : (
            content || (isStreaming && <span className="chat-message__cursor">▋</span>)
          )}
        </div>
      </div>
    </div>
  );
});
