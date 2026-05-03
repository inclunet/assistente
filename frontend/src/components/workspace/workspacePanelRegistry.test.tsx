import { render, screen } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';
import { WorkspaceDomainPanel, type WorkspacePanelProps } from './workspacePanelRegistry';
import type { WorkspaceTab } from '../../store/workspaceStore';

vi.mock('../../pages/ChatPage', () => ({
  default: () => <div>chat-panel</div>,
}));

vi.mock('../editor/EditorWorkspacePanel', () => ({
  EditorWorkspacePanel: (props: WorkspacePanelProps) => <div>editor-panel:{props.tabId}:{String(props.isActive)}</div>,
}));

vi.mock('../terminal/TerminalWorkspacePanel', () => ({
  TerminalWorkspacePanel: (props: WorkspacePanelProps) => <div>terminal-panel:{props.tabId}:{String(props.isActive)}</div>,
}));

vi.mock('../taskLists/TaskListWorkspacePanel', () => ({
  TaskListWorkspacePanel: (props: WorkspacePanelProps) => <div>tasklist-panel:{String(props.state.tasklistId)}</div>,
}));

const makeTab = (type: WorkspaceTab['type'], state: Record<string, unknown> = {}): WorkspaceTab => ({
  id: `tab-${type}`,
  type,
  title: type,
  position: 0,
  state,
});

describe('WorkspaceDomainPanel', () => {
  it('roteia abas para o painel declarativo do domínio', async () => {
    render(
      <>
        <WorkspaceDomainPanel tab={makeTab('chat')} tabId="tab-chat" isActive state={{}} />
        <WorkspaceDomainPanel tab={makeTab('editor')} tabId="tab-editor" isActive={false} state={{}} />
        <WorkspaceDomainPanel tab={makeTab('terminal')} tabId="tab-terminal" isActive state={{}} />
        <WorkspaceDomainPanel tab={makeTab('tasklist', { tasklistId: '42' })} tabId="tab-tasklist" isActive state={{ tasklistId: '42' }} />
      </>,
    );

    expect(await screen.findByText('chat-panel')).toBeInTheDocument();
    expect(screen.getByText('editor-panel:tab-editor:false')).toBeInTheDocument();
    expect(screen.getByText('terminal-panel:tab-terminal:true')).toBeInTheDocument();
    expect(screen.getByText('tasklist-panel:42')).toBeInTheDocument();
  });
});
