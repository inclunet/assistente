import React from 'react';
import { BulbOutlined, DownOutlined } from '@ant-design/icons';
import { useTranslation } from 'react-i18next';
import { MarkdownRenderer } from '../ui/MarkdownRenderer';
import './ReasoningSection.css';

export interface ReasoningSectionProps {
  reasoning: string;
  isStreaming?: boolean;
  isExpanded?: boolean; // Controlado externamente
  onToggle?: () => void; // Callback para toggle
  tabNavigationEnabled?: boolean;
}

export const ReasoningSection = React.memo<ReasoningSectionProps>(function ReasoningSection({
  reasoning,
  isStreaming = false,
  isExpanded = false,
  onToggle,
  tabNavigationEnabled = false,
}) {
  const { t } = useTranslation();
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
    ? t('chat.thinking') 
    : `${charCount} ${t('chat.characters')}, ${lineCount} ${t('chat.lines')}`;

  return (
    <div 
      className={`reasoning-section ${isExpanded ? 'reasoning-section--expanded' : ''} ${isStreaming ? 'reasoning-section--streaming' : ''}`}
      aria-label={isStreaming ? t('chat.modelThinking') : t('chat.modelReasoning')}
      tabIndex={-1}
    >
      <button
        className="reasoning-section__header"
        onClick={handleToggle}
        onKeyDown={handleKeyDown}
        aria-expanded={isExpanded}
        aria-controls="reasoning-content"
        type="button"
        tabIndex={tabNavigationEnabled ? 0 : -1}
      >
        <span className="reasoning-section__icon" aria-hidden="true">
          <BulbOutlined />
        </span>
        <span className="reasoning-section__title">
          {isStreaming ? t('chat.thinking') : t('chat.reasoning')}
        </span>
        <span className="reasoning-section__summary">
          {summaryText}
        </span>
        <span 
          className={`reasoning-section__chevron ${isExpanded ? 'reasoning-section__chevron--expanded' : ''}`}
          aria-hidden="true"
        >
          <DownOutlined />
        </span>
      </button>
      
      {isExpanded && (
        <div 
          id="reasoning-content"
          className="reasoning-section__content"
          role="region"
          aria-label={t('chat.reasoningContent')}
        >
          {reasoning ? (
            <MarkdownRenderer
              content={reasoning}
              tabNavigation={tabNavigationEnabled ? 'enabled' : 'disabled'}
            />
          ) : (
            <span className="reasoning-section__cursor">▋</span>
          )}
        </div>
      )}
    </div>
  );
});
