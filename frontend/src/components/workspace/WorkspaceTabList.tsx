import { logger } from '../../utils/logger';
import React, { type ReactNode, useCallback, useLayoutEffect, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import {
  MessageOutlined,
  FileTextOutlined,
  CodeOutlined,
  CheckSquareOutlined,
} from '@ant-design/icons';
import { useWorkspaceStore, type WorkspaceTab, type TabType } from '../../store/workspaceStore';
import { useShallow } from 'zustand/shallow';
import { useAnnouncer } from '../../hooks/useAnnouncer';
import { useAnchoredContextMenu } from '../../hooks/useAnchoredContextMenu';
import { playBumpSound } from '../../services/audioFeedback';
import { Tabs, TabList, Tab } from '../ui/tabs';
import { ContextMenu } from '../menu';
import type { MenuItem } from '../menu';
import { WORKSPACE_TABLIST_TAB_ACTIVATED_EVENT } from './workspaceFocusEvents';
import './WorkspaceTabList.css';

const TAB_TYPE_ICONS: Record<TabType, ReactNode> = {
  chat: <MessageOutlined />,
  editor: <FileTextOutlined />,
  terminal: <CodeOutlined />,
  tasklist: <CheckSquareOutlined />,
};

export const WorkspaceTabList = React.memo(function WorkspaceTabList() {
  const { t } = useTranslation();
  const { workspace, workspaces, setActiveTab, removeTab, updateTab, reorderTabs, moveTabToWorkspace, renameTabContent } = useWorkspaceStore(
    useShallow((s) => ({ workspace: s.workspace, workspaces: s.workspaces, setActiveTab: s.setActiveTab, removeTab: s.removeTab, updateTab: s.updateTab, reorderTabs: s.reorderTabs, moveTabToWorkspace: s.moveTabToWorkspace, renameTabContent: s.renameTabContent }))
  );
  const { announce } = useAnnouncer();
  const tabListRef = useRef<HTMLDivElement>(null);

  const [editingTabId, setEditingTabId] = useState<string | null>(null);
  const [editingTitle, setEditingTitle] = useState('');
  const [dragTabId, setDragTabId] = useState<string | null>(null);
  const [dropTargetId, setDropTargetId] = useState<string | null>(null);
  const editInputRef = useRef<HTMLInputElement>(null);
  const contextTargetRef = useRef<string | null>(null);
  const lastEditIntentRef = useRef<'enter' | 'escape' | 'blur' | null>(null);
  const pendingFocusTabIdRef = useRef<string | null>(null);
  const { menu: ctxMenu, openAtPoint: openCtx, closeMenu: closeCtx, onSelectItem: onCtxSelect } = useAnchoredContextMenu();

  const tabs = workspace?.tabs || [];
  const activeTabId = workspace?.activeTabId || '';

  const handleSelect = useCallback((tabId: string) => {
    if (!tabId) return;
    const activeElement = document.activeElement as HTMLElement | null;
    if (activeElement?.closest?.('button[role="tab"]')) {
      pendingFocusTabIdRef.current = tabId;
      window.dispatchEvent(new CustomEvent(WORKSPACE_TABLIST_TAB_ACTIVATED_EVENT, { detail: { tabId } }));
    }
    void setActiveTab(tabId);
  }, [setActiveTab]);

  useLayoutEffect(() => {
    if (!activeTabId || pendingFocusTabIdRef.current !== activeTabId) return;
    const nextTab = tabListRef.current?.querySelector(
      `button[role="tab"][data-tab-value="${activeTabId}"]`
    ) as HTMLButtonElement | null;
    nextTab?.focus();
    pendingFocusTabIdRef.current = null;
  }, [activeTabId]);

  const handleDelete = useCallback((tabId: string) => {
    if (tabs.length <= 1) return;
    void removeTab(tabId);
  }, [removeTab, tabs.length]);

  const startEditing = useCallback((tabId: string, currentTitle: string) => {
    lastEditIntentRef.current = null;
    setEditingTabId(tabId);
    setEditingTitle(currentTitle);
  }, []);

  useLayoutEffect(() => {
    if (!editingTabId) return;
    const input = editInputRef.current;
    if (!input) return;
    input.focus();
    input.select();
    const rafId = requestAnimationFrame(() => {
      const current = editInputRef.current;
      if (!current || document.activeElement === current) return;
      current.focus();
      current.select();
    });
    return () => cancelAnimationFrame(rafId);
  }, [editingTabId]);

  const confirmEditing = useCallback((source: 'enter' | 'blur') => {
    lastEditIntentRef.current = source;
    const tabIdToFocus = editingTabId;
    const tabIdToRename = editingTabId;
    const trimmedTitle = editingTitle.trim();

    setEditingTabId(null);
    setEditingTitle('');

    if (tabIdToRename && trimmedTitle) {
      void updateTab(tabIdToRename, { title: trimmedTitle });
      try {
        renameTabContent(tabIdToRename, trimmedTitle);
      } catch (error) {
        logger.error('[WorkspaceTabList] Rename tab content error:', error);
      }
    }

    if (tabIdToFocus) {
      setTimeout(() => {
        const list = tabListRef.current;
        if (!list) return;
        const btn = list.querySelector(
          `button[role="tab"][data-tab-value="${tabIdToFocus}"]`
        ) as HTMLButtonElement | null;
        btn?.focus();
      }, 10);
    }
  }, [editingTabId, editingTitle, updateTab, renameTabContent]);

  useLayoutEffect(() => {
    if (!editingTabId) return;

    const handlePointerDownOutside = (event: PointerEvent) => {
      const input = editInputRef.current;
      const target = event.target as Node | null;
      if (!input || !target) return;
      if (input.contains(target)) return;
      if (lastEditIntentRef.current) return;
      confirmEditing('blur');
    };

    document.addEventListener('pointerdown', handlePointerDownOutside, true);
    return () => document.removeEventListener('pointerdown', handlePointerDownOutside, true);
  }, [confirmEditing, editingTabId]);

  const cancelEditing = useCallback(() => {
    setEditingTabId(null);
    setEditingTitle('');
  }, []);

  const handleEditKeyDown = useCallback((e: React.KeyboardEvent<HTMLInputElement>) => {
    const navKeys = ['ArrowLeft', 'ArrowRight', 'ArrowUp', 'ArrowDown', 'Home', 'End', 'Delete', 'Backspace'];
    if (navKeys.includes(e.key)) {
      e.stopPropagation();
      return;
    }
    if (e.key === 'Enter') {
      e.preventDefault();
      e.stopPropagation();
      lastEditIntentRef.current = 'enter';
      confirmEditing('enter');
    } else if (e.key === 'Escape') {
      e.preventDefault();
      e.stopPropagation();
      lastEditIntentRef.current = 'escape';
      cancelEditing();
    }
  }, [confirmEditing, cancelEditing]);

  const handleEditBlur = useCallback(() => {
    if (lastEditIntentRef.current) {
      lastEditIntentRef.current = null;
      return;
    }
    confirmEditing('blur');
  }, [confirmEditing, editingTabId, editingTitle]);

  const handleListKeyDown = useCallback((e: React.KeyboardEvent<HTMLDivElement>) => {
    if (editingTabId) return;
    if (e.defaultPrevented) {
      const tabNavigationKeys = ['ArrowLeft', 'ArrowRight', 'ArrowUp', 'ArrowDown', 'Home', 'End', 'PageUp', 'PageDown'];
      if (!e.altKey && !e.ctrlKey && !e.metaKey && !e.shiftKey && tabNavigationKeys.includes(e.key)) {
        const selected = tabListRef.current?.querySelector('button[role="tab"][aria-selected="true"]') as HTMLButtonElement | null;
        const selectedTabId = selected?.getAttribute('data-tab-value');
        if (selectedTabId) {
          pendingFocusTabIdRef.current = selectedTabId;
          window.dispatchEvent(new CustomEvent(WORKSPACE_TABLIST_TAB_ACTIVATED_EVENT, { detail: { tabId: selectedTabId } }));
        }
      }
      return;
    }

    const focused = tabListRef.current?.querySelector('button[role="tab"]:focus') as HTMLButtonElement | null;
    const tabId = focused?.getAttribute('data-tab-value');
    if (!tabId) return;

    if (e.key === 'F2') {
      e.preventDefault();
      const tab = tabs.find(t => t.id === tabId);
      if (tab) startEditing(tabId, tab.title);
      return;
    }

    // Alt+Left/Up: move aba para esquerda; Alt+Right/Down: move para direita
    if (e.altKey && !e.ctrlKey && !e.shiftKey && !e.metaKey) {
      const isLeft = e.key === 'ArrowLeft' || e.key === 'ArrowUp';
      const isRight = e.key === 'ArrowRight' || e.key === 'ArrowDown';
      if (!isLeft && !isRight) return;

      e.preventDefault();
      e.stopPropagation();

      const idx = tabs.findIndex(t => t.id === tabId);
      if (idx === -1) return;

      const targetIdx = isLeft ? idx - 1 : idx + 1;
      if (targetIdx < 0 || targetIdx >= tabs.length) {
        playBumpSound();
        return;
      }

      const ids = tabs.map(t => t.id);
      [ids[idx], ids[targetIdx]] = [ids[targetIdx], ids[idx]];
      void reorderTabs(ids);
      const pos = targetIdx + 1;
      announce(t('workspace.announce.tabMovedToPosition', { title: tabs[idx].title, position: pos, total: tabs.length }));

      // Re-foca a aba movida após o re-render
      requestAnimationFrame(() => {
        const btn = tabListRef.current?.querySelector(
          `button[role="tab"][data-tab-value="${tabId}"]`
        ) as HTMLButtonElement | null;
        btn?.focus();
      });
    }
  }, [editingTabId, tabs, startEditing, reorderTabs, announce, t]);

  const handleActivate = useCallback(() => {
    return !!editingTabId;
  }, [editingTabId]);

  const handleTabContextMenu = useCallback((e: React.MouseEvent, tabId: string) => {
    e.preventDefault();
    e.stopPropagation();
    contextTargetRef.current = tabId;
    const tab = tabs.find(t => t.id === tabId);
    if (!tab) return;

    const otherWorkspaces = workspaces.filter(ws => !ws.is_active);

    const items: MenuItem[] = [
      {
        id: 'close',
        label: t('workspace.closeTab', 'Fechar'),
        disabled: tabs.length <= 1,
        action: () => void removeTab(tabId),
      },
      {
        id: 'close-others',
        label: t('workspace.closeOtherTabs', 'Fechar outras'),
        disabled: tabs.length <= 1,
        action: () => {
          for (const other of tabs) {
            if (other.id !== tabId) void removeTab(other.id);
          }
        },
      },
      { id: 'sep-1', separator: true },
      {
        id: 'rename',
        label: t('workspace.renameTab', 'Renomear'),
        shortcut: 'F2',
        action: () => startEditing(tabId, tab.title),
      },
    ];

    if (otherWorkspaces.length > 0) {
      items.push({ id: 'sep-2', separator: true });
      items.push({
        id: 'move-to',
        label: t('workspace.moveTabTo', 'Mover para...'),
        submenu: otherWorkspaces.map(ws => ({
          id: `move-${ws.id}`,
          label: ws.name,
          action: async () => {
            try {
              await moveTabToWorkspace(tabId, ws.id);
              announce(t('workspace.announce.tabMovedToWorkspace', { title: tab.title, workspace: ws.name }));
            } catch (error) {
              logger.error('[WorkspaceTabList] Move tab error:', error);
            }
          },
        })),
      });
    }

    openCtx(e.clientX, e.clientY, t('workspace.tabContextMenu', 'Menu da aba'), items);
  }, [tabs, workspaces, removeTab, startEditing, openCtx, moveTabToWorkspace, announce, t]);

  const handleDragStart = useCallback((e: React.DragEvent, tabId: string) => {
    setDragTabId(tabId);
    e.dataTransfer.effectAllowed = 'move';
    e.dataTransfer.setData('text/plain', tabId);
    if (e.currentTarget instanceof HTMLElement) {
      e.currentTarget.style.opacity = '0.5';
    }
  }, []);

  const handleDragEnd = useCallback((e: React.DragEvent) => {
    setDragTabId(null);
    setDropTargetId(null);
    if (e.currentTarget instanceof HTMLElement) {
      e.currentTarget.style.opacity = '';
    }
  }, []);

  const handleDragOver = useCallback((e: React.DragEvent, tabId: string) => {
    e.preventDefault();
    e.dataTransfer.dropEffect = 'move';
    if (dragTabId && tabId !== dragTabId) {
      setDropTargetId(tabId);
    }
  }, [dragTabId]);

  const handleDrop = useCallback((e: React.DragEvent, targetTabId: string) => {
    e.preventDefault();
    setDropTargetId(null);
    if (!dragTabId || dragTabId === targetTabId) return;

    const ids = tabs.map(t => t.id);
    const fromIdx = ids.indexOf(dragTabId);
    const toIdx = ids.indexOf(targetTabId);
    if (fromIdx === -1 || toIdx === -1) return;

    ids.splice(fromIdx, 1);
    ids.splice(toIdx, 0, dragTabId);
    void reorderTabs(ids);

    const tab = tabs.find(t => t.id === dragTabId);
    if (tab) {
      announce(t('workspace.announce.tabMovedToPosition', { title: tab.title, position: toIdx + 1, total: tabs.length }));
    }
  }, [dragTabId, tabs, reorderTabs, announce, t]);

  const renderTab = (tab: WorkspaceTab) => {
    const isActive = tab.id === activeTabId;
    const isEditing = editingTabId === tab.id;
    const isDragging = dragTabId === tab.id;
    const isDropTarget = dropTargetId === tab.id;
    const icon = TAB_TYPE_ICONS[tab.type] || <FileTextOutlined />;

    return (
      <div
        key={tab.id}
        className={`ws-tabs__tab-wrapper${isActive ? ' ws-tabs__tab-wrapper--active' : ''}${isDragging ? ' ws-tabs__tab-wrapper--dragging' : ''}${isDropTarget ? ' ws-tabs__tab-wrapper--drop-target' : ''}`}
        role="presentation"
        draggable={!isEditing}
        onDragStart={(e) => handleDragStart(e, tab.id)}
        onDragEnd={handleDragEnd}
        onDragOver={(e) => handleDragOver(e, tab.id)}
        onDragLeave={() => setDropTargetId(null)}
        onDrop={(e) => handleDrop(e, tab.id)}
        onContextMenu={(e) => handleTabContextMenu(e, tab.id)}
        onMouseDown={(e) => {
          if (e.button === 1 && tabs.length > 1) {
            e.preventDefault();
            void removeTab(tab.id);
          }
        }}
      >
        <Tab
          value={tab.id}
          className={`ws-tabs__tab${isActive ? ' ws-tabs__tab--active' : ''}${isEditing ? ' ws-tabs__tab--editing' : ''}`}
          controlsId={null}
          ariaLabel={`${tab.title}, ${t(`workspace.tabType.${tab.type}`)}`}
        >
          <span className="ws-tabs__tab-icon" aria-hidden="true">{icon}</span>
          <span className="ws-tabs__tab-title" aria-hidden="true">{tab.title}</span>
        </Tab>

        {isEditing && (
          <input
            ref={editInputRef}
            type="text"
            className="ws-tabs__tab-edit"
            autoFocus
            value={editingTitle}
            onChange={(e) => setEditingTitle(e.target.value)}
            onFocus={(e) => e.currentTarget.select()}
            onKeyDown={handleEditKeyDown}
            onBlur={handleEditBlur}
            onClick={(e) => e.stopPropagation()}
            aria-label={t('workspace.editTabTitle')}
          />
        )}

        {tabs.length > 1 && (
          <span
            className="ws-tabs__tab-close"
            aria-hidden="true"
            onClick={(e) => {
              e.preventDefault();
              e.stopPropagation();
              void removeTab(tab.id);
            }}
          >
            ×
          </span>
        )}
      </div>
    );
  };

  return (
    <>
      <Tabs
        value={activeTabId}
        onValueChange={handleSelect}
        idBase="workspace"
        onBump={playBumpSound}
        onDelete={handleDelete}
        onActivate={handleActivate}
        pageJump={10}
        activationMode="auto"
      >
        <div
          className="ws-tabs"
        >
          <TabList
            listRef={tabListRef}
            className="ws-tabs__list"
            ariaLabel={t('workspace.tabListLabel')}
            onKeyDown={handleListKeyDown}
          >
            {tabs.map(renderTab)}
          </TabList>
        </div>
      </Tabs>

      <ContextMenu
        {...ctxMenu}
        onClose={closeCtx}
        onSelect={onCtxSelect}
      />
    </>
  );
});
