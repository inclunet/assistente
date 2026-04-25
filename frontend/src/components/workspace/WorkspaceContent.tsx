import { Suspense, lazy, useRef } from 'react';
import { useTranslation } from 'react-i18next';
import { useActiveTab } from '../../store/workspaceStore';
import type { TabType } from '../../store/workspaceStore';
import './WorkspaceContent.css';

const ChatPage = lazy(() => import('../../pages/ChatPage'));
const EditorPage = lazy(() => import('../../pages/EditorPage'));
const TerminalPage = lazy(() => import('../../pages/TerminalPage'));
const TaskListView = lazy(() => import('../taskLists/TaskListView'));

const Loading = () => (
  <div className="ws-content__loading" aria-busy="true" />
);

/**
 * Keep-alive: mantém páginas já montadas vivas com display:none
 * para evitar unmount/remount completo ao trocar tipo de aba.
 */
export function WorkspaceContent() {
  const { t } = useTranslation();
  const activeTab = useActiveTab();

  // Rastreia quais tipos de aba já foram montados ao menos uma vez
  const mountedTypesRef = useRef<Set<TabType>>(new Set());

  if (!activeTab) {
    return (
      <div className="ws-content ws-content--empty">
        <p>{t('workspace.noTabs', 'Nenhuma aba aberta')}</p>
      </div>
    );
  }

  // Registra o tipo ativo como montado
  mountedTypesRef.current.add(activeTab.type);

  const activeType = activeTab.type;
  const mounted = mountedTypesRef.current;

  return (
    <div className="ws-content" data-tab-type={activeType}>
      <Suspense fallback={<Loading />}>
        {mounted.has('chat') && (
          <div className="ws-content__pane" style={{ display: activeType === 'chat' ? 'contents' : 'none' }} aria-hidden={activeType !== 'chat'}>
            <ChatPage />
          </div>
        )}
        {mounted.has('editor') && (
          <div className="ws-content__pane" style={{ display: activeType === 'editor' ? 'contents' : 'none' }} aria-hidden={activeType !== 'editor'}>
            <EditorPage />
          </div>
        )}
        {mounted.has('terminal') && (
          <div className="ws-content__pane" style={{ display: activeType === 'terminal' ? 'contents' : 'none' }} aria-hidden={activeType !== 'terminal'}>
            <TerminalPage />
          </div>
        )}
        {mounted.has('tasklist') && (
          <div className="ws-content__pane" style={{ display: activeType === 'tasklist' ? 'contents' : 'none' }} aria-hidden={activeType !== 'tasklist'}>
            {activeTab.state?.tasklistId
              ? <TaskListView taskListId={parseInt(activeTab.state.tasklistId as string, 10)} />
              : <Loading />}
          </div>
        )}
      </Suspense>
    </div>
  );
}
