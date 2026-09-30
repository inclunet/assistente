import { getModalRegistrySnapshot, isModalOpen } from './modalRegistry';
import { useAuthStore } from '../store/authStore';
import { useWorkspaceStore } from '../store/workspaceStore';
import {
  canFocusWorkspacePanelImmediately,
  getWorkspacePanelImmediateFocusHandler,
} from '../components/workspace/workspacePanelFocusRegistry';

export interface WorkspaceTabCreateFocusLease {
  apply(): Promise<void>;
  dispose(): void;
}

const COMMAND_TYPES: Readonly<Record<string, string>> = {
  'workspace.tab.chat.create': 'chat',
  'workspace.tab.editor.create': 'editor',
  'workspace.tab.tasklist.create': 'tasklist',
  'workspace.tab.terminal.create': 'terminal',
};
const MAX_FOCUS_WAIT_MS = 5_000;

function hidden(element: HTMLElement): boolean {
  for (let current: HTMLElement | null = element; current; current = current.parentElement) {
    if (current.hidden || current.hasAttribute('inert') || current.getAttribute('aria-hidden') === 'true') return true;
  }
  return false;
}

export function captureWorkspaceTabCreateFocus(
  readPathname: () => string,
  commandID?: string,
  isOriginCurrent: () => boolean = () => true,
): WorkspaceTabCreateFocusLease | undefined {
  if (typeof document === 'undefined') return undefined;
  try {
    if (!['', '/'].includes(readPathname()) || isModalOpen() || !isOriginCurrent()) return undefined;
  } catch { return undefined; }

  const auth = useAuthStore.getState();
  const initial = useWorkspaceStore.getState().workspace;
  const root = document.querySelector<HTMLElement>('.workspace-layout');
  const focused = document.activeElement;
  const active = initial?.tabs.find((tab) => tab.id === initial.activeTabId);
  const sourcePanel = active ? document.querySelector<HTMLElement>(
    `.ws-content__panel[data-tab-id="${CSS.escape(active.id)}"]`,
  ) : undefined;
  if (!auth.isAuthenticated || !auth.user || !initial || !root || !focused || !active ||
      !root.isConnected || hidden(root) || !sourcePanel || !root.contains(focused)) return undefined;

  const expectedType = commandID ? COMMAND_TYPES[commandID] : undefined;
  if (commandID && !expectedType) return undefined;
  const originalIds = new Set(initial.tabs.map((tab) => tab.id));
  const captured = {
    owner: auth.user.userId,
    session: auth.user.sessionId,
    workspaceId: initial.id,
    root,
    focused,
    modalGeneration: getModalRegistrySnapshot().generation,
  };

  let disposed = false;
  let invalid = false;
  let applied = false;
  let frame: number | undefined;
  let timeout: ReturnType<typeof setTimeout> | undefined;
  let newTabId: string | undefined;
  let resolving: ((value: void) => void) | undefined;
  const cleanups: Array<() => void> = [];
  const sourceGone = () => !sourcePanel.isConnected || hidden(sourcePanel);
  const focusExpected = () => document.activeElement === focused ||
    (document.activeElement === document.body && sourceGone());
  const cleanup = () => {
    cleanups.splice(0).forEach((fn) => fn());
    if (frame !== undefined) cancelAnimationFrame(frame);
    frame = undefined;
    if (timeout !== undefined) clearTimeout(timeout);
    timeout = undefined;
  };
  const finish = () => {
    if (!resolving) return;
    const done = resolving;
    resolving = undefined;
    cleanup();
    done();
  };
  const invalidate = () => { invalid = true; cleanup(); finish(); };
  const snapshot = () => useWorkspaceStore.getState().workspace;
  const identifyCreation = () => {
    const ws = snapshot();
    if (!ws || ws.id !== captured.workspaceId) return undefined;
    const added = ws.tabs.filter((tab) => !originalIds.has(tab.id));
    if (added.length !== 1 || ws.tabs.length !== originalIds.size + 1 ||
        ![...originalIds].every((id) => ws.tabs.some((tab) => tab.id === id)) ||
        ws.activeTabId !== added[0].id ||
        (expectedType && added[0].type !== expectedType)) return undefined;
    return added[0];
  };
  const contextCurrent = () => {
    if (disposed || invalid || !document.hasFocus() || !focusExpected() || isModalOpen() ||
        getModalRegistrySnapshot().generation !== captured.modalGeneration ||
        document.querySelector('.workspace-layout') !== captured.root || hidden(captured.root)) return false;
    try {
      const nowAuth = useAuthStore.getState();
      const ws = snapshot();
      return ['', '/'].includes(readPathname()) && isOriginCurrent() && nowAuth.isAuthenticated &&
        nowAuth.user?.userId === captured.owner && nowAuth.user.sessionId === captured.session &&
        ws?.id === captured.workspaceId;
    } catch { return false; }
  };
  const current = () => contextCurrent() && Boolean(newTabId && snapshot()?.activeTabId === newTabId &&
    snapshot()?.tabs.some((tab) => tab.id === newTabId && (!expectedType || tab.type === expectedType)) &&
    identifyCreation()?.id === newTabId);
  const authUnsub = useAuthStore.subscribe(() => {
    const now = useAuthStore.getState();
    if (!now.isAuthenticated || now.user?.userId !== captured.owner || now.user.sessionId !== captured.session) invalidate();
  });
  const workspaceUnsub = useWorkspaceStore.subscribe(() => {
    const ws = snapshot();
    if (!ws || ws.id !== captured.workspaceId) { invalidate(); return; }
    const created = identifyCreation();
    if (created) {
      if (newTabId && created.id !== newTabId) invalidate();
      else newTabId = created.id;
    } else if (newTabId || ws.activeTabId !== initial.activeTabId ||
      ws.tabs.length !== originalIds.size || ws.tabs.some((tab) => !originalIds.has(tab.id))) invalidate();
  });
  cleanups.push(authUnsub, workspaceUnsub);
  const onFocus = (event: FocusEvent) => {
    const createdPanel = newTabId && document.querySelector<HTMLElement>(
      `.ws-content__panel[data-tab-id="${CSS.escape(newTabId)}"]`,
    );
    if (applied && createdPanel && event.target instanceof Node && createdPanel.contains(event.target)) return;
    if (event.target === captured.focused || (event.target === document.body && sourceGone())) return;
    invalidate();
  };
  window.addEventListener('focusin', onFocus);
  window.addEventListener('blur', invalidate);
  window.addEventListener('popstate', invalidate);
  window.addEventListener('hashchange', invalidate);
  cleanups.push(
    () => window.removeEventListener('focusin', onFocus),
    () => window.removeEventListener('blur', invalidate),
    () => window.removeEventListener('popstate', invalidate),
    () => window.removeEventListener('hashchange', invalidate),
  );

  return {
    apply: () => {
      if (disposed || invalid || applied) return Promise.resolve();
      if (!contextCurrent()) return Promise.resolve();
      applied = true;
      return new Promise<void>((resolve) => {
        resolving = resolve;
        timeout = setTimeout(finish, MAX_FOCUS_WAIT_MS);
        const attempt = () => {
          frame = undefined;
          if (!contextCurrent()) { invalidate(); finish(); return; }
          const created = identifyCreation();
          if (created) {
            if (newTabId && created.id !== newTabId) { invalidate(); finish(); return; }
            newTabId = created.id;
          }
          if (!newTabId) {
            frame = requestAnimationFrame(attempt);
            return;
          }
          if (!current()) { invalidate(); finish(); return; }
          const panel = document.querySelector<HTMLElement>(
            `.ws-content__panel[data-tab-id="${CSS.escape(newTabId!)}"]`,
          );
          if (panel && expectedType && panel.dataset.tabType !== expectedType) {
            invalidate(); finish(); return;
          }
          if (panel?.isConnected && !hidden(panel) && canFocusWorkspacePanelImmediately(newTabId!)) {
            try {
              const focus = getWorkspacePanelImmediateFocusHandler(newTabId!);
              if (focus?.()) { finish(); return; }
            } catch { /* Foco é apresentação; a criação já foi confirmada. */ }
          }
          frame = requestAnimationFrame(attempt);
        };
        attempt();
      });
    },
    dispose: () => {
      if (disposed) return;
      disposed = true;
      cleanup();
      finish();
    },
  };
}
