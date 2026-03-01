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
 * - Shift+F10 ou Menu: Menu de contexto (renomear, fechar, atribuir canal)
 */

import { useEffect, useRef, useState } from 'react';
import { useChatStore, ChatTab } from '../../store/chatStore';
import { useAnnouncer } from '../../hooks/useAnnouncer';
import { playBumpSound } from '../../services/audioFeedback';
import { ContextMenu, MenuItem } from '../ui/ContextMenu';
import { GetAvailableChannels } from '@wailsjs/go/main/App';
import { useUIStore } from '../../store/uiStore';
import './ChatTabs.css';

// Ícones visuais por canal
const channelIcons: Record<string, string> = {
  signal: '📡',
  telegram: '✈️',
};

export function ChatTabs() {
  const { tabs, activeTabId, isLoading, deleteTab, setActiveTab, updateTabTitle, assignChannelToTab, unassignChannelFromTab } = useChatStore();
  const { addToast } = useUIStore();
  const tabListRef = useRef<HTMLDivElement>(null);
  const { announce } = useAnnouncer();
  
  // Debug: Log tabs quando mudarem
  useEffect(() => {
    console.log('[ChatTabs] Tabs atualizadas:', tabs.map(t => ({ id: t.id, title: t.title })));
  }, [tabs]);
  
  // Estado para edição de título
  const [editingTabId, setEditingTabId] = useState<string | null>(null);
  const [editingTitle, setEditingTitle] = useState('');

  // Estado do menu de contexto
  const [contextMenu, setContextMenu] = useState<{
    visible: boolean; x: number; y: number; tabId: string;
  }>({ visible: false, x: 0, y: 0, tabId: '' });
  const [availableChannels, setAvailableChannels] = useState<{ name: string; connected: boolean; contacts?: { id: string; display_name: string; username?: string }[]; maxContacts?: number }[]>([]);

  // Carrega canais disponíveis quando o menu precisa
  useEffect(() => {
    if (contextMenu.visible) {
      GetAvailableChannels().then((channels) => {
        setAvailableChannels(channels || []);
      }).catch(() => setAvailableChannels([]));
    }
  }, [contextMenu.visible]);
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

      case 'F10':
        // Shift+F10: abre menu de contexto (padrão Windows)
        if (event.shiftKey) {
          event.preventDefault();
          const tabButton = tabListRef.current?.querySelector(
            `[data-tab-id="${tabId}"]`
          ) as HTMLButtonElement;
          if (tabButton) {
            const rect = tabButton.getBoundingClientRect();
            openContextMenu(tabId, rect.left, rect.bottom);
          }
          handled = true;
        }
        break;

      case 'ContextMenu':
        // Tecla Menu (se disponível no teclado)
        event.preventDefault();
        const ctxTabButton = tabListRef.current?.querySelector(
          `[data-tab-id="${tabId}"]`
        ) as HTMLButtonElement;
        if (ctxTabButton) {
          const rect = ctxTabButton.getBoundingClientRect();
          openContextMenu(tabId, rect.left, rect.bottom);
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
   * Abre menu de contexto na aba (clique direito ou Shift+F10)
   */
  const openContextMenu = (tabId: string, x: number, y: number) => {
    setContextMenu({ visible: true, x, y, tabId });
  };

  const closeContextMenu = () => {
    setContextMenu({ visible: false, x: 0, y: 0, tabId: '' });
  };

  const handleTabContextMenu = (event: React.MouseEvent, tabId: string) => {
    event.preventDefault();
    event.stopPropagation();
    openContextMenu(tabId, event.clientX, event.clientY);
  };

  /**
   * Constrói os items do menu de contexto para uma aba
   */
  const buildContextMenuItems = (tab: ChatTab): MenuItem[] => {
    const items: MenuItem[] = [
      {
        id: 'rename',
        label: 'Renomear',
        icon: '✏️',
        shortcut: 'F2',
        ariaLabel: 'Renomear conversa',
        action: () => {
          closeContextMenu();
          startEditingTab(tab.id, tab.title || 'Nova conversa');
        },
      },
    ];

    if (tabs.length > 1) {
      items.push({
        id: 'close',
        label: 'Fechar',
        icon: '✕',
        shortcut: 'Delete',
        ariaLabel: 'Fechar conversa',
        danger: true,
        action: () => {
          closeContextMenu();
          deleteTab(tab.id);
        },
      });
    }

    // Opções de canal
    const connectedChannels = availableChannels.filter((ch) => ch.connected);

    if (tab.channel) {
      // Já tem canal atribuído — mostra qual e opção de remover
      items.push({ id: 'sep-channel', separator: true });
      items.push({
        id: 'channel-info',
        label: `Canal: ${tab.channel}`,
        icon: channelIcons[tab.channel] || '🔗',
        ariaLabel: `Canal atribuído: ${tab.channel}`,
      });
      items.push({
        id: 'unassign-channel',
        label: 'Remover canal',
        icon: '🚫',
        ariaLabel: 'Remover vinculação de canal',
        action: async () => {
          closeContextMenu();
          try {
            await unassignChannelFromTab(tab.id);
            addToast('Canal removido da conversa', 'success');
            announce('Canal removido da conversa');
          } catch (err: any) {
            addToast(err.message || 'Erro ao remover canal', 'error');
          }
        },
      });
    } else if (connectedChannels.length > 0 && tab.conversationId) {
      // Sem canal — oferece submenu para atribuir
      items.push({ id: 'sep-channel', separator: true });

      const channelSubmenu: MenuItem[] = connectedChannels.map((ch) => {
        const chContacts = ch.contacts || [];
        if (chContacts.length === 1) {
          // Um contato — atribui direto
          const c = chContacts[0];
          const contactLabel = c.display_name || c.id;
          return {
            id: `assign-${ch.name}`,
            label: `${ch.name} (${contactLabel})`,
            icon: channelIcons[ch.name] || '🔗',
            ariaLabel: `Atribuir ao ${ch.name}, contato ${contactLabel}`,
            action: async () => {
              closeContextMenu();
              try {
                await assignChannelToTab(tab.id, ch.name, c.id);
                addToast(`Conversa vinculada ao ${ch.name}`, 'success');
                announce(`Conversa vinculada ao ${ch.name}`);
              } catch (err: any) {
                addToast(err.message || 'Erro ao atribuir canal', 'error');
              }
            },
          };
        } else if (chContacts.length > 1) {
          // Vários contatos — submenu
          return {
            id: `assign-${ch.name}`,
            label: ch.name,
            icon: channelIcons[ch.name] || '🔗',
            ariaLabel: `Atribuir ao ${ch.name}`,
            submenu: chContacts.map((c) => ({
              id: `assign-${ch.name}-${c.id}`,
              label: c.display_name || c.id,
              ariaLabel: `Contato ${c.display_name || c.id}`,
              action: async () => {
                closeContextMenu();
                try {
                  await assignChannelToTab(tab.id, ch.name, c.id);
                  addToast(`Conversa vinculada ao ${ch.name}`, 'success');
                  announce(`Conversa vinculada ao ${ch.name}`);
                } catch (err: any) {
                  addToast(err.message || 'Erro ao atribuir canal', 'error');
                }
              },
            })),
          };
        }
        // Sem contatos autorizados — item desabilitado
        return {
          id: `assign-${ch.name}`,
          label: `${ch.name} (sem contatos)`,
          icon: channelIcons[ch.name] || '🔗',
          ariaLabel: `${ch.name} sem contatos autorizados`,
        };
      });

      items.push({
        id: 'assign-channel',
        label: 'Atribuir a canal',
        icon: '🔗',
        ariaLabel: 'Atribuir conversa a um canal de mensageria',
        submenu: channelSubmenu,
      });
    }

    return items;
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
        {tabs.map(tab => {
          console.log('[ChatTabs] Renderizando aba:', { id: tab.id, title: tab.title, conversationId: tab.conversationId });
          return (
          <button
            key={tab.id}
            data-tab-id={tab.id}
            className={`chat-tabs__tab ${
              tab.id === activeTabId ? 'chat-tabs__tab--active' : ''
            } ${editingTabId === tab.id ? 'chat-tabs__tab--editing' : ''} ${
              tab.channel ? 'chat-tabs__tab--channel' : ''
            }`}
            role="tab"
            aria-selected={tab.id === activeTabId}
            aria-controls={`tabpanel-${tab.id}`}
            aria-description={tab.channel ? `Canal: ${tab.channel}` : undefined}
            tabIndex={tab.id === activeTabId ? 0 : -1}
            onClick={() => handleTabClick(tab.id)}
            onKeyDown={(e) => handleKeyDown(e, tab.id)}
            onContextMenu={(e) => handleTabContextMenu(e, tab.id)}
          >
            <span className="chat-tabs__tab-icon" aria-hidden="true">
              {tab.channel ? (channelIcons[tab.channel] || '🔗') : '💬'}
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
        )})}
      </div>

      {/* Menu de contexto das abas */}
      <ContextMenu
        visible={contextMenu.visible}
        x={contextMenu.x}
        y={contextMenu.y}
        items={contextMenu.visible ? buildContextMenuItems(tabs.find(t => t.id === contextMenu.tabId)!) : []}
        ariaLabel="Menu de contexto da aba"
        onClose={closeContextMenu}
      />
    </div>
  );
}
