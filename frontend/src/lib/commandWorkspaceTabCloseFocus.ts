import { isModalOpen } from './modalRegistry';
import i18next from 'i18next';
import { useAuthStore } from '../store/authStore';
import { useWorkspaceStore, type WorkspaceData } from '../store/workspaceStore';
import { announce } from '../hooks/useAnnouncer';
import {
  canFocusWorkspacePanelImmediately,
  getWorkspacePanelImmediateFocusHandler,
} from '../components/workspace/workspacePanelFocusRegistry';

export interface WorkspaceTabCloseFocusLease {
  apply(): void;
  dispose(): void;
}

interface CapturedCloseTarget {
  ownerId: string;
  sessionId: string;
  workspaceId: string;
  sourceTabId: string;
  expectedSuccessorId?: string;
  sourceWasOnlyTab: boolean;
  capturedTabIds: ReadonlySet<string>;
  workspaceRoot: HTMLElement;
  root: HTMLElement;
  focusedElement: Element;
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
    ) {
      return true;
    }
  }
  return false;
}

function activeTabSnapshot() {
  const workspace = useWorkspaceStore.getState().workspace;
  if (!workspace?.activeTabId) return undefined;
  const tab = workspace.tabs.find((candidate) => candidate.id === workspace.activeTabId);
  return tab ? { workspace, tab } : undefined;
}

function findPanel(tabId: string): HTMLElement | undefined {
  const root = document.querySelector<HTMLElement>(
    `.ws-content__panel[data-tab-id="${CSS.escape(tabId)}"]`,
  );
  if (!root || !root.isConnected || root.dataset.tabId !== tabId || isHiddenByAncestor(root)) {
    return undefined;
  }
  return root;
}

function findTabButton(workspaceRoot: HTMLElement, tabId: string): HTMLButtonElement | undefined {
  const button = workspaceRoot.querySelector<HTMLButtonElement>(
    `button[role="tab"][data-tab-value="${CSS.escape(tabId)}"]`,
  );
  if (!button || !button.isConnected || isHiddenByAncestor(button)) return undefined;
  return button;
}

export function captureWorkspaceTabCloseFocus(
  readPathname: () => string,
): WorkspaceTabCloseFocusLease | undefined {
  if (typeof document === 'undefined') return undefined;

  try {
    if (!isWorkspaceRoute(readPathname()) || isModalOpen()) return undefined;
  } catch {
    return undefined;
  }

  const auth = useAuthStore.getState();
  const target = activeTabSnapshot();
  const root = target ? findPanel(target.tab.id) : undefined;
  const workspaceRoot = document.querySelector<HTMLElement>('.workspace-layout');
  const focusedElement = document.activeElement;
  if (!auth.isAuthenticated || !auth.user || !target || !root || !workspaceRoot || !focusedElement) return undefined;
  const sourceIndex = target.workspace.tabs.findIndex((tab) => tab.id === target.tab.id);

  const captured: CapturedCloseTarget = {
    ownerId: auth.user.userId,
    sessionId: auth.user.sessionId,
    workspaceId: target.workspace.id,
    sourceTabId: target.tab.id,
    expectedSuccessorId: target.workspace.tabs.length > 1
      ? target.workspace.tabs[sourceIndex + 1]?.id ?? target.workspace.tabs[sourceIndex - 1]?.id
      : undefined,
    sourceWasOnlyTab: target.workspace.tabs.length === 1,
    capturedTabIds: new Set(target.workspace.tabs.map((tab) => tab.id)),
    workspaceRoot,
    root,
    focusedElement,
  };
  let disposed = false;
  let invalidated = false;
  let applied = false;

  const matchesExpectedRemoval = (workspace: WorkspaceData): boolean => {
    if (
      !captured.workspaceRoot.isConnected ||
      isHiddenByAncestor(captured.workspaceRoot) ||
      document.querySelector('.workspace-layout') !== captured.workspaceRoot
    ) return false;
    if (workspace.id !== captured.workspaceId || workspace.tabs.some((tab) => tab.id === captured.sourceTabId)) return false;
    if (captured.sourceWasOnlyTab) {
      if (workspace.tabs.length !== 1 || workspace.tabs[0]?.type !== 'chat' || workspace.activeTabId !== workspace.tabs[0]?.id) return false;
      if (!captured.expectedSuccessorId) {
        captured.expectedSuccessorId = workspace.tabs[0].id;
        return true;
      }
      return captured.expectedSuccessorId === workspace.tabs[0].id;
    }
    if (!captured.expectedSuccessorId || workspace.activeTabId !== captured.expectedSuccessorId) return false;
    const remainingIds = new Set(workspace.tabs.map((tab) => tab.id));
    for (const tabId of captured.capturedTabIds) {
      if (tabId === captured.sourceTabId) continue;
      if (!remainingIds.has(tabId)) return false;
    }
    return remainingIds.size === captured.capturedTabIds.size - 1;
  };

  const unsubscribeAuth = useAuthStore.subscribe(() => {
    const current = useAuthStore.getState();
    if (
      !current.isAuthenticated ||
      current.user?.userId !== captured.ownerId ||
      current.user?.sessionId !== captured.sessionId
    ) {
      invalidated = true;
    }
  });
  const unsubscribeWorkspace = useWorkspaceStore.subscribe(() => {
    const workspace = useWorkspaceStore.getState().workspace;
    if (!workspace || workspace.id !== captured.workspaceId) {
      invalidated = true;
      return;
    }
    if (matchesExpectedRemoval(workspace)) {
      return;
    }
    invalidated = true;
  });
  const invalidate = () => { invalidated = true; };
  const invalidateFocusChange = (event: FocusEvent) => {
    // Removing the focused source naturally moves focus to body; that is the
    // only focus transition allowed after the expected close.
    if (!captured.root.isConnected && event.target === document.body) return;
    invalidated = true;
  };
  window.addEventListener('blur', invalidate);
  window.addEventListener('focusin', invalidateFocusChange);

  return {
    apply: () => {
      if (disposed || invalidated || applied) return;

      let pathname: string;
      try {
        pathname = readPathname();
      } catch {
        return;
      }
      if (!isWorkspaceRoute(pathname) || isModalOpen() || !document.hasFocus()) return;

      const currentAuth = useAuthStore.getState();
      const currentWorkspace = useWorkspaceStore.getState().workspace;
      const currentFocus = document.activeElement;
      if (
        !currentAuth.isAuthenticated ||
        currentAuth.user?.userId !== captured.ownerId ||
        currentAuth.user?.sessionId !== captured.sessionId ||
        !currentWorkspace ||
        currentWorkspace.id !== captured.workspaceId ||
        !matchesExpectedRemoval(currentWorkspace)
      ) return;

      const sourceWasRemoved = !captured.root.isConnected;
      const focusUnchanged = currentFocus === captured.focusedElement ||
        (sourceWasRemoved && currentFocus === document.body);
      if (!focusUnchanged) return;

      const successorId = currentWorkspace.activeTabId;
      if (!successorId || !currentWorkspace.tabs.some((tab) => tab.id === successorId)) return;

      applied = true;
      announce(i18next.t('workspace.announce.tabClosed'));

      try {
        const immediateHandler = getWorkspacePanelImmediateFocusHandler(successorId);
        if (immediateHandler && canFocusWorkspacePanelImmediately(successorId) && immediateHandler()) {
          return;
        }
      } catch {
        // A focus failure must not turn a successful backend mutation into an error.
      }

      findTabButton(captured.workspaceRoot, successorId)?.focus();
    },
    dispose: () => {
      if (disposed) return;
      disposed = true;
      unsubscribeAuth();
      unsubscribeWorkspace();
      window.removeEventListener('blur', invalidate);
      window.removeEventListener('focusin', invalidateFocusChange);
    },
  };
}
