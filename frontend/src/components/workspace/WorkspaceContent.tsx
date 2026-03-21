import { Suspense, lazy } from 'react';
import { useWorkspaceStore } from '../../store/workspaceStore';
import './WorkspaceContent.css';

const ChatPage = lazy(() => import('../../pages/ChatPage'));
const EditorPage = lazy(() => import('../../pages/EditorPage'));
const TerminalPage = lazy(() => import('../../pages/TerminalPage'));

const Loading = () => (
  <div className="ws-content__loading" aria-busy="true" />
);

export function WorkspaceContent() {
  const activeTab = useWorkspaceStore((s) => s.getActiveTab());

  if (!activeTab) {
    return (
      <div className="ws-content ws-content--empty">
        <p>Nenhuma aba aberta</p>
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
          <div className="ws-content__placeholder">
            <span aria-hidden="true">✅</span>
            <p>Tasklist será implementada em breve</p>
          </div>
        )}
      </Suspense>
    </div>
  );
}
