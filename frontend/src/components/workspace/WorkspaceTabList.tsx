import { useCallback, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useWorkspaceStore, type WorkspaceTab, type TabType } from '../../store/workspaceStore';
import { playBumpSound } from '../../services/audioFeedback';
import { Tabs, TabList, Tab } from '../ui/tabs';
import './WorkspaceTabList.css';

const TAB_TYPE_ICONS: Record<TabType, string> = {
  chat: '💬',
  editor: '📝',
  terminal: '>_',
  tasklist: '✅',
};

export function WorkspaceTabList() {
  const { t } = useTranslation();
  const { workspace, setActiveTab, removeTab, updateTab } = useWorkspaceStore();
  const tabListRef = useRef<HTMLDivElement>(null);

  const [editingTabId, setEditingTabId] = useState<string | null>(null);
  const [editingTitle, setEditingTitle] = useState('');
  const editInputRef = useRef<HTMLInputElement>(null);

  const tabs = workspace?.tabs || [];
  const activeTabId = workspace?.activeTabId || '';

  const handleSelect = useCallback((tabId: string) => {
    if (!tabId) return;
    void setActiveTab(tabId);
  }, [setActiveTab]);

  const handleDelete = useCallback((tabId: string) => {
    if (tabs.length <= 1) return;
    void removeTab(tabId);
  }, [removeTab, tabs.length]);

  const startEditing = useCallback((tabId: string, currentTitle: string) => {
    setEditingTabId(tabId);
    setEditingTitle(currentTitle);
    setTimeout(() => {
      editInputRef.current?.focus();
      editInputRef.current?.select();
    }, 10);
  }, []);

  const confirmEditing = useCallback(() => {
    const tabIdToFocus = editingTabId;
    if (editingTabId && editingTitle.trim()) {
      void updateTab(editingTabId, { title: editingTitle.trim() });
    }
    setEditingTabId(null);
    setEditingTitle('');

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
  }, [editingTabId, editingTitle, updateTab]);

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
      confirmEditing();
    } else if (e.key === 'Escape') {
      e.preventDefault();
      e.stopPropagation();
      cancelEditing();
    }
  }, [confirmEditing, cancelEditing]);

  const handleListKeyDown = useCallback((e: React.KeyboardEvent<HTMLDivElement>) => {
    if (editingTabId) return;
    if (e.defaultPrevented) return;

    const focused = tabListRef.current?.querySelector('button[role="tab"]:focus') as HTMLButtonElement | null;
    const tabId = focused?.getAttribute('data-tab-value');
    if (!tabId) return;

    if (e.key === 'F2') {
      e.preventDefault();
      const tab = tabs.find(t => t.id === tabId);
      if (tab) startEditing(tabId, tab.title);
    }
  }, [editingTabId, tabs, startEditing]);

  const handleActivate = useCallback(() => {
    return !!editingTabId;
  }, [editingTabId]);

  const renderTab = (tab: WorkspaceTab) => {
    const isActive = tab.id === activeTabId;
    const isEditing = editingTabId === tab.id;
    const icon = TAB_TYPE_ICONS[tab.type] || '📄';

    return (
      <div
        key={tab.id}
        className={`ws-tabs__tab-wrapper${isActive ? ' ws-tabs__tab-wrapper--active' : ''}`}
        role="presentation"
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
          ariaDescription={`${t(`workspace.tabType.${tab.type}`)} - ${tab.title}`}
        >
          <span className="ws-tabs__tab-icon" aria-hidden="true">{icon}</span>
          <span className="ws-tabs__tab-title">{tab.title}</span>
        </Tab>

        {isEditing && (
          <input
            ref={editInputRef}
            type="text"
            className="ws-tabs__tab-edit"
            value={editingTitle}
            onChange={(e) => setEditingTitle(e.target.value)}
            onKeyDown={handleEditKeyDown}
            onBlur={confirmEditing}
            onClick={(e) => e.stopPropagation()}
            aria-label={t('workspace.editTabTitle')}
          />
        )}

        {tabs.length > 1 && (
          <button
            className="ws-tabs__tab-close"
            onClick={(e) => {
              e.preventDefault();
              e.stopPropagation();
              void removeTab(tab.id);
            }}
            aria-label={`${t('workspace.closeTab')} ${tab.title}`}
            tabIndex={-1}
            type="button"
          >
            ×
          </button>
        )}
      </div>
    );
  };

  return (
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
        role="region"
        aria-label={t('workspace.tabsRegion')}
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
  );
}
