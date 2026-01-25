import { ReactNode, forwardRef, useRef, useEffect } from 'react';
import { useToolbarKeyboardNav } from '../../hooks/useToolbarKeyboardNav';
import './Toolbar.css';

export interface ToolbarAction {
  key: string;
  label: string;
  icon?: string;
  onClick: () => void;
  disabled?: boolean;
  variant?: 'primary' | 'secondary' | 'danger';
  shortcut?: string;
}

export interface ToolbarProps {
  /** Conteúdo à esquerda (ex: título, logo) */
  left?: ReactNode;
  /** Conteúdo no centro (ex: search bar, pickers) */
  center?: ReactNode;
  /** Conteúdo à direita (ex: botões de ação) */
  right?: ReactNode;
  /** Valor do search (controlado) */
  searchValue?: string;
  /** Callback quando search muda */
  onSearchChange?: (value: string) => void;
  /** Placeholder do search */
  searchPlaceholder?: string;
  /** Array de ações como botões */
  actions?: ToolbarAction[];
  /** Aria label da toolbar */
  ariaLabel?: string;
  /** Classe CSS adicional */
  className?: string;
  /** Callback para focar o grid ao pressionar Enter no campo de busca */
  onFocusGrid?: (() => void) | null;
}

export const Toolbar = forwardRef<HTMLDivElement, ToolbarProps>(({
  left,
  center,
  right,
  searchValue,
  onSearchChange,
  searchPlaceholder = 'Buscar...',
  actions = [],
  ariaLabel = 'Barra de ferramentas. Use setas para navegar entre os botões',
  className = '',
  onFocusGrid,
}, ref) => {
  const toolbarRef = useToolbarKeyboardNav(onFocusGrid);
  const searchInputRef = useRef<HTMLInputElement>(null);
  
  // Combina refs se fornecido
  const combinedRef = ref || toolbarRef;

  // Atalho Ctrl+F para focar no campo de busca
  useEffect(() => {
    if (!onSearchChange) return; // Só ativa se houver campo de busca

    const handleKeyDown = (e: KeyboardEvent) => {
      if (e.ctrlKey && e.key === 'f') {
        e.preventDefault();
        searchInputRef.current?.focus();
      }
    };

    window.addEventListener('keydown', handleKeyDown);
    return () => window.removeEventListener('keydown', handleKeyDown);
  }, [onSearchChange]);

  return (
    <div 
      className={`toolbar ${className}`}
      role="toolbar"
      aria-label={ariaLabel}
      ref={combinedRef}
    >
      {left && (
        <div className="toolbar__section toolbar__left">
          {left}
        </div>
      )}

      {(center || onSearchChange) && (
        <div className="toolbar__section toolbar__center">
          {onSearchChange && (
            <input
              ref={searchInputRef}
              type="text"
              className="toolbar__search"
              placeholder={searchPlaceholder}
              value={searchValue}
              onChange={(e) => onSearchChange(e.target.value)}
              aria-label={searchPlaceholder}
              tabIndex={0}
            />
          )}
          {center}
        </div>
      )}

      {(right || actions.length > 0) && (
        <div className="toolbar__section toolbar__right">
          {right}
          {actions.map((action) => (
            <button
              key={action.key}
              className={`toolbar__button toolbar__button--${action.variant || 'secondary'}`}
              onClick={action.onClick}
              disabled={action.disabled}
              title={action.shortcut ? `${action.label} (${action.shortcut})` : action.label}
              aria-label={action.shortcut ? `${action.label}, ${action.shortcut}` : action.label}
              tabIndex={-1}
            >
              {action.icon && (
                <span className="toolbar__button-icon" aria-hidden="true">
                  {action.icon}
                </span>
              )}
              <span className="toolbar__button-label">{action.label}</span>
              {action.shortcut && (
                <span className="toolbar__button-shortcut" aria-hidden="true">
                  {action.shortcut}
                </span>
              )}
            </button>
          ))}
        </div>
      )}
    </div>
  );
});

Toolbar.displayName = 'Toolbar';
