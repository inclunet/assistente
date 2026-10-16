import { isModalOpen } from './modalRegistry';
import { useAuthStore } from '../store/authStore';
import { useWorkspaceStore, type TabType, type WorkspaceTab } from '../store/workspaceStore';
import {
  getWorkspacePanelImmediateFocusHandler,
  canFocusWorkspacePanelImmediately,
  type WorkspacePanelFocusHandler,
} from '../components/workspace/workspacePanelFocusRegistry';

export const WORKSPACE_PANEL_FOCUS_COMMAND_ID = 'workspace.panel.focus';

export interface WorkspacePanelTargetLease {
  isCurrent(): boolean;
  focus(): undefined;
  dispose(): void;
}

interface CapturedTarget {
  ownerId: string;
  sessionId: string;
  workspaceId: string;
  tabId: string;
  type: TabType;
  root: HTMLElement;
  handler: WorkspacePanelFocusHandler;
}

function routeIsWorkspace(pathname: string): boolean {
  return pathname === '' || pathname === '/';
}

function isHiddenByAncestor(element: HTMLElement): boolean {
  for (let current: HTMLElement | null = element; current; current = current.parentElement) {
    if (
      current.hidden ||
      current.hasAttribute('hidden') ||
      current.hasAttribute('inert') ||
      current.getAttribute('aria-hidden') === 'true'
    ) {
      return true;
    }
  }
  return false;
}

function activeTabSnapshot() {
  const workspace = useWorkspaceStore.getState().workspace;
  if (!workspace || !workspace.activeTabId) return undefined;
  const tab = workspace.tabs.find((candidate) => candidate.id === workspace.activeTabId);
  return tab ? { workspace, tab } : undefined;
}

function sameTab(left: WorkspaceTab | undefined, right: CapturedTarget): boolean {
  return Boolean(left && left.id === right.tabId && left.type === right.type);
}

function findPanel(tabId: string, type: TabType): HTMLElement | undefined {
  const root = document.querySelector<HTMLElement>(
    `.ws-content__panel[data-tab-id="${CSS.escape(tabId)}"]`,
  );
  if (
    !root ||
    !root.isConnected ||
    root.dataset.tabId !== tabId ||
    root.dataset.tabType !== type ||
    root.dataset.active !== 'true' ||
    isHiddenByAncestor(root)
  ) {
    return undefined;
  }
  return root;
}

export function captureWorkspacePanelTarget(
  readPathname: () => string,
): WorkspacePanelTargetLease | undefined {
  if (typeof document === 'undefined') return undefined;
  try {
    if (!routeIsWorkspace(readPathname()) || isModalOpen()) return undefined;
  } catch {
    return undefined;
  }

  const auth = useAuthStore.getState();
  const target = activeTabSnapshot();
  if (!auth.isAuthenticated || !auth.user || !target) return undefined;

  const root = findPanel(target.tab.id, target.tab.type);
  const handler = getWorkspacePanelImmediateFocusHandler(target.tab.id);
  if (!root || !handler) return undefined;
  try {
    if (!canFocusWorkspacePanelImmediately(target.tab.id)) return undefined;
  } catch {
    return undefined;
  }

  const captured: CapturedTarget = {
    ownerId: auth.user.userId,
    sessionId: auth.user.sessionId,
    workspaceId: target.workspace.id,
    tabId: target.tab.id,
    type: target.tab.type,
    root,
    handler,
  };
  let disposed = false;
  let invalidated = false;

  const invalidateIfRelevantAuthChanged = () => {
    const current = useAuthStore.getState();
    if (
      !current.isAuthenticated ||
      current.user?.userId !== captured.ownerId ||
      current.user?.sessionId !== captured.sessionId
    ) {
      invalidated = true;
    }
  };
  const invalidateIfRelevantWorkspaceChanged = () => {
    const current = activeTabSnapshot();
    if (
      !current ||
      current.workspace.id !== captured.workspaceId ||
      !sameTab(current.tab, captured)
    ) {
      invalidated = true;
    }
  };

  const unsubscribeAuth = useAuthStore.subscribe(invalidateIfRelevantAuthChanged);
  const unsubscribeWorkspace = useWorkspaceStore.subscribe(invalidateIfRelevantWorkspaceChanged);
  const invalidate = () => { invalidated = true; };
  window.addEventListener('blur', invalidate);

  const isCurrent = (): boolean => {
    try {
      if (disposed || invalidated || !routeIsWorkspace(readPathname()) || isModalOpen()) return false;
      const authNow = useAuthStore.getState();
      const activeNow = activeTabSnapshot();
      const currentRoot = findPanel(captured.tabId, captured.type);
      return Boolean(
        authNow.isAuthenticated &&
        authNow.user?.userId === captured.ownerId &&
        authNow.user.sessionId === captured.sessionId &&
        activeNow?.workspace.id === captured.workspaceId &&
        sameTab(activeNow.tab, captured) &&
        currentRoot === captured.root &&
        getWorkspacePanelImmediateFocusHandler(captured.tabId) === captured.handler &&
        canFocusWorkspacePanelImmediately(captured.tabId)
      );
    } catch {
      return false;
    }
  };

  return {
    isCurrent,
    focus: () => {
      if (!isCurrent()) throw new Error('workspace panel focus target is no longer current');
      if (!captured.handler()) {
        throw new Error('workspace panel focus handler had no effect');
      }
      if (!captured.root.contains(document.activeElement)) {
        throw new Error('workspace panel focus handler did not focus the captured panel');
      }
      return undefined;
    },
    dispose: () => {
      if (disposed) return;
      disposed = true;
      unsubscribeAuth();
      unsubscribeWorkspace();
      window.removeEventListener('blur', invalidate);
    },
  };
}
