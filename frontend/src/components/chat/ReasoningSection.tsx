import React from 'react';
import { MarkdownRenderer } from '../ui/MarkdownRenderer';
import './ReasoningSection.css';

export interface ReasoningSectionProps {
  reasoning: string;
  isStreaming?: boolean;
  isExpanded?: boolean; // Controlado externamente
  onToggle?: () => void; // Callback para toggle
}

export const ReasoningSection: React.FC<ReasoningSectionProps> = ({
  reasoning,
  isStreaming = false,
  isExpanded = false,
  onToggle,
}) => {
  if (!reasoning && !isStreaming) return null;

  const handleToggle = () => {
    onToggle?.();
  };

  const handleKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === 'Enter' || e.key === ' ') {
      e.preventDefault();
      handleToggle();
    }
  };

  // Conta caracteres e linhas para resumo
  const charCount = reasoning.length;
  const lineCount = (reasoning.match(/\n/g) || []).length + 1;
  const summaryText = isStreaming 
    ? 'Pensando...' 
    : `${charCount} caracteres, ${lineCount} linhas`;

  return (
    <div 
      className={`reasoning-section ${isExpanded ? 'reasoning-section--expanded' : ''} ${isStreaming ? 'reasoning-section--streaming' : ''}`}
      aria-label={isStreaming ? 'O modelo está pensando' : 'Raciocínio do modelo'}
      tabIndex={-1}
    >
      <button
        className="reasoning-section__header"
        onClick={handleToggle}
        onKeyDown={handleKeyDown}
        aria-expanded={isExpanded}
        aria-controls="reasoning-content"
        type="button"
        tabIndex={-1}
      >
        <span className="reasoning-section__icon" aria-hidden="true">
          {isStreaming ? '🧠' : '💭'}
        </span>
        <span className="reasoning-section__title">
          {isStreaming ? 'Pensando...' : 'Raciocínio'}
        </span>
        <span className="reasoning-section__summary">
          {summaryText}
        </span>
        <span 
          className={`reasoning-section__chevron ${isExpanded ? 'reasoning-section__chevron--expanded' : ''}`}
          aria-hidden="true"
        >
          ▼
        </span>
      </button>
      
      {isExpanded && (
        <div 
          id="reasoning-content"
          className="reasoning-section__content"
          role="region"
          aria-label="Conteúdo do raciocínio"
        >
          {reasoning ? (
            <MarkdownRenderer content={reasoning} />
          ) : (
            <span className="reasoning-section__cursor">▋</span>
          )}
        </div>
      )}
    </div>
  );
};
