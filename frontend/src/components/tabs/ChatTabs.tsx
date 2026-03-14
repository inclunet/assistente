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

import { useCallback, useEffect, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useChatStore, ChatTab } from '../../store/chatStore';
import { useAnnouncer } from '../../hooks/useAnnouncer';
import { playBumpSound } from '../../services/audioFeedback';
import { ContextMenu, MenuItem } from '../menu';
import { GetAvailableChannels } from '@wailsjs/go/main/App';
import { useUIStore } from '../../store/uiStore';
import { Tabs, TabList, Tab } from '../ui/tabs';
import './ChatTabs.css';

// Ícones visuais por canal
const channelIcons: Record<string, string> = {
  signal: '📡',
  telegram: '✈️',
};

export function ChatTabs() {
  const { t } = useTranslation();
  const { tabs, activeTabId, isLoading, deleteTab, setActiveTab, updateTabTitle, assignChannelToTab, unassignChannelFromTab } = useChatStore();
  const { addToast } = useUIStore();
  const tabListRef = useRef<HTMLDivElement>(null);
  const { announce } = useAnnouncer();
  
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

  const escapeForAttrSelector = useCallback((value: string) => {
    const esc = (globalThis as any).CSS?.escape as ((v: string) => string) | undefined;
    if (esc) return esc(value);
    // Fallback simples: evita quebrar o seletor em casos comuns.
    return value.replace(/"/g, '\\"');
  }, []);

  const focusTabButton = useCallback((tabId: string) => {
    const list = tabListRef.current;
    if (!list) return;
    const btn = list.querySelector(
      `button[role="tab"][data-tab-value="${escapeForAttrSelector(tabId)}"]`
    ) as HTMLButtonElement | null;
    btn?.focus();
  }, [escapeForAttrSelector]);

  const pendingCloseAnnouncementRef = useRef<string | null>(null);

  useEffect(() => {
    const msg = pendingCloseAnnouncementRef.current;
    if (!msg) return;
    pendingCloseAnnouncementRef.current = null;
    // Aguarda a store atualizar `activeTabId` e foco.
    const timerId = window.setTimeout(() => announce(msg), 50);
    return () => window.clearTimeout(timerId);
  }, [announce, tabs]);

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
    announce(t('chatTabs.renamingHint'));
  };

  /**
   * Cancela edição do título
   */
  const cancelEditingTab = () => {
    setEditingTabId(null);
    setEditingTitle('');
    announce(t('chatTabs.editCancelled'));
  };

  /**
   * Confirma edição do título
   */
  const confirmEditingTab = () => {
    // Salva o ID antes de resetar o estado
    const tabIdToFocus = editingTabId;
    
    if (editingTabId && editingTitle.trim()) {
      updateTabTitle(editingTabId, editingTitle.trim());
      announce(`${t('chatTabs.titleChanged')} ${editingTitle.trim()}`);
    }
    
    setEditingTabId(null);
    setEditingTitle('');
    
    // Retorna foco para a tab
    setTimeout(() => {
      if (tabIdToFocus) focusTabButton(tabIdToFocus);
    }, 10);
  };

  /**
   * Fecha aba com clique no botão X
   */
  const requestClose = useCallback(
    (tabId: string) => {
      // Não permite fechar se for a única aba.
      if (tabs.length <= 1) return;

      const currentIndex = tabs.findIndex((t) => t.id === tabId);
      if (currentIndex === -1) return;

      const nextFocusIndex = currentIndex < tabs.length - 1 ? currentIndex : currentIndex - 1;
      const nextFocusTab = tabs[nextFocusIndex];

      if (nextFocusTab && tabs.length > 1) {
        const tabTitle = nextFocusTab.title || t('chatTabs.newConversation');
        const newTabNumber = Math.min(nextFocusIndex + 1, tabs.length - 1);
        pendingCloseAnnouncementRef.current = `${t('chatTabs.tabClosed')} ${tabTitle}, ${newTabNumber} ${t('chatTabs.of')} ${tabs.length - 1}`;
      } else {
        pendingCloseAnnouncementRef.current = null;
      }

      void deleteTab(tabId);
    },
    [deleteTab, tabs, t]
  );

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

  const getFocusedTabId = useCallback(() => {
    const list = tabListRef.current;
    if (!list) return null;
    const focused = list.querySelector('button[role="tab"]:focus') as HTMLButtonElement | null;
    const v = focused?.getAttribute('data-tab-value');
    return v && v.trim() ? v : null;
  }, []);

  const handleListKeyDown = useCallback(
    (event: React.KeyboardEvent<HTMLDivElement>) => {
      if (editingTabId) return;
      if (event.defaultPrevented) return;
      const tabId = getFocusedTabId();
      if (!tabId) return;

      if (event.key === 'F2') {
        event.preventDefault();
        const tab = tabs.find((tabItem) => tabItem.id === tabId);
        startEditingTab(tabId, tab?.title || t('chatTabs.newConversation'));
        return;
      }

      if (event.key === 'F10' && event.shiftKey) {
        event.preventDefault();
        const tabButton = tabListRef.current?.querySelector(
          `button[role="tab"][data-tab-value="${escapeForAttrSelector(tabId)}"]`
        ) as HTMLButtonElement | null;
        if (tabButton) {
          const rect = tabButton.getBoundingClientRect();
          openContextMenu(tabId, rect.left, rect.bottom);
        }
        return;
      }

      if (event.key === 'ContextMenu') {
        event.preventDefault();
        const tabButton = tabListRef.current?.querySelector(
          `button[role="tab"][data-tab-value="${escapeForAttrSelector(tabId)}"]`
        ) as HTMLButtonElement | null;
        if (tabButton) {
          const rect = tabButton.getBoundingClientRect();
          openContextMenu(tabId, rect.left, rect.bottom);
        }
      }
    },
    [editingTabId, escapeForAttrSelector, getFocusedTabId, tabs]
  );

  /**
   * Constrói os items do menu de contexto para uma aba
   */
  const buildContextMenuItems = (tab: ChatTab): MenuItem[] => {
    const       items: MenuItem[] = [
      {
        id: 'rename',
        label: t('chatTabs.rename'),
        icon: '✏️',
        shortcut: 'F2',
        ariaLabel: t('chatTabs.renameConversation'),
        action: () => {
          closeContextMenu();
          startEditingTab(tab.id, tab.title || t('chatTabs.newConversation'));
        },
      },
    ];

    if (tabs.length > 1) {
      items.push({
        id: 'close',
        label: t('chatTabs.close'),
        icon: '✕',
        shortcut: 'Delete',
        ariaLabel: t('chatTabs.closeConversation'),
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
        label: `${t('chatTabs.channelLabel')} ${tab.channel}`,
        icon: channelIcons[tab.channel] || '🔗',
        ariaLabel: `${t('chatTabs.channelAssigned')} ${tab.channel}`,
      });
      items.push({
        id: 'unassign-channel',
        label: t('chatTabs.removeChannel'),
        icon: '🚫',
        ariaLabel: 'Remover vinculação de canal',
        action: async () => {
          closeContextMenu();
          try {
            await unassignChannelFromTab(tab.id);
            addToast(t('chatTabs.channelRemoved'), 'success');
            announce(t('chatTabs.channelRemoved'));
          } catch (err: any) {
            addToast(err.message || t('chatTabs.removeChannelError'), 'error');
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
            ariaLabel: `${t('chatTabs.assignTo')} ${ch.name}, ${t('chatTabs.contact')} ${contactLabel}`,
            action: async () => {
              closeContextMenu();
              try {
                await assignChannelToTab(tab.id, ch.name, c.id);
                addToast(`${t('chatTabs.assignedTo')} ${ch.name}`, 'success');
                announce(`${t('chatTabs.assignedTo')} ${ch.name}`);
              } catch (err: any) {
                addToast(err.message || t('chatTabs.assignError'), 'error');
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
              ariaLabel: `${t('chatTabs.contact')} ${c.display_name || c.id}`,
              action: async () => {
                closeContextMenu();
                try {
                  await assignChannelToTab(tab.id, ch.name, c.id);
                  addToast(`${t('chatTabs.assignedTo')} ${ch.name}`, 'success');
                  announce(`${t('chatTabs.assignedTo')} ${ch.name}`);
                } catch (err: any) {
                  addToast(err.message || t('chatTabs.assignError'), 'error');
                }
              },
            })),
          };
        }
        // Sem contatos autorizados — item desabilitado
        return {
          id: `assign-${ch.name}`,
          label: `${ch.name} (${t('chatTabs.noContacts')})`,
          icon: channelIcons[ch.name] || '🔗',
          ariaLabel: `${ch.name} sem contatos autorizados`,
        };
      });

      items.push({
        id: 'assign-channel',
        label: t('chatTabs.assignChannel'),
        icon: '🔗',
        ariaLabel: t('chatTabs.assignChannelLabel'),
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

  const handleSelect = useCallback(
    (tabId: string) => {
      if (!tabId) return;
      void setActiveTab(tabId);
    },
    [setActiveTab]
  );

  const handleDelete = useCallback(
    (tabId: string) => {
      requestClose(tabId);
    },
    [requestClose]
  );

  return (
    <Tabs
      value={activeTabId ?? ''}
      onValueChange={handleSelect}
      idBase="chat"
      onBump={playBumpSound}
      onDelete={handleDelete}
      pageJump={10}
      activationMode="auto"
    >
      <div
        className={`chat-tabs ${isLoading ? 'chat-tabs--loading' : ''}`}
        role="region"
        aria-label={t('chatTabs.tabsLabel')}
      >
        <TabList
          listRef={tabListRef}
          className="chat-tabs__list"
          ariaLabel={t('chatTabs.listLabel')}
          onKeyDown={handleListKeyDown}
        >
          {tabs.map((tab) => {
            const isActive = tab.id === activeTabId;
            const isEditing = editingTabId === tab.id;

            return (
              <div
                key={tab.id}
                className={`chat-tabs__tab-wrapper${isActive ? ' chat-tabs__tab-wrapper--active' : ''}`}
                role="presentation"
              >
                <Tab
                  value={tab.id}
                  className={`chat-tabs__tab${
                    isActive ? ' chat-tabs__tab--active' : ''
                  }${isEditing ? ' chat-tabs__tab--editing' : ''}${
                    tab.channel ? ' chat-tabs__tab--channel' : ''
                  }`}
                  controlsId={null}
                  ariaDescription={tab.channel ? `${t('chatTabs.channelLabel')} ${tab.channel}` : undefined}
                  onContextMenu={(e) => handleTabContextMenu(e, tab.id)}
                >
                  <span className="chat-tabs__tab-icon" aria-hidden="true">
                    {tab.channel ? (channelIcons[tab.channel] || '🔗') : '💬'}
                  </span>

                  <span className="chat-tabs__tab-title">{tab.title}</span>
                </Tab>

                {isEditing && (
                  <input
                    ref={editInputRef}
                    type="text"
                    className="chat-tabs__tab-edit"
                    value={editingTitle}
                    onChange={(e) => setEditingTitle(e.target.value)}
                    onKeyDown={handleEditKeyDown}
                    onBlur={confirmEditingTab}
                    onClick={(e) => e.stopPropagation()}
                    onContextMenu={(e) => e.stopPropagation()}
                    aria-label={t('chatTabs.editTitle')}
                  />
                )}

                {tabs.length > 1 && (
                  <button
                    className="chat-tabs__tab-close"
                    onClick={(e) => {
                      e.preventDefault();
                      e.stopPropagation();
                      requestClose(tab.id);
                    }}
                    aria-label={`${t('chatTabs.close')} ${tab.title}`}
                    tabIndex={-1}
                    type="button"
                  >
                    ×
                  </button>
                )}
              </div>
            );
          })}
        </TabList>

        {/* Menu de contexto das abas */}
        <ContextMenu
          visible={contextMenu.visible}
          x={contextMenu.x}
          y={contextMenu.y}
          items={contextMenu.visible ? buildContextMenuItems(tabs.find(tab => tab.id === contextMenu.tabId)!) : []}
          ariaLabel={t('chatTabs.contextMenuLabel')}
          onClose={closeContextMenu}
        />
      </div>
    </Tabs>
  );
}
