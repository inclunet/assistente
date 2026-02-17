/**
 * Hook para gerenciar atalhos globais de teclado das abas
 * Ctrl+T ou Ctrl+N: Nova aba
 * Ctrl+W: Fechar aba atual
 * Ctrl+Tab: Próxima aba
 * Ctrl+Shift+Tab: Aba anterior
 * Ctrl+1-9: Ir para aba N
 */

import { useEffect } from 'react';
import { useChatStore } from '../store/chatStore';
import { useAnnouncer } from './useAnnouncer';

export function useTabsKeyboardShortcuts() {
  const { tabs: chatTabs, activeTabId, createTab, deleteTab, setActiveTab } = useChatStore();
  const { announce } = useAnnouncer();

  useEffect(() => {
    const handleKeyDown = (event: KeyboardEvent) => {
      const target = event.target as HTMLElement;
      const isDataGrid = target.closest('.datagrid-container') !== null;
      
      // Se estiver em DataGrid, não interceptar nenhuma tecla
      if (isDataGrid) {
        return;
      }

      // Ctrl+T ou Ctrl+N: Nova aba
      // Apenas bloquear se estiver em campo de entrada (evita conflito com digitação)
      if (event.ctrlKey && (event.key === 't' || event.key === 'n') && !event.shiftKey && !event.altKey) {
        const isInput = target.tagName === 'INPUT' || target.tagName === 'TEXTAREA' || target.isContentEditable;
        if (isInput) {
          return; // Permite Ctrl+T no navegador para abas
        }
        event.preventDefault();
        createTab();
        announce('Nova guia criada');
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
        deleteTab(activeTabId);
        announce('Guia fechada');
        return;
      }

      // Ctrl+Tab: Próxima aba (com navegação circular)
      if (event.ctrlKey && event.key === 'Tab' && !event.shiftKey) {
        event.preventDefault();
        const currentIndex = chatTabs.findIndex(t => t.id === activeTabId);
        if (currentIndex !== -1 && chatTabs.length > 1) {
          // Navegação circular: volta ao início após o último
          const nextIndex = currentIndex < chatTabs.length - 1 ? currentIndex + 1 : 0;
          const nextTab = chatTabs[nextIndex];
          if (nextTab) {
            setActiveTab(nextTab.id);
            const tabNumber = nextIndex + 1;
            const tabTitle = nextTab.title || 'Nova conversa';
            announce(`${tabTitle}, ${tabNumber} de ${chatTabs.length}`);
          }
        }
        return;
      }

      // Ctrl+Shift+Tab: Aba anterior (com navegação circular)
      if (event.ctrlKey && event.key === 'Tab' && event.shiftKey) {
        event.preventDefault();
        const currentIndex = chatTabs.findIndex(t => t.id === activeTabId);
        if (currentIndex !== -1 && chatTabs.length > 1) {
          // Navegação circular: volta ao final após o primeiro
          const prevIndex = currentIndex > 0 ? currentIndex - 1 : chatTabs.length - 1;
          const prevTab = chatTabs[prevIndex];
          if (prevTab) {
            setActiveTab(prevTab.id);
            const tabNumber = prevIndex + 1;
            const tabTitle = prevTab.title || 'Nova conversa';
            announce(`${tabTitle}, ${tabNumber} de ${chatTabs.length}`);
          }
        }
        return;
      }

      // Ctrl+Page Down: Próxima aba (redundância para Ctrl+Tab)
      if (event.ctrlKey && event.key === 'PageDown') {
        event.preventDefault();
        const currentIndex = chatTabs.findIndex(t => t.id === activeTabId);
        if (currentIndex !== -1 && chatTabs.length > 1) {
          const nextIndex = currentIndex < chatTabs.length - 1 ? currentIndex + 1 : 0;
          const nextTab = chatTabs[nextIndex];
          if (nextTab) {
            setActiveTab(nextTab.id);
            const tabNumber = nextIndex + 1;
            const tabTitle = nextTab.title || 'Nova conversa';
            announce(`${tabTitle}, ${tabNumber} de ${chatTabs.length}`);
          }
        }
        return;
      }

      // Ctrl+Page Up: Aba anterior (redundância para Ctrl+Shift+Tab)
      if (event.ctrlKey && event.key === 'PageUp') {
        event.preventDefault();
        const currentIndex = chatTabs.findIndex(t => t.id === activeTabId);
        if (currentIndex !== -1 && chatTabs.length > 1) {
          const prevIndex = currentIndex > 0 ? currentIndex - 1 : chatTabs.length - 1;
          const prevTab = chatTabs[prevIndex];
          if (prevTab) {
            setActiveTab(prevTab.id);
            const tabNumber = prevIndex + 1;
            const tabTitle = prevTab.title || 'Nova conversa';
            announce(`${tabTitle}, ${tabNumber} de ${chatTabs.length}`);
          }
        }
        return;
      }

      // Ctrl+1-9: Ir para aba N
      if (event.ctrlKey && !event.shiftKey && !event.altKey) {
        const num = parseInt(event.key, 10);
        if (num >= 1 && num <= 9) {
          event.preventDefault();
          const targetTab = chatTabs[num - 1];
          if (targetTab) {
            setActiveTab(targetTab.id);
            const tabTitle = targetTab.title || 'Nova conversa';
            announce(`${tabTitle}, ${num} de ${chatTabs.length}`);
          }
        }
      }
    };

    window.addEventListener('keydown', handleKeyDown, true); // Use capture phase
    return () => window.removeEventListener('keydown', handleKeyDown, true);
  }, [chatTabs, activeTabId, createTab, deleteTab, setActiveTab, announce]);
}
