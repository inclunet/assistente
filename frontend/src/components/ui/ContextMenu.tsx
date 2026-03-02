import React, { useEffect, useRef, useState } from 'react';
import './ContextMenu.css';

export interface MenuItem {
  id: string;
  label?: string;
  icon?: string;
  shortcut?: string;
  separator?: boolean;
  disabled?: boolean;
  danger?: boolean;
  submenu?: MenuItem[];
  action?: () => void;
  ariaLabel?: string; // Label para leitores de tela (sem emoji)
}

export interface ContextMenuProps {
  items: MenuItem[];
  x?: number;
  y?: number;
  visible?: boolean;
  ariaLabel?: string;
  onClose?: () => void;
  onSelect?: (item: MenuItem) => void;
}

export const ContextMenu: React.FC<ContextMenuProps> = ({
  items,
  x = 0,
  y = 0,
  visible = false,
  ariaLabel = 'Menu de contexto',
  onClose,
  onSelect,
}) => {
  const menuRef = useRef<HTMLDivElement>(null);
  const [position, setPosition] = useState({ x, y });
  // Stack de submenus abertos (IDs dos items)
  const [submenuStack, setSubmenuStack] = useState<string[]>([]);
  // Stack de índices focados em cada nível
  const [focusStack, setFocusStack] = useState<number[]>([0]);
  const [announcement, setAnnouncement] = useState('');

  // Anuncia mudanças para leitores de tela
  const announce = (message: string) => {
    setAnnouncement(message);
    setTimeout(() => setAnnouncement(''), 100);
  };

  // Helpers para navegação multinível
  const getCurrentFocusIndex = () => focusStack[focusStack.length - 1] || 0;

  const isFocusableItem = (item: MenuItem) => !item.separator && !item.disabled;

  const firstFocusableIndex = (itemsList: MenuItem[]) => {
    const idx = itemsList.findIndex((i) => isFocusableItem(i));
    return idx >= 0 ? idx : 0;
  };

  const nextFocusableIndex = (itemsList: MenuItem[], startIndex: number, direction: 1 | -1) => {
    if (itemsList.length === 0) return 0;

    // Evita loop infinito se tudo estiver desabilitado
    for (let step = 1; step <= itemsList.length; step += 1) {
      const idx = (startIndex + direction * step + itemsList.length) % itemsList.length;
      if (isFocusableItem(itemsList[idx])) return idx;
    }

    return startIndex;
  };
  
  const getCurrentItems = (): MenuItem[] => {
    let currentItems = items;
    for (const submenuId of submenuStack) {
      const parentItem = currentItems.find(item => item.id === submenuId);
      if (parentItem?.submenu) {
        currentItems = parentItem.submenu;
      }
    }
    return currentItems.filter(item => !item.separator);
  };

  // Atualiza posição quando x ou y mudam
  useEffect(() => {
    if (visible && menuRef.current) {
      const menu = menuRef.current;
      const rect = menu.getBoundingClientRect();
      let newX = x;
      let newY = y;

      // Ajusta se sair da tela à direita
      if (x + rect.width > window.innerWidth) {
        newX = window.innerWidth - rect.width - 10;
      }

      // Ajusta se sair da tela abaixo
      if (y + rect.height > window.innerHeight) {
        newY = window.innerHeight - rect.height - 10;
      }

      setPosition({ x: Math.max(10, newX), y: Math.max(10, newY) });
    }
  }, [x, y, visible]);

  // Fecha ao clicar fora
  useEffect(() => {
    if (!visible) return;

    const handleClickOutside = (e: MouseEvent) => {
      if (menuRef.current && !menuRef.current.contains(e.target as Node)) {
        onClose?.();
      }
    };

    const handleEscape = (e: KeyboardEvent) => {
      if (e.key === 'Escape') {
        onClose?.();
      }
    };

    document.addEventListener('mousedown', handleClickOutside);
    document.addEventListener('keydown', handleEscape);

    return () => {
      document.removeEventListener('mousedown', handleClickOutside);
      document.removeEventListener('keydown', handleEscape);
    };
  }, [visible, onClose]);

  // Foca no primeiro item quando abre
  useEffect(() => {
    if (visible && menuRef.current) {
      setSubmenuStack([]);
      setFocusStack([firstFocusableIndex(items.filter((i) => !i.separator))]);
      const firstButton = menuRef.current.querySelector('button:not([disabled])');
      if (firstButton instanceof HTMLElement) {
        firstButton.focus();
      }
    }
  }, [visible, items]);

  // Move o foco quando muda
  useEffect(() => {
    if (visible && menuRef.current) {
      const currentItems = getCurrentItems();
      const currentFocusIndex = getCurrentFocusIndex();
      const targetItem = currentItems[currentFocusIndex];
      
      if (targetItem) {
        const button = menuRef.current.querySelector(`button#${CSS.escape(targetItem.id)}`);
        if (button instanceof HTMLElement) {
          button.focus();
        }
      }
    }
  }, [focusStack, visible]);

  // Navegação por teclado
  const handleKeyDown = (e: React.KeyboardEvent) => {
    const currentItems = getCurrentItems();
    const currentFocusIndex = getCurrentFocusIndex();
    const currentItem = currentItems[currentFocusIndex];

    switch (e.key) {
      case 'ArrowDown':
        e.preventDefault();
        setFocusStack(prev => {
          const newStack = [...prev];
          newStack[newStack.length - 1] = nextFocusableIndex(currentItems, currentFocusIndex, 1);
          return newStack;
        });
        break;

      case 'ArrowUp':
        e.preventDefault();
        setFocusStack(prev => {
          const newStack = [...prev];
          newStack[newStack.length - 1] = nextFocusableIndex(currentItems, currentFocusIndex, -1);
          return newStack;
        });
        break;

      case 'ArrowRight':
        e.preventDefault();
        if (currentItem?.submenu && currentItem.submenu.length > 0 && !currentItem.disabled) {
          setSubmenuStack(prev => [...prev, currentItem.id]);
          const submenuItems = currentItem.submenu.filter(item => !item.separator);
          setFocusStack(prev => [...prev, firstFocusableIndex(submenuItems)]);
          announce(`Submenu aberto: ${currentItem.label}. ${submenuItems.length} opções disponíveis.`);
        }
        break;

      case 'ArrowLeft':
        e.preventDefault();
        if (submenuStack.length > 0) {
          // Fecha o submenu atual e volta para o nível anterior
          setSubmenuStack(prev => prev.slice(0, -1));
          setFocusStack(prev => prev.slice(0, -1));
          announce('Submenu fechado. Voltando ao menu anterior.');
        }
        // Se estiver no menu raiz, não faz nada
        break;

      case 'Escape':
        e.preventDefault();
        if (submenuStack.length > 0) {
          // Se está em submenu, volta para o nível anterior
          setSubmenuStack(prev => prev.slice(0, -1));
          setFocusStack(prev => prev.slice(0, -1));
          announce('Submenu fechado. Voltando ao menu anterior.');
        } else {
          // Se está no menu raiz, fecha o menu
          onClose?.();
        }
        break;

      case 'Enter':
      case ' ':
        e.preventDefault();
        if (currentItem?.submenu && currentItem.submenu.length > 0 && !currentItem.disabled) {
          // Abre submenu
          setSubmenuStack(prev => [...prev, currentItem.id]);
          const submenuItems = currentItem.submenu.filter(item => !item.separator);
          setFocusStack(prev => [...prev, firstFocusableIndex(submenuItems)]);
          announce(`Submenu aberto: ${currentItem.label}. ${submenuItems.length} opções disponíveis.`);
        } else if (currentItem?.action && !currentItem?.disabled) {
          // Executa ação
          currentItem.action();
          onSelect?.(currentItem);
          onClose?.();
        }
        break;
    }
  };

  // Renderiza items recursivamente
  const renderItems = (itemsList: MenuItem[], level: number): JSX.Element[] => {
    const currentLevelFocus = focusStack[level] || 0;
    const isCurrentLevel = level === submenuStack.length;

    // O focusStack index é relativo apenas a itens não-separadores.
    let visibleIndex = -1;

    return itemsList.map((item) => {
      if (item.separator) {
        return <div key={item.id} className="context-menu__separator" role="separator" />;
      }

      visibleIndex += 1;
      const index = visibleIndex;

      const hasSubmenu = item.submenu && item.submenu.length > 0;
      const isSubmenuOpen = submenuStack[level] === item.id;
      const isFocused = isCurrentLevel && index === currentLevelFocus;

      return (
        <React.Fragment key={item.id}>
          <button
            id={item.id}
            className={`context-menu__item ${item.danger ? 'context-menu__item--danger' : ''} ${
              isFocused ? 'context-menu__item--focused' : ''
            }`}
            disabled={!!item.disabled}
            onClick={(e) => {
              e.preventDefault();
              if (item.disabled) return;
              if (hasSubmenu) {
                setSubmenuStack((prev) => [...prev.slice(0, level), item.id]);
                const submenuItems = item.submenu!.filter((subitem) => !subitem.separator);
                setFocusStack((prev) => [...prev.slice(0, level + 1), firstFocusableIndex(submenuItems)]);
                announce(`Submenu aberto: ${item.label}. ${submenuItems.length} opções disponíveis.`);
              } else {
                item.action?.();
                onSelect?.(item);
                onClose?.();
              }
            }}
            role="menuitem"
            aria-haspopup={hasSubmenu ? 'menu' : undefined}
            aria-expanded={hasSubmenu ? isSubmenuOpen : undefined}
            aria-label={item.ariaLabel || item.label}
            tabIndex={-1}
          >
            {item.icon && <span className="context-menu__icon" aria-hidden="true">{item.icon}</span>}
            <span className="context-menu__label">{item.label}</span>
            {item.shortcut && <span className="context-menu__shortcut">{item.shortcut}</span>}
            {hasSubmenu && <span className="context-menu__arrow">▶</span>}
          </button>
          {hasSubmenu && isSubmenuOpen && (
            <div className="context-menu context-menu--submenu" role="menu">
              {renderItems(item.submenu!, level + 1)}
            </div>
          )}
        </React.Fragment>
      );
    });
  };

  if (!visible) return null;

  const currentItems = getCurrentItems();
  const currentFocusIndex = getCurrentFocusIndex();
  const activeItemId = currentItems[currentFocusIndex]?.id;

  return (
    <>
      {/* Região de anúncio para leitores de tela */}
      <div 
        role="status" 
        aria-live="polite" 
        aria-atomic="true"
        className="sr-only"
      >
        {announcement}
      </div>
      
      <div
        ref={menuRef}
        className="context-menu"
        style={{ left: `${position.x}px`, top: `${position.y}px` }}
        role="menu"
        aria-label={ariaLabel}
        aria-activedescendant={activeItemId}
        tabIndex={-1}
        onKeyDown={handleKeyDown}
      >
        {renderItems(items, 0)}
      </div>
    </>
  );
};
