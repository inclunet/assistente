import { isModalOpen } from './modalRegistry';
import { useAuthStore } from '../store/authStore';
import { useWorkspaceStore, type WorkspaceTab } from '../store/workspaceStore';

export interface WorkspaceTabTargetLease {
  isCurrent(): boolean;
  dispose(): void;
}

interface CapturedTarget {
  ownerId: string;
  sessionId: string;
  workspaceId: string;
  tabId: string;
  root: HTMLElement;
}

function isWorkspaceRoute(pathname: string): boolean {
  return pathname === '' || pathname === '/';
}

function isHiddenByAncestor(element: HTMLElement): boolean {
  for (let current: HTMLElement | null = element; current; current = current.parentElement) {
    if (
      current.hidden ||
      current.hasAttribute('hidden') ||
      current.hasAttribute('inert') ||
      current.getAttribute('aria-hidden') === 'true'
    ) return true;
  }
  return false;
}

function activeTabSnapshot(): { workspace: NonNullable<ReturnType<typeof useWorkspaceStore.getState>['workspace']>; tab: WorkspaceTab } | undefined {
  const workspace = useWorkspaceStore.getState().workspace;
  if (!workspace || !Array.isArray(workspace.tabs) || !workspace.activeTabId) return undefined;
  const tab = workspace.tabs.find((candidate) => candidate.id === workspace.activeTabId);
  return tab ? { workspace, tab } : undefined;
}

function findWorkspaceRoot(): HTMLElement | undefined {
  const root = document.querySelector<HTMLElement>('.workspace-layout');
  if (!root || !root.isConnected || isHiddenByAncestor(root)) return undefined;
  return root;
}

function sameActiveTab(target: CapturedTarget): boolean {
  const current = activeTabSnapshot();
  return Boolean(
    current &&
    current.workspace.id === target.workspaceId &&
    current.tab.id === target.tabId,
  );
}

export function captureWorkspaceTabTarget(
  readPathname: () => string,
): WorkspaceTabTargetLease | undefined {
  if (typeof document === 'undefined' || !isWorkspaceRoute(readPathname()) || isModalOpen()) return undefined;
  const auth = useAuthStore.getState();
  const active = activeTabSnapshot();
  const root = findWorkspaceRoot();
  if (!auth.isAuthenticated || !auth.user || !active || !root) return undefined;

  const captured: CapturedTarget = {
    ownerId: auth.user.userId,
    sessionId: auth.user.sessionId,
    workspaceId: active.workspace.id,
    tabId: active.tab.id,
    root,
  };
  let disposed = false;
  let invalidated = false;

  const checkAuth = () => {
    const current = useAuthStore.getState();
    if (
      !current.isAuthenticated ||
      current.user?.userId !== captured.ownerId ||
      current.user?.sessionId !== captured.sessionId
    ) invalidated = true;
  };
  const checkWorkspace = () => {
    if (!sameActiveTab(captured)) invalidated = true;
  };
  const unsubscribeAuth = useAuthStore.subscribe(checkAuth);
  const unsubscribeWorkspace = useWorkspaceStore.subscribe(checkWorkspace);
  const onBlur = () => { invalidated = true; };
  window.addEventListener('blur', onBlur);

  return {
    isCurrent: () => {
      if (disposed || invalidated || !isWorkspaceRoute(readPathname()) || isModalOpen()) return false;
      const authNow = useAuthStore.getState();
      const currentRoot = findWorkspaceRoot();
      return Boolean(
        authNow.isAuthenticated &&
        authNow.user?.userId === captured.ownerId &&
        authNow.user.sessionId === captured.sessionId &&
        sameActiveTab(captured) &&
        currentRoot === captured.root,
      );
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
