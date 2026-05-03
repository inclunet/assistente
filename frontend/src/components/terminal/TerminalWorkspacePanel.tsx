import TerminalPage from '../../pages/TerminalPage';
import type { WorkspacePanelProps } from '../workspace/workspacePanelRegistry';
import { useTerminalSurfaceController } from './useTerminalSurfaceController';

export function TerminalWorkspacePanel({ tab, isActive }: WorkspacePanelProps) {
  useTerminalSurfaceController(tab, isActive);

  return <TerminalPage />;
}
