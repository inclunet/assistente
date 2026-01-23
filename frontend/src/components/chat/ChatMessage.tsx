import React from 'react';
import { Message } from '../../store/chatStore';
import { MarkdownRenderer } from '../ui/MarkdownRenderer';
import { ThreadIndicator } from './ThreadIndicator';
import { formatAgentName, isAgentMessage } from '../../lib/chatUtils';
import { messageAudioService } from '../../services/messageAudio';
import { ttsService } from '../../services/tts';
import { stripMarkdown } from '../../lib/stripMarkdown';
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
}

export const ChatMessage: React.FC<ChatMessageProps> = ({ 
  message,
  hasThreadIndicator = false,
  threadChildCount = 0,
  isThreadExpanded = false,
  isThreadLoading = false,
  onThreadToggle,
  onContextMenu,
  onOpenDetail,
}) => {
  const { id, role, content, timestamp, isStreaming, agentName, toolName } = message;

  const formatTime = (timestamp: number) => {
    const date = new Date(timestamp);
    return date.toLocaleTimeString('pt-BR', {
      hour: '2-digit',
      minute: '2-digit',
    });
  };

  const getRelativeTime = (timestamp: number): string => {
    const now = Date.now();
    const diff = now - timestamp;
    const seconds = Math.floor(diff / 1000);
    const minutes = Math.floor(seconds / 60);
    const hours = Math.floor(minutes / 60);
    const days = Math.floor(hours / 24);

    if (seconds < 60) {
      return seconds <= 5 ? 'agora' : `há ${seconds} segundos`;
    } else if (minutes < 2) {
      return 'há 1 minuto';
    } else if (minutes < 5) {
      return `há ${minutes} minutos`;
    } else if (minutes < 10) {
      return 'há mais de 5 minutos';
    } else if (minutes < 30) {
      return 'há mais de 10 minutos';
    } else if (minutes < 60) {
      return 'há mais de 30 minutos';
    } else if (hours < 2) {
      return 'há mais de uma hora';
    } else if (hours < 24) {
      return `há mais de ${hours} horas`;
    } else if (days === 1) {
      return 'há 1 dia';
    } else {
      return `há ${days} dias`;
    }
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
    const relativeTime = getRelativeTime(timestamp);
    const timePrefix = role === 'user' ? 'enviado' : 'recebido';
    
    return `${roleLabel}: ${contentPreview}. ${timePrefix} ${relativeTime}.${playHint}`;
  };

  const handleKeyDown = (e: React.KeyboardEvent) => {
    // Spacebar reproduz áudio - Lock global previne duplicação
    if (e.key === ' ' && role === 'assistant' && !isStreaming) {
      e.preventDefault();
      const volume = ttsService.getVolume();
      messageAudioService.playMessage(id, volume);
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
    // Tecla ContextMenu/Application (Windows) abre menu de contexto
    // Usa onKeyUp para prevenir menu nativo do navegador
    if (e.key === 'ContextMenu') {
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
        } as React.MouseEvent;
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
};
