import React, { useCallback } from 'react';
import { useTranslation } from 'react-i18next';
import { LoadingOutlined, CaretRightOutlined } from '@ant-design/icons';
import './ThreadIndicator.css';

export interface ThreadIndicatorProps {
  childCount: number;
  isExpanded: boolean;
  isLoading?: boolean;
  onToggle: () => void;
}

// React.memo com comparação de props para evitar re-renders desnecessários
export const ThreadIndicator: React.FC<ThreadIndicatorProps> = React.memo(({
  childCount,
  isExpanded,
  isLoading = false,
  onToggle,
}) => {
  const { t } = useTranslation();
  if (childCount === 0) return null;

  const handleClick = useCallback((e: React.MouseEvent) => {
    e.stopPropagation();
    if (!isLoading) {
      onToggle();
    }
  }, [isLoading, onToggle]);

  return (
    <button
      className={`thread-indicator ${isExpanded ? 'expanded' : ''}`}
      onClick={handleClick}
      aria-expanded={isExpanded}
      aria-label={isExpanded
        ? `${t('chat.collapseThread')}, ${t('chat.interactionCount', { count: childCount })}`
        : `${t('chat.expandThread')}, ${t('chat.interactionCount', { count: childCount })}`
      }
      disabled={isLoading}
      tabIndex={-1}
    >
      {isLoading ? (
        <span className="thread-indicator__spinner" aria-hidden="true"><LoadingOutlined /></span>
      ) : (
        <span className={`thread-indicator__arrow ${isExpanded ? 'expanded' : ''}`} aria-hidden="true"><CaretRightOutlined /></span>
      )}
      <span className="thread-indicator__count">
        {t('chat.interactionCount', { count: childCount })}
      </span>
    </button>
  );
}, (prevProps, nextProps) => {
  // Comparação customizada - só re-renderiza se props realmente mudaram
  return (
    prevProps.childCount === nextProps.childCount &&
    prevProps.isExpanded === nextProps.isExpanded &&
    prevProps.isLoading === nextProps.isLoading &&
    prevProps.onToggle === nextProps.onToggle
  );
});
