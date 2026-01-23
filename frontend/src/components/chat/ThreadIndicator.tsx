import React from 'react';
import './ThreadIndicator.css';

export interface ThreadIndicatorProps {
  childCount: number;
  isExpanded: boolean;
  isLoading?: boolean;
  onToggle: () => void;
}

export const ThreadIndicator: React.FC<ThreadIndicatorProps> = ({
  childCount,
  isExpanded,
  isLoading = false,
  onToggle,
}) => {
  console.log('[ThreadIndicator] Renderizando:', { childCount, isExpanded, isLoading });
  
  if (childCount === 0) return null;

  const handleClick = (e: React.MouseEvent) => {
    e.stopPropagation();
    if (!isLoading) {
      onToggle();
    }
  };

  return (
    <button
      className={`thread-indicator ${isExpanded ? 'expanded' : ''}`}
      onClick={handleClick}
      aria-expanded={isExpanded}
      aria-label={isExpanded ? 'Recolher interações' : 'Expandir interações'}
      disabled={isLoading}
      tabIndex={-1}
    >
      {isLoading ? (
        <span className="thread-indicator__spinner">⏳</span>
      ) : (
        <span className={`thread-indicator__arrow ${isExpanded ? 'expanded' : ''}`}>▶</span>
      )}
      <span className="thread-indicator__count">
        {childCount} {childCount === 1 ? 'interação' : 'interações'}
      </span>
    </button>
  );
};
