import { useState, useRef, useEffect, forwardRef, useImperativeHandle } from 'react';
import './MenuButton.css';

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
  const [isOpen, setIsOpen] = useState(false);
  const [focusedIndex, setFocusedIndex] = useState(-1);
  const menuButtonRef = useRef<HTMLButtonElement>(null);
  const menuRef = useRef<HTMLUListElement>(null);

  // Expõe funções para o componente pai via ref
  useImperativeHandle(ref, () => ({
    toggleMenu: () => {
      setIsOpen((prev) => !prev);
    },
    isOpen,
  }));

  // Fecha menu ao clicar fora
  useEffect(() => {
    if (!isOpen) return;

    const handleClickOutside = (event: MouseEvent) => {
      if (
        menuRef.current &&
        menuButtonRef.current &&
        !menuRef.current.contains(event.target as Node) &&
        !menuButtonRef.current.contains(event.target as Node)
      ) {
        closeMenu();
      }
    };

    document.addEventListener('mousedown', handleClickOutside);
    return () => document.removeEventListener('mousedown', handleClickOutside);
  }, [isOpen]);

  // Foca o item ativo quando o menu abre
  useEffect(() => {
    if (isOpen) {
      const currentIndex = currentItemId
        ? items.findIndex((item) => item.id === currentItemId)
        : 0;
      setFocusedIndex(currentIndex >= 0 ? currentIndex : 0);
      
      // Foca o item após o próximo render
      setTimeout(() => {
        const menuItems = menuRef.current?.querySelectorAll('[role="menuitem"]');
        const itemToFocus = menuItems?.[currentIndex >= 0 ? currentIndex : 0] as HTMLElement;
        itemToFocus?.focus();
      }, 0);
    }
  }, [isOpen, currentItemId, items]);

  const toggleMenu = () => {
    setIsOpen(!isOpen);
  };

  const closeMenu = () => {
    setIsOpen(false);
    setFocusedIndex(-1);
    menuButtonRef.current?.focus();
  };

  const handleMenuKeyDown = (event: React.KeyboardEvent) => {
    if (!isOpen) return;

    const itemCount = items.length;

    switch (event.key) {
      case 'ArrowDown':
        event.preventDefault();
        event.stopPropagation();
        setFocusedIndex((prev) => {
          const nextIndex = (prev + 1) % itemCount;
          focusItem(nextIndex);
          return nextIndex;
        });
        break;

      case 'ArrowUp':
        event.preventDefault();
        event.stopPropagation();
        setFocusedIndex((prev) => {
          const nextIndex = prev <= 0 ? itemCount - 1 : prev - 1;
          focusItem(nextIndex);
          return nextIndex;
        });
        break;

      case 'Home':
        event.preventDefault();
        event.stopPropagation();
        setFocusedIndex(0);
        focusItem(0);
        break;

      case 'End':
        event.preventDefault();
        event.stopPropagation();
        setFocusedIndex(itemCount - 1);
        focusItem(itemCount - 1);
        break;

      case 'Enter':
      case ' ':
        event.preventDefault();
        event.stopPropagation();
        if (focusedIndex >= 0 && items[focusedIndex]) {
          items[focusedIndex].onClick();
          closeMenu();
        }
        break;

      case 'Escape':
        event.preventDefault();
        event.stopPropagation();
        closeMenu();
        break;

      case 'Tab':
        // Fecha menu ao sair com Tab
        closeMenu();
        break;
    }
  };

  const focusItem = (index: number) => {
    const menuItems = menuRef.current?.querySelectorAll('[role="menuitem"]');
    const itemToFocus = menuItems?.[index] as HTMLElement;
    itemToFocus?.focus();
  };

  const handleItemClick = (item: MenuItem) => {
    item.onClick();
    closeMenu();
  };

  return (
    <div className="menu-wrapper">
      <button
        ref={menuButtonRef}
        className="menu-toggle"
        onClick={toggleMenu}
        aria-expanded={isOpen}
        aria-haspopup="menu"
        aria-label={buttonLabel}
        title={buttonLabel}
      >
        <span className="menu-icon" aria-hidden="true">
          ☰
        </span>
      </button>

      {isOpen && (
        <ul
          ref={menuRef}
          className="nav-list"
          role="menu"
          aria-label="Navegação principal"
          onKeyDown={handleMenuKeyDown}
        >
          {items.map((item, index) => {
            const isActive = currentItemId === item.id;
            return (
              <li key={item.id} role="none">
                <button
                  role="menuitem"
                  className={`nav-item ${isActive ? 'active' : ''}`}
                  aria-current={isActive ? 'page' : undefined}
                  tabIndex={focusedIndex === index ? 0 : -1}
                  onClick={() => handleItemClick(item)}
                >
                  <span className="nav-icon" aria-hidden="true">
                    {item.icon}
                  </span>
                  <span className="nav-label">{item.label}</span>
                  {item.shortcut && <span className="nav-shortcut">{item.shortcut}</span>}
                </button>
              </li>
            );
          })}
        </ul>
      )}
    </div>
  );
});
