import React, { useCallback } from 'react';
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
  // Retorna null cedo se não há filhos (não precisa renderizar nada)
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
}, (prevProps, nextProps) => {
  // Comparação customizada - só re-renderiza se props realmente mudaram
  return (
    prevProps.childCount === nextProps.childCount &&
    prevProps.isExpanded === nextProps.isExpanded &&
    prevProps.isLoading === nextProps.isLoading &&
    prevProps.onToggle === nextProps.onToggle
  );
});
