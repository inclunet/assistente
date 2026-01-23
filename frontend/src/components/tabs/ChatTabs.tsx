/**
 * Componente de abas de chat
 * Implementa navegação por teclado completa e ARIA
 */

import { useRef } from 'react';
import { useChatStore } from '../../store/chatStore';
import { useAnnouncer } from '../../hooks/useAnnouncer';
import { playBumpSound } from '../../services/audioFeedback';
import './ChatTabs.css';

export function ChatTabs() {
  const { tabs, activeTabId, isLoading, deleteTab, setActiveTab } = useChatStore();
  const tabListRef = useRef<HTMLDivElement>(null);
  const { announce } = useAnnouncer();

  /**
   * Navegação por teclado entre abas
   */
  const handleKeyDown = (event: React.KeyboardEvent, tabId: string) => {
    const currentIndex = tabs.findIndex(t => t.id === tabId);
    if (currentIndex === -1) return;

    let handled = false;
    let nextIndex = currentIndex;

    switch (event.key) {
      case 'ArrowLeft':
      case 'ArrowUp':
        // Aba anterior (SEM navegação circular - para no primeiro)
        if (currentIndex === 0) {
          playBumpSound(); // Bateu no limite
          return;
        }
        nextIndex = Math.max(currentIndex - 1, 0);
        handled = true;
        break;

      case 'ArrowRight':
      case 'ArrowDown':
        // Próxima aba (SEM navegação circular - para no último)
        if (currentIndex === tabs.length - 1) {
          playBumpSound(); // Bateu no limite
          return;
        }
        nextIndex = Math.min(currentIndex + 1, tabs.length - 1);
        handled = true;
        break;

      case 'Home':
        // Primeira aba
        nextIndex = 0;
        handled = true;
        break;

      case 'End':
        // Última aba
        nextIndex = tabs.length - 1;
        handled = true;
        break;

      case 'Delete':
        // Fecha aba com Delete
        if (!event.shiftKey && !event.ctrlKey && !event.altKey) {
          event.preventDefault();
          // Determina qual aba receberá o foco após a deleção
          const nextFocusIndex = currentIndex < tabs.length - 1 ? currentIndex : currentIndex - 1;
          const nextFocusTab = tabs[nextFocusIndex];
          
          deleteTab(tabId);
          
          // Foca na próxima guia disponível
          if (nextFocusTab && tabs.length > 1) {
            setTimeout(() => {
              const nextButton = tabListRef.current?.querySelector(
                `[data-tab-id="${nextFocusTab.id}"]`
              ) as HTMLButtonElement;
              nextButton?.focus();
              
              // Anuncia a mudança
              const tabTitle = nextFocusTab.title || 'Nova conversa';
              const newTabNumber = Math.min(nextFocusIndex + 1, tabs.length - 1);
              announce(`Guia fechada. ${tabTitle}, ${newTabNumber} de ${tabs.length - 1}`);
            }, 50);
          }
          handled = true;
        }
        break;
    }

    if (handled) {
      event.preventDefault();
      if (nextIndex !== currentIndex) {
        const nextTab = tabs[nextIndex];
        if (nextTab) {
          setActiveTab(nextTab.id);
          // Anuncia mudança de guia
          const tabNumber = nextIndex + 1;
          const tabTitle = nextTab.title || 'Nova conversa';
          announce(`${tabTitle}, ${tabNumber} de ${tabs.length}`);
          // Foca na próxima aba
          setTimeout(() => {
            const nextButton = tabListRef.current?.querySelector(
              `[data-tab-id="${nextTab.id}"]`
            ) as HTMLButtonElement;
            nextButton?.focus();
          }, 0);
        }
      }
    }
  };

  /**
   * Fecha aba com clique no botão X
   */
  const handleCloseTab = (event: React.MouseEvent, tabId: string) => {
    event.stopPropagation();
    
    const currentIndex = tabs.findIndex(t => t.id === tabId);
    if (currentIndex === -1) return;
    
    // Determina qual aba receberá o foco após a deleção
    const nextFocusIndex = currentIndex < tabs.length - 1 ? currentIndex : currentIndex - 1;
    const nextFocusTab = tabs[nextFocusIndex];
    
    deleteTab(tabId);
    
    // Foca na próxima guia disponível
    if (nextFocusTab && tabs.length > 1) {
      setTimeout(() => {
        const nextButton = tabListRef.current?.querySelector(
          `[data-tab-id="${nextFocusTab.id}"]`
        ) as HTMLButtonElement;
        nextButton?.focus();
      }, 50);
    }
  };

  /**
   * Ativa aba ao clicar
   */
  const handleTabClick = (tabId: string) => {
    console.log('[ChatTabs] 🖱️ handleTabClick chamado com tabId:', tabId);
    setActiveTab(tabId);
  };

  return (
    <div
      className={`chat-tabs ${isLoading ? 'chat-tabs--loading' : ''}`}
      role="region"
      aria-label="Abas de conversa"
    >
      <div
        ref={tabListRef}
        className="chat-tabs__list"
        role="tablist"
        aria-label="Lista de conversas abertas"
      >
        {tabs.map(tab => (
          <button
            key={tab.id}
            data-tab-id={tab.id}
            className={`chat-tabs__tab ${
              tab.id === activeTabId ? 'chat-tabs__tab--active' : ''
            }`}
            role="tab"
            aria-selected={tab.id === activeTabId}
            aria-controls={`tabpanel-${tab.id}`}
            tabIndex={tab.id === activeTabId ? 0 : -1}
            onClick={() => handleTabClick(tab.id)}
            onKeyDown={(e) => handleKeyDown(e, tab.id)}
          >
            <span className="chat-tabs__tab-icon" aria-hidden="true">
              💬
            </span>
            <span className="chat-tabs__tab-title">{tab.title}</span>
            {tabs.length > 1 && (
              <button
                className="chat-tabs__tab-close"
                onClick={(e) => handleCloseTab(e, tab.id)}
                aria-label={`Fechar ${tab.title}`}
                tabIndex={-1}
                type="button"
              >
                ×
              </button>
            )}
          </button>
        ))}
      </div>
    </div>
  );
}
