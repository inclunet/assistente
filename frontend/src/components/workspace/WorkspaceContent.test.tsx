import { render, screen } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { WorkspaceContent } from './WorkspaceContent';
import type { WorkspaceTab } from '../../store/workspaceStore';

const workspaceState = vi.hoisted(() => ({
  activeTabId: 'editor-1',
  tabs: [] as WorkspaceTab[],
}));

vi.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (_key: string, fallback?: string) => fallback ?? _key }),
}));

vi.mock('../../store/workspaceStore', () => ({
  useActiveTab: () => workspaceState.tabs.find((tab) => tab.id === workspaceState.activeTabId) ?? null,
  useWorkspaceTabs: () => workspaceState.tabs,
}));

vi.mock('./workspacePanelRegistry', () => ({
  WorkspaceDomainPanel: (props: { tab: WorkspaceTab; tabId: string; isActive: boolean; state: Record<string, unknown> }) => (
    <div>
      panel:{props.tabId}:{String(props.isActive)}:{String(props.state.sessionId ?? props.state.filePath ?? props.state.tasklistId ?? '')}
      <button type="button">focus-{props.tabId}</button>
    </div>
  ),
}));

describe('WorkspaceContent', () => {
  beforeEach(() => {
    workspaceState.activeTabId = 'editor-1';
    workspaceState.tabs = [
      {
        id: 'editor-1',
        type: 'editor',
        title: 'Editor',
        position: 0,
        state: { filePath: 'a.md' },
      },
      {
        id: 'terminal-1',
        type: 'terminal',
        title: 'Terminal',
        position: 1,
        state: { sessionId: 'session-1' },
      },
    ];
  });

  it('mantém painéis visitados montados com identidades e estados próprios', () => {
    const { rerender } = render(<WorkspaceContent />);

    expect(screen.getByText('panel:editor-1:true:a.md')).toBeInTheDocument();

    workspaceState.activeTabId = 'terminal-1';
    rerender(<WorkspaceContent />);

    expect(screen.getByText('panel:editor-1:false:a.md')).toBeInTheDocument();
    expect(screen.getByText('panel:terminal-1:true:session-1')).toBeInTheDocument();
  });

  it('remove o foco do painel que se torna inativo', () => {
    const { rerender } = render(<WorkspaceContent />);
    const editorButton = screen.getByRole('button', { name: 'focus-editor-1' });
    editorButton.focus();

    workspaceState.activeTabId = 'terminal-1';
    rerender(<WorkspaceContent />);

    expect(editorButton).not.toHaveFocus();
    expect(document.body).toHaveFocus();
  });
});
