import { createContext, useContext } from 'react';
import type { WorkspaceTab } from '../../store/workspaceStore';

interface WorkspacePanelContextValue {
  tab: WorkspaceTab;
  isActive: boolean;
}

const WorkspacePanelContext = createContext<WorkspacePanelContextValue | null>(null);

export const WorkspacePanelProvider = WorkspacePanelContext.Provider;

export function useOptionalWorkspacePanel() {
  return useContext(WorkspacePanelContext);
}

export function useWorkspacePanel() {
  const panel = useContext(WorkspacePanelContext);
  if (!panel) {
    throw new Error('useWorkspacePanel must be used within WorkspacePanelProvider');
  }

  return panel;
}
