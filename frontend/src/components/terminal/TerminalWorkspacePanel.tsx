import TerminalPage from '../../pages/TerminalPage';
import type { WorkspacePanelProps } from '../workspace/workspacePanelRegistry';
import { useTerminalSurfaceController } from './useTerminalSurfaceController';

export function TerminalWorkspacePanel({ tab, isActive, state }: WorkspacePanelProps) {
  useTerminalSurfaceController(tab, isActive);

  const sessionId = typeof state.sessionId === 'string' ? state.sessionId : undefined;
  return <TerminalPage sessionId={sessionId} />;
}
