import React, { useRef, useEffect } from 'react';
import './CheckboxGrid.css';

export interface CheckboxGridProps<T> {
  /** Lista de itens a renderizar */
  items: T[];
  /** IDs dos itens selecionados */
  selectedIds: string[];
  /** Callback quando um item é marcado/desmarcado */
  onToggle: (id: string) => void;
  /** Função para extrair ID único de cada item */
  getItemId: (item: T) => string;
  /** Função para extrair label de exibição */
  getItemLabel: (item: T) => string;
  /** Função opcional para extrair descrição */
  getItemDescription?: (item: T) => string;
  /** Função opcional para extrair badge/tag */
  getBadge?: (item: T) => string;
  /** Classe para adicionar ao contêiner */
  className?: string;
  /** aria-label para acessibilidade */
  ariaLabel: string;
}

/**
 * Componente reutilizável para grade de checkboxes.
 * Usado em ProfileSkillsSection, ProfileToolsSection.
 * Suporta navegação por teclado (arrows, home, end, space).
 *
 * @example
 * ```tsx
 * <CheckboxGrid
 *   items={skills}
 *   selectedIds={selectedSkillIds}
 *   onToggle={(id) => toggleSkill(id)}
 *   getItemId={(skill) => skill.id}
 *   getItemLabel={(skill) => skill.name}
 *   getItemDescription={(skill) => skill.description}
 *   ariaLabel="Selecione skills"
 * />
 * ```
 */
export function CheckboxGrid<T>({
  items,
  selectedIds,
  onToggle,
  getItemId,
  getItemLabel,
  getItemDescription,
  getBadge,
  className,
  ariaLabel,
}: CheckboxGridProps<T>) {
  const containerRef = useRef<HTMLDivElement>(null);
  const checkboxRefs = useRef<Map<string, HTMLInputElement>>(new Map());

  const handleKeyDown = (e: React.KeyboardEvent, id: string) => {
    const allIds = items.map(getItemId);
    const currentIndex = allIds.indexOf(id);

    let nextIndex: number | null = null;

    switch (e.key) {
      case 'ArrowDown':
        e.preventDefault();
        nextIndex = Math.min(currentIndex + 1, allIds.length - 1);
        break;
      case 'ArrowUp':
        e.preventDefault();
        nextIndex = Math.max(currentIndex - 1, 0);
        break;
      case 'Home':
        e.preventDefault();
        nextIndex = 0;
        break;
      case 'End':
        e.preventDefault();
        nextIndex = allIds.length - 1;
        break;
      case ' ':
        e.preventDefault();
        onToggle(id);
        return;
      default:
        return;
    }

    if (nextIndex !== null && nextIndex !== currentIndex) {
      const nextId = allIds[nextIndex];
      const nextCheckbox = checkboxRefs.current.get(nextId);
      nextCheckbox?.focus();
    }
  };

  useEffect(() => {
    // Limpar refs de itens que não existem mais
    return () => {
      checkboxRefs.current.clear();
    };
  }, []);

  return (
    <div
      ref={containerRef}
      className={`checkbox-grid ${className || ''}`}
      role="group"
      aria-label={ariaLabel}
      data-testid="checkbox-grid"
    >
      {items.length === 0 ? (
        <div className="checkbox-grid__empty" data-testid="empty-state">
          Nenhum item disponível
        </div>
      ) : (
        items.map((item) => {
          const id = getItemId(item);
          const label = getItemLabel(item);
          const description = getItemDescription?.(item);
          const badge = getBadge?.(item);
          const isSelected = selectedIds.includes(id);

          return (
            <div key={id} className="checkbox-grid__item">
              <input
                ref={(el) => {
                  if (el) {
                    checkboxRefs.current.set(id, el);
                  } else {
                    checkboxRefs.current.delete(id);
                  }
                }}
                type="checkbox"
                id={`checkbox-${id}`}
                checked={isSelected}
                onChange={() => onToggle(id)}
                onKeyDown={(e) => handleKeyDown(e, id)}
                className="checkbox-grid__checkbox"
                aria-label={label}
                data-testid={`checkbox-${id}`}
              />
              <label htmlFor={`checkbox-${id}`} className="checkbox-grid__label">
                <div className="checkbox-grid__label-text">{label}</div>
                {description && (
                  <div className="checkbox-grid__description">{description}</div>
                )}
              </label>
              {badge && (
                <div className="checkbox-grid__badge" data-testid={`badge-${id}`}>
                  {badge}
                </div>
              )}
            </div>
          );
        })
      )}
    </div>
  );
}
