import { getModalRegistrySnapshot, isModalOpen } from './modalRegistry';
import { useAuthStore } from '../store/authStore';
import { useWorkspaceStore } from '../store/workspaceStore';
import {
  canFocusWorkspacePanelImmediately,
  cancelWorkspacePanelFocus,
  getWorkspacePanelImmediateFocusHandler,
  queueWorkspacePanelFocus,
} from '../components/workspace/workspacePanelFocusRegistry';

export interface WorkspaceTabActivationRollbackDetail {
  readonly failedTabId: string;
  readonly rollbackTabId: string;
}

export interface WorkspaceTabActivationRollbackFocusLease {
  apply(): void;
  dispose(): void;
}

interface CapturedRollback {
  readonly ownerId: string;
  readonly sessionId: string;
  readonly workspaceId: string;
  readonly failedTabId: string;
  readonly rollbackTabId: string;
  readonly tabIds: readonly string[];
  readonly root: HTMLElement;
  readonly focusedElement: Element | null;
  readonly sourcePanel?: HTMLElement;
  readonly modalGeneration: string;
}

function isHiddenByAncestor(element: HTMLElement): boolean {
  for (let current: HTMLElement | null = element; current; current = current.parentElement) {
    if (current.hidden || current.hasAttribute('hidden') || current.hasAttribute('inert') || current.getAttribute('aria-hidden') === 'true') return true;
  }
  return false;
}

function findRoot(): HTMLElement | undefined {
  const root = document.querySelector<HTMLElement>('.workspace-layout');
  return root && root.isConnected && !isHiddenByAncestor(root) ? root : undefined;
}

function findTabButton(root: HTMLElement, tabId: string): HTMLButtonElement | undefined {
  const button = root.querySelector<HTMLButtonElement>(
    `.ws-tabs button[role="tab"][data-tab-value="${CSS.escape(tabId)}"]`,
  );
  return button && button.isConnected && !isHiddenByAncestor(button) ? button : undefined;
}

function sameOrder(workspace: { tabs: readonly { id: string }[] }, captured: CapturedRollback): boolean {
  return workspace.tabs.length === captured.tabIds.length && workspace.tabs.every((tab, index) => tab.id === captured.tabIds[index]);
}

function captureContext(
  readPathname: () => string,
  detail: WorkspaceTabActivationRollbackDetail,
): CapturedRollback | undefined {
  if (!detail || detail.failedTabId === detail.rollbackTabId) return undefined;
  const auth = useAuthStore.getState();
  const workspace = useWorkspaceStore.getState().workspace;
  const root = findRoot();
  const pathname = readPathname();
  if (!auth.isAuthenticated || !auth.user || !workspace || !root || (pathname !== '/' && pathname !== '')) return undefined;
  if (workspace.activeTabId !== detail.rollbackTabId) return undefined;
  if (!workspace.tabs.some((tab) => tab.id === detail.failedTabId) || !workspace.tabs.some((tab) => tab.id === detail.rollbackTabId)) return undefined;
  const sourcePanel = root.querySelector<HTMLElement>(
    `.ws-content__panel[data-tab-id="${CSS.escape(detail.failedTabId)}"]`,
  ) ?? undefined;
  const failedTabButton = findTabButton(root, detail.failedTabId);
  const focusedElement = document.activeElement;
  const focusBelongsToFailedTab = focusedElement === failedTabButton ||
    Boolean(sourcePanel && focusedElement && sourcePanel.contains(focusedElement)) ||
    (focusedElement === document.body && Boolean(sourcePanel && (!sourcePanel.isConnected || isHiddenByAncestor(sourcePanel))));
  if (!focusBelongsToFailedTab) return undefined;
  return {
    ownerId: auth.user.userId,
    sessionId: auth.user.sessionId,
    workspaceId: workspace.id,
    failedTabId: detail.failedTabId,
    rollbackTabId: detail.rollbackTabId,
    tabIds: workspace.tabs.map((tab) => tab.id),
    root,
    focusedElement,
    sourcePanel,
    modalGeneration: getModalRegistrySnapshot().generation,
  };
}

export function captureWorkspaceTabActivationRollbackFocus(
  readPathname: () => string,
  detail: WorkspaceTabActivationRollbackDetail,
): WorkspaceTabActivationRollbackFocusLease | undefined {
  if (typeof document === 'undefined') return undefined;
  let captured: CapturedRollback | undefined;
  try {
    captured = captureContext(readPathname, detail);
  } catch {
    return undefined;
  }
  if (!captured) return undefined;

  let disposed = false;
  let invalidated = false;
  let applied = false;
  let queued = false;
  let cleaned = false;
  const unsubscribes: Array<() => void> = [];
  const sourcePanelTransitioned = () => Boolean(
    captured.sourcePanel && (!captured.sourcePanel.isConnected || isHiddenByAncestor(captured.sourcePanel)),
  );
  const focusStillExpected = () => (
    document.activeElement === captured.focusedElement ||
    (document.activeElement === document.body && sourcePanelTransitioned())
  );
  const cleanup = () => {
    if (cleaned) return;
    cleaned = true;
    unsubscribes.forEach((unsubscribe) => unsubscribe());
    window.removeEventListener('focusin', onFocusIn);
    window.removeEventListener('blur', invalidate);
  };
  const invalidate = () => {
    invalidated = true;
    if (queued) {
      queued = false;
      cancelWorkspacePanelFocus(captured.rollbackTabId);
    }
    cleanup();
  };
  const onFocusIn = (event: FocusEvent) => {
    if (event.target === captured.focusedElement) return;
    if (event.target === document.body && sourcePanelTransitioned()) return;
    invalidate();
  };
  const current = () => {
    if (disposed || invalidated || !document.hasFocus() || isModalOpen() || !focusStillExpected()) return false;
    try {
      const auth = useAuthStore.getState();
      const workspace = useWorkspaceStore.getState().workspace;
      return readPathname() === '/' && auth.isAuthenticated && auth.user?.userId === captured.ownerId &&
        auth.user.sessionId === captured.sessionId && workspace?.id === captured.workspaceId &&
        workspace.activeTabId === captured.rollbackTabId && sameOrder(workspace, captured) &&
        workspace.tabs.some((tab) => tab.id === captured.failedTabId) &&
        workspace.tabs.some((tab) => tab.id === captured.rollbackTabId) &&
        findRoot() === captured.root && getModalRegistrySnapshot().generation === captured.modalGeneration;
    } catch {
      return false;
    }
  };
  const authUnsubscribe = useAuthStore.subscribe(() => {
    const auth = useAuthStore.getState();
    if (!auth.isAuthenticated || auth.user?.userId !== captured.ownerId || auth.user?.sessionId !== captured.sessionId) invalidate();
  });
  const workspaceUnsubscribe = useWorkspaceStore.subscribe(() => {
    const workspace = useWorkspaceStore.getState().workspace;
    if (!workspace || workspace.id !== captured.workspaceId || workspace.activeTabId !== captured.rollbackTabId || !sameOrder(workspace, captured)) invalidate();
  });
  unsubscribes.push(authUnsubscribe, workspaceUnsubscribe);
  window.addEventListener('focusin', onFocusIn);
  window.addEventListener('blur', invalidate);

  return {
    apply: () => {
      if (disposed || invalidated || applied || !current()) return;
      applied = true;
      const focusImmediate = () => {
        if (!current()) return false;
        const immediate = getWorkspacePanelImmediateFocusHandler(captured.rollbackTabId);
        return Boolean(immediate && canFocusWorkspacePanelImmediately(captured.rollbackTabId) && immediate());
      };
      try {
        if (focusImmediate()) {
          cleanup();
          return;
        }
      } catch {
        // Foco é apresentação e não altera o resultado do rollback.
      }
      queueWorkspacePanelFocus(captured.rollbackTabId, current, cleanup, () => {
        if (current()) findTabButton(captured.root, captured.rollbackTabId)?.focus();
        cleanup();
      }, cleanup, true);
      queued = true;
    },
    dispose: () => {
      if (disposed) return;
      disposed = true;
      cancelWorkspacePanelFocus(captured.rollbackTabId);
      cleanup();
    },
  };
}
