import { describe, expect, it, vi } from 'vitest';
import { render, screen } from '@testing-library/react';
import type { WorkspacePanelProps } from '../workspace/workspacePanelRegistry';
import { TerminalWorkspacePanel } from './TerminalWorkspacePanel';

const controllerMock = vi.fn();

vi.mock('./useTerminalSurfaceController', () => ({
  useTerminalSurfaceController: (...args: unknown[]) => controllerMock(...args),
}));

vi.mock('../../pages/TerminalPage', () => ({
  default: (props: { sessionId?: string }) => <div>terminal:{props.sessionId}</div>,
}));

describe('TerminalWorkspacePanel', () => {
  it('renderiza a página do terminal com sessionId explícito', () => {
    const tab: WorkspacePanelProps['tab'] = {
      id: 'terminal-tab-1',
      type: 'terminal',
      title: 'Terminal',
      position: 0,
      state: { sessionId: 'session-1' },
    };

    render(<TerminalWorkspacePanel tab={tab} tabId={tab.id} isActive state={tab.state ?? {}} />);

    expect(controllerMock).toHaveBeenCalledWith(tab, true);
    expect(screen.getByText('terminal:session-1')).toBeInTheDocument();
  });
});
