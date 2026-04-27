import { Suspense, lazy } from 'react';
import { useTranslation } from 'react-i18next';
import { useActiveTab } from '../../store/workspaceStore';
import './WorkspaceContent.css';

const ChatPage = lazy(() => import('../../pages/ChatPage'));
const EditorPage = lazy(() => import('../../pages/EditorPage'));
const TerminalPage = lazy(() => import('../../pages/TerminalPage'));
const TaskListView = lazy(() => import('../taskLists/TaskListView'));

const Loading = () => (
  <div className="ws-content__loading" aria-busy="true" />
);

export function WorkspaceContent() {
  const { t } = useTranslation();
  const activeTab = useActiveTab();

  if (!activeTab) {
    return (
      <div className="ws-content ws-content--empty">
        <p>{t('workspace.noTabs', 'Nenhuma aba aberta')}</p>
      </div>
    );
  }

  return (
    <div className="ws-content" data-tab-type={activeTab.type}>
      <Suspense fallback={<Loading />}>
        {activeTab.type === 'chat' && <ChatPage />}
        {activeTab.type === 'editor' && <EditorPage />}
        {activeTab.type === 'terminal' && <TerminalPage />}
        {activeTab.type === 'tasklist' && (
          activeTab.state?.tasklistId
            ? <TaskListView taskListId={activeTab.state.tasklistId as string} />
            : <Loading />
        )}
      </Suspense>
    </div>
  );
}
