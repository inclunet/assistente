import { useEffect } from 'react';
import { useAnnouncer } from './useAnnouncer';
import { useEditorStore } from '../store/editorStore';
import { isModalOpen } from '../components/ui/Modal';

/**
 * Atalhos globais para abas do editor:
 * - Ctrl+T ou Ctrl+N: nova aba
 * - Ctrl+W: fechar aba atual
 * - Ctrl+Tab / Ctrl+Shift+Tab: navegar entre abas
 * - Ctrl+1-9: ir para aba N
 */
export function useEditorTabsKeyboardShortcuts() {
  const { announce } = useAnnouncer();

  const tabs = useEditorStore((s) => s.tabs);
  const activeTabId = useEditorStore((s) => s.activeTabId);
  const createTab = useEditorStore((s) => s.createTab);
  const closeTab = useEditorStore((s) => s.closeTab);
  const setActiveTab = useEditorStore((s) => s.setActiveTab);

  useEffect(() => {
    const handleKeyDown = (event: KeyboardEvent) => {
      const target = event.target as HTMLElement;

      // Não intercepta atalhos quando um modal está aberto
      if (isModalOpen()) return;

      // Detecta quando o foco está em inputs/editors (para atalhos que poderiam atrapalhar edição)
      const isInput =
        target?.tagName === 'INPUT' ||
        target?.tagName === 'TEXTAREA' ||
        (target as any)?.isContentEditable ||
        target?.closest?.('.monaco-editor') !== null;

      // Ctrl+T ou Ctrl+N: Nova aba
      if (event.ctrlKey && (event.key === 't' || event.key === 'n') && !event.shiftKey && !event.altKey) {
        event.preventDefault();
        event.stopPropagation();
        window.dispatchEvent(new Event('assistente:flush-rich-editor'));
        createTab();
        window.dispatchEvent(new Event('assistente:focus-editor'));
        announce('Nova aba do editor criada');
        return;
      }

      // Ctrl+W: Fechar aba atual
      if (event.ctrlKey && event.key === 'w' && !event.shiftKey && !event.altKey && activeTabId) {
        event.preventDefault();
        event.stopPropagation();
        window.dispatchEvent(new Event('assistente:flush-rich-editor'));
        closeTab(activeTabId);
        if (!(document.activeElement as HTMLElement | null)?.closest?.('.editor-tabs')) {
          window.dispatchEvent(new Event('assistente:focus-editor'));
        }
        announce('Aba do editor fechada');
        return;
      }

      // Ctrl+F4: Fechar aba atual (padrão Windows)
      if (event.ctrlKey && (event.key === 'F4' || event.key === 'f4') && !event.shiftKey && !event.altKey && activeTabId) {
        // Evita conflito com alguns inputs que possam usar F4
        if (isInput && target?.tagName === 'INPUT') return;
        event.preventDefault();
        event.stopPropagation();
        window.dispatchEvent(new Event('assistente:flush-rich-editor'));
        closeTab(activeTabId);
        if (!(document.activeElement as HTMLElement | null)?.closest?.('.editor-tabs')) {
          window.dispatchEvent(new Event('assistente:focus-editor'));
        }
        announce('Aba do editor fechada');
        return;
      }

      // Ctrl+Tab / Ctrl+Shift+Tab: navegação
      if (event.ctrlKey && event.key === 'Tab') {
        if (tabs.length < 2) return;
        event.preventDefault();

        const currentIndex = tabs.findIndex((t) => t.id === activeTabId);
        if (currentIndex === -1) return;

        const nextIndex = event.shiftKey
          ? currentIndex > 0
            ? currentIndex - 1
            : tabs.length - 1
          : currentIndex < tabs.length - 1
            ? currentIndex + 1
            : 0;

        const nextTab = tabs[nextIndex];
        if (nextTab) {
          window.dispatchEvent(new Event('assistente:flush-rich-editor'));
          setActiveTab(nextTab.id);
          announce(`${nextTab.title}, ${nextIndex + 1} de ${tabs.length}`);
          window.dispatchEvent(new Event('assistente:focus-editor'));
        }
        return;
      }

      // Ctrl+1-9
      if (event.ctrlKey && !event.shiftKey && !event.altKey) {
        const num = parseInt(event.key, 10);
        if (num >= 1 && num <= 9) {
          const targetTab = tabs[num - 1];
          if (!targetTab) return;
          event.preventDefault();
          window.dispatchEvent(new Event('assistente:flush-rich-editor'));
          setActiveTab(targetTab.id);
          announce(`${targetTab.title}, ${num} de ${tabs.length}`);
          window.dispatchEvent(new Event('assistente:focus-editor'));
        }
      }
    };

    window.addEventListener('keydown', handleKeyDown, true);
    return () => window.removeEventListener('keydown', handleKeyDown, true);
  }, [tabs, activeTabId, createTab, closeTab, setActiveTab, announce]);
}

