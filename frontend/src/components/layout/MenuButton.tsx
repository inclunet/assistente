import { useMemo, useRef, forwardRef, useImperativeHandle, useCallback } from 'react';
import './MenuButton.css';
import { Menu, type MenuItem as MenuModelItem } from '../menu';
import { useAnchoredContextMenu } from '../../hooks/useAnchoredContextMenu';

export interface MenuItem {
  id: string;
  label: string;
  icon: string;
  shortcut?: string;
  onClick: () => void;
}

interface MenuButtonProps {
  items: MenuItem[];
  currentItemId?: string;
  buttonLabel?: string;
}

export interface MenuButtonRef {
  toggleMenu: () => void;
  isOpen: boolean;
}

/**
 * Menu acessível com navegação por teclado
 * Implementa padrão ARIA Menu Button:
 * - Alt+M: Abre/fecha menu (atalho global)
 * - Enter/Space: Abre menu
 * - Arrow Down/Up: Navega entre itens
 * - Home/End: Primeiro/último item
 * - Enter/Space no item: Executa ação
 * - Escape: Fecha menu
 * - Tab: Fecha menu e move foco
 */
export const MenuButton = forwardRef<MenuButtonRef, MenuButtonProps>(
  function MenuButton({ items, currentItemId, buttonLabel = 'Menu de navegação' }, ref) {
  const menuButtonRef = useRef<HTMLButtonElement>(null);

  const menuItems: MenuModelItem[] = useMemo(() => {
    return items.map((item) => ({
      id: item.id,
      label: item.label,
      icon: item.icon,
      shortcut: item.shortcut,
      checked: currentItemId === item.id,
      ariaLabel: item.label,
      action: item.onClick,
    }));
  }, [items, currentItemId]);

  const {
    menu,
    openForTrigger,
    closeMenu,
    onSelectItem,
  } = useAnchoredContextMenu({
    onAfterSelect: () => {
      menuButtonRef.current?.focus?.();
    },
  });

  const openMenu = useCallback(() => {
    const trigger = menuButtonRef.current;
    if (!trigger) return;
    openForTrigger(trigger, buttonLabel, menuItems);
  }, [buttonLabel, menuItems, openForTrigger]);

  const toggleMenu = useCallback(() => {
    if (menu.visible) {
      closeMenu();
      return;
    }
    openMenu();
  }, [closeMenu, menu.visible, openMenu]);

  // Expõe funções para o componente pai via ref
  useImperativeHandle(ref, () => ({
    toggleMenu,
    isOpen: menu.visible,
  }));

  return (
    <div className="menu-wrapper">
      <button
        ref={menuButtonRef}
        className="menu-toggle"
        onClick={toggleMenu}
        aria-expanded={menu.visible}
        aria-haspopup="menu"
        aria-label={buttonLabel}
        title={buttonLabel}
      >
        <span className="menu-icon" aria-hidden="true">
          ☰
        </span>
      </button>

      <Menu
        items={menu.items}
        x={menu.x}
        y={menu.y}
        visible={menu.visible}
        ariaLabel={menu.ariaLabel || 'Navegação principal'}
        initialFocusItemId={currentItemId}
        onClose={closeMenu}
        onSelect={onSelectItem}
      />
    </div>
  );
});
