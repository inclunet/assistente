import { useMemo, useRef, forwardRef, useImperativeHandle, useCallback, useLayoutEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import './MenuButton.css';
import { Menu, type MenuItem as MenuModelItem } from '../menu';
import { useAnchoredContextMenu } from '../../hooks/useAnchoredContextMenu';

export interface MenuItem {
  id: string;
  label: string;
  icon: string;
  shortcut?: string;
  onClick?: () => void;
  submenu?: MenuItem[];
}

interface MenuButtonProps {
  items: MenuItem[];
  currentItemId?: string;
  buttonLabel?: string;
  tabIndex?: number;
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
  function MenuButton({ items, currentItemId, buttonLabel, tabIndex }, ref) {
  const { t } = useTranslation();
  const resolvedButtonLabel = buttonLabel ?? t('menu.navLabel');
  const menuButtonRef = useRef<HTMLButtonElement>(null);
  const [autoTabIndex, setAutoTabIndex] = useState<number | null>(null);

  const mapItems = (srcItems: MenuItem[]): MenuModelItem[] =>
    srcItems.map((item) => ({
      id: item.id,
      label: item.label,
      icon: item.icon,
      shortcut: item.shortcut,
      checked: currentItemId === item.id,
      ariaLabel: item.label,
      action: item.onClick,
      submenu: item.submenu ? mapItems(item.submenu) : undefined,
    }));

  const menuItems: MenuModelItem[] = useMemo(
    () => mapItems(items),
    [items, currentItemId],
  );

  const resolveTriggerElement = useCallback((): HTMLElement | null => {
    const buttonEl = menuButtonRef.current;
    if (!buttonEl) return null;
    const candidate = buttonEl.closest('.datagrid-cell') ?? buttonEl;
    return candidate instanceof HTMLElement ? candidate : null;
  }, []);

  const {
    menu,
    openForTrigger,
    closeMenu,
    onSelectItem,
  } = useAnchoredContextMenu({
    onAfterSelect: () => {
      resolveTriggerElement()?.focus();
    },
    onAfterDismiss: () => {
      resolveTriggerElement()?.focus();
    },
  });

  const openMenu = useCallback(() => {
    const trigger = resolveTriggerElement();
    if (!trigger) return;
    openForTrigger(trigger, resolvedButtonLabel, menuItems);
  }, [resolvedButtonLabel, menuItems, openForTrigger, resolveTriggerElement]);

  const toggleMenu = useCallback(() => {
    if (menu.visible) {
      closeMenu();
      return;
    }
    openMenu();
  }, [closeMenu, menu.visible, openMenu]);

  useLayoutEffect(() => {
    if (tabIndex !== undefined) return;
    const buttonEl = menuButtonRef.current;
    if (!buttonEl) return;
    const isInsideGrid = buttonEl.closest('.datagrid-cell') !== null;
    setAutoTabIndex(isInsideGrid ? -1 : 0);
  }, [tabIndex]);

  // Expõe funções para o componente pai via ref
  useImperativeHandle(ref, () => ({
    toggleMenu,
    isOpen: menu.visible,
  }));

  return (
    <div className="menu-wrapper">
      <button
        ref={menuButtonRef}
        type="button"
        className="menu-toggle"
        onClick={() => {
          toggleMenu();
        }}
        onMouseDown={(event) => {
          event.preventDefault();
          if (event.button === 0) {
            resolveTriggerElement()?.focus();
          }
        }}
        aria-expanded={menu.visible}
        aria-haspopup="menu"
        aria-label={resolvedButtonLabel}
        title={resolvedButtonLabel}
        tabIndex={tabIndex ?? autoTabIndex ?? 0}
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
        ariaLabel={menu.ariaLabel || t('menu.navMain')}
        initialFocusItemId={currentItemId}
        onClose={closeMenu}
        onSelect={onSelectItem}
      />
    </div>
  );
});
