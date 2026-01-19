/**
 * Hook para gerenciar atalhos globais de teclado das abas
 * Ctrl+T: Nova aba
 * Ctrl+W: Fechar aba atual
 * Ctrl+Tab: Próxima aba
 * Ctrl+Shift+Tab: Aba anterior
 * Ctrl+1-9: Ir para aba N
 */

import { useEffect } from 'react';
import { useTabsStore } from '../store/tabsStore';

export function useTabsKeyboardShortcuts() {
  const { tabs, activeTabId, createTab, closeTab, setActiveTab } = useTabsStore();

  useEffect(() => {
    const handleKeyDown = (event: KeyboardEvent) => {
      // Ignora se estiver em um input/textarea (exceto para Ctrl+T, Ctrl+W, Ctrl+Tab)
      const target = event.target as HTMLElement;
      const isInputField =
        target.tagName === 'INPUT' ||
        target.tagName === 'TEXTAREA' ||
        target.isContentEditable;

      // Ctrl+T: Nova aba
      if (event.ctrlKey && event.key === 't' && !event.shiftKey && !event.altKey) {
        event.preventDefault();
        createTab();
        return;
      }

      // Ctrl+W: Fechar aba atual
      if (
        event.ctrlKey &&
        event.key === 'w' &&
        !event.shiftKey &&
        !event.altKey &&
        activeTabId
      ) {
        event.preventDefault();
        closeTab(activeTabId);
        return;
      }

      // Ctrl+Tab: Próxima aba
      if (event.ctrlKey && event.key === 'Tab' && !event.shiftKey) {
        event.preventDefault();
        const currentIndex = tabs.findIndex(t => t.id === activeTabId);
        if (currentIndex !== -1) {
          const nextIndex = currentIndex < tabs.length - 1 ? currentIndex + 1 : 0;
          const nextTab = tabs[nextIndex];
          if (nextTab) {
            setActiveTab(nextTab.id);
          }
        }
        return;
      }

      // Ctrl+Shift+Tab: Aba anterior
      if (event.ctrlKey && event.key === 'Tab' && event.shiftKey) {
        event.preventDefault();
        const currentIndex = tabs.findIndex(t => t.id === activeTabId);
        if (currentIndex !== -1) {
          const prevIndex = currentIndex > 0 ? currentIndex - 1 : tabs.length - 1;
          const prevTab = tabs[prevIndex];
          if (prevTab) {
            setActiveTab(prevTab.id);
          }
        }
        return;
      }

      // Ctrl+1-9: Ir para aba N
      if (event.ctrlKey && !event.shiftKey && !event.altKey) {
        const num = parseInt(event.key, 10);
        if (num >= 1 && num <= 9) {
          event.preventDefault();
          const targetTab = tabs[num - 1];
          if (targetTab) {
            setActiveTab(targetTab.id);
          }
        }
      }
    };

    window.addEventListener('keydown', handleKeyDown);
    return () => window.removeEventListener('keydown', handleKeyDown);
  }, [tabs, activeTabId, createTab, closeTab, setActiveTab]);
}
