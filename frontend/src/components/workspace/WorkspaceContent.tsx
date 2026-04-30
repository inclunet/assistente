import { Suspense, lazy, useEffect, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useActiveTab, useWorkspaceStore, type WorkspaceTab } from '../../store/workspaceStore';
import { WorkspacePanelProvider } from './WorkspacePanelContext';
import './WorkspaceContent.css';

const ChatPage = lazy(() => import('../../pages/ChatPage'));
const EditorPage = lazy(() => import('../../pages/EditorPage'));
const TerminalPage = lazy(() => import('../../pages/TerminalPage'));
const TaskListView = lazy(() => import('../taskLists/TaskListView'));

const Loading = () => (
  <div className="ws-content__loading" aria-busy="true" />
);

function WorkspaceTabPanel({ tab, isActive }: { tab: WorkspaceTab; isActive: boolean }) {
  return (
    <div
      className="ws-content__panel"
      data-tab-id={tab.id}
      data-tab-type={tab.type}
      data-active={isActive ? 'true' : 'false'}
      hidden={!isActive}
      aria-hidden={!isActive}
    >
      <WorkspacePanelProvider value={{ tab, isActive }}>
        <Suspense fallback={<Loading />}>
          {tab.type === 'chat' && <ChatPage />}
          {tab.type === 'editor' && <EditorPage />}
          {tab.type === 'terminal' && <TerminalPage />}
          {tab.type === 'tasklist' && (
            tab.state?.tasklistId
              ? <TaskListView taskListId={tab.state.tasklistId as string} />
              : <Loading />
          )}
        </Suspense>
      </WorkspacePanelProvider>
    </div>
  );
}

export function WorkspaceContent() {
  const { t } = useTranslation();
  const activeTab = useActiveTab();
  const tabs = useWorkspaceStore((s) => s.workspace?.tabs ?? []);
  const [visitedTabIds, setVisitedTabIds] = useState<Set<string>>(new Set());

  useEffect(() => {
    if (!activeTab) return;
    setVisitedTabIds((prev) => {
      if (prev.has(activeTab.id)) return prev;
      const next = new Set(prev);
      next.add(activeTab.id);
      return next;
    });
  }, [activeTab?.id]);

  useEffect(() => {
    setVisitedTabIds((prev) => {
      if (prev.size === 0) return prev;
      const openIds = new Set(tabs.map((tab) => tab.id));
      let changed = false;
      const next = new Set<string>();
      for (const id of prev) {
        if (openIds.has(id)) {
          next.add(id);
        } else {
          changed = true;
        }
      }
      return changed ? next : prev;
    });
  }, [tabs]);

  const mountedPanels = useMemo(() => {
    const panels: Array<{ key: string; tab: WorkspaceTab }> = [];
    let editorPanel: WorkspaceTab | null = null;

    for (const tab of tabs) {
      const shouldMount = tab.id === activeTab?.id || visitedTabIds.has(tab.id);
      if (!shouldMount) continue;

      if (tab.type === 'editor') {
        if (tab.id === activeTab?.id) {
          editorPanel = tab;
        } else if (!editorPanel) {
          editorPanel = tab;
        }
        continue;
      }

      panels.push({ key: tab.id, tab });
    }

    if (editorPanel) {
      panels.push({ key: 'editor', tab: editorPanel });
    }

    return panels;
  }, [activeTab?.id, tabs, visitedTabIds]);

  if (!activeTab) {
    return (
      <div className="ws-content ws-content--empty">
        <p>{t('workspace.noTabs', 'Nenhuma aba aberta')}</p>
      </div>
    );
  }

  return (
    <div className="ws-content" data-tab-type={activeTab.type}>
      {mountedPanels.map(({ key, tab }) => (
        <WorkspaceTabPanel
          key={key}
          tab={tab}
          isActive={tab.id === activeTab.id}
        />
      ))}
    </div>
  );
}
