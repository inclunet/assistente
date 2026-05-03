import { lazy, type ComponentType } from 'react';
import type { WorkspaceTab } from '../../store/workspaceStore';

const ChatPage = lazy(() => import('../../pages/ChatPage'));
const EditorPage = lazy(() => import('../../pages/EditorPage'));
const TerminalPage = lazy(() => import('../../pages/TerminalPage'));
const TaskListView = lazy(() => import('../taskLists/TaskListView'));

export interface WorkspacePanelProps<TState extends Record<string, unknown> = Record<string, unknown>> {
  tab: WorkspaceTab;
  tabId: string;
  isActive: boolean;
  state: TState;
}

export type WorkspacePanelComponent<TState extends Record<string, unknown> = Record<string, unknown>> = ComponentType<WorkspacePanelProps<TState>>;

function ChatWorkspacePanel() {
  return <ChatPage />;
}

function EditorWorkspacePanel() {
  return <EditorPage />;
}

function TerminalWorkspacePanel() {
  return <TerminalPage />;
}

function TaskListWorkspacePanel({ state }: WorkspacePanelProps) {
  const taskListId = state.tasklistId;
  if (typeof taskListId !== 'string' || !taskListId) {
    return <div className="ws-content__loading" aria-busy="true" />;
  }

  return <TaskListView taskListId={taskListId} />;
}

const workspacePanelRegistry: Record<WorkspaceTab['type'], WorkspacePanelComponent> = {
  chat: ChatWorkspacePanel,
  editor: EditorWorkspacePanel,
  terminal: TerminalWorkspacePanel,
  tasklist: TaskListWorkspacePanel,
};

export function WorkspaceDomainPanel(props: WorkspacePanelProps) {
  const Panel = workspacePanelRegistry[props.tab.type];
  return <Panel {...props} />;
}
