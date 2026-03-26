import React, { useEffect, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { CheckOutlined, RightOutlined } from '@ant-design/icons';
import type { MenuItem, MenuProps } from './types';
import { restoreDefaultFocus } from '../../../hooks/useDefaultFocus';

import '../ContextMenu.css';

export const Menu: React.FC<MenuProps> = ({
  items,
  x = 0,
  y = 0,
  visible = false,
  ariaLabel = 'Menu',
  initialFocusItemId,
  searchable = false,
  searchPlaceholder = 'Buscar...',
  onClose,
  onSelect,
  onItemKeyDown,
}) => {
  const { t } = useTranslation();
  const menuRef = useRef<HTMLDivElement>(null);
  const searchInputRef = useRef<HTMLInputElement>(null);
  const onCloseRef = useRef(onClose);
  onCloseRef.current = onClose;
  const itemsRef = useRef(items);
  itemsRef.current = items;
  const [position, setPosition] = useState({ x, y });
  const [searchQuery, setSearchQuery] = useState('');
  // Stack de submenus abertos (IDs dos items)
  const [submenuStack, setSubmenuStack] = useState<string[]>([]);
  // Stack de índices focados em cada nível
  const [focusStack, setFocusStack] = useState<number[]>([0]);
  const [announcement, setAnnouncement] = useState('');
  const [searchFocused, setSearchFocused] = useState(false);

  const filteredItems = searchable && searchQuery.trim()
    ? items.filter(item => item.separator || item.label?.toLowerCase().includes(searchQuery.toLowerCase()))
    : items;

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
    let currentItems = filteredItems;
    for (const submenuId of submenuStack) {
      const parentItem = currentItems.find((item) => item.id === submenuId);
      if (parentItem?.submenu) {
        currentItems = parentItem.submenu;
      }
    }
    return currentItems.filter((item) => !item.separator);
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
        onCloseRef.current?.();
        requestAnimationFrame(() => restoreDefaultFocus());
      }
    };

    document.addEventListener('mousedown', handleClickOutside);

    return () => {
      document.removeEventListener('mousedown', handleClickOutside);
    };
  }, [visible]);

  // Foca no primeiro item (ou no campo de busca) quando abre
  useEffect(() => {
    if (visible && menuRef.current) {
      setSubmenuStack([]);
      setSearchQuery('');
      const topLevelItems = itemsRef.current.filter((i) => !i.separator);
      const preferredIndex =
        initialFocusItemId
          ? topLevelItems.findIndex((i) => i.id === initialFocusItemId && isFocusableItem(i))
          : -1;
      setFocusStack([preferredIndex >= 0 ? preferredIndex : firstFocusableIndex(topLevelItems)]);

      if (searchable) {
        setSearchFocused(true);
        requestAnimationFrame(() => searchInputRef.current?.focus());
      } else {
        setSearchFocused(false);
        const firstButton = menuRef.current.querySelector('button:not([disabled])');
        if (firstButton instanceof HTMLElement) {
          firstButton.focus();
        }
      }
    }
  }, [visible, initialFocusItemId, searchable]);

  // Resetar foco quando o filtro muda
  useEffect(() => {
    if (searchable && visible) {
      setSubmenuStack([]);
      const topItems = filteredItems.filter(i => !i.separator);
      setFocusStack([firstFocusableIndex(topItems)]);
    }
  }, [searchQuery]); // eslint-disable-line react-hooks/exhaustive-deps

  // Move o foco quando muda
  useEffect(() => {
    if (visible && menuRef.current) {
      if (searchFocused) return;
      let currentItems = filteredItems as MenuItem[];
      for (const submenuId of submenuStack) {
        const parentItem = currentItems.find((item) => item.id === submenuId);
        if (parentItem?.submenu) {
          currentItems = parentItem.submenu;
        }
      }
      const focusableItems = currentItems.filter((item) => !item.separator);
      const currentFocusIndex = focusStack[focusStack.length - 1] || 0;
      const targetItem = focusableItems[currentFocusIndex];

      if (targetItem) {
        const button = menuRef.current.querySelector(`button#${CSS.escape(targetItem.id)}`);
        if (button instanceof HTMLElement) {
          button.focus();
        }
      }
    }
  }, [focusStack, submenuStack, visible]);

  const handleSearchKeyDown = (e: React.KeyboardEvent<HTMLInputElement>) => {
    if (e.key === 'ArrowDown') {
      e.preventDefault();
      setSearchFocused(false);
      const currentItems = getCurrentItems();
      setFocusStack([firstFocusableIndex(currentItems)]);
    } else if (e.key === 'Escape') {
      e.preventDefault();
      if (searchQuery) {
        setSearchQuery('');
      } else {
        onClose?.();
        requestAnimationFrame(() => restoreDefaultFocus());
      }
    } else if (e.key === 'Tab') {
      e.preventDefault();
      onClose?.();
      requestAnimationFrame(() => restoreDefaultFocus());
    } else if (e.key === 'Enter') {
      e.preventDefault();
      const currentItems = getCurrentItems();
      const first = currentItems.find(i => isFocusableItem(i));
      if (first?.action) {
        first.action();
        onSelect?.(first);
        onClose?.();
        requestAnimationFrame(() => restoreDefaultFocus());
      }
    }
  };

  // Navegação por teclado
  const handleKeyDown = (e: React.KeyboardEvent) => {
    if (['ArrowDown', 'ArrowUp', 'ArrowLeft', 'ArrowRight', 'Escape', 'Enter', ' ', 'Tab'].includes(e.key)) {
      e.stopPropagation();
    }

    // Tab fecha o menu (padrão ARIA Menu Button)
    if (e.key === 'Tab') {
      e.preventDefault();
      onClose?.();
      requestAnimationFrame(() => restoreDefaultFocus());
      return;
    }
    const currentItems = getCurrentItems();
    const currentFocusIndex = getCurrentFocusIndex();
    const currentItem = currentItems[currentFocusIndex];

    // Allow consumer to handle custom keys (e.g. F2 for rename)
    if (currentItem && onItemKeyDown?.(e, currentItem)) {
      return;
    }

    switch (e.key) {
      case 'ArrowDown':
        e.preventDefault();
        setFocusStack((prev) => {
          const newStack = [...prev];
          newStack[newStack.length - 1] = nextFocusableIndex(currentItems, currentFocusIndex, 1);
          return newStack;
        });
        break;

      case 'ArrowUp':
        e.preventDefault();
        if (searchable && submenuStack.length === 0 && currentFocusIndex <= firstFocusableIndex(currentItems)) {
          setSearchFocused(true);
          requestAnimationFrame(() => searchInputRef.current?.focus());
          break;
        }
        setFocusStack((prev) => {
          const newStack = [...prev];
          newStack[newStack.length - 1] = nextFocusableIndex(currentItems, currentFocusIndex, -1);
          return newStack;
        });
        break;

      case 'ArrowRight':
        e.preventDefault();
        if (currentItem?.submenu && currentItem.submenu.length > 0 && !currentItem.disabled) {
          setSubmenuStack((prev) => [...prev, currentItem.id]);
          const submenuItems = currentItem.submenu.filter((item) => !item.separator);
          setFocusStack((prev) => [...prev, firstFocusableIndex(submenuItems)]);
          announce(`Submenu aberto: ${currentItem.label}. ${submenuItems.length} opções disponíveis.`);
        }
        break;

      case 'ArrowLeft':
        e.preventDefault();
        if (submenuStack.length > 0) {
          // Fecha o submenu atual e volta para o nível anterior
          setSubmenuStack((prev) => prev.slice(0, -1));
          setFocusStack((prev) => prev.slice(0, -1));
          announce('Submenu fechado. Voltando ao menu anterior.');
        }
        // Se estiver no menu raiz, não faz nada
        break;

      case 'Escape':
        e.preventDefault();
        if (submenuStack.length > 0) {
          // Se está em submenu, volta para o nível anterior
          setSubmenuStack((prev) => prev.slice(0, -1));
          setFocusStack((prev) => prev.slice(0, -1));
          announce('Submenu fechado. Voltando ao menu anterior.');
        } else {
          onClose?.();
          requestAnimationFrame(() => restoreDefaultFocus());
        }
        break;

      case 'Enter':
      case ' ': {
        e.preventDefault();
        if (currentItem?.submenu && currentItem.submenu.length > 0 && !currentItem.disabled) {
          // Abre submenu
          setSubmenuStack((prev) => [...prev, currentItem.id]);
          const submenuItems = currentItem.submenu.filter((item) => !item.separator);
          setFocusStack((prev) => [...prev, firstFocusableIndex(submenuItems)]);
          announce(`Submenu aberto: ${currentItem.label}. ${submenuItems.length} opções disponíveis.`);
        } else if (currentItem?.action && !currentItem?.disabled) {
          currentItem.action();
          onSelect?.(currentItem);
          onClose?.();
          requestAnimationFrame(() => restoreDefaultFocus());
        }
        break;
      }
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

      const buttonElement = (
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
                requestAnimationFrame(() => restoreDefaultFocus());
              }
            }}
            onMouseEnter={() => {
              if (hasSubmenu && !item.disabled) {
                setSubmenuStack((prev) => [...prev.slice(0, level), item.id]);
                const submenuItems = item.submenu!.filter((subitem) => !subitem.separator);
                setFocusStack((prev) => [...prev.slice(0, level + 1), firstFocusableIndex(submenuItems)]);
              } else if (!hasSubmenu) {
                // Fecha submenus abertos neste nível ao hover em item sem submenu
                setSubmenuStack((prev) => prev.slice(0, level));
                setFocusStack((prev) => {
                  const newStack = prev.slice(0, level + 1);
                  newStack[level] = index;
                  return newStack;
                });
              }
            }}
            role="menuitem"
            aria-haspopup={hasSubmenu ? 'menu' : undefined}
            aria-expanded={hasSubmenu ? isSubmenuOpen : undefined}
            aria-label={
              (item.ariaLabel || item.label) + (item.shortcut ? `, ${item.shortcut}` : '')
            }
            tabIndex={isFocused ? 0 : -1}
          >
            {item.icon && (
              <span className="context-menu__icon" aria-hidden="true">
                {item.icon}
              </span>
            )}
            <span className="context-menu__label">{item.label}</span>
            {item.checked && (
              <span className="context-menu__check" aria-hidden="true">
                <CheckOutlined />
              </span>
            )}
            {item.shortcut && <span className="context-menu__shortcut" aria-hidden="true">{item.shortcut}</span>}
            {hasSubmenu && <span className="context-menu__arrow" aria-hidden="true"><RightOutlined /></span>}
          </button>
      );

      if (hasSubmenu) {
        return (
          <div key={item.id} className="context-menu__item-wrapper">
            {buttonElement}
            {isSubmenuOpen && (
              <div className="context-menu context-menu--submenu" role="menu">
                {renderItems(item.submenu!, level + 1)}
              </div>
            )}
          </div>
        );
      }

      return (
        <React.Fragment key={item.id}>
          {buttonElement}
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
      <div role="status" aria-live="polite" aria-atomic="true" className="sr-only">
        {announcement}
      </div>

      <div
        ref={menuRef}
        className={`context-menu${searchable ? ' context-menu--searchable' : ''}`}
        style={{ left: `${position.x}px`, top: `${position.y}px` }}
        role="menu"
        aria-label={ariaLabel}
        aria-activedescendant={searchFocused ? undefined : activeItemId}
        tabIndex={-1}
        onKeyDown={handleKeyDown}
      >
        {searchable && (
          <div className="context-menu__search" role="presentation">
            <input
              ref={searchInputRef}
              type="text"
              className="context-menu__search-input"
              placeholder={searchPlaceholder}
              value={searchQuery}
              onChange={(e) => setSearchQuery(e.target.value)}
              onKeyDown={handleSearchKeyDown}
              onFocus={() => setSearchFocused(true)}
              aria-label={searchPlaceholder}
              autoComplete="off"
            />
          </div>
        )}
        {renderItems(filteredItems, 0)}
        {searchable && searchQuery && filteredItems.filter(i => !i.separator).length === 0 && (
          <div className="context-menu__empty" role="presentation">
            {t('common.noResults', 'Nenhum resultado')}
          </div>
        )}
      </div>
    </>
  );
};
