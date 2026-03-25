import { type ReactNode, forwardRef, useRef, useEffect } from 'react';
import { useToolbarKeyboardNav } from '../../hooks/useToolbarKeyboardNav';
import './Toolbar.css';

export type ToolbarButtonVariant = 'primary' | 'secondary' | 'danger';

export interface ToolbarButtonProps
  extends Omit<React.ButtonHTMLAttributes<HTMLButtonElement>, 'children'> {
  label: string;
  icon?: ReactNode;
  endIcon?: ReactNode;
  shortcut?: string;
  variant?: ToolbarButtonVariant;
}

export const ToolbarButton = forwardRef<HTMLButtonElement, ToolbarButtonProps>(
  ({
    label,
    icon,
    endIcon,
    shortcut,
    variant = 'secondary',
    className,
    title,
    'aria-label': ariaLabel,
    tabIndex,
    ...buttonProps
  }, ref) => {
    const computedAriaLabel = ariaLabel ?? (shortcut ? `${label}, ${shortcut}` : label);
    const { type, ...restButtonProps } = buttonProps;

    return (
      <button
        ref={ref}
        className={`toolbar__button toolbar__button--${variant}${className ? ` ${className}` : ''}`}
        title={title ?? undefined}
        aria-label={computedAriaLabel}
        tabIndex={typeof tabIndex === 'number' ? tabIndex : -1}
        type={type ?? 'button'}
        {...restButtonProps}
      >
        {icon && (
          <span className="toolbar__button-icon" aria-hidden="true">
            {icon}
          </span>
        )}
        <span className="toolbar__button-label">{label}</span>
        {shortcut && (
          <span className="toolbar__button-shortcut" aria-hidden="true">
            {shortcut}
          </span>
        )}
        {endIcon && (
          <span className="toolbar__button-icon" aria-hidden="true">
            {endIcon}
          </span>
        )}
      </button>
    );
  }
);

ToolbarButton.displayName = 'ToolbarButton';

export function ToolbarSeparator() {
  return <div className="toolbar__separator" aria-hidden="true"></div>;
}

export interface ToolbarAction extends ToolbarButtonProps {
  key: string;
  /** Permite passar `ref` para o botão (ex.: trigger de menu/anchor) */
  buttonRef?: React.Ref<HTMLButtonElement>;
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
  /** Conteúdo à direita, após os botões de ação (ex: ProfilePicker) */
  rightEnd?: ReactNode;
  /** Aria label da toolbar */
  ariaLabel?: string;
  /** Classe CSS adicional */
  className?: string;
  /**
   * Callback para focar a área de conteúdo ao pressionar Enter no campo de busca.
   * Se omitido, usa restoreDefaultFocus() automaticamente.
   */
  onFocusContent?: (() => void) | null;
  /** Indica se a toolbar está em estado de carregamento/processamento */
  isLoading?: boolean;
}

export const Toolbar = forwardRef<HTMLDivElement, ToolbarProps>(({
  left,
  center,
  right,
  searchValue,
  onSearchChange,
  searchPlaceholder = 'Buscar...',
  actions = [],
  rightEnd,
  ariaLabel = 'Barra de ferramentas. Use setas para navegar entre os botões',
  className = '',
  onFocusContent,
  isLoading = false,
}, ref) => {
  const toolbarRef = useToolbarKeyboardNav(onFocusContent);
  const searchInputRef = useRef<HTMLInputElement>(null);

  const combinedRef: React.RefCallback<HTMLDivElement> = (node) => {
    (toolbarRef as React.MutableRefObject<HTMLDivElement | null>).current = node;
    if (!ref) return;
    if (typeof ref === 'function') {
      ref(node);
      return;
    }
    (ref as React.MutableRefObject<HTMLDivElement | null>).current = node;
  };

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
      aria-busy={isLoading}
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

      {(right || rightEnd || actions.length > 0) && (
        <div className="toolbar__section toolbar__right">
          {right}
          {actions.map(({ key, buttonRef, ...buttonProps }) => (
            <ToolbarButton key={key} ref={buttonRef} {...buttonProps} />
          ))}
          {rightEnd}
        </div>
      )}
    </div>
  );
});

Toolbar.displayName = 'Toolbar';
