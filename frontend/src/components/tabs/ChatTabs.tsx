/**
 * Componente de abas de chat
 * Implementa navegação por teclado completa e ARIA
 * 
 * Atalhos de teclado:
 * - Setas Left/Right ou Up/Down: Navegar entre abas
 * - Home: Primeira aba
 * - End: Última aba
 * - PageUp/PageDown: Pular 10 abas
 * - Delete: Fechar aba atual
 * - F2: Renomear aba atual
 */

import { useRef, useState } from 'react';
import { useChatStore } from '../../store/chatStore';
import { useAnnouncer } from '../../hooks/useAnnouncer';
import { playBumpSound } from '../../services/audioFeedback';
import './ChatTabs.css';

export function ChatTabs() {
  const { tabs, activeTabId, isLoading, deleteTab, setActiveTab, updateTabTitle } = useChatStore();
  const tabListRef = useRef<HTMLDivElement>(null);
  const { announce } = useAnnouncer();
  
  // Estado para edição de título
  const [editingTabId, setEditingTabId] = useState<string | null>(null);
  const [editingTitle, setEditingTitle] = useState('');
  const editInputRef = useRef<HTMLInputElement>(null);

  /**
   * Inicia edição do título da aba
   */
  const startEditingTab = (tabId: string, currentTitle: string) => {
    setEditingTabId(tabId);
    setEditingTitle(currentTitle);
    // Foca no input após renderização
    setTimeout(() => {
      editInputRef.current?.focus();
      editInputRef.current?.select();
    }, 10);
    announce('Editando título da conversa. Digite o novo título e pressione Enter para confirmar ou Escape para cancelar.');
  };

  /**
   * Cancela edição do título
   */
  const cancelEditingTab = () => {
    setEditingTabId(null);
    setEditingTitle('');
    announce('Edição cancelada');
  };

  /**
   * Confirma edição do título
   */
  const confirmEditingTab = () => {
    // Salva o ID antes de resetar o estado
    const tabIdToFocus = editingTabId;
    
    if (editingTabId && editingTitle.trim()) {
      updateTabTitle(editingTabId, editingTitle.trim());
      announce(`Título alterado para: ${editingTitle.trim()}`);
    }
    
    setEditingTabId(null);
    setEditingTitle('');
    
    // Retorna foco para a tab
    setTimeout(() => {
      if (tabIdToFocus) {
        const tabButton = tabListRef.current?.querySelector(
          `[data-tab-id="${tabIdToFocus}"]`
        ) as HTMLButtonElement;
        tabButton?.focus();
      }
    }, 10);
  };

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
        if (currentIndex === 0) {
          playBumpSound();
          return;
        }
        nextIndex = 0;
        handled = true;
        break;

      case 'End':
        // Última aba
        if (currentIndex === tabs.length - 1) {
          playBumpSound();
          return;
        }
        nextIndex = tabs.length - 1;
        handled = true;
        break;

      case 'PageDown':
        // Pula 10 abas para frente (SEM navegação circular)
        if (currentIndex === tabs.length - 1) {
          playBumpSound();
          return;
        }
        nextIndex = Math.min(currentIndex + 10, tabs.length - 1);
        handled = true;
        break;

      case 'PageUp':
        // Pula 10 abas para trás (SEM navegação circular)
        if (currentIndex === 0) {
          playBumpSound();
          return;
        }
        nextIndex = Math.max(currentIndex - 10, 0);
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

      case 'F2':
        // Renomeia aba com F2
        event.preventDefault();
        const tab = tabs[currentIndex];
        if (tab) {
          startEditingTab(tab.id, tab.title || 'Nova conversa');
        }
        handled = true;
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

  /**
   * Handler para input de edição
   */
  const handleEditKeyDown = (event: React.KeyboardEvent<HTMLInputElement>) => {
    // Para propagação de eventos de navegação e edição para permitir edição normal
    const navigationKeys = ['ArrowLeft', 'ArrowRight', 'ArrowUp', 'ArrowDown', 'Home', 'End', 'Delete', 'Backspace'];
    if (navigationKeys.includes(event.key)) {
      event.stopPropagation();
      return;
    }

    if (event.key === 'Enter') {
      event.preventDefault();
      event.stopPropagation();
      confirmEditingTab();
    } else if (event.key === 'Escape') {
      event.preventDefault();
      event.stopPropagation();
      cancelEditingTab();
    }
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
            } ${editingTabId === tab.id ? 'chat-tabs__tab--editing' : ''}`}
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
            {editingTabId === tab.id ? (
              <input
                ref={editInputRef}
                type="text"
                className="chat-tabs__tab-edit"
                value={editingTitle}
                onChange={(e) => setEditingTitle(e.target.value)}
                onKeyDown={handleEditKeyDown}
                onBlur={confirmEditingTab}
                onClick={(e) => e.stopPropagation()}
                aria-label="Editar título da conversa"
              />
            ) : (
              <span className="chat-tabs__tab-title">{tab.title}</span>
            )}
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
