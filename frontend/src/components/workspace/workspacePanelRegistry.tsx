import { lazy, type ComponentType, type LazyExoticComponent } from 'react';
import type { WorkspaceTab } from '../../store/workspaceStore';

const ChatPage = lazy(() => import('../../pages/ChatPage'));
const EditorWorkspacePanel = lazy(() => import('../editor/EditorWorkspacePanel')
  .then((module) => ({ default: module.EditorWorkspacePanel })));
const TerminalWorkspacePanel = lazy(() => import('../terminal/TerminalWorkspacePanel')
  .then((module) => ({ default: module.TerminalWorkspacePanel })));
const TaskListWorkspacePanel = lazy(() => import('../taskLists/TaskListWorkspacePanel')
  .then((module) => ({ default: module.TaskListWorkspacePanel })));

export interface WorkspacePanelProps<TState extends Record<string, unknown> = Record<string, unknown>> {
  tab: WorkspaceTab;
  tabId: string;
  isActive: boolean;
  state: TState;
}

export type WorkspacePanelComponent<TState extends Record<string, unknown> = Record<string, unknown>> =
  | ComponentType<WorkspacePanelProps<TState>>
  | LazyExoticComponent<ComponentType<WorkspacePanelProps<TState>>>;

function ChatWorkspacePanel() {
  return <ChatPage />;
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
