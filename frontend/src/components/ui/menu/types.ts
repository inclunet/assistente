export interface MenuItem {
  id: string;
  label?: string;
  icon?: string;
  shortcut?: string;
  checked?: boolean;
  separator?: boolean;
  disabled?: boolean;
  danger?: boolean;
  submenu?: MenuItem[];
  action?: () => void;
  ariaLabel?: string; // Label para leitores de tela (sem emoji)
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
  onClose?: () => void;
  onSelect?: (item: MenuItem) => void;
}

// Compat: o “ContextMenu” é uma variação de Menu (abrir em coordenadas/âncora externa).
export type ContextMenuProps = MenuProps;
