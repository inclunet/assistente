import { lazy } from 'react';
import type { WorkspacePanelProps } from '../workspace/workspacePanelRegistry';
import { useTerminalSurfaceController } from './useTerminalSurfaceController';

const TerminalPage = lazy(() => import('../../pages/TerminalPage'));

export function TerminalWorkspacePanel({ tab, isActive }: WorkspacePanelProps) {
  useTerminalSurfaceController(tab, isActive);

  return <TerminalPage />;
}
