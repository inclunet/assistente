import { getModalRegistrySnapshot, isModalOpen } from './modalRegistry';
import { useAuthStore } from '../store/authStore';
import { useWorkspaceStore } from '../store/workspaceStore';

export interface WorkspaceCreateTargetLease {
  isCurrent(): boolean;
  dispose(): void;
}

interface CapturedTarget {
  ownerId: string;
  sessionId: string;
  workspaceId: string;
  activeTabId: string;
  routeIdentity: string;
  modalGeneration: number;
}

function activeWorkspaceTab() {
  const workspace = useWorkspaceStore.getState().workspace;
  if (!workspace || !workspace.activeTabId) return undefined;
  const tab = workspace.tabs.find((candidate) => candidate.id === workspace.activeTabId);
  return tab ? { workspace, tab } : undefined;
}

/** Captures the authenticated workspace origin for the generic workspace.create mutation. */
export function captureWorkspaceCreateTarget(
  readPathname: () => string,
): WorkspaceCreateTargetLease | undefined {
  if (typeof document === 'undefined') return undefined;

  let routeIdentity: string;
  try {
    routeIdentity = readPathname();
    if (isModalOpen() || !document.hasFocus()) return undefined;
  } catch {
    return undefined;
  }

  const auth = useAuthStore.getState();
  const active = activeWorkspaceTab();
  if (!auth.isAuthenticated || !auth.user || !active) return undefined;

  const captured: CapturedTarget = {
    ownerId: auth.user.userId,
    sessionId: auth.user.sessionId,
    workspaceId: active.workspace.id,
    activeTabId: active.tab.id,
    routeIdentity,
    modalGeneration: getModalRegistrySnapshot().generationNumber,
  };
  let disposed = false;
  let invalidated = false;

  const invalidateAuth = () => {
    const current = useAuthStore.getState();
    if (!current.isAuthenticated || current.user?.userId !== captured.ownerId ||
        current.user?.sessionId !== captured.sessionId) invalidated = true;
  };
  const invalidateWorkspace = () => {
    const current = activeWorkspaceTab();
    if (!current || current.workspace.id !== captured.workspaceId || current.tab.id !== captured.activeTabId) {
      invalidated = true;
    }
  };
  const unsubscribeAuth = useAuthStore.subscribe(invalidateAuth);
  const unsubscribeWorkspace = useWorkspaceStore.subscribe(invalidateWorkspace);
  const onBlur = () => { invalidated = true; };
  window.addEventListener('blur', onBlur);

  return {
    isCurrent: () => {
      try {
        if (disposed || invalidated || !document.hasFocus() || isModalOpen()) return false;
        if (readPathname() !== captured.routeIdentity) return false;
        if (getModalRegistrySnapshot().generationNumber !== captured.modalGeneration) return false;
        const currentAuth = useAuthStore.getState();
        const current = activeWorkspaceTab();
        return Boolean(
          currentAuth.isAuthenticated &&
          currentAuth.user?.userId === captured.ownerId &&
          currentAuth.user.sessionId === captured.sessionId &&
          current?.workspace.id === captured.workspaceId &&
          current.tab.id === captured.activeTabId,
        );
      } catch {
        return false;
      }
    },
    dispose: () => {
      if (disposed) return;
      disposed = true;
      unsubscribeAuth();
      unsubscribeWorkspace();
      window.removeEventListener('blur', onBlur);
    },
  };
}
