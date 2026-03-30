import type { ReactNode } from 'react';

export interface MenuItem {
  id: string;
  label?: string;
  icon?: ReactNode;
  shortcut?: string;
  checked?: boolean;
  separator?: boolean;
  disabled?: boolean;
  danger?: boolean;
  submenu?: MenuItem[];
  action?: () => void;
  ariaLabel?: string;
}

export interface MenuProps {
  items: MenuItem[];
  x?: number;
  y?: number;
  visible?: boolean;
  ariaLabel?: string;
  /**
   * Quando definido, tenta focar esse item ao abrir (se for focável).
   * Útil para menus de navegação principal.
   */
  initialFocusItemId?: string;
  /** Renderiza um campo de busca acima dos itens para filtrar por label. */
  searchable?: boolean;
  /** Placeholder do campo de busca (default: "Buscar..."). */
  searchPlaceholder?: string;
  onClose?: () => void;
  onSelect?: (item: MenuItem) => void;
  /** Callback for custom key handling on the focused item. Return true to prevent default menu behavior. */
  onItemKeyDown?: (event: React.KeyboardEvent, item: MenuItem) => boolean | void;
}

// Compat: o “ContextMenu” é uma variação de Menu (abrir em coordenadas/âncora externa).
export type ContextMenuProps = MenuProps;
