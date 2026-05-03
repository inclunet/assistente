import { lazy, type ComponentType } from 'react';
import type { WorkspaceTab } from '../../store/workspaceStore';
import { EditorWorkspacePanel } from '../editor/EditorWorkspacePanel';
import { TaskListWorkspacePanel } from '../taskLists/TaskListWorkspacePanel';
import { TerminalWorkspacePanel } from '../terminal/TerminalWorkspacePanel';

const ChatPage = lazy(() => import('../../pages/ChatPage'));

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
