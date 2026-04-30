import { createContext, useContext } from 'react';
import { useActiveTab, type WorkspaceTab } from '../../store/workspaceStore';

interface WorkspacePanelContextValue {
  tab: WorkspaceTab;
  isActive: boolean;
}

const WorkspacePanelContext = createContext<WorkspacePanelContextValue | null>(null);

export const WorkspacePanelProvider = WorkspacePanelContext.Provider;

export function useWorkspacePanel() {
  const panel = useContext(WorkspacePanelContext);
  const activeTab = useActiveTab();

  return {
    tab: panel?.tab ?? activeTab,
    isActive: panel?.isActive ?? true,
  };
}
