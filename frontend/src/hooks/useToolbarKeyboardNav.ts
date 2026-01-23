import { useEffect, useRef, useState } from 'react';

/**
 * Hook para gerenciar navegação por teclado em uma toolbar com ARIA
 * Implementa o padrão ARIA Toolbar com roving tabindex:
 * - Tab: foca o elemento atual da toolbar (roving tabindex)
 * - Setas (←→↑↓): navega entre elementos dentro da toolbar
 * - Apenas um elemento tem tabindex="0" por vez
 */
export const useToolbarKeyboardNav = () => {
  const toolbarRef = useRef<HTMLDivElement>(null);
  const [focusedIndex, setFocusedIndex] = useState(0);

  // Obtém todos os elementos focáveis na toolbar
  const getFocusableItems = (): HTMLElement[] => {
    if (!toolbarRef.current) return [];
    return Array.from(
      toolbarRef.current.querySelectorAll<HTMLElement>(
        'button:not([disabled]), [role="combobox"]:not([aria-disabled="true"]), input[role="combobox"]'
      )
    );
  };

  // Atualiza tabindex de todos os itens
  const updateTabIndexes = (currentIndex: number) => {
    const items = getFocusableItems();
    items.forEach((item, index) => {
      item.setAttribute('tabindex', index === currentIndex ? '0' : '-1');
    });
  };

  useEffect(() => {
    const toolbar = toolbarRef.current;
    if (!toolbar) return;

    // Inicializa tabindexes
    updateTabIndexes(focusedIndex);

    const handleKeyDown = (event: KeyboardEvent) => {
      // Apenas processar se o foco está dentro da toolbar
      if (!toolbar.contains(document.activeElement)) return;

      const target = event.target as HTMLElement;
      
      // NÃO interceptar teclas se o foco está dentro de um picker aberto
      // (input com role="combobox" ou listbox)
      const isInsidePicker = 
        target.matches('input[role="combobox"]') ||
        target.closest('[role="listbox"]') !== null ||
        target.closest('.picker-dropdown') !== null;
      
      if (isInsidePicker) {
        // Deixa o picker processar suas próprias teclas
        return;
      }

      const items = getFocusableItems();
      if (items.length === 0) return;

      // Previne comportamento padrão para todas as teclas de navegação na toolbar
      // (mas apenas quando NÃO estamos dentro de um picker)
      if (['ArrowRight', 'ArrowDown', 'ArrowLeft', 'ArrowUp', 'Home', 'End'].includes(event.key)) {
        event.preventDefault();
        event.stopPropagation();
      }

      const currentIndex = items.indexOf(document.activeElement as HTMLElement);
      let nextIndex = currentIndex >= 0 ? currentIndex : focusedIndex;

      switch (event.key) {
        case 'ArrowRight':
        case 'ArrowDown':
          // Não circular: para no último elemento
          nextIndex = Math.min(nextIndex + 1, items.length - 1);
          break;

        case 'ArrowLeft':
        case 'ArrowUp':
          // Não circular: para no primeiro elemento
          nextIndex = Math.max(nextIndex - 1, 0);
          break;

        case 'Home':
          nextIndex = 0;
          break;

        case 'End':
          nextIndex = items.length - 1;
          break;

        default:
          return;
      }

      setFocusedIndex(nextIndex);
      updateTabIndexes(nextIndex);
      items[nextIndex]?.focus();
    };

    // Quando um item recebe foco (via clique ou Tab)
    const handleFocusIn = (event: FocusEvent) => {
      const items = getFocusableItems();
      const index = items.indexOf(event.target as HTMLElement);
      if (index >= 0) {
        setFocusedIndex(index);
        updateTabIndexes(index);
      }
    };

    toolbar.addEventListener('keydown', handleKeyDown);
    toolbar.addEventListener('focusin', handleFocusIn);

    // Observer para detectar mudanças nos itens (adição/remoção de botões)
    const observer = new MutationObserver(() => {
      updateTabIndexes(focusedIndex);
    });

    observer.observe(toolbar, {
      childList: true,
      subtree: true,
      attributes: true,
      attributeFilter: ['disabled'],
    });

    return () => {
      toolbar.removeEventListener('keydown', handleKeyDown);
      toolbar.removeEventListener('focusin', handleFocusIn);
      observer.disconnect();
    };
  }, [focusedIndex]);

  return toolbarRef;
};
