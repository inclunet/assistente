import { describe, expect, it, vi } from 'vitest';
import { render, screen } from '@testing-library/react';
import type { WorkspacePanelProps } from '../workspace/workspacePanelRegistry';
import { EditorWorkspacePanel } from './EditorWorkspacePanel';

const controllerMock = vi.fn();

vi.mock('./useEditorSurfaceController', () => ({
  useEditorSurfaceController: (...args: unknown[]) => controllerMock(...args),
}));

vi.mock('../../pages/EditorPage', () => ({
  default: (props: { documentId?: string; workspaceTab?: WorkspacePanelProps['tab']; isPanelActive?: boolean }) => (
    <div>
      editor:{props.documentId}:{props.workspaceTab?.id}:{String(props.isPanelActive)}
    </div>
  ),
}));

describe('EditorWorkspacePanel', () => {
  it('renderiza a página do editor com a identidade explícita do painel', () => {
    const tab: WorkspacePanelProps['tab'] = {
      id: 'editor-tab-1',
      type: 'editor',
      title: 'Editor',
      position: 0,
      state: { filePath: 'doc.md' },
    };

    render(<EditorWorkspacePanel tab={tab} tabId={tab.id} isActive={false} state={tab.state ?? {}} />);

    expect(controllerMock).toHaveBeenCalledWith(tab, false);
    expect(screen.getByText('editor:editor-tab-1:editor-tab-1:false')).toBeInTheDocument();
  });
});
