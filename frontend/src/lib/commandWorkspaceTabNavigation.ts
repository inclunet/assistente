import { getModalRegistrySnapshot, isModalOpen } from './modalRegistry';
import i18next from 'i18next';
import { useAuthStore } from '../store/authStore';
import { useWorkspaceStore, type WorkspaceData } from '../store/workspaceStore';
import { announce } from '../hooks/useAnnouncer';
import {
  canFocusWorkspacePanelImmediately,
  getWorkspacePanelImmediateFocusHandler,
  queueWorkspacePanelFocus,
  cancelWorkspacePanelFocus,
} from '../components/workspace/workspacePanelFocusRegistry';

export const WORKSPACE_TAB_NAVIGATION_COMMAND_IDS = [
  'workspace.tab.next',
  'workspace.tab.previous',
  'workspace.tab.first',
  'workspace.tab.second',
  'workspace.tab.third',
  'workspace.tab.fourth',
  'workspace.tab.fifth',
  'workspace.tab.sixth',
  'workspace.tab.seventh',
  'workspace.tab.eighth',
  'workspace.tab.ninth',
] as const;

export type WorkspaceTabNavigationCommand = typeof WORKSPACE_TAB_NAVIGATION_COMMAND_IDS[number];

export interface WorkspaceTabNavigationTargetLease {
  readonly navigation: WorkspaceTabNavigationSnapshot;
  isCurrent(): boolean;
  dispose(): void;
}

export interface WorkspaceTabNavigationSnapshot {
  readonly workspaceID: string;
  readonly sourceTabID: string;
  readonly targetTabID: string;
}

export interface WorkspaceTabNavigationFocusLease {
  apply(): void;
  dispose(): void;
}

interface CapturedNavigation {
  ownerId: string;
  sessionId: string;
  workspaceId: string;
  sourceTabId: string;
  targetTabId: string;
  tabIds: readonly string[];
  root: HTMLElement;
  focusedElement?: Element;
  sourcePanel?: HTMLElement;
  modalGeneration: string;
}

const POSITION_COMMANDS: readonly WorkspaceTabNavigationCommand[] = [
  'workspace.tab.first',
  'workspace.tab.second',
  'workspace.tab.third',
  'workspace.tab.fourth',
  'workspace.tab.fifth',
  'workspace.tab.sixth',
  'workspace.tab.seventh',
  'workspace.tab.eighth',
  'workspace.tab.ninth',
];

export function isWorkspaceTabNavigationCommand(commandID: unknown): commandID is WorkspaceTabNavigationCommand {
  return WORKSPACE_TAB_NAVIGATION_COMMAND_IDS.some((id) => id === commandID);
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

function findWorkspaceRoot(): HTMLElement | undefined {
  const root = document.querySelector<HTMLElement>('.workspace-layout');
  if (!root || !root.isConnected || isHiddenByAncestor(root)) return undefined;
  return root;
}

function tabIds(workspace: WorkspaceData): string[] {
  return workspace.tabs.map((tab) => tab.id);
}

function sameTabOrder(workspace: WorkspaceData, captured: CapturedNavigation): boolean {
  const current = tabIds(workspace);
  return current.length === captured.tabIds.length && current.every((id, index) => id === captured.tabIds[index]);
}

function currentSnapshot(captured: CapturedNavigation, expectedActiveTabId: string): boolean {
  const auth = useAuthStore.getState();
  const workspace = useWorkspaceStore.getState().workspace;
  const root = findWorkspaceRoot();
  return Boolean(
    auth.isAuthenticated &&
    auth.user?.userId === captured.ownerId &&
    auth.user.sessionId === captured.sessionId &&
    workspace?.id === captured.workspaceId &&
    workspace.activeTabId === expectedActiveTabId &&
    workspace.tabs.some((tab) => tab.id === captured.sourceTabId) &&
    workspace.tabs.some((tab) => tab.id === captured.targetTabId) &&
    sameTabOrder(workspace, captured) &&
    root === captured.root &&
    getModalRegistrySnapshot().generation === captured.modalGeneration
  );
}

function captureContext(
  readPathname: () => string,
  commandID: unknown,
  withFocus: boolean,
): CapturedNavigation | undefined {
  if (typeof document === 'undefined' || !isWorkspaceTabNavigationCommand(commandID)) return undefined;
  try {
    if (!isWorkspaceRoute(readPathname()) || isModalOpen()) return undefined;
  } catch {
    return undefined;
  }

  const auth = useAuthStore.getState();
  const workspace = useWorkspaceStore.getState().workspace;
  const root = findWorkspaceRoot();
  const focusedElement = withFocus ? document.activeElement ?? undefined : undefined;
  if (!auth.isAuthenticated || !auth.user || !workspace || !workspace.activeTabId || !root) return undefined;
  if (withFocus && !focusedElement) return undefined;

  const target = resolveWorkspaceTabNavigationTarget(workspace, commandID);
  if (!target) return undefined;

  return {
    ownerId: auth.user.userId,
    sessionId: auth.user.sessionId,
    workspaceId: workspace.id,
    sourceTabId: workspace.activeTabId,
    targetTabId: target.id,
    tabIds: tabIds(workspace),
    root,
    focusedElement,
    sourcePanel: withFocus
      ? document.querySelector<HTMLElement>(
        `.ws-content__panel[data-tab-id="${CSS.escape(workspace.activeTabId)}"]`,
      ) ?? undefined
      : undefined,
    modalGeneration: getModalRegistrySnapshot().generation,
  };
}

export function resolveWorkspaceTabNavigationTarget(
  workspace: WorkspaceData | null | undefined,
  commandID: unknown,
) {
  if (!workspace || !isWorkspaceTabNavigationCommand(commandID) || workspace.tabs.length === 0) return undefined;
  const activeIndex = workspace.tabs.findIndex((tab) => tab.id === workspace.activeTabId);
  if (activeIndex < 0) return undefined;

  if (commandID === 'workspace.tab.next') {
    return workspace.tabs[(activeIndex + 1) % workspace.tabs.length];
  }
  if (commandID === 'workspace.tab.previous') {
    return workspace.tabs[(activeIndex - 1 + workspace.tabs.length) % workspace.tabs.length];
  }

  const position = POSITION_COMMANDS.indexOf(commandID);
  return position >= 0 ? workspace.tabs[position] : undefined;
}

export function captureWorkspaceTabNavigationTarget(
  readPathname: () => string,
  commandID: unknown,
): WorkspaceTabNavigationTargetLease | undefined {
  const captured = captureContext(readPathname, commandID, false);
  if (!captured) return undefined;
  return createNavigationTargetLease(readPathname, captured);
}

function createNavigationTargetLease(
  readPathname: () => string,
  captured: CapturedNavigation,
): WorkspaceTabNavigationTargetLease {
  const navigation = Object.freeze({
    workspaceID: captured.workspaceId,
    sourceTabID: captured.sourceTabId,
    targetTabID: captured.targetTabId,
  });
  let disposed = false;
  let invalidated = false;
  const invalidate = () => { invalidated = true; };
  const authUnsubscribe = useAuthStore.subscribe(() => {
    const auth = useAuthStore.getState();
    if (!auth.isAuthenticated || auth.user?.userId !== captured.ownerId || auth.user?.sessionId !== captured.sessionId) invalidate();
  });
  const workspaceUnsubscribe = useWorkspaceStore.subscribe(() => {
    const workspace = useWorkspaceStore.getState().workspace;
    if (!workspace || workspace.id !== captured.workspaceId || !sameTabOrder(workspace, captured)) {
      invalidate();
      return;
    }
    if (workspace.activeTabId !== captured.sourceTabId) invalidate();
  });
  const unsubscribes = [authUnsubscribe, workspaceUnsubscribe];
  const invalidateBlur = () => { invalidated = true; };
  window.addEventListener('blur', invalidateBlur);

  return {
    navigation,
    isCurrent: () => {
      if (disposed || invalidated) return false;
      try {
        return isWorkspaceRoute(readPathname()) && !isModalOpen() && currentSnapshot(captured, captured.sourceTabId);
      } catch {
        return false;
      }
    },
    dispose: () => {
      if (disposed) return;
      disposed = true;
      unsubscribes.forEach((unsubscribe) => unsubscribe());
      window.removeEventListener('blur', invalidateBlur);
    },
  };
}

function findTabButton(root: HTMLElement, tabId: string): HTMLButtonElement | undefined {
  const button = root.querySelector<HTMLButtonElement>(
    `.ws-tabs button[role="tab"][data-tab-value="${CSS.escape(tabId)}"]`,
  );
  return button && button.isConnected && !isHiddenByAncestor(button) ? button : undefined;
}

export function captureWorkspaceTabNavigationFocus(
  readPathname: () => string,
  commandID: unknown,
): WorkspaceTabNavigationFocusLease | undefined {
  const captured = captureContext(readPathname, commandID, true);
  if (!captured) return undefined;
  let disposed = false;
  let invalidated = false;
  let applied = false;
  let queuedFocus = false;
  let cleanedUp = false;
  let destinationObserved = false;
  const cleanup = () => {
    if (cleanedUp) return;
    cleanedUp = true;
    unsubscribes.forEach((unsubscribe) => unsubscribe());
    window.removeEventListener('focusin', invalidateFocus);
    window.removeEventListener('blur', invalidateBlur);
  };
  const invalidate = () => {
    invalidated = true;
    if (queuedFocus) {
      queuedFocus = false;
      cancelWorkspacePanelFocus(captured.targetTabId);
    }
    cleanup();
  };
  const authUnsubscribe = useAuthStore.subscribe(() => {
    const auth = useAuthStore.getState();
    if (!auth.isAuthenticated || auth.user?.userId !== captured.ownerId || auth.user?.sessionId !== captured.sessionId) {
      invalidate();
    }
  });
  const workspaceUnsubscribe = useWorkspaceStore.subscribe(() => {
    const workspace = useWorkspaceStore.getState().workspace;
    if (!workspace || workspace.id !== captured.workspaceId || !sameTabOrder(workspace, captured)) {
      invalidate();
      return;
    }
    if (workspace.activeTabId === captured.targetTabId) {
      destinationObserved = true;
      return;
    }
    if (destinationObserved || workspace.activeTabId !== captured.sourceTabId) invalidate();
  });
  const unsubscribes = [authUnsubscribe, workspaceUnsubscribe];
  const invalidateFocus = (event: FocusEvent) => {
    const sourcePanelTransitioned = Boolean(
      captured.sourcePanel &&
      (!captured.sourcePanel.isConnected || isHiddenByAncestor(captured.sourcePanel)),
    );
    if (event.target === document.body && sourcePanelTransitioned) return;
    invalidate();
  };
  const invalidateBlur = () => { invalidate(); };
  window.addEventListener('focusin', invalidateFocus);
  window.addEventListener('blur', invalidateBlur);

  const stillValid = () => {
    if (invalidated || !document.hasFocus() || isModalOpen()) return false;
    const sourcePanelTransitioned = Boolean(
      captured.sourcePanel &&
      (!captured.sourcePanel.isConnected || isHiddenByAncestor(captured.sourcePanel)),
    );
    const focusUnchanged = document.activeElement === captured.focusedElement ||
      (document.activeElement === document.body && sourcePanelTransitioned);
    try {
      return isWorkspaceRoute(readPathname()) && focusUnchanged &&
        currentSnapshot(captured, captured.targetTabId) &&
        (document.activeElement !== document.body || sourcePanelTransitioned || captured.focusedElement === document.body);
    } catch {
      return false;
    }
  };

  return {
    apply: () => {
      if (disposed || invalidated || applied) return;
      let pathname: string;
      try {
        pathname = readPathname();
      } catch {
        return;
      }
      const sourcePanelTransitioned = Boolean(
        captured.sourcePanel &&
        (!captured.sourcePanel.isConnected || isHiddenByAncestor(captured.sourcePanel)),
      );
      const focusUnchanged = document.activeElement === captured.focusedElement ||
        (document.activeElement === document.body && sourcePanelTransitioned);
      if (!stillValid() || pathname !== readPathname() || !focusUnchanged) return;

      applied = true;
      const workspace = useWorkspaceStore.getState().workspace;
      const target = workspace?.tabs.find((tab) => tab.id === captured.targetTabId);
      if (!workspace || !target) return;
      const targetPosition = workspace.tabs.findIndex((tab) => tab.id === target.id);
      const announceTarget = () => announce(i18next.t('workspace.announce.tabPosition', {
        title: target.title,
        position: targetPosition + 1,
        total: workspace.tabs.length,
      }));
      const focusImmediatePanel = () => {
        if (!stillValid()) return false;
        try {
          const immediate = getWorkspacePanelImmediateFocusHandler(captured.targetTabId);
          if (immediate && canFocusWorkspacePanelImmediately(captured.targetTabId) && immediate()) return true;
        } catch {
          // Foco é apresentação; nunca transforma sucesso backend em falha.
        }
        return false;
      };
      try {
        if (focusImmediatePanel()) {
          announceTarget();
          return;
        }
      } catch {
        // Foco é apresentação; nunca transforma sucesso backend em falha.
      }
      queueWorkspacePanelFocus(captured.targetTabId, () => {
        return stillValid();
      }, () => {
        announceTarget();
        queuedFocus = false;
        cleanup();
      }, () => {
        if (!stillValid()) {
          cleanup();
          return;
        }
        const button = findTabButton(captured.root, captured.targetTabId);
        if (button) {
          button.focus();
          announceTarget();
        }
        queuedFocus = false;
        cleanup();
      }, cleanup, true);
      queuedFocus = true;
    },
    dispose: () => {
      if (disposed) return;
      disposed = true;
      cancelWorkspacePanelFocus(captured.targetTabId);
      cleanup();
    },
  };
}
