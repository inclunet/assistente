import { useEffect, useRef, useState } from 'react';
import { playBumpSound } from '../services/audioFeedback';

/**
 * Hook para gerenciar navegação por teclado em uma toolbar com ARIA
 * Implementa o padrão ARIA Toolbar com roving tabindex:
 * - Tab: foca o elemento atual da toolbar (roving tabindex)
 * - Setas (←→↑↓): navega entre elementos dentro da toolbar
 * - Apenas um elemento tem tabindex="0" por vez
 * @param onFocusGrid Callback para focar o grid ao pressionar Enter no campo de busca
 */
export const useToolbarKeyboardNav = (onFocusGrid?: (() => void) | null) => {
  const toolbarRef = useRef<HTMLDivElement>(null);
  const [focusedIndex, setFocusedIndex] = useState(0);

  // Obtém todos os elementos focáveis na toolbar (EXCETO campo de busca para navegação por setas)
  const getFocusableItems = (): HTMLElement[] => {
    if (!toolbarRef.current) return [];
    return Array.from(
      toolbarRef.current.querySelectorAll<HTMLElement>(
        'button:not([disabled]), [role="combobox"]:not([aria-disabled="true"]), input[role="combobox"]'
      )
    );
  };

  // Atualiza tabindex de todos os itens (botões e pickers, não o campo de busca)
  const updateTabIndexes = (currentIndex: number) => {
    const items = getFocusableItems();
    items.forEach((item, index) => {
      item.setAttribute('tabindex', index === currentIndex ? '0' : '-1');
    });
    
    // Campo de busca sempre tem tabindex="0" para ser alcançado por Tab
    const searchInput = toolbarRef.current?.querySelector<HTMLInputElement>('.toolbar__search');
    if (searchInput) {
      searchInput.setAttribute('tabindex', '0');
    }
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

      // NÃO interceptar teclas de navegação se o foco está no campo de busca
      // O campo de busca precisa de navegação livre para edição de texto
      const isSearchInput = 
        target.matches('input[type="text"].toolbar__search') ||
        target.classList.contains('toolbar__search');
      
      if (isSearchInput) {
        // Permite edição normal do texto no campo de busca
        // Apenas processa Enter para ir ao grid
        if (event.key === 'Enter') {
          event.preventDefault();
          
          // Usa o callback para focar a primeira célula do grid
          if (onFocusGrid) {
            onFocusGrid();
          }
          // Se não há callback, mantém o foco no campo de busca
        }
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
          if (nextIndex === items.length - 1) {
            playBumpSound(); // Bateu no limite
            return;
          }
          nextIndex = Math.min(nextIndex + 1, items.length - 1);
          break;

        case 'ArrowLeft':
        case 'ArrowUp':
          // Não circular: para no primeiro elemento
          if (nextIndex === 0) {
            playBumpSound(); // Bateu no limite
            return;
          }
          nextIndex = Math.max(nextIndex - 1, 0);
          break;

        case 'Home':
          if (nextIndex === 0) {
            playBumpSound();
            return;
          }
          nextIndex = 0;
          break;

        case 'End':
          if (nextIndex === items.length - 1) {
            playBumpSound();
            return;
          }
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
      const target = event.target as HTMLElement;
      
      // Ignora se o foco foi para o campo de busca
      const isSearchInput = 
        target.matches('input[type="text"].toolbar__search') ||
        target.classList.contains('toolbar__search');
      
      if (isSearchInput) {
        return; // Campo de busca não faz parte da navegação por setas
      }
      
      const items = getFocusableItems();
      const index = items.indexOf(target);
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
  }, [focusedIndex, onFocusGrid]); // Adicionado onFocusGrid para capturar mudanças

  return toolbarRef;
};
