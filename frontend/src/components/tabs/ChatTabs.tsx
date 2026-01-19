/**
 * Componente de abas de chat
 * Implementa navegação por teclado completa e ARIA
 */

import { useEffect, useRef } from 'react';
import { useTabsStore } from '../../store/tabsStore';
import './ChatTabs.css';

export function ChatTabs() {
  const { tabs, activeTabId, isLoading, createTab, closeTab, setActiveTab } = useTabsStore();
  const tabListRef = useRef<HTMLDivElement>(null);

  /**
   * Navegação por teclado entre abas
   */
  const handleKeyDown = (event: React.KeyboardEvent, tabId: number) => {
    const currentIndex = tabs.findIndex(t => t.id === tabId);
    if (currentIndex === -1) return;

    let handled = false;
    let nextIndex = currentIndex;

    switch (event.key) {
      case 'ArrowLeft':
        // Aba anterior (com wrap)
        nextIndex = currentIndex > 0 ? currentIndex - 1 : tabs.length - 1;
        handled = true;
        break;

      case 'ArrowRight':
        // Próxima aba (com wrap)
        nextIndex = currentIndex < tabs.length - 1 ? currentIndex + 1 : 0;
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
          closeTab(tabId);
          handled = true;
        }
        break;
    }

    if (handled && nextIndex !== currentIndex) {
      event.preventDefault();
      const nextTab = tabs[nextIndex];
      if (nextTab) {
        setActiveTab(nextTab.id);
        // Foca na próxima aba
        setTimeout(() => {
          const nextButton = tabListRef.current?.querySelector(
            `[data-tab-id="${nextTab.id}"]`
          ) as HTMLButtonElement;
          nextButton?.focus();
        }, 0);
      }
    }
  };

  /**
   * Fecha aba com clique no botão X
   */
  const handleCloseTab = (event: React.MouseEvent, tabId: number) => {
    event.stopPropagation();
    closeTab(tabId);
  };

  /**
   * Ativa aba ao clicar
   */
  const handleTabClick = (tabId: number) => {
    setActiveTab(tabId);
  };

  /**
   * Cria nova aba
   */
  const handleNewTab = () => {
    createTab();
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
              tab.isActive ? 'chat-tabs__tab--active' : ''
            }`}
            role="tab"
            aria-selected={tab.isActive}
            aria-controls={`tabpanel-${tab.id}`}
            tabIndex={tab.isActive ? 0 : -1}
            onClick={() => handleTabClick(tab.id)}
            onKeyDown={(e) => handleKeyDown(e, tab.id)}
          >
            <span className="chat-tabs__tab-icon" aria-hidden="true">
              {tab.icon}
            </span>
            <span className="chat-tabs__tab-title">{tab.title}</span>
            <button
              className="chat-tabs__tab-close"
              onClick={(e) => handleCloseTab(e, tab.id)}
              aria-label={`Fechar ${tab.title}`}
              tabIndex={-1}
              type="button"
            >
              ×
            </button>
          </button>
        ))}
      </div>

      <button
        className="chat-tabs__new-tab"
        onClick={handleNewTab}
        aria-label="Nova aba (Ctrl+T)"
        title="Nova aba (Ctrl+T)"
        type="button"
      >
        +
      </button>
    </div>
  );
}
