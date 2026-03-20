import React, { useState, useEffect, useCallback } from 'react';
import { createPortal } from 'react-dom';
import { useTranslation } from 'react-i18next';
import { ToolbarButton } from '../ui/Toolbar';
import { Menu, type MenuItem } from '../menu';
import { useAnchoredContextMenu } from '../../hooks/useAnchoredContextMenu';
import { useTaskListStore } from '../../store/taskListStore';
import { TaskListSelectorModal } from './TaskListSelectorModal';
import { TaskListViewerModal } from './TaskListViewerModal';
import type { database } from '@wailsjs/go/models';

interface ResourcesMenuProps {
  conversationId?: number;
  disabled?: boolean;
}

export const ResourcesMenu: React.FC<ResourcesMenuProps> = ({
  conversationId,
  disabled,
}) => {
  const { t } = useTranslation();
  const { getTaskListsByConversation } = useTaskListStore();

  const [linkedTaskLists, setLinkedTaskLists] = useState<database.TaskList[]>([]);
  const [selectorOpen, setSelectorOpen] = useState(false);
  const [viewerOpen, setViewerOpen] = useState(false);
  const [viewerTaskListId, setViewerTaskListId] = useState<number>(0);

  const {
    menu,
    openForTrigger,
    closeMenu,
    onSelectItem,
  } = useAnchoredContextMenu();

  // Load linked task lists when conversation changes
  const refreshLinkedLists = useCallback(async () => {
    if (!conversationId) {
      setLinkedTaskLists([]);
      return;
    }
    const lists = await getTaskListsByConversation(conversationId);
    setLinkedTaskLists(lists);
  }, [conversationId, getTaskListsByConversation]);

  useEffect(() => {
    void refreshLinkedLists();
  }, [refreshLinkedLists]);

  const handleViewTaskList = useCallback((taskListId: number) => {
    setViewerTaskListId(taskListId);
    setViewerOpen(true);
  }, []);

  const buildMenuItems = useCallback((): MenuItem[] => {
    // TaskList submenu
    const taskListItems: MenuItem[] = [];

    if (linkedTaskLists.length > 0) {
      for (const tl of linkedTaskLists) {
        taskListItems.push({
          id: `view-tl-${tl.id}`,
          label: `✅ ${tl.title}`,
          icon: '📋',
          action: () => handleViewTaskList(tl.id),
        });
      }
      taskListItems.push({ id: 'tl-sep', separator: true });
    }

    taskListItems.push({
      id: 'link-tasklist',
      label: t('tasklist.linkTaskList'),
      icon: '🔗',
      action: () => setSelectorOpen(true),
      disabled: !conversationId,
    });

    // Editor submenu (placeholder)
    const editorItems: MenuItem[] = [
      {
        id: 'editor-soon',
        label: t('tasklist.comingSoon'),
        disabled: true,
      },
    ];

    // Terminal submenu (placeholder)
    const terminalItems: MenuItem[] = [
      {
        id: 'terminal-soon',
        label: t('tasklist.comingSoon'),
        disabled: true,
      },
    ];

    const badge = linkedTaskLists.length > 0 ? ` (${linkedTaskLists.length})` : '';

    return [
      {
        id: 'tasklists-group',
        label: `${t('tasklist.linkedTaskLists')}${badge}`,
        icon: '📋',
        submenu: taskListItems,
      },
      {
        id: 'editor-group',
        label: t('tasklist.editorSessions'),
        icon: '📝',
        submenu: editorItems,
      },
      {
        id: 'terminal-group',
        label: t('tasklist.terminalSessions'),
        icon: '💻',
        submenu: terminalItems,
      },
    ];
  }, [linkedTaskLists, conversationId, t, handleViewTaskList]);

  const handleClick = useCallback(
    (e: React.MouseEvent<HTMLButtonElement>) => {
      openForTrigger(e.currentTarget, t('tasklist.resourcesMenu'), buildMenuItems());
    },
    [openForTrigger, buildMenuItems, t]
  );

  const handleSelectorClose = useCallback(() => {
    setSelectorOpen(false);
    void refreshLinkedLists();
  }, [refreshLinkedLists]);

  const handleViewerClose = useCallback(() => {
    setViewerOpen(false);
    setViewerTaskListId(0);
    void refreshLinkedLists();
  }, [refreshLinkedLists]);

  const handleUnlink = useCallback(() => {
    void refreshLinkedLists();
  }, [refreshLinkedLists]);

  const badgeText = linkedTaskLists.length > 0 ? `${linkedTaskLists.length}` : undefined;

  return (
    <>
      <ToolbarButton
        label={t('tasklist.resources')}
        icon={`📎${badgeText ? `\u00A0${badgeText}` : ''}`}
        onClick={handleClick}
        disabled={disabled}
        aria-haspopup="menu"
      />

      {createPortal(
        <Menu
          visible={menu.visible}
          x={menu.x}
          y={menu.y}
          items={menu.items}
          ariaLabel={menu.ariaLabel}
          onClose={closeMenu}
          onSelect={onSelectItem}
        />,
        document.body
      )}

      {conversationId && (
        <>
          <TaskListSelectorModal
            isOpen={selectorOpen}
            onClose={handleSelectorClose}
            conversationId={conversationId}
            linkedTaskListIds={linkedTaskLists.map((tl) => tl.id)}
          />

          <TaskListViewerModal
            isOpen={viewerOpen}
            onClose={handleViewerClose}
            taskListId={viewerTaskListId}
            onUnlink={handleUnlink}
          />
        </>
      )}
    </>
  );
};
